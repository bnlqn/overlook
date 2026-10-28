package radarr

import (
	"context"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/adapters/servarr"
)

// testdata holds Radarr's answers with every title, path, number and date
// invented, read at 10:00 UTC on 28 October 2026: five films, four of them
// on disk, two added this week (one at its very start, one a second before
// it), and four on the calendar.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{Config: servarr.Config{URL: src.URL, Key: "0123abcd"}}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := read(context.Background(), c, time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	if len(p.Figures) != 1 || p.Figures[0].Value != 4 || p.Figures[0].Detail.Args["n"] != 2 {
		t.Errorf("4 films, 2 this week: %+v", p.Figures)
	}
	if f := p.Figures[0]; f.Join == nil || f.Join.Key != "servarr.library" || f.Part == nil ||
		f.Part.Value != float64(4812345678+3204567890+9876543210+2109876543) || f.Part.Tone != "orange" {
		t.Errorf("the films join the library's tile, their part the room they take: %+v %+v", f.Join, f.Part)
	}
	if len(p.Sections) != 2 {
		t.Fatalf("the films and what's coming: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 3 {
		t.Fatalf("the library, the queue and one warning: %+v", rows)
	}
	if lib := rows[0]; lib.Value.Args["n"] != 4 || lib.Detail.Args["size"] != int64(4812345678+3204567890+9876543210+2109876543) || lib.Detail.Args["n"] != 2 {
		t.Errorf("the library: %+v, %+v", lib.Value, lib.Detail)
	}
	if q := rows[1]; q.Value.Key != "radarr.downloading_n" || q.Value.Args["n"] != 1 {
		t.Errorf("one film downloading: %+v", q.Value)
	}
	if h := rows[2]; h.Title.Name != "Radarr" || h.Tone != "orange" || h.Value.Key != "radarr.warning" || h.Detail.Name != "Indexers unavailable due to failures: Example Indexer" {
		t.Errorf("a warning, as Radarr words it: %+v", h)
	}

	// Caligari is only in cinemas this week, and Häxan's disc comes after
	// its stream.
	want := []struct{ title, value, what string }{
		{"The Phantom Carriage (1921)", "radarr.on", "radarr.to_stream"},
		{"Häxan (1922)", "radarr.on", "radarr.to_stream"},
		{"Safety Last! (1923)", "radarr.have", "radarr.on_disc"},
	}
	coming := p.Sections[1].Rows
	if len(coming) != len(want) {
		t.Fatalf("three films coming, soonest first: %+v", coming)
	}
	for i, w := range want {
		if r := coming[i]; r.Title.Name != w.title || r.Value.Key != w.value || r.Detail.Key != w.what {
			t.Errorf("coming[%d]: want %v, got %+v %+v %+v", i, w, r.Title, r.Value, r.Detail)
		}
	}
	if at := coming[0].Value.Args["at"]; at != time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC) {
		t.Errorf("The Phantom Carriage streams on 30 October: %v", at)
	}
}

func TestNothingComing(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	if rows := coming(nil, start, start.AddDate(0, 0, 8)); len(rows) != 1 || rows[0].Title.Key != "radarr.coming_none" {
		t.Errorf("an empty week says so: %+v", rows)
	}
}
