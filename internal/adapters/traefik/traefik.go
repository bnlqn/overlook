// Package traefik reads Traefik's API: how many HTTP routers, services and
// middlewares it has, each it holds in error or warns about, as Traefik
// words it, each service whose servers it checks, in red when one is down,
// and its certificates by when each expires, those within 30 days as
// chores. Everything it gives goes in Network.
package traefik

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "network"

// Adapter says what this adapter is, for the config and its test.
var Adapter = snapshot.Adapter[Config]{Key: "traefik", Area: Area, Read: Read}

// renew is how long before a certificate expires it becomes a chore: when
// Traefik's own API warns of it, and when Traefik renews a 90-day one from
// Let's Encrypt, as theirs are by default, so such a certificate is a chore
// only once its renewal fails.
const renew = 30 * 24 * time.Hour

// page asks Traefik for each list on one page, which it otherwise splits
// into pages of 100.
//
// ponytail: a Traefik of more than 10,000 routers shows its first 10,000;
// follow its X-Next-Page if a homelab ever has more.
const page = "?per_page=10000"

// Config is one Traefik, under sources.traefik in overlook.yaml.
type Config struct {
	Name string `yaml:"name" setup:"text,optional"`
	// URL is where Overlook reads Traefik's API, the address its /api is
	// under, such as http://traefik:8080.
	URL string `yaml:"url" setup:"text" example:"https://traefik.example.com"`
	// User and Password are the login of the basicAuth middleware in front
	// of the API, if it has one.
	User     string `yaml:"user" setup:"text,optional"`
	Password string `yaml:"password" setup:"key,optional"`
	// Link is where the page sends the owner, if not URL, such as its
	// dashboard.
	Link string `yaml:"link"`

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL, Header: fetch.Basic(c.User, c.Password)}
	if err := c.source.Check(); err != nil {
		return err
	}
	if (c.User == "") != (c.Password == "") {
		return errors.New("give user and password together, or neither")
	}
	c.Name = cmp.Or(c.Name, "Traefik")
	return fetch.CheckLink(&c.Link, c.URL)
}

// item is one router, service or middleware: enabled, warning, or disabled
// for one Traefik holds in error, with what it says of it.
type item struct {
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Error  []string `json:"error"`
}

// service is one service, with how its servers are, by address, UP or
// DOWN, where Traefik checks them, actively or by the requests it sends.
type service struct {
	item
	LoadBalancer *struct {
		HealthCheck        any `json:"healthCheck"`
		PassiveHealthCheck any `json:"passiveHealthCheck"`
	} `json:"loadBalancer"`
	ServerStatus map[string]string `json:"serverStatus"`
}

// checked says whether Traefik checks the service's servers, which it
// otherwise always takes to be up.
func (s service) checked() bool {
	return s.LoadBalancer != nil && (s.LoadBalancer.HealthCheck != nil || s.LoadBalancer.PassiveHealthCheck != nil)
}

// certificate is one Traefik serves, from a file or a resolver such as
// Let's Encrypt.
type certificate struct {
	CommonName string    `json:"commonName"`
	SANs       []string  `json:"sans"`
	NotAfter   time.Time `json:"notAfter"`
	Issuer     string    `json:"issuerCN"`
}

// name is what the certificate is for: its common name, or, with none, as
// a certificate may have, its first other name.
func (c certificate) name() string {
	if c.CommonName != "" || len(c.SANs) == 0 {
		return c.CommonName
	}
	return c.SANs[0]
}

// read is everything Read reads.
type read struct {
	routers, middlewares []item
	services             []service
	certificates         []certificate
}

// Read reads the Traefik at c.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var r read
	for _, at := range []struct {
		path string
		v    any
	}{
		{"/api/http/routers", &r.routers},
		{"/api/http/services", &r.services},
		{"/api/http/middlewares", &r.middlewares},
		{"/api/certificates", &r.certificates},
	} {
		if err := c.source.JSON(ctx, at.path+page, at.v); err != nil {
			return snapshot.Parts{}, err
		}
	}
	return parts(r, c.Link, now), nil
}

// tone is the colour of an item's status: red in error, orange warned of.
func tone(status string) string {
	switch status {
	case "disabled":
		return "red"
	case "warning":
		return "orange"
	}
	return ""
}

