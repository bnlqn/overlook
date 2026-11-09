package npm

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// recorded is the moment testdata was recorded.
var recorded = time.Date(2026, 11, 9, 20, 36, 56, 0, time.UTC)

// replay replays testdata, with edits, to Overlook's user, and returns what
// it makes of them and what it was sent.
func replay(t *testing.T, edits ...string) (snapshot.Parts, []adaptertest.Request) {
	t.Helper()
	src := adaptertest.Replay(t, "testdata", edits...)
	c := Config{URL: src.URL, User: "overlook@example.com", Password: "Overlook-password-1", Link: "https://npm.example.com/"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Adapter, p)
	return p, src.Sent()
}

// testdata holds answers recorded from a scratch Nginx Proxy Manager 2.16.0
// (ADR 0008), which testdata/fill.sh fills: photos and cinema, also
// films, proxy hosts with certificates, Films' ending in 12 days; wiki,
// pointing at an address nothing answers on, which nginx can't tell; dns,
// whose advanced config nginx refused; grafana, disabled; blog, a
// redirection; old, a 404 host; a stream on port 2222, whose upstream nginx
// couldn't find, and one on 51820. Its version is the latest.
func TestRead(t *testing.T) {
	t.Parallel()
	p, sent := replay(t)

	login := sent[0]
	if login.Method != http.MethodPost || login.URL.Path != "/api/tokens" ||
		login.Body != `{"identity":"overlook@example.com","secret":"Overlook-password-1"}` {
		t.Errorf("first, the login: %+v", login)
	}
	// The four lists of hosts, each with its certificates, and the version.
	if len(sent) != 6 {
		t.Errorf("one login and five reads, got %d requests", len(sent))
	}
	for _, r := range sent[1:] {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer scratch-token-not-a-real-one" {
			t.Errorf("%s %s: read without the token", r.Method, r.URL)
		}
	}

	if len(p.Sections) != 3 || p.Sections[0].Title.Key != "npm.domains" || p.Sections[1].Title.Key != "npm.streams" ||
		p.Sections[2].Title.Key != "npm.certificates" {
		t.Fatalf("three sections, the domains, the streams and the certificates: %+v", p.Sections)
	}
	rows := slices.Concat(p.Sections[0].Rows, p.Sections[1].Rows)
	want := []struct{ title, key, tone, detail string }{
		{"blog.example.com", "npm.redirect", "", "→ https://www.example.com/blog"},
		{"cinema.example.com", "npm.proxy", "", "films.example.com · → http://jellyfin.example.com:8096"},
		{"dns.example.com", "npm.refused", "red", `unknown directive "proxy_read_timout" in /data/nginx/proxy_host/4.conf:37`},
		{"grafana.example.com", "npm.disabled", "gray", "→ http://grafana.example.com:3000"},
		{"old.example.com", "npm.dead", "", ""},
		{"photos.example.com", "npm.proxy", "", "→ http://immich.example.com:2283"},
		{"wiki.example.com", "npm.proxy", "", "→ http://10.0.10.99:8080"},
		{"version", "", "", ""},
		{"2222", "npm.refused", "red", `host not found in upstream "git.example.com" in /data/nginx/stream/1.conf:10`},
		{"51820", "npm.udp", "", "→ 10.0.10.1:51820"},
	}
	if len(rows) != len(want) {
		t.Fatalf("seven domains and the version, then two streams: %+v", rows)
	}
	for i, w := range want {
		r := rows[i]
		title := r.Title.Name
		switch r.Title.Key {
		case "npm.port":
			title = r.Title.Args["port"].(string)
		case "version":
			if r.Value.Name != "v2.16.0" || r.Tone != "" {
				t.Errorf("its version, the latest: %+v", r)
			}
			continue
		}
		detail := ""
		if r.Detail != nil {
			detail = r.Detail.Name
		}
		if title != w.title || r.Value.Key != w.key || r.Tone != w.tone || detail != w.detail {
			t.Errorf("%s: want %s in %q, %q, got %+v", w.title, w.key, w.tone, w.detail, r)
		}
	}
	if blog := rows[0].Value.Args["code"]; blog != "301" {
		t.Errorf("blog redirects with a 301: %v", blog)
	}

	certs := p.Sections[2].Rows
	if len(certs) != 2 {
		t.Fatalf("two certificates, Wildcard once though two hosts serve it: %+v", certs)
	}
	films, wildcard := certs[0], certs[1]
	if films.Title.Name != "Films" || films.Tone != "orange" || films.Value.Key != "npm.expires" ||
		films.Value.Args["ms"] != (12*24*time.Hour-2*time.Second).Milliseconds() ||
		films.Detail.Key != "npm.custom" || films.Detail.Args["domains"] != "films.example.com" {
		t.Errorf("Films, uploaded, ends in 12 days, in orange: %+v", films)
	}
	if wildcard.Title.Name != "Wildcard" || wildcard.Tone != "" || wildcard.Detail.Args["domains"] != "*.example.com" {
		t.Errorf("Wildcard, ending in a year: %+v", wildcard)
	}

	if len(p.Chores) != 1 {
		t.Fatalf("one chore, Films: %+v", p.Chores)
	}
	if c := p.Chores[0]; c.Name != "Films" || c.Note != "films.example.com · 2026-11-21" || c.From != "2026-10-22" ||
		c.URL != "https://npm.example.com/certificates" {
		t.Errorf("Films, due since 30 days before it ends, renewed in Nginx Proxy Manager: %+v", c)
	}
}

