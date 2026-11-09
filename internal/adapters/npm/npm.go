// Package npm reads Nginx Proxy Manager, in Network: each proxy host,
// redirection and 404 host, by its domain, and each stream, by its port, in
// red when nginx refused its config,
// with why, or grey when it's disabled; the certificates they serve, by when
// each expires, those within 30 days as chores; and its version and whether
// a newer one is out.
package npm

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "network"

// Adapter says what this adapter is, for the config and its test.
var Adapter = snapshot.Adapter[Config]{Key: "npm", Area: Area, Read: Read}

// Config is one Nginx Proxy Manager, under sources.npm in overlook.yaml.
type Config struct {
	Name string `yaml:"name" setup:"text,optional"`
	// URL is where Overlook reads Nginx Proxy Manager's admin, such as
	// http://npm:81.
	URL string `yaml:"url" setup:"text" example:"http://npm.example.com:81"`
	// User and Password are an Nginx Proxy Manager user's email and
	// password: one who isn't an admin, sees every item and may view the
	// hosts and streams. Nginx Proxy Manager has no API keys and lets
	// Overlook in only with a login, one POST (ADR 0009).
	User     string `yaml:"user" setup:"text" example:"overlook@example.com"`
	Password string `yaml:"password" setup:"key"`
	// Link is where the page sends the owner to renew a certificate, if not
	// URL.
	Link string `yaml:"link"`

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL, Login: &fetch.Login{
		Path: "/api/tokens", Credentials: map[string]string{"identity": c.User, "secret": c.Password}, Take: token,
		// A token lasts a day, and Nginx Proxy Manager answers one past it
		// with a 400.
		Lapsed: http.StatusBadRequest,
	}}
	if err := c.source.Check(); err != nil {
		return err
	}
	if c.User == "" || c.Password == "" {
		return errors.New("user and password are missing: Nginx Proxy Manager lets Overlook in only with a login")
	}
	c.Name = cmp.Or(c.Name, "Nginx Proxy Manager")
	return fetch.CheckLink(&c.Link, c.URL)
}

// token takes the session from a login's answer: the token Nginx Proxy
// Manager gives, sent back as a bearer's. A user with two-factor
// authentication gets a challenge instead, which Overlook can't answer.
func token(_ http.Header, body []byte) (http.Header, error) {
	var a struct {
		Token     string `json:"token"`
		TwoFactor bool   `json:"requires_2fa"`
	}
	switch {
	case json.Unmarshal(body, &a) != nil:
		return nil, errors.New("no token in the answer")
	case a.TwoFactor:
		return nil, errors.New("the user has two-factor authentication, which Overlook can't answer: give it a user without")
	case a.Token == "":
		return nil, errors.New("no token in the answer")
	}
	return http.Header{"Authorization": {"Bearer " + a.Token}}, nil
}

// item is a proxy host, a redirection, a 404 host or a stream, as Nginx
// Proxy Manager calls each of them, with the fields of its own: whether it's
// on, what nginx made of its config, where it sends what it receives, and
// the certificate it serves.
type item struct {
	DomainNames []string `json:"domain_names"`
	Enabled     bool     `json:"enabled"`
	Meta        struct {
		// Online is false when nginx refused the config, and Err is then
		// what nginx said. An item not yet configured has neither.
		Online *bool  `json:"nginx_online"`
		Err    string `json:"nginx_err"`
	} `json:"meta"`
	Certificate *certificate `json:"certificate"`

	// A proxy host's and a redirection's: http, https, or auto for a
	// redirection that keeps the scheme it was asked with.
	Scheme string `json:"forward_scheme"`
	// A proxy host's.
	ForwardHost string `json:"forward_host"`
	ForwardPort int    `json:"forward_port"`
	// A redirection's, which may hold a path.
	ForwardDomain string `json:"forward_domain_name"`
	Code          int    `json:"forward_http_code"`
	// A stream's.
	Port           int    `json:"incoming_port"`
	ForwardingHost string `json:"forwarding_host"`
	ForwardingPort int    `json:"forwarding_port"`
	TCP            bool   `json:"tcp_forwarding"`
	UDP            bool   `json:"udp_forwarding"`
}

