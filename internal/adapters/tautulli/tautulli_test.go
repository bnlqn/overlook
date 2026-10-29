package tautulli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// source answers as Tautulli does, every command at /api/v2 with the key in
// the address, from testdata/api/v2/<command>.json with each of edits made,
// as old and new, and keeps the commands it was sent.
func source(t *testing.T, edits ...string) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var sent []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		mu.Lock()
		sent = append(sent, q.Get("cmd"))
		mu.Unlock()
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2" || q.Get("apikey") != "0123abcd" {
			http.Error(w, `{"response":{"result":"error","message":"Invalid apikey"}}`, http.StatusUnauthorized)
			return
		}
		answer, err := os.ReadFile(filepath.Join("testdata", "api", "v2", filepath.Base(q.Get("cmd"))+".json"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		body := string(answer)
		for i := 0; i+1 < len(edits); i += 2 {
			body = strings.ReplaceAll(body, edits[i], edits[i+1])
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s, func() []string { mu.Lock(); defer mu.Unlock(); return slices.Clone(sent) }
}

// testdata holds answers recorded from a scratch Tautulli 2.18.2 watching a
// scratch Plex 1.43.5 (ADR 0008), which testdata/fill.sh fills: five films and
// a show of three episodes, four plays finished, Tautulli logging the
// episode's twice, and two going, the TV's as it is and Kim's phone's
// transcoded. The Plex server is unclaimed, so every play is its local
// user's and Tautulli has no news of its releases. record.sh read them at
// 12:37:33.
func TestRead(t *testing.T) {
	t.Parallel()
	src, sent := source(t)
	c := Config{URL: src.URL, Key: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Date(2026, 10, 29, 12, 37, 33, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	// Each command it sent reads, and is one of reads, which name only
	// commands that read.
	for _, cmd := range sent() {
		if !slices.Contains(reads, cmd) {
			t.Errorf("sent %s, which isn't in reads", cmd)
		}
	}
	for _, cmd := range reads {
		if !strings.HasPrefix(cmd, "get_") && cmd != "server_status" {
			t.Errorf("%s doesn't read", cmd)
		}
	}

	rows := p.Sections[0].Rows
	if len(rows) != 5 {
		t.Fatalf("two playing, the bandwidth and two libraries: %+v", rows)
	}
	if r := rows[0]; r.Title.Name != "Local" || r.Value.Name != "Sintel (2010)" || r.Detail.Key != "tautulli.transcoding" || r.Detail.Args["device"] != "Kim's phone" {
		t.Errorf("Kim's phone first by player, transcoded: %+v %+v", r, r.Detail)
	}
	if r := rows[1]; r.Value.Name != "Metropolis (1927)" || r.Detail.Key != "tautulli.direct" {
		t.Errorf("the TV's, as it is: %+v %+v", r, r.Detail)
	}
	if r := rows[2]; r.Title.Key != "tautulli.bandwidth" || r.Value.Args["n"] != 596*1000/8 {
		t.Errorf("596 kb/s, all of it on the LAN: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[3]; r.Title.Name != "Films" || r.Value.Key != "tautulli.films_n" || r.Value.Args["n"] != 5 {
		t.Errorf("5 films: %+v", r)
	}
	if r := rows[4]; r.Title.Name != "Shows" || r.Value.Args["n"] != 1 || r.Value.Args["children"] != 3 {
		t.Errorf("a show of 3 episodes: %+v", r.Value)
	}

	f := p.Figures[0]
	if f.Label.Key != "figure.plays" || f.Value != 7 || f.Hours[23] != 7 || f.Detail.Args["n"] != 2 || !f.Adds {
		t.Errorf("7 plays since midnight, the two going too, adding up with any media server's: %+v", f)
	}
}

// A newer Plex is said, when Tautulli has news of it, and a Plex server
// Tautulli can't reach is said in red.
func TestPlex(t *testing.T) {
	t.Parallel()
	src, _ := source(t, `"data": {}`, `"data": {"update_available": true, "version": "1.43.6.11200-0a1b2c3d4"}`)
	c := Config{URL: src.URL, Key: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Date(2026, 10, 29, 12, 37, 33, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	rows := p.Sections[0].Rows
	if r := rows[len(rows)-1]; r.Title.Key != "tautulli.plex" || r.Tone != "orange" || r.Value.Args["version"] != "1.43.6.11200-0a1b2c3d4" {
		t.Errorf("a newer Plex: %+v", r)
	}

	src, _ = source(t, `"connected": true`, `"connected": false`)
	c = Config{URL: src.URL, Key: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if p, err = Read(context.Background(), c, time.Date(2026, 10, 29, 12, 37, 33, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if r := p.Sections[0].Rows[0]; r.Value.Key != "tautulli.unreachable" || r.Tone != "red" || p.Figures[0].Tone != "orange" {
		t.Errorf("Plex out of reach: %+v", r)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://tautulli:8181", Key: "0123abcd"}
	if err := c.Check(); err != nil || c.Name != "Plex" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Key: "k"}, "url is missing"},
		{Config{URL: "tautulli:8181", Key: "k"}, "url needs"},
		{Config{URL: "http://tautulli:8181"}, "key is missing"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
	if err := (Config{URL: "http://tautulli:8181", Key: "k"}).get(context.Background(), "delete_all_history", nil, nil); err == nil {
		t.Error("a command that isn't a read was let through")
	}
}
