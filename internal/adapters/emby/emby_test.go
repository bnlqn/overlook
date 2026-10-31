package emby

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// testdata holds answers recorded from a scratch Emby 4.10 (ADR 0008),
// which testdata/fill.sh fills: eight films and two shows, Alex playing a
// film as it is, Sam an episode, paused, Robin a film transcoded, and two
// failed logins from Robin. A new Emby wants a restart after its first
// start, and this one does too. Two things are invented beside them in the
// same shape: last night's library scan failed, and Alex played a film at
// 21:14 the evening before.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Key: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Date(2026, 10, 31, 7, 30, 30, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Area, p)
	for _, r := range src.Sent() {
		if r.Header.Get("X-Emby-Token") != "0123abcd" {
			t.Errorf("%s: read without the key", r.URL)
		}
	}

	rows := p.Sections[0].Rows
	if len(rows) != 8 {
		t.Fatalf("three playing, films, shows, failed logins, the failed scan and the version: %+v", rows)
	}
	if r := rows[0]; r.Title.Name != "Alex" || r.Value.Name != "Metropolis" || r.Detail.Key != "mediaserver.direct" || r.Detail.Args["device"] != "Living room TV" {
		t.Errorf("Alex playing a film as it is: %+v %+v", r, r.Detail)
	}
	if r := rows[1]; r.Title.Name != "Robin" || r.Detail.Key != "mediaserver.transcoding" {
		t.Errorf("Robin's film, transcoded: %+v %+v", r, r.Detail)
	}
	if r := rows[2]; r.Title.Name != "Sam" || r.Value.Key != "emby.episode" || r.Value.Args["series"] != "Harbour Lights" || r.Detail.Key != "mediaserver.paused" {
		t.Errorf("Sam's episode, paused: %+v %+v", r, r.Value)
	}
	if r := rows[3]; r.Title.Key != "emby.films" || r.Value.Args["n"] != 8 {
		t.Errorf("8 films: %+v", r.Value)
	}
	if r := rows[4]; r.Title.Key != "emby.shows" || r.Value.Args["n"] != 2 || r.Value.Args["episodes"] != 10 {
		t.Errorf("2 shows of 10 episodes, and no music or books: %+v", r.Value)
	}
	if r := rows[5]; r.Title.Key != "emby.failed_logins" || r.Value.Args["n"] != 2 || r.Tone != "orange" ||
		r.Detail.Name != "Failed Login Attempt from Robin on 3212bdb3a9e5" {
		t.Errorf("Robin's two failed logins, as Emby words them, naming the server: %+v", r)
	}
	if r := rows[6]; r.Title.Name != "Scan media library" || r.Tone != "red" || !strings.Contains(r.Detail.Name, "denied") {
		t.Errorf("the failed scan, with why: %+v", r)
	}
	if r := rows[7]; r.Title.Key != "emby.version" || r.Value.Key != "emby.restart" || r.Tone != "orange" || r.Detail.Name != "4.10.2.0" {
		t.Errorf("a restart pending, with the version: %+v", r)
	}

	f := p.Figures[0]
	if f.Value != 3 || f.Detail.Args["n"] != 3 {
		t.Errorf("3 plays since midnight, 3 playing now: %+v", f)
	}
	if f.Hours[23] != 3 || f.Hours[13] != 1 {
		t.Errorf("3 plays this hour and one at 21:00 the evening before: %v", f.Hours)
	}
}

func TestNoRestartPending(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata", `"HasPendingRestart": true`, `"HasPendingRestart": false`)
	c := Config{URL: src.URL, Key: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Date(2026, 10, 31, 7, 30, 30, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	rows := p.Sections[0].Rows
	if r := rows[len(rows)-1]; r.Value.Name != "4.10.2.0" || r.Tone != "" {
		t.Errorf("the version alone: %+v", r)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://emby:8096", Key: "0123abcd"}
	if err := c.Check(); err != nil || c.Name != "Emby" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Key: "0123abcd"}, "url is missing"},
		{Config{URL: "emby:8096", Key: "0123abcd"}, "url needs"},
		{Config{URL: "http://emby:8096"}, "key is missing"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
