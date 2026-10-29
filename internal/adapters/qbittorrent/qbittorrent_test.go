package qbittorrent

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

// recorded is soon after the torrents' last activity: qBittorrent's adapter
// uses it only to keep its totals for ten minutes.
var recorded = time.Date(2026, 10, 28, 9, 35, 0, 0, time.UTC)

// testdata holds the transfer's info as qBittorrent 5.2 answered it, its
// address replaced and its numbers nudged, and the torrents rebuilt with
// every field of a recorded one but invented names, hashes, paths and
// trackers: three downloading, three seeding, one stopped, one whose files
// are missing and one in error. It is firewalled. Its login's answer is
// written in the shape qBittorrent gives it. Before 5.2, Overlook logs in
// once with a form and sends the cookie it gets with every read.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, User: "overlook", Password: "s3cret"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Area, p)
	if _, err := Read(context.Background(), c, recorded.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	sent := src.Sent()
	if login := sent[0]; login.Method != http.MethodPost || login.URL.Path != "/api/v2/auth/login" || login.Body != "password=s3cret&username=overlook" {
		t.Errorf("first, the login, as a form: %+v", login)
	}
	for _, r := range sent[1:] {
		if r.Header.Get("Cookie") != "SID=a1b2c3" {
			t.Errorf("%s: read without the cookie", r.URL)
		}
	}

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

// From 5.2, Overlook sends its key with every read, and never logs in; a
// login qBittorrent refuses is an error that says so.
func TestKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS("testdata")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "api", "v2", "auth", "login.http")); err != nil {
		t.Fatal(err)
	}
	src := adaptertest.Replay(t, dir)
	c := Config{URL: src.URL, Key: "qbt_0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c, recorded); err != nil {
		t.Fatal(err)
	}
	for _, r := range src.Sent() {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer qbt_0123abcd" {
			t.Errorf("%s %s: a login, or a read without the key", r.Method, r.URL)
		}
	}

	refused := t.TempDir()
	login := filepath.Join(refused, "api", "v2", "auth", "login.http")
	if err := os.MkdirAll(filepath.Dir(login), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(login, []byte("HTTP/1.1 200 OK\r\ncontent-type: text/plain; charset=UTF-8\r\n\r\nFails."), 0o600); err != nil {
		t.Fatal(err)
	}
	c = Config{URL: adaptertest.Replay(t, refused).URL, User: "overlook", Password: "wrong"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c, recorded); err == nil || !strings.Contains(err.Error(), "login: refused") {
		t.Errorf("a login refused: %v", err)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://qbittorrent:8080"}
	if err := c.Check(); err != nil || c.Name != "qBittorrent" || c.source.Login != nil {
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
