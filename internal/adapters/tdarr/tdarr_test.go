package tdarr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

var recorded = time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC)

// testdata holds the nodes' answer as Tdarr 2.60 gives it, with invented
// nodes, files and numbers (a worker is shaped as Tdarr's own sample of one is, since
// the Tdarr it was recorded from had none at work): one node transcoding a film on its GPU, checking
// an episode on its processor and keeping a third worker idle, with nineteen
// files waiting, and a second node, paused, with nothing to do.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Key: "tdarr_0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Adapter, p)
	for _, r := range src.Sent() {
		if r.Header.Get("x-api-key") != "tdarr_0123abcd" {
			t.Errorf("%s: read without the key", r.URL)
		}
		if r.Method != "GET" {
			t.Errorf("%s: %s, not a GET", r.URL, r.Method)
		}
	}

	if len(p.Sections) != 1 || p.Sections[0].Title.Key != "tdarr.title" || len(p.Figures) != 1 {
		t.Fatalf("one section and one figure: %+v", p)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 3 {
		t.Fatalf("two files in work and the queue, the idle worker left out: %+v", rows)
	}
	if r := rows[1]; r.Title.Name != "Harbour.Lights.S01E03.mkv" || r.Value.Args["n"] != 8.0 || r.Detail.Key != "tdarr.checking_start" || r.Meter.Used != 8 {
		t.Errorf("the health check, still starting: %+v %+v", r, r.Detail)
	}
	if r := rows[0]; r.Title.Name != "Metropolis.1927.1080p.BluRay.x264.mkv" || r.Value.Args["n"] != 42.0 || r.Detail.Key != "tdarr.transcoding_detail" ||
		r.Detail.Args["node"] != "transcode-1" || r.Detail.Args["fps"] != 187 || r.Detail.Args["eta"] != "0:12:08" || r.Meter.Tone != "blue" {
		t.Errorf("the film first, by path, 42%% done at 187 fps: %+v %+v", r, r.Detail)
	}
	if f := p.Figures[0]; f.Value != 2 || f.Detail.Args["n"] != 19 || f.Meters[0].Used != 25 || f.Icon != "refresh" || f.Adds {
		t.Errorf("two files in work, 25%% done on average, 19 waiting: %+v", f)
	}
	if r := rows[2]; r.Title.Key != "tdarr.waiting" || r.Value.Args["n"] != 19 {
		t.Errorf("19 files waiting: %+v", r)
	}
}

// With every worker idle, Tdarr gives no part at all, so no card.
func TestIdle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	b, err := os.ReadFile(filepath.Join("testdata", "api", "v2", "get-nodes.json"))
	if err != nil {
		t.Fatal(err)
	}
	idle := strings.NewReplacer(`"idle": false`, `"idle": true`).Replace(string(b))
	if err := os.MkdirAll(filepath.Join(dir, "api", "v2"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api", "v2", "get-nodes.json"), []byte(idle), 0o600); err != nil {
		t.Fatal(err)
	}
	src := adaptertest.Replay(t, dir)
	c := Config{URL: src.URL}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Adapter, p)
	if len(p.Sections) != 0 || len(p.Figures) != 0 {
		t.Errorf("nothing in work, no card: %+v", p)
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	if err := (&Config{}).Check(); err == nil {
		t.Error("no url should be refused")
	}
	c := Config{URL: "http://tdarr:8265"}
	if err := c.Check(); err != nil || c.Name != "Tdarr" {
		t.Errorf("a url is enough, named Tdarr: %v %q", err, c.Name)
	}
}

func TestBend(t *testing.T) {
	t.Parallel()
	adaptertest.Bend(t, "testdata", func(url string) (snapshot.Parts, error) {
		c := Config{URL: url}
		if err := c.Check(); err != nil {
			return snapshot.Parts{}, err
		}
		return Read(context.Background(), c, recorded)
	})
}
