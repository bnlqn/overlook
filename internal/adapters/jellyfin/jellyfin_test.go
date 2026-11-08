package jellyfin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/adapters/mediabrowser"
	"github.com/bnlqn/overlook/internal/snapshot"
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
	c := Config{Config: mediabrowser.Config{URL: src.URL, Key: "0123abcd"}}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Date(2026, 10, 29, 10, 45, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Area, p)
	for _, r := range src.Sent() {
		if r.Header.Get("Authorization") != `MediaBrowser Token="0123abcd"` {
			t.Errorf("%s: read without the key", r.URL)
		}
	}

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
	if r := rows[2]; r.Title.Name != "Sam" || r.Value.Key != "mediabrowser.episode" || r.Value.Args["series"] != "Harbour Lights" || r.Detail.Key != "mediaserver.paused" {
		t.Errorf("Sam's episode, paused: %+v %+v", r, r.Value)
	}
	if r := rows[3]; r.Title.Key != "mediabrowser.films" || r.Value.Args["n"] != 8 {
		t.Errorf("8 films: %+v", r.Value)
	}
	if r := rows[4]; r.Title.Key != "mediabrowser.shows" || r.Value.Args["n"] != 2 || r.Value.Args["episodes"] != 10 {
		t.Errorf("2 shows of 10 episodes, and no music or books: %+v", r.Value)
	}
	if r := rows[5]; r.Title.Name != "/media" || r.Meter == nil || r.Value.Args["n"] != int64(924_874_973_184) {
		t.Errorf("the films and shows on one disk, once: %+v", r)
	}
	if r := rows[6]; r.Title.Key != "mediabrowser.failed_logins" || r.Value.Args["n"] != 2 || r.Tone != "orange" ||
		r.Detail.Name != "Failed login attempt from Robin" {
		t.Errorf("Robin's two failed logins, as Jellyfin words them: %+v", r)
	}
	if r := rows[7]; r.Title.Name != "Scan Media Library" || r.Tone != "red" || !strings.Contains(r.Detail.Name, "denied") {
		t.Errorf("the failed scan, with why: %+v", r)
	}
	if r := rows[8]; r.Title.Key != "version" || r.Value.Name != "12.4.0" {
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

// An odd item in one of its answers costs a row, never the source.
func TestBend(t *testing.T) {
	t.Parallel()
	adaptertest.Bend(t, "testdata", func(url string) (snapshot.Parts, error) {
		c := Config{Config: mediabrowser.Config{URL: url, Key: "0123abcd"}}
		if err := c.Check(); err != nil {
			return snapshot.Parts{}, err
		}
		return Read(context.Background(), c, time.Date(2026, 10, 29, 10, 45, 0, 0, time.UTC))
	})
}
