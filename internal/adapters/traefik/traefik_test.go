package traefik

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// recorded is the moment testdata was recorded.
var recorded = time.Date(2026, 11, 9, 20, 39, 30, 0, time.UTC)

// testdata holds answers recorded from a scratch Traefik 3.7.17
// (compose.yaml, dynamic.yaml, fill.sh and record.sh beside them), read
// through its API's own router behind a basicAuth middleware: photos, with
// a health check, and blog work, though blog names an entry point that
// doesn't exist; films' health check finds its second server down; wiki
// names a middleware that doesn't exist; printer's middleware allows a
// range that isn't one; shop weighs a service that doesn't exist; and its
// two self-signed certificates are good for a year, photos', and 10 days,
// films'.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, User: "overlook", Password: "scratch-instance-only-not-a-password"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Adapter, p)
	login := fetch.Basic("overlook", "scratch-instance-only-not-a-password").Get("Authorization")
	for _, r := range src.Sent() {
		if r.Header.Get("Authorization") != login {
			t.Errorf("%s: read without the login", r.URL)
		}
	}

	if len(p.Sections) != 2 || p.Sections[0].Title.Key != "traefik.routing" || p.Sections[1].Title.Key != "traefik.certificates" {
		t.Fatalf("two sections, the routing and the certificates: %+v", p.Sections)
	}
	want := []struct{ title, value, tone, detail string }{
		{"traefik.routers", "number", "red", ""},
		{"traefik.services", "number", "red", ""},
		{"traefik.middlewares", "number", "red", ""},
		{"blog@file", "traefik.router_warning", "orange", `entryPoint "old-web" doesn't exist`},
		{"printer@file", "traefik.router_disabled", "red", "cannot parse CIDRs [192.168.1.0/33]"},
		{"shop@file", "traefik.router_disabled", "red", `the service "shop-green@file" does not exist`},
		{"wiki@file", "traefik.router_disabled", "red", `middleware "sso@file" does not exist`},
		{"shop@file", "traefik.service_disabled", "red", `the service "shop-green@file" does not exist`},
		{"lan-only@file", "traefik.middleware_disabled", "red", "cannot parse CIDRs [192.168.1.0/33]"},
		{"films@file", "traefik.servers_down", "red", "http://films-standby"},
		{"photos@file", "traefik.servers_up", "", ""},
	}
	rows := p.Sections[0].Rows
	if len(rows) != len(want) {
		t.Fatalf("%d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i, w := range want {
		r := rows[i]
		title := r.Title.Key + r.Title.Name
		if title != w.title || r.Value.Key != w.value || r.Tone != w.tone || (w.detail != "" && (r.Detail == nil || !strings.Contains(r.Detail.Name, w.detail))) {
			t.Errorf("row %d: want %s, %s in %q, with %q, got %+v", i, w.title, w.value, w.tone, w.detail, r)
		}
	}
	for i, n := range []int{7, 10, 2} {
		if rows[i].Value.Args["n"] != n {
			t.Errorf("%s: want %d, got %v", rows[i].Title.Key, n, rows[i].Value.Args)
		}
	}
	if a := rows[9].Value.Args; a["down"] != 1 || a["n"] != 2 {
		t.Errorf("films: 1 of its 2 servers down: %v", a)
	}

	certs := p.Sections[1].Rows
	if len(certs) != 2 {
		t.Fatalf("two certificates: %+v", certs)
	}
	films, photos := certs[0], certs[1]
	if films.Title.Name != "films.example.com" || films.Value.Key != "traefik.expires" || films.Tone != "orange" ||
		films.Value.Args["ms"] != time.Date(2026, 11, 19, 20, 28, 56, 0, time.UTC).Sub(recorded).Milliseconds() ||
		films.Detail.Args["issuer"] != "films.example.com" {
		t.Errorf("films' expires first, within 30 days, in orange, self-signed: %+v", films)
	}
	if photos.Title.Name != "photos.example.com" || photos.Tone != "" {
		t.Errorf("photos' is good for a year: %+v", photos)
	}
	if len(p.Chores) != 1 || p.Chores[0] != (snapshot.Chore{Name: "films.example.com", From: "2026-10-20", Note: "2026-11-19", URL: src.URL}) {
		t.Errorf("films' certificate is a chore, from 30 days before it expires: %+v", p.Chores)
	}
}

// A Traefik with nothing wrong shows its counts alone; a certificate that
// has expired is red, and a chore; a service checked by the requests
// Traefik sends shows its servers too.
func TestParts(t *testing.T) {
	t.Parallel()
	var passive service
	passive.Name, passive.Status = "music@docker", "enabled"
	passive.LoadBalancer = &struct {
		HealthCheck        any `json:"healthCheck"`
		PassiveHealthCheck any `json:"passiveHealthCheck"`
	}{PassiveHealthCheck: map[string]any{}}
	passive.ServerStatus = map[string]string{"http://192.0.2.10:4533": "UP", "http://192.0.2.11:4533": "UP"}
	r := read{
		routers:      []item{{Name: "music@docker", Status: "enabled"}},
		services:     []service{passive},
		certificates: []certificate{{SANs: []string{"*.example.com"}, NotAfter: recorded.Add(-36 * time.Hour), Issuer: "R12"}},
	}
	p := parts(r, "https://traefik.example.com/dashboard/", recorded)
	adaptertest.Validate(t, Adapter, p)
	rows := p.Sections[0].Rows
	if len(rows) != 4 || rows[0].Tone != "" || rows[3].Title.Name != "music@docker" ||
		rows[3].Value.Key != "traefik.servers_up" || rows[3].Value.Args["n"] != 2 {
		t.Errorf("the counts, none in colour, then music's two servers up: %+v", rows)
	}
	cert := p.Sections[1].Rows[0]
	if cert.Title.Name != "*.example.com" || cert.Value.Key != "traefik.expired" || cert.Tone != "red" ||
		cert.Value.Args["ms"] != (36*time.Hour).Milliseconds() {
		t.Errorf("a certificate with no common name, by its other name, expired 36 hours ago: %+v", cert)
	}
	if len(p.Chores) != 1 || p.Chores[0].From != "2026-10-09" || p.Chores[0].URL != "https://traefik.example.com/dashboard/" {
		t.Errorf("the expired certificate is a chore: %+v", p.Chores)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://traefik:8080"}
	if err := c.Check(); err != nil || c.Name != "Traefik" || c.Link != "http://traefik:8080" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{}, "url is missing"},
		{Config{URL: "http://traefik:8080", User: "overlook"}, "user and password together"},
		{Config{URL: "http://traefik:8080", Link: "traefik.example.com"}, "link"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}

// An odd item in one of its answers costs a row, never the source.
func TestBend(t *testing.T) {
	t.Parallel()
	adaptertest.Bend(t, "testdata", func(url string) (snapshot.Parts, error) {
		c := Config{URL: url, User: "overlook", Password: "scratch-instance-only-not-a-password"}
		if err := c.Check(); err != nil {
			return snapshot.Parts{}, err
		}
		return Read(context.Background(), c, recorded)
	})
}
