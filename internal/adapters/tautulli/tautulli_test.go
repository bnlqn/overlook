package tautulli

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// testdata holds answers recorded from a scratch Tautulli 2.18.2 watching a
// scratch Plex 1.43.5 (ADR 0008), which testdata/fill.sh fills: five films and
// a show of three episodes, four plays finished, Tautulli logging the
// episode's twice, and two going, the TV's as it is and Kim's phone's
// transcoded. The Plex server is unclaimed, so every play is its local
// user's and Tautulli has no news of its releases. record.sh read them at
// 12:37:33, each command's answer at api/v2@cmd=<command>.json, since
// Tautulli answers every command at /api/v2, with the key in the address.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Key: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Date(2026, 10, 29, 12, 37, 33, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Area, p)

	// Each command it sent reads, and is one of reads, which name only
	// commands that read.
	for _, r := range src.Sent() {
		if q := r.URL.Query(); q.Get("apikey") != "0123abcd" || !slices.Contains(reads, q.Get("cmd")) {
			t.Errorf("sent %s, without the key or not in reads", q.Get("cmd"))
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
	if r := rows[0]; r.Title.Name != "Local" || r.Value.Name != "Sintel (2010)" || r.Detail.Key != "mediaserver.transcoding" || r.Detail.Args["device"] != "Kim's phone" {
		t.Errorf("Kim's phone first by player, transcoded: %+v %+v", r, r.Detail)
	}
	if r := rows[1]; r.Value.Name != "Metropolis (1927)" || r.Detail.Key != "mediaserver.direct" {
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
	if f.Value != 7 || f.Hours[23] != 7 || f.Detail.Args["n"] != 2 {
		t.Errorf("7 plays since midnight, the two going too: %+v", f)
	}
}

// A newer Plex is said, when Tautulli has news of it, and a Plex server
// Tautulli can't reach is said in red.
func TestPlex(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata", `"data": {}`, `"data": {"update_available": true, "version": "1.43.6.11200-0a1b2c3d4"}`)
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

	src = adaptertest.Replay(t, "testdata", `"connected": true`, `"connected": false`)
	c = Config{URL: src.URL, Key: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if p, err = Read(context.Background(), c, time.Date(2026, 10, 29, 12, 37, 33, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if r := p.Sections[0].Rows[2]; r.Value.Key != "tautulli.unreachable" || r.Tone != "red" {
		t.Errorf("Plex out of reach, after what plays: %+v", r)
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