// certificate is the one an item serves, as the item's list brings it,
// without its key, which only the list of certificates gives, and which
// Overlook's user therefore has no need to see.
type certificate struct {
	ID          int      `json:"id"`
	Provider    string   `json:"provider"`
	NiceName    string   `json:"nice_name"`
	DomainNames []string `json:"domain_names"`
	// ExpiresOn is as 2026-11-21 20:36:54, in Nginx Proxy Manager's own
	// time zone, UTC unless its owner set TZ, which Overlook can't know.
	// A few hours either way don't matter to a 30-day warning.
	ExpiresOn string `json:"expires_on"`
}

// soon is how near its end a certificate becomes a chore: when Nginx Proxy
// Manager starts trying to renew one from Let's Encrypt, so one still
// within it is one that isn't renewing, or one you upload.
const soon = 30 * 24 * time.Hour

// read is everything Read reads.
type read struct {
	proxies, redirections, deads, streams []item
	version, newer                        string
}

// Read reads the Nginx Proxy Manager at c.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var r read
	for _, at := range []struct {
		path string
		v    *[]item
	}{
		{"/api/nginx/proxy-hosts", &r.proxies},
		{"/api/nginx/redirection-hosts", &r.redirections},
		{"/api/nginx/dead-hosts", &r.deads},
		{"/api/nginx/streams", &r.streams},
	} {
		if err := c.source.JSON(ctx, at.path+"?expand=certificate", at.v); err != nil {
			return snapshot.Parts{}, err
		}
	}
	// Nginx Proxy Manager asks GitHub for its latest release, and gives
	// none when it couldn't, which leaves its version off the page.
	var v struct {
		Current string `json:"current"`
		Latest  string `json:"latest"`
		Update  bool   `json:"update_available"`
	}
	if c.source.JSON(ctx, "/api/version/check", &v) == nil && v.Current != "" {
		r.version = v.Current
		if v.Update {
			r.newer = v.Latest
		}
	}
	return parts(r, c.Link, now), nil
}