// Nginx Proxy Manager says when a newer one is out.
func TestReadNewer(t *testing.T) {
	t.Parallel()
	p, _ := replay(t, `"latest": "v2.16.0"`, `"latest": "v2.17.0"`, `"update_available": false`, `"update_available": true`)
	rows := p.Sections[0].Rows
	if v := rows[len(rows)-1]; v.Value.Key != "version.newer" || v.Value.Args["version"] != "v2.17.0" ||
		v.Tone != "orange" || v.Detail.Name != "v2.16.0" {
		t.Errorf("v2.17.0 is out, in orange, over v2.16.0: %+v", v)
	}
}

// One that couldn't ask GitHub gives no version, and the page shows none.
func TestReadNoVersion(t *testing.T) {
	t.Parallel()
	p, _ := replay(t, `"current": "v2.16.0"`, `"current": null`)
	rows := p.Sections[0].Rows
	if v := rows[len(rows)-1]; v.Title.Key == "version" {
		t.Errorf("no version: %+v", v)
	}
}

// A Let's Encrypt certificate that has ended is red, and a chore, and a
// redirection that keeps the scheme it was asked with names none.
func TestParts(t *testing.T) {
	t.Parallel()
	ended := &certificate{ID: 3, Provider: "letsencrypt", DomainNames: []string{"git.example.com"}, ExpiresOn: "2026-11-06 20:36:56"}
	p := parts(read{
		redirections: []item{{DomainNames: []string{"git.example.com"}, Enabled: true, Scheme: "auto", ForwardDomain: "code.example.com", Code: 302, Certificate: ended}},
		streams:      []item{{Port: 53, Enabled: true, TCP: true, UDP: true, ForwardingHost: "10.0.10.53", ForwardingPort: 53}},
	}, "https://npm.example.com", recorded)
	adaptertest.Validate(t, Adapter, p)
	if r := p.Sections[0].Rows[0]; r.Detail.Name != "→ code.example.com" {
		t.Errorf("a redirection that keeps its scheme: %+v", r)
	}
	if r := p.Sections[1].Rows[0]; r.Value.Key != "npm.tcp_udp" || r.Title.Args["port"] != "53" {
		t.Errorf("a stream of both TCP and UDP on port 53: %+v", r)
	}
	c := p.Sections[2].Rows[0]
	if c.Title.Name != "git.example.com" || c.Tone != "red" || c.Value.Key != "npm.expired" ||
		c.Value.Args["ms"] != (3*24*time.Hour).Milliseconds() || c.Detail.Key != "npm.letsencrypt" {
		t.Errorf("ended three days ago, named after its domain, from Let's Encrypt: %+v", c)
	}
	if len(p.Chores) != 1 || p.Chores[0].Name != "git.example.com" || p.Chores[0].Note != "2026-11-06" {
		t.Errorf("a chore, its note the day it ended, since its name is its domain: %+v", p.Chores)
	}
}

// A user with two-factor authentication can't be logged in as, and says so.
func TestTokenTwoFactor(t *testing.T) {
	t.Parallel()
	if _, err := token(nil, []byte(`{"requires_2fa": true, "challenge_token": "abc"}`)); err == nil || !strings.Contains(err.Error(), "two-factor") {
		t.Errorf("want an error naming two-factor authentication, got %v", err)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://npm:81", User: "overlook@example.com", Password: "s3cret"}
	if err := c.Check(); err != nil || c.Name != "Nginx Proxy Manager" || c.Link != "http://npm:81" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	if c.source.Login.Lapsed != http.StatusBadRequest {
		t.Errorf("a token past its day is refused with a 400, which logs in again: %d", c.source.Login.Lapsed)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{User: "overlook@example.com", Password: "s3cret"}, "url is missing"},
		{Config{URL: "http://npm:81", User: "overlook@example.com"}, "user and password are missing"},
		{Config{URL: "http://npm:81", Password: "s3cret"}, "user and password are missing"},
		{Config{URL: "http://npm:81", User: "overlook@example.com", Password: "s3cret", Link: "npm.example.com"}, "link"},
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
		c := Config{URL: url, User: "overlook@example.com", Password: "Overlook-password-1"}
		if err := c.Check(); err != nil {
			return snapshot.Parts{}, err
		}
		return Read(context.Background(), c, recorded)
	})
}
