package adaptertest

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/snapshot"
	"github.com/bnlqn/overlook/web"
)

func TestProblems(t *testing.T) {
	t.Parallel()
	english := map[string]any{
		"pbs.backed_up": map[string]any{"one": "{n} guest backed up", "other": "{n} guests backed up"},
	}
	good := snapshot.Parts{Events: []snapshot.Event{{
		At: time.Now(), Area: "storage", Icon: "archive",
		Text: snapshot.Text{Key: "pbs.backed_up", Args: map[string]any{"n": 4}},
	}}}
	if got := Problems(good, english); len(got) != 0 {
		t.Errorf("a good event is refused: %q", got)
	}

	bad := snapshot.Parts{
		Figures: []snapshot.Figure{{
			Area: "attic", Label: snapshot.Text{Key: "nope.missing"}, Hours: []float64{1, 2}, Tone: "pink",
			Meters: []snapshot.Meter{{Label: &snapshot.Text{}, Total: 0, Tone: "teal"}},
		}},
		Events: []snapshot.Event{{Area: "storage", Text: snapshot.Text{Key: "pbs.backed_up"}}},
		Checks: []snapshot.Check{{Key: "k", Name: "n", URL: "u", Threshold: 3, Run: 0, Problem: true}},
	}
	if got := Problems(bad, english); len(got) != 10 {
		t.Errorf("want the area, the key, the hours, the tone, the meter's label, tone and total, the missing {n}, the missing time and the problem refused, got %d: %q", len(got), got)
	}

	adds := snapshot.Parts{Figures: []snapshot.Figure{{
		Area: "servers", Label: snapshot.Text{Key: "pbs.backed_up", Args: map[string]any{"n": 4}}, Adds: true,
		Detail: &snapshot.Text{Key: "pbs.backed_up", Args: map[string]any{"n": "four"}},
	}}}
	if got := Problems(adds, english); len(got) != 1 {
		t.Errorf("want a figure that adds up with a {n} that isn't a count refused, got %d: %q", len(got), got)
	}

	english["x.formats"] = "{span:span} {at:when} {size:bytes} {ms:ms} {area:area} {areas:areas} {names:list}"
	english["x.nope"] = "{n:nope}"
	formatted := func(args map[string]any) snapshot.Parts {
		return snapshot.Parts{Events: []snapshot.Event{{At: time.Now(), Area: "apps", Text: snapshot.Text{Key: "x.formats", Args: args}}}}
	}
	suits := formatted(map[string]any{
		"span": int64(60_000), "at": time.Now(), "size": 1.5e9, "ms": (*float64)(nil),
		"area": "apps", "areas": []string{"network", "apps"}, "names": []string{"Sam", "Kim"},
	})
	if got := Problems(suits, english); len(got) != 0 {
		t.Errorf("values that suit their formats are refused: %q", got)
	}
	unsuited := formatted(map[string]any{
		"span": "a minute", "at": "today", "size": nil, "ms": "fast",
		"area": "attic", "areas": []string{"apps", "attic"}, "names": "Sam",
	})
	unsuited.Events = append(unsuited.Events, snapshot.Event{At: time.Now(), Area: "apps", Text: snapshot.Text{Key: "x.nope", Args: map[string]any{"n": 1}}})
	if got := Problems(unsuited, english); len(got) != 8 {
		t.Errorf("want each of seven values that don't suit their formats, and a format the page doesn't know, refused, got %d: %q", len(got), got)
	}

	name := snapshot.Text{Name: "x"}
	charts := snapshot.Parts{
		Figures: []snapshot.Figure{{
			Area: "network", Label: name, Hours: make([]float64, 24),
			Within: &snapshot.Series{Label: name, Hours: make([]float64, 23)},
			Shares: []snapshot.Share{{Label: name, Value: 0, Tone: "pink"}},
			Part:   &snapshot.Share{Label: name, Value: -1},
		}},
		Charts: []snapshot.Chart{
			{Area: "network", Title: name, Series: []snapshot.Series{{Label: name, Hours: []float64{1}}}},
			{Area: "network", Title: name, Series: []snapshot.Series{{Label: name, Hours: []float64{1, 2}}, {Label: name, Hours: []float64{1}}}},
		},
	}
	if got := Problems(charts, english); len(got) != 7 {
		t.Errorf("want the figure's hours within, its share's tone, its shares of nothing, its part of no tile and below nothing, a chart of one hour and a part shorter than its whole refused, got %d: %q", len(got), got)
	}
}