func parts(r read, link string, now time.Time) snapshot.Parts {
	// The domains, by name, each saying where it sends what it receives,
	// and Nginx Proxy Manager's version after them; then the streams, by
	// their port.
	var domains, streams []snapshot.Row
	for _, h := range r.proxies {
		to := h.Scheme + "://" + h.ForwardHost + ":" + strconv.Itoa(h.ForwardPort)
		domains = appendRow(domains, h, snapshot.Msg("npm.proxy", nil), to)
	}
	for _, h := range r.redirections {
		to := h.ForwardDomain
		if h.Scheme == "http" || h.Scheme == "https" {
			to = h.Scheme + "://" + to
		}
		domains = appendRow(domains, h, snapshot.Msg("npm.redirect", map[string]any{"code": strconv.Itoa(h.Code)}), to)
	}
	for _, h := range r.deads {
		domains = appendRow(domains, h, snapshot.Msg("npm.dead", nil), "")
	}
	slices.SortStableFunc(domains, func(a, b snapshot.Row) int { return cmp.Compare(a.Title.Name, b.Title.Name) })
	if r.version != "" {
		domains = append(domains, snapshot.VersionRow(r.version, r.newer))
	}
	slices.SortStableFunc(r.streams, func(a, b item) int { return cmp.Compare(a.Port, b.Port) })
	for _, h := range r.streams {
		if h.Port == 0 {
			continue
		}
		key := "npm.tcp_udp"
		if !h.UDP {
			key = "npm.tcp"
		} else if !h.TCP {
			key = "npm.udp"
		}
		// A port is written as it is, never as a number the viewer's way,
		// which would make 51820 51,820.
		port := strconv.Itoa(h.Port)
		streams = append(streams, row(h, snapshot.Text{Key: "npm.port", Args: map[string]any{"port": port}},
			snapshot.Msg(key, nil), h.ForwardingHost+":"+strconv.Itoa(h.ForwardingPort)))
	}
	// The certificates the items serve, each once, the one that ends first
	// first, in orange within 30 days, as a chore too, and in red once it
	// has ended.
	type served struct {
		certificate
		ends time.Time
	}
	var certs []served
	for _, h := range slices.Concat(r.proxies, r.redirections, r.deads, r.streams) {
		c := h.Certificate
		if c == nil || c.ID == 0 || slices.ContainsFunc(certs, func(s served) bool { return s.ID == c.ID }) {
			continue
		}
		ends, err := time.Parse(time.DateTime, c.ExpiresOn)
		if err != nil {
			continue
		}
		certs = append(certs, served{*c, ends})
	}
	slices.SortFunc(certs, func(a, b served) int { return cmp.Or(a.ends.Compare(b.ends), cmp.Compare(a.ID, b.ID)) })
	var crows []snapshot.Row
	chores := []snapshot.Chore{}
	day := func(t time.Time) string { return t.In(now.Location()).Format(time.DateOnly) }
	for _, c := range certs {
		name := c.NiceName
		if name == "" && len(c.DomainNames) > 0 {
			name = c.DomainNames[0]
		}
		names := strings.Join(c.DomainNames, ", ")
		// One from Let's Encrypt is named after its domains, and one
		// uploaded as its owner named it.
		detail := snapshot.Msg("npm.custom", map[string]any{"domains": names})
		if c.Provider == "letsencrypt" {
			detail = snapshot.Msg("npm.letsencrypt", nil)
		}
		left := c.ends.Sub(now)
		cr := snapshot.Row{
			Title:  snapshot.Text{Name: name},
			Value:  snapshot.Msg("npm.expires", map[string]any{"ms": left.Milliseconds()}),
			Detail: detail,
		}
		switch {
		case left <= 0:
			cr.Value, cr.Tone = snapshot.Msg("npm.expired", map[string]any{"ms": -left.Milliseconds()}), "red"
		case left <= soon:
			cr.Tone = "orange"
		}
		crows = append(crows, cr)
		if left <= soon {
			// Its note is the day it ends, after its domains when its name
			// isn't them.
			note := day(c.ends)
			if names != name {
				note = names + " · " + note
			}
			chores = append(chores, snapshot.Chore{
				Name: name, Note: note, From: day(c.ends.Add(-soon)),
				URL: strings.TrimSuffix(link, "/") + "/certificates",
			})
		}
	}

	sections := []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "npm.domains"}, Rows: domains}}
	if len(streams) > 0 {
		sections = append(sections, snapshot.Section{Area: Area, Title: snapshot.Text{Key: "npm.streams"}, Rows: streams})
	}
	if len(crows) > 0 {
		sections = append(sections, snapshot.Section{Area: Area, Title: snapshot.Text{Key: "npm.certificates"}, Rows: crows})
	}
	return snapshot.Parts{Sections: sections, Chores: chores}
}

// appendRow appends h's row to rows, by its first domain, the others before
// where it sends what it receives; one with no domain is left out.
func appendRow(rows []snapshot.Row, h item, value *snapshot.Text, to string) []snapshot.Row {
	if len(h.DomainNames) == 0 || h.DomainNames[0] == "" {
		return rows
	}
	return append(rows, row(h, snapshot.Text{Name: h.DomainNames[0]}, value, to))
}

// row is h's row: titled, with its value and where it sends what it
// receives, after its other names; grey and disabled when it is, or red
// with what nginx said when nginx refused its config.
func row(h item, title snapshot.Text, value *snapshot.Text, to string) snapshot.Row {
	r := snapshot.Row{Title: title, Value: value}
	var detail []string
	if len(h.DomainNames) > 1 {
		detail = append(detail, h.DomainNames[1:]...)
	}
	if to != "" {
		detail = append(detail, "→ "+to)
	}
	if len(detail) > 0 {
		r.Detail = &snapshot.Text{Name: strings.Join(detail, " · ")}
	}
	switch {
	case !h.Enabled:
		r.Value, r.Tone = snapshot.Msg("npm.disabled", nil), "gray"
	case h.Meta.Online != nil && !*h.Meta.Online:
		r.Value, r.Tone = snapshot.Msg("npm.refused", nil), "red"
		if why := why(h.Meta.Err); why != "" {
			r.Detail = &snapshot.Text{Name: why}
		}
	}
	return r
}

// why is the first line of what nginx said of a config it refused, without
// its prefix: of "nginx: [emerg] unknown directive …", "unknown directive
// …".
func why(err string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(err), "\n")
	line = strings.TrimPrefix(line, "nginx: ")
	if strings.HasPrefix(line, "[") {
		if _, after, ok := strings.Cut(line, "] "); ok {
			line = after
		}
	}
	return line
}
