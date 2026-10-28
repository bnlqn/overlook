package sonarr

import (
	"context"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/adapters/servarr"
)

// testdata holds Sonarr's answers with every title, path, number and date
// invented, read at 10:00 UTC on 28 October 2026: three shows, 34 episodes
// on disk, two imported this week (one of them twice, as a better copy came),
// and three on the calendar.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{Config: servarr.Config{URL: src.URL, Key: "4567efab"}}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := read(context.Background(), c, time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	if len(p.Figures) != 1 || p.Figures[0].Value != 3 || p.Figures[0].Detail.Args["n"] != 2 {
		t.Errorf("3 shows, 2 episodes this week: %+v", p.Figures)
	}
	if len(p.Sections) != 2 {
		t.Fatalf("the shows and what's coming: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 3 {
		t.Fatalf("the library, the queue and one error: %+v", rows)
	}
	if lib := rows[0]; lib.Value.Args["n"] != 3 || lib.Detail.Args["n"] != 34 || lib.Detail.Args["size"] != int64(31456789012+12345678901) || lib.Detail.Args["week"] != 2 {
		t.Errorf("the library: %+v, %+v", lib.Value, lib.Detail)
	}
	if q := rows[1]; q.Value.Key != "sonarr.nothing" {
		t.Errorf("nothing downloading: %+v", q.Value)
	}
	if h := rows[2]; h.Title.Name != "Sonarr" || h.Tone != "red" || h.Value.Key != "sonarr.error" || h.Detail.Name != "Missing root folder: /data/media/shows" {
		t.Errorf("an error, as Sonarr words it: %+v", h)
	}

	// The second has no title yet.
	want := []struct{ title, value, what string }{
		{"Harbour Street 1×10", "sonarr.have", "The Tide Turns"},
		{"Night Ferry 1×01", "sonarr.on", ""},
		{"The Lighthouse Keepers 3×02", "sonarr.on", "Fog Signal"},
	}
	rows = p.Sections[1].Rows
	if len(rows) != len(want) {
		t.Fatalf("three episodes coming: %+v", rows)
	}
	for i, w := range want {
		if r := rows[i]; r.Title.Name != w.title || r.Value.Key != w.value || r.Detail.Name != w.what {
			t.Errorf("coming[%d]: want %v, got %+v %+v %+v", i, w, r.Title, r.Value, r.Detail)
		}
	}
	if rows[1].Detail.Key != "sonarr.untitled" {
		t.Errorf("an episode with no title yet: %+v", rows[1].Detail)
	}
}

func TestNothingComing(t *testing.T) {
	t.Parallel()
	if rows := coming(nil); len(rows) != 1 || rows[0].Title.Key != "sonarr.coming_none" {
		t.Errorf("an empty week says so: %+v", rows)
	}
}
