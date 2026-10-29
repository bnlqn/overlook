// Package pihole reads Pi-hole, a resolver: its lookups and blocks, whether
// it blocks, its top clients, domains and blocked domains, and its
// upstreams, as any resolver's (snapshot.Resolver); and its blocklists,
// what it warns about and its versions.
package pihole

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "network"

// Config is one Pi-hole, under sources.pihole in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Pi-hole's web interface, such as
	// http://pihole.
	URL string `yaml:"url"`
	// Password is an app password, which Overlook logs in with once
	// (ADR 0009). With none, Pi-hole must ask for none.
	Password string `yaml:"password"`

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL}
	if c.Password != "" {
		c.source.Login = &fetch.Login{Path: "/api/auth", Credentials: map[string]string{"password": c.Password}, Take: sid}
	}
	if err := c.source.Check(); err != nil {
		return err
	}
	c.Name = cmp.Or(c.Name, "Pi-hole")
	return nil
}

// sid takes the session from a login's answer, which Pi-hole sends back as
// X-FTL-SID with each read. A password it refuses answers 401, so a login
// that gets here was let in.
func sid(_ http.Header, body []byte) (http.Header, error) {
	var answer struct {
		Session struct {
			Valid bool   `json:"valid"`
			SID   string `json:"sid"`
		} `json:"session"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil, err
	}
	if !answer.Session.Valid || answer.Session.SID == "" {
		return nil, errors.New("refused")
	}
	return http.Header{"X-FTL-SID": {answer.Session.SID}}, nil
}

type summary struct {
	Queries struct {
		Total   int `json:"total"`
		Blocked int `json:"blocked"`
	} `json:"queries"`
	Gravity struct {
		Blocking   int   `json:"domains_being_blocked"`
		LastUpdate int64 `json:"last_update"`
	} `json:"gravity"`
}

// slot is ten minutes of Pi-hole's lookups, from the time it starts.
type slot struct {
	Timestamp int64 `json:"timestamp"`
	Total     int   `json:"total"`
	Blocked   int   `json:"blocked"`
}

type named struct {
	Name   string `json:"name"`
	IP     string `json:"ip"`
	Domain string `json:"domain"`
	Count  int    `json:"count"`
	Port   int    `json:"port"`
	Stats  struct {
		Response float64 `json:"response"` // seconds
	} `json:"statistics"`
}

// version is one part of Pi-hole as it runs and as it's released.
type version struct {
	Local struct {
		Version string `json:"version"`
	} `json:"local"`
	Remote struct {
		Version string `json:"version"`
	} `json:"remote"`
}

// Read reads the Pi-hole at c, which Check has filled, as at now, whose
// time zone says when today started.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var (
		sum      summary
		history  struct{ History []slot }
		blocking struct {
			Blocking string   `json:"blocking"`
			Timer    *float64 `json:"timer"` // seconds until it's back on
		}
		clients   struct{ Clients []named }
		domains   struct{ Domains []named }
		blocked   struct{ Domains []named }
		upstreams struct{ Upstreams []named }
		versions  struct {
			Version struct {
				Core, Web, FTL version
				// Docker is the image's, in an image only.
				Docker struct {
					Local  string `json:"local"`
					Remote string `json:"remote"`
				} `json:"docker"`
			} `json:"version"`
		}
		messages struct {
			Messages []struct {
				Plain string `json:"plain"`
			} `json:"messages"`
		}
	)
	count := url.Values{"count": {fmt.Sprint(snapshot.TopRows)}}
	for _, r := range []struct {
		path string
		v    any
	}{
		{"/api/stats/summary", &sum},
		{"/api/history", &history},
		{"/api/dns/blocking", &blocking},
		{"/api/stats/top_clients?" + count.Encode(), &clients},
		{"/api/stats/top_domains?" + count.Encode(), &domains},
		{"/api/stats/top_domains?" + url.Values{"count": count["count"], "blocked": {"true"}}.Encode(), &blocked},
		{"/api/stats/upstreams", &upstreams},
		{"/api/info/version", &versions},
		{"/api/info/messages", &messages},
	} {
		if err := c.source.JSON(ctx, r.path, r.v); err != nil {
			return snapshot.Parts{}, err
		}
	}

	// Pi-hole keeps its last day in slots of ten minutes, each at its
	// start.
	r := snapshot.Resolver{
		Total: sum.Queries.Total, Blocked: sum.Queries.Blocked, Over: 24 * time.Hour,
		Blocking: blocking.Blocking == "enabled",
	}
	for _, s := range history.History {
		r.Counts = append(r.Counts, snapshot.Count{At: time.Unix(s.Timestamp, 0), Lookups: s.Total, Blocked: s.Blocked})
	}
	if blocking.Timer != nil {
		r.BackIn = time.Duration(*blocking.Timer * float64(time.Second))
	}
	// Its own blocklist and cache answer without an upstream.
	for _, u := range upstreams.Upstreams {
		if u.Port >= 0 {
			r.Upstreams = append(r.Upstreams, snapshot.Upstream{Name: cmp.Or(u.Name, u.IP), Answers: u.Count, Average: time.Duration(u.Stats.Response * float64(time.Second))})
		}
	}
	for _, cl := range clients.Clients {
		t := snapshot.Top{Name: cmp.Or(cl.Name, cl.IP), N: cl.Count}
		if cl.Name != "" {
			t.Address = cl.IP
		}
		r.Clients = append(r.Clients, t)
	}
	for _, d := range domains.Domains {
		r.Domains = append(r.Domains, snapshot.Top{Name: d.Domain, N: d.Count})
	}
	for _, d := range blocked.Domains {
		r.BlockedDomains = append(r.BlockedDomains, snapshot.Top{Name: d.Domain, N: d.Count})
	}

	r.Rows = []snapshot.Row{{
		Title:  snapshot.Text{Key: "pihole.blocklists"},
		Value:  snapshot.Msg("pihole.domains", map[string]any{"n": sum.Gravity.Blocking}),
		Detail: snapshot.Msg("pihole.updated", map[string]any{"at": time.Unix(sum.Gravity.LastUpdate, 0)}),
	}}
	// What Pi-hole warns about, as it words it, such as a client it
	// rate-limits.
	for _, m := range messages.Messages {
		r.Rows = append(r.Rows, snapshot.Row{Title: snapshot.Text{Key: "pihole.warning"}, Tone: "orange", Detail: &snapshot.Text{Name: m.Plain}})
	}
	// Each part of Pi-hole, and the release that's out if it's newer: the
	// image's alone in an image, which is updated whole.
	v := versions.Version
	type release struct{ key, local, remote string }
	parts := []release{
		{"pihole.core", v.Core.Local.Version, v.Core.Remote.Version},
		{"pihole.ftl", v.FTL.Local.Version, v.FTL.Remote.Version},
		{"pihole.web", v.Web.Local.Version, v.Web.Remote.Version},
	}
	if v.Docker.Local != "" {
		parts = []release{{"pihole.image", v.Docker.Local, v.Docker.Remote}}
	}
	for _, p := range parts {
		row := snapshot.Row{Title: snapshot.Text{Key: p.key}, Value: &snapshot.Text{Name: p.local}}
		if p.remote != "" && p.remote != p.local {
			row.Value, row.Tone = snapshot.Msg("pihole.newer", map[string]any{"version": p.remote}), "orange"
			row.Detail = &snapshot.Text{Name: p.local}
		}
		r.Rows = append(r.Rows, row)
	}
	return r.Parts(Area, now), nil
}
