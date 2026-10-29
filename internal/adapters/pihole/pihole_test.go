package pihole

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// testdata holds answers recorded from a scratch Pi-hole 2026.10 (ADR 0008),
// which testdata/fill.sh fills: 57 lookups from four invented clients and
// Pi-hole itself, 18 of them of two denied domains, and the living-room TV
// rate-limited for asking too many. One slot of its history is invented
// beside them, in the same shape: 120 lookups at 21:15 the evening before.
// With a password, Overlook logs in once, and sends the session it gets with
// every read, as X-FTL-SID.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Password: "app-password"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Date(2026, 10, 29, 11, 20, 0, 0, london))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Area, p)
	if _, err := Read(context.Background(), c, time.Date(2026, 10, 29, 11, 20, 30, 0, london)); err != nil {
		t.Fatal(err)
	}
	sent := src.Sent()
	if login := sent[0]; login.Method != http.MethodPost || login.URL.Path != "/api/auth" || login.Body != `{"password":"app-password"}` {
		t.Errorf("first, the login: %+v", login)
	}
	for _, r := range sent[1:] {
		if r.Header.Get("X-FTL-SID") != "0123abcd0123abcd0123ab=" {
			t.Errorf("%s: read without the session", r.URL)
		}
	}

	// Its ten minutes in the hours of Overlook's time zone.
	f := p.Figures[0]
	if f.Value != 57 || f.Detail.Args["blocked"] != 18 {
		t.Errorf("57 lookups since midnight, 18 blocked: %+v", f)
	}
	if f.Hours[23] != 57 || f.Hours[9] != 120 || f.Within.Hours[23] != 18 || f.Within.Hours[9] != 31 {
		t.Errorf("this hour's and 21:00's, by the hour: %v %v", f.Hours, f.Within.Hours)
	}
	if ch := p.Charts[0]; len(ch.Series[0].Hours) != 24 || ch.Series[0].Hours[9] != 120 {
		t.Errorf("the last day by the hour: %+v", ch)
	}

	rows := p.Sections[0].Rows
	if len(rows) != 7 {
		t.Fatalf("blocking, lookups, blocked, one upstream, blocklists, the rate limit and the image: %+v", rows)
	}
	if r := rows[0]; r.Value.Key != "resolver.on" {
		t.Errorf("blocking: %+v", r)
	}
	if r := rows[2]; r.Value.Args["n"] != 31.6 || r.Detail.Args["n"] != 18 {
		t.Errorf("31.6%% blocked: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[3]; r.Title.Name != "dns.google" || r.Value.Args["n"] != 15 {
		t.Errorf("the one upstream, not the blocklist or the cache: %+v", r)
	}
	if r := rows[4]; r.Value.Args["n"] != 76148 {
		t.Errorf("the blocklists' domains: %+v", r.Value)
	}
	if r := rows[5]; r.Tone != "orange" || !strings.Contains(r.Detail.Name, "Rate-limiting 172.31.53.12") {
		t.Errorf("the rate limit, as Pi-hole words it: %+v", r)
	}
	if r := rows[6]; r.Title.Key != "pihole.image" || r.Value.Name != "2026.10.0" || r.Tone != "" {
		t.Errorf("the image, up to date, and not its parts: %+v %+v", r, rows[7:])
	}
	clients := p.Sections[1].Rows
	if r := clients[0]; r.Title.Name != "living-room-tv" || r.Detail.Name != "172.31.53.12" || r.Value.Args["n"] != 20 {
		t.Errorf("the TV first, by its name: %+v", r)
	}
	if r := clients[len(clients)-1]; r.Title.Name != "127.0.0.1" || r.Detail != nil {
		t.Errorf("Pi-hole itself, by its address: %+v", r)
	}
	if denied := p.Sections[3].Rows; len(denied) != 2 || denied[0].Title.Name != "telemetry.example.net" {
		t.Errorf("the two denied domains: %+v", denied)
	}
}

// Without a password, for a Pi-hole that has none, Overlook never logs in,
// and a password Pi-hole refuses is an error that says so.
func TestLogin(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("testdata")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "api", "auth.json")); err != nil {
		t.Fatal(err)
	}
	src := adaptertest.Replay(t, dir)
	c := Config{URL: src.URL}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c, time.Date(2026, 10, 29, 11, 20, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	for _, r := range src.Sent() {
		if r.Method != http.MethodGet || r.Header.Get("X-FTL-SID") != "" {
			t.Errorf("%s %s: without a password, a login or a session", r.Method, r.URL)
		}
	}

	refused := t.TempDir()
	if err := os.Mkdir(filepath.Join(refused, "api"), 0o750); err != nil {
		t.Fatal(err)
	}
	answer := "HTTP/1.1 401 Unauthorized\r\nContent-Type: application/json; charset=utf-8\r\n\r\n" +
		`{"session":{"valid":false,"totp":false,"sid":null,"validity":-1,"message":"password incorrect"},"took":0.00012}`
	if err := os.WriteFile(filepath.Join(refused, "api", "auth.http"), []byte(answer), 0o600); err != nil {
		t.Fatal(err)
	}
	c = Config{URL: adaptertest.Replay(t, refused).URL, Password: "wrong"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c, time.Date(2026, 10, 29, 11, 20, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "login: HTTP 401") {
		t.Errorf("a password refused: %v", err)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://pihole"}
	if err := c.Check(); err != nil || c.Name != "Pi-hole" || c.source.Login != nil {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{}, "url is missing"},
		{Config{URL: "pihole"}, "url needs"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
