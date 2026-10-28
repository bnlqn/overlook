package immich

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

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
	if s := f.Shares; len(s) != 2 || s[1].Label.Name != "Sam" || s[0].Value+s[1].Value != f.Value {
		t.Errorf("each person's share of them: %+v", s)
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

// Every read carries the key, and without its statistics Immich didn't
// answer.
func TestReadNoStatistics(t *testing.T) {
	t.Parallel()
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "k3y" {
			t.Errorf("%s: read without the key", r.URL.Path)
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(src.Close)
	if _, err := Read(context.Background(), Config{URL: src.URL, Key: "k3y"}); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
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
	c := Config{URL: adaptertest.Replay(t, dir).URL, Key: "k3y"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)
	return p
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
