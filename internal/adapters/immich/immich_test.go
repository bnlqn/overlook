package immich

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// recorded is noon on the day testdata was recorded: Immich counts the week
// to it by UTC's days.
var recorded = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

// testdata holds answers recorded from an Immich 3.3.5 on 28 October 2026,
// with its people, ids and numbers invented: two people, a newer release out,
// and two queues at work.
func TestRead(t *testing.T) {
	t.Parallel()
	p := read(t, "testdata")

	if len(p.Figures) != 1 || len(p.Sections) != 1 {
		t.Fatalf("one figure and one section: %+v", p)
	}
	f := p.Figures[0]
	if f.Area != "apps" || f.Value != 25019+1205 || !said(f.Detail, "immich.this_week", "n", int64(7+27)) {
		t.Errorf("today's figure is everyone's photos and videos, +34 this week: %+v %+v", f, f.Detail)
	}
	if s := f.Shares; len(s) != 2 || s[1].Label.Name != "Sam" || len(s[1].Rows) != 4 || !said(s[1].Rows[3].Value, "immich.plus", "n", int64(27)) {
		t.Errorf("each person's share of the room, with their photos, videos, space and week: %+v", s)
	}

	rows := p.Sections[0].Rows
	want := []string{"Robin", "Sam", "immich.space", "Immich", "immich.jobs_of", "immich.jobs_of"}
	if got := titles(rows); !reflect.DeepEqual(got, want) {
		t.Fatalf("rows %v, want %v", got, want)
	}
	if r := rows[1]; !said(r.Value, "immich.this_week", "n", int64(27)) || !said(r.Detail, "immich.person", "photos", int64(18207)) {
		t.Errorf("Sam uploaded 27 this week and keeps 18,207 photos: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[2]; r.Meter == nil || r.Meter.Used != 7213418184704 || r.Meter.Total != 11994126090240 || !said(r.Detail, "immich.space_photos", "usage", int64(164360768300)) {
		t.Errorf("the disk is 60%% full, 164 GB of it photos: %+v", r)
	}
	if r := rows[3]; r.Value.Name != "v3.3.5" || r.Tone != "orange" || !said(r.Detail, "immich.newer", "version", "v3.4.0") {
		t.Errorf("v3.4.0 is out: %+v %+v", r, r.Detail)
	}
	if r := rows[4]; r.Title.Args["name"] != "thumbnailGeneration" || !said(r.Value, "immich.queued", "n", int64(2+41)) {
		t.Errorf("thumbnails: 2 running and 41 waiting: %+v", r)
	}
}

// With its statistics and nothing else, Immich still gives each person and
// today's figure, and leaves out what it couldn't read.
func TestReadStatisticsOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	stats, err := os.ReadFile("testdata/api/server/statistics.json")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "api/server/statistics.json"), stats)
	write(t, filepath.Join(dir, "api/queues.json"), []byte(`[{"name": "thumbnailGeneration", "statistics": {"active": 0, "waiting": 0}}]`))
	p := read(t, dir)

	if f := p.Figures[0]; !said(f.Detail, "immich.videos", "n", int64(1205)) {
		t.Errorf("without the week, the figure's line is the videos: %+v", f.Detail)
	}
	rows := p.Sections[0].Rows
	if got, want := titles(rows), []string{"Robin", "Sam", "immich.jobs"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rows %v, want %v", got, want)
	}
	if rows[0].Value != nil || rows[2].Value.Key != "immich.idle" {
		t.Errorf("no week for Robin, and the jobs are idle: %+v %+v", rows[0].Value, rows[2].Value)
	}
}

// Without its statistics, Immich didn't answer.
func TestReadNoStatistics(t *testing.T) {
	t.Parallel()
	if _, err := Read(context.Background(), source(t, t.TempDir()), recorded); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("want HTTP 404, got %v", err)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://immich:2283", Key: "k3y"}
	if err := c.Check(); err != nil || c.Name != "Immich" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Key: "k3y"}, "url is missing"},
		{Config{URL: "immich:2283", Key: "k3y"}, "url needs"},
		{Config{URL: "http://immich:2283"}, "key is missing"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}

// read replays the answers in dir and checks the page can draw what Immich
// gave.
func read(t *testing.T, dir string) snapshot.Parts {
	t.Helper()
	p, err := Read(context.Background(), source(t, dir), recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Area, p)
	return p
}

// source is an Immich that replays the answers in dir, and checks at the
// end of the test that each read carried the key.
func source(t *testing.T, dir string) Config {
	t.Helper()
	src := adaptertest.Replay(t, dir)
	t.Cleanup(func() {
		for _, r := range src.Sent() {
			if r.Header.Get("X-Api-Key") != "k3y" {
				t.Errorf("%s: read without the key", r.URL)
			}
		}
	})
	c := Config{URL: src.URL, Key: "k3y"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	return c
}

func said(x *snapshot.Text, key, arg string, v any) bool {
	return x != nil && x.Key == key && x.Args[arg] == v
}

func titles(rows []snapshot.Row) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Title.Key+r.Title.Name)
	}
	return out
}

func write(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}