func TestStrays(t *testing.T) {
	t.Parallel()
	name := snapshot.Text{Name: "x"}
	p := snapshot.Parts{
		Sections: []snapshot.Section{{Area: "apps", Title: name}, {Area: "network", Title: name}},
		Figures:  []snapshot.Figure{{Area: "apps", Label: name}},
		Checks:   []snapshot.Check{{Area: "network"}},
	}
	if got := strays("apps", p); len(got) != 1 || !strings.Contains(got[0], `in "network", but it goes in "apps"`) {
		t.Errorf("want the section in another area refused, and not the check: %q", got)
	}
	if got := strays("", p); len(got) != 3 {
		t.Errorf("want every part of an adapter with no home area refused, got %d: %q", len(got), got)
	}

	// Stores and backup runs go in Storage, whatever the home area.
	now := time.Now()
	storage := snapshot.Stores{{Name: name, Healthy: true}}.Parts()
	runs := snapshot.BackupRuns{{Start: now, Backups: 1}}.Parts(now)
	storage.Sections = append(storage.Sections, runs.Sections...)
	storage.Events = runs.Events
	if got := strays("servers", storage); len(got) != 0 {
		t.Errorf("want stores and backup runs let into Storage: %q", got)
	}
	storage.Sections[0].Area = "servers"
	if got := strays("servers", storage); len(got) != 1 || !strings.Contains(got[0], `store.section: in "servers", but it goes in "storage"`) {
		t.Errorf("want a store outside Storage refused, even in the home area: %q", got)
	}
}

// Every row and event the stores' and backup runs' modules can build is one
// the page can draw with the English table, whatever each source can say.
func TestStorageWords(t *testing.T) {
	t.Parallel()
	english, err := web.English()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 28, 10, 0, 0, 0, time.UTC)
	stores := snapshot.Stores{
		{Name: snapshot.Text{Name: "rpool"}, Used: 1, Size: 2, Health: "ONLINE", Healthy: true},
		{Name: snapshot.Text{Name: "tank"}, Used: 1, Size: 2, Health: "DEGRADED"},
		{Name: snapshot.Text{Name: "local"}, Used: 1, Size: 2, Healthy: true},
		{Name: snapshot.Text{Name: "nfs"}, Health: "unknown"},
		{Name: snapshot.Text{Name: "gone"}},
		{Name: snapshot.Text{Name: "far"}, Health: "ONLINE", Healthy: true},
	}
	p := stores.Parts()
	var runs snapshot.BackupRuns
	for i, r := range []snapshot.BackupRun{
		{Running: true},
		{},
		{Backups: 3},
		{Warnings: true},
		{Warnings: true, Why: "WARNINGS: 1"},
		{Failed: true},
		{Failed: true, Why: "job errors"},
	} {
		// Each how, with and without an end, on a host of its own.
		for _, end := range []time.Duration{0, time.Minute} {
			r.Host = fmt.Sprint(i, end)
			r.Start = now.Add(-time.Hour)
			if end > 0 && !r.Running {
				r.End = r.Start.Add(end)
			}
			runs = append(runs, r)
		}
	}
	q := runs.Parts(now)
	p.Sections = append(p.Sections, q.Sections...)
	p.Events = q.Events
	for _, problem := range append(Problems(p, english), strays("servers", p)...) {
		t.Error(problem)
	}
	if len(p.Events) != len(runs) || len(q.Sections[0].Rows) != len(runs) {
		t.Errorf("want an event and a row for each run, got %d and %d", len(p.Events), len(q.Sections[0].Rows))
	}
}
