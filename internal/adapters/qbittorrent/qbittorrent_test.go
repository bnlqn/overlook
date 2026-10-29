package qbittorrent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// recorded is soon after the torrents' last activity: qBittorrent's adapter
// uses it only to keep its totals for ten minutes.
var recorded = time.Date(2026, 10, 28, 9, 35, 0, 0, time.UTC)

// testdata holds the transfer's info as qBittorrent 5.2 answered it, its
// address replaced and its numbers nudged, and the torrents rebuilt with
// every field of a recorded one but invented names, hashes, paths and
// trackers: three downloading, three seeding, one stopped, one whose files
// are missing and one in error. It is firewalled.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	if len(p.Sections) != 1 || p.Sections[0].Title.Key != "qbittorrent.torrents" {
		t.Fatalf("one section, the torrents: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 6 {
		t.Fatalf("downloading, seeding, free space, the connection and two in error: %+v", rows)
	}
	if r := rows[0]; r.Value.Args["n"] != 3 || r.Detail.Args["n"] != int64(4_718_592) {
		t.Errorf("3 downloading, at 4.5 MiB/s: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[1]; r.Value.Args["n"] != 3 || r.Detail.Args["n"] != int64(1_310_720) {
		t.Errorf("3 seeding, the stopped one left out: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[2]; r.Value.Args["n"] != int64(1_879_048_192_000) {
		t.Errorf("the room left: %+v", r.Value)
	}
	if r := rows[3]; r.Value.Key != "qbittorrent.firewalled" || r.Tone != "orange" {
		t.Errorf("firewalled, in orange: %+v", r)
	}
	if r := rows[4]; r.Title.Name != "Big Buck Bunny (2008) 2160p" || r.Value.Key != "qbittorrent.missing_files" || r.Tone != "red" || r.Detail.Name != "/downloads/films" {
		t.Errorf("the missing files first by name, with where they're saved: %+v", r)
	}
	if r := rows[5]; r.Value.Key != "qbittorrent.error" || r.Tone != "red" {
		t.Errorf("the one in error: %+v", r)
	}
	if f := p.Figures[0]; f.Value != 3 || f.Detail.Args["n"] != 3 || f.Tone != "orange" || !f.Adds {
		t.Errorf("3 downloading and 3 seeding, adding up with another qBittorrent's: %+v", f)
	}
}

// source answers as qBittorrent does: GETs with testdata's answers when in
// lets them in, and a 403 otherwise, and a login with a cookie for overlook
// and s3cret.
func source(t *testing.T, in func(*http.Request) bool) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	logins := &atomic.Int32{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/auth/login":
			logins.Add(1)
			if r.FormValue("username") != "overlook" || r.FormValue("password") != "s3cret" {
				_, _ = w.Write([]byte("Fails."))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "a1b2c3", HttpOnly: true})
			_, _ = w.Write([]byte("Ok."))
		case r.Method == http.MethodGet && in(r):
			http.ServeFile(w, r, filepath.Join("testdata", r.URL.Path+".json"))
		default:
			http.Error(w, "Forbidden", http.StatusForbidden)
		}
	}))
	t.Cleanup(s.Close)
	return s, logins
}

// Before 5.2, Overlook logs in once with a form and sends the cookie it gets
// with every read.
func TestLogin(t *testing.T) {
	t.Parallel()
	src, logins := source(t, func(r *http.Request) bool {
		c, err := r.Cookie("SID")
		return err == nil && c.Value == "a1b2c3"
	})
	c := Config{URL: src.URL, User: "overlook", Password: "s3cret"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := Read(context.Background(), c, recorded); err != nil {
			t.Fatal(err)
		}
	}
	if n := logins.Load(); n != 1 {
		t.Errorf("two reads logged in %d times, want once", n)
	}

	c = Config{URL: src.URL, User: "overlook", Password: "wrong"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c, recorded); err == nil || !strings.Contains(err.Error(), "login: refused") {
		t.Errorf("a login refused: %v", err)
	}
}

// From 5.2, Overlook sends its key with every read, and never logs in.
func TestKey(t *testing.T) {
	t.Parallel()
	src, logins := source(t, func(r *http.Request) bool { return r.Header.Get("Authorization") == "Bearer qbt_0123abcd" })
	c := Config{URL: src.URL, Key: "qbt_0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c, recorded); err != nil {
		t.Fatal(err)
	}
	if n := logins.Load(); n != 0 {
		t.Errorf("a key logged in %d times", n)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://qbittorrent:8080"}
	if err := c.Check(); err != nil || c.Name != "qBittorrent" || c.session != nil {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{}, "url is missing"},
		{Config{URL: "qbittorrent:8080"}, "url needs"},
		{Config{URL: "http://qbittorrent:8080", Key: "qbt_0123abcd", User: "overlook"}, "key or user"},
		{Config{URL: "http://qbittorrent:8080", Password: "s3cret"}, "password without a user"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
