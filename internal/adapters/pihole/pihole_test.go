package pihole

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// testdata holds answers recorded from a scratch Pi-hole 2026.10 (ADR 0008),
// which testdata/fill.sh fills: 57 lookups from four invented clients and
// Pi-hole itself, 18 of them of two denied domains, and the living-room TV
// rate-limited for asking too many. One slot of its history is invented
// beside them, in the same shape: 120 lookups at 21:15 the evening before.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL}
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
	adaptertest.Validate(t, p)

	f := p.Figures[0]
	if f.Label.Key != snapshot.LookupsFigure.Key || f.Value != 57 || f.Detail.Args["blocked"] != 18 || !f.Adds {
		t.Errorf("57 lookups since midnight, 18 blocked, adding up with any resolver's: %+v", f)
	}
	if f.Hours[23] != 57 || f.Hours[9] != 120 || f.Within.Hours[23] != 18 || f.Within.Hours[9] != 31 {
		t.Errorf("this hour's and 21:00's, by the hour: %v %v", f.Hours, f.Within.Hours)
	}
	if ch := p.Charts[0]; ch.Title.Key != snapshot.LookupsChart.Key || !ch.Adds || len(ch.Series) != 2 || ch.Series[0].Hours[9] != 120 {
		t.Errorf("the last day by the hour, adding up with any resolver's: %+v", ch)
	}

	if len(p.Sections) != 4 || p.Sections[0].Title.Key != snapshot.ResolverSection.Key || p.Sections[3].Title.Key != snapshot.TopBlocked.Key {
		t.Fatalf("the resolver and its three top lists, as any resolver's: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 7 {
		t.Fatalf("blocking, lookups, blocked, blocklists, one upstream, the rate limit and the image: %+v", rows)
	}
	if r := rows[0]; r.Value.Key != "pihole.blocking_on" || r.Tone != "green" {
		t.Errorf("blocking: %+v", r)
	}
	if r := rows[2]; r.Value.Args["n"] != 31.6 || r.Detail.Args["n"] != 18 {
		t.Errorf("31.6%% blocked: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[3]; r.Value.Args["n"] != 76148 {
		t.Errorf("the blocklists' domains: %+v", r.Value)
	}
	if r := rows[4]; r.Title.Name != "dns.google" || r.Value.Args["n"] != 15 {
		t.Errorf("the one upstream, not the blocklist or the cache: %+v", r)
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

// With a password, Overlook logs in once and sends the session it gets with
// every read, as X-FTL-SID; Pi-hole refuses a read without it with a 401.
func TestLogin(t *testing.T) {
	t.Parallel()
	answer, err := os.ReadFile(filepath.Join("testdata", "api", "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	logins := &atomic.Int32{}
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/auth":
			logins.Add(1)
			if body, _ := io.ReadAll(r.Body); !strings.Contains(string(body), `"password":"app-password"`) {
				http.Error(w, `{"session":{"valid":false}}`, http.StatusUnauthorized)
				return
			}
			_, _ = w.Write(answer)
		case r.Method == http.MethodGet && r.Header.Get("X-FTL-SID") == "0123abcd0123abcd0123ab=":
			name := strings.TrimPrefix(r.URL.Path, "/")
			if r.URL.Query().Get("blocked") != "" {
				name += "@" + r.URL.RawQuery
			}
			http.ServeFile(w, r, filepath.Join("testdata", name+".json"))
		default:
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}
	}))
	t.Cleanup(src.Close)

	c := Config{URL: src.URL, Password: "app-password"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := Read(context.Background(), c, time.Date(2026, 10, 29, 11, 20, 0, 0, time.UTC)); err != nil {
			t.Fatal(err)
		}
	}
	if n := logins.Load(); n != 1 {
		t.Errorf("two reads logged in %d times, want once", n)
	}

	c = Config{URL: src.URL, Password: "wrong"}
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
	if err := c.Check(); err != nil || c.Name != "Pi-hole" || c.session != nil {
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