func parts(r read, link string, now time.Time) snapshot.Parts {
	services := make([]item, len(r.services))
	for i, s := range r.services {
		services[i] = s.item
	}

	// How many routers, services and middlewares there are, each in the
	// colour of the worst, then each in error or warned of, by kind and by
	// name, with what Traefik says of it.
	kinds := []struct {
		key   string
		items []item
	}{{"router", r.routers}, {"service", services}, {"middleware", r.middlewares}}
	rows := []snapshot.Row{}
	for _, k := range kinds {
		row := snapshot.Row{Title: snapshot.Text{Key: "traefik." + k.key + "s"}, Value: snapshot.Number(len(k.items))}
		for _, it := range k.items {
			if t := tone(it.Status); t == "red" || row.Tone == "" {
				row.Tone = cmp.Or(t, row.Tone)
			}
		}
		rows = append(rows, row)
	}
	for _, k := range kinds {
		items := slices.SortedFunc(slices.Values(k.items), func(a, b item) int { return cmp.Compare(a.Name, b.Name) })
		for _, it := range items {
			if it.Name == "" || tone(it.Status) == "" {
				continue
			}
			row := snapshot.Row{
				Title: snapshot.Text{Name: it.Name},
				Value: snapshot.Msg("traefik."+k.key+"_"+it.Status, nil), Tone: tone(it.Status),
			}
			if errs := slices.DeleteFunc(slices.Clone(it.Error), func(e string) bool { return e == "" }); len(errs) > 0 {
				row.Detail = &snapshot.Text{Name: strings.Join(errs, "; ")}
			}
			rows = append(rows, row)
		}
	}

	// Each service whose servers Traefik checks, by name: all up, or in red
	// with how many are down, and which.
	checked := slices.DeleteFunc(slices.Clone(r.services), func(s service) bool { return s.Name == "" || !s.checked() || len(s.ServerStatus) == 0 })
	slices.SortFunc(checked, func(a, b service) int { return cmp.Compare(a.Name, b.Name) })
	for _, s := range checked {
		var down []string
		for _, server := range slices.Sorted(maps.Keys(s.ServerStatus)) {
			if s.ServerStatus[server] != "UP" {
				down = append(down, server)
			}
		}
		row := snapshot.Row{Title: snapshot.Text{Name: s.Name}, Value: snapshot.Msg("traefik.servers_up", map[string]any{"n": len(s.ServerStatus)})}
		if len(down) > 0 {
			row.Value = snapshot.Msg("traefik.servers_down", map[string]any{"down": len(down), "n": len(s.ServerStatus)})
			row.Tone, row.Detail = "red", &snapshot.Text{Name: strings.Join(down, ", ")}
		}
		rows = append(rows, row)
	}

	// Each certificate, by when it expires, sooner first: in orange within
	// 30 days, then a chore from the day it came within them, and in red
	// once it has expired.
	certs := slices.DeleteFunc(slices.Clone(r.certificates), func(c certificate) bool { return c.name() == "" || c.NotAfter.IsZero() })
	slices.SortFunc(certs, func(a, b certificate) int {
		return cmp.Or(a.NotAfter.Compare(b.NotAfter), cmp.Compare(a.name(), b.name()))
	})
	crows := []snapshot.Row{}
	chores := []snapshot.Chore{}
	day := func(t time.Time) string { return t.In(now.Location()).Format(time.DateOnly) }
	for _, c := range certs {
		left := c.NotAfter.Sub(now)
		row := snapshot.Row{Title: snapshot.Text{Name: c.name()}, Value: snapshot.Msg("traefik.expires", map[string]any{"ms": left.Milliseconds()})}
		switch {
		case left <= 0:
			row.Value, row.Tone = snapshot.Msg("traefik.expired", map[string]any{"ms": -left.Milliseconds()}), "red"
		case left < renew:
			row.Tone = "orange"
		}
		if c.Issuer != "" {
			row.Detail = snapshot.Msg("traefik.issued_by", map[string]any{"issuer": c.Issuer})
		}
		crows = append(crows, row)
		if left < renew {
			chores = append(chores, snapshot.Chore{Name: c.name(), From: day(c.NotAfter.Add(-renew)), Note: day(c.NotAfter), URL: link})
		}
	}

	sections := []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "traefik.routing"}, Rows: rows}}
	if len(crows) > 0 {
		sections = append(sections, snapshot.Section{Area: Area, Title: snapshot.Text{Key: "traefik.certificates"}, Rows: crows})
	}
	return snapshot.Parts{Sections: sections, Chores: chores}
}
