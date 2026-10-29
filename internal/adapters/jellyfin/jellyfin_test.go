package jellyfin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// testdata holds answers recorded from a scratch Jellyfin 12.4 (ADR 0008),
// which testdata/fill.sh fills: eight films and two shows, Alex playing a
// film as it is, Sam an episode, paused, Robin a film transcoded, and two
// failed logins from Robin. Two things are invented beside them in the same
// shape: last night's library scan failed, and Alex played a film at 21:14
// the evening before.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Key: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := read(context.Background(), c, time.Date(2026, 10, 29, 10, 45, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	rows := p.Sections[0].Rows
	if len(rows) != 9 {
		t.Fatalf("three playing, films, shows, a disk, failed logins, the failed scan and the version: %+v", rows)
	}
	if r := rows[0]; r.Title.Name != "Alex" || r.Value.Name != "Metropolis (1927)" || r.Detail.Key != "mediaserver.direct" || r.Detail.Args["device"] != "Living room TV" {
		t.Errorf("Alex playing a film as it is: %+v %+v", r, r.Detail)
	}
	if r := rows[1]; r.Title.Name != "Robin" || r.Detail.Key != "mediaserver.transcoding" {
		t.Errorf("Robin's film, transcoded: %+v %+v", r, r.Detail)
	}
	if r := rows[2]; r.Title.Name != "Sam" || r.Value.Key != "jellyfin.episode" || r.Value.Args["series"] != "Harbour Lights" || r.Detail.Key != "mediaserver.paused" {
		t.Errorf("Sam's episode, paused: %+v %+v", r, r.Value)
	}
	if r := rows[3]; r.Title.Key != "jellyfin.films" || r.Value.Args["n"] != 8 {
		t.Errorf("8 films: %+v", r.Value)
	}
	if r := rows[4]; r.Title.Key != "jellyfin.shows" || r.Value.Args["n"] != 2 || r.Value.Args["episodes"] != 10 {
		t.Errorf("2 shows of 10 episodes, and no music or books: %+v", r.Value)
	}
	if r := rows[5]; r.Title.Name != "/media" || r.Meter == nil || r.Value.Args["n"] != int64(924_874_973_184) {
		t.Errorf("the films and shows on one disk, once: %+v", r)
	}
	if r := rows[6]; r.Title.Key != "jellyfin.failed_logins" || r.Value.Args["n"] != 2 || r.Tone != "orange" ||
		r.Detail.Name != "Failed login attempt from Robin" {
		t.Errorf("Robin's two failed logins, as Jellyfin words them: %+v", r)
	}
	if r := rows[7]; r.Title.Name != "Scan Media Library" || r.Tone != "red" || !strings.Contains(r.Detail.Name, "denied") {
		t.Errorf("the failed scan, with why: %+v", r)
	}
	if r := rows[8]; r.Title.Key != "jellyfin.version" || r.Value.Name != "12.4.0" {
		t.Errorf("the version: %+v", r)
	}

	f := p.Figures[0]
	if f.Value != 3 || f.Detail.Args["n"] != 3 {
		t.Errorf("3 plays since midnight, 3 playing now: %+v", f)
	}
	if f.Hours[23] != 3 || f.Hours[10] != 1 {
		t.Errorf("3 plays this hour and one at 21:00 yesterday: %v", f.Hours)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://jellyfin:8096", Key: "0123abcd"}
	if err := c.Check(); err != nil || c.Name != "Jellyfin" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Key: "0123abcd"}, "url is missing"},
		{Config{URL: "jellyfin:8096", Key: "0123abcd"}, "url needs"},
		{Config{URL: "http://jellyfin:8096"}, "key is missing"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
