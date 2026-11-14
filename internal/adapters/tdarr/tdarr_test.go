package tdarr

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

var recorded = time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC)

// testdata holds the nodes' answer as Tdarr 2.60 gave it, with invented nodes,
// files and numbers: one node transcoding a film on its GPU and checking an
// episode on its processor, with a third worker idle, and a second node,
// paused, with nothing to do, both with the same queues, of 12 files to check
// and 7 to transcode.
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
		if r.Header.Get("X-Api-Key") != "tdarr_0123abcd" {
			t.Errorf("%s: read without the key", r.URL)
		}
		if r.Method != http.MethodGet {
			t.Errorf("%s: %s, not a GET", r.URL, r.Method)
		}
	}

	if len(p.Sections) != 1 || p.Sections[0].Title.Key != "tdarr.title" || len(p.Figures) != 1 {
		t.Fatalf("one section and one figure: %+v", p)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 3 {
		t.Fatalf("two files in work and the queue, the idle worker and the paused node left out: %+v", rows)
	}
	if r := rows[0]; r.Title.Name != "Metropolis.1927.1080p.BluRay.x264.mkv" || r.Value.Args["n"] != 42.0 || r.Detail.Key != "tdarr.transcoding_full" ||
		r.Detail.Args["node"] != "transcode-1" || r.Detail.Args["fps"] != 187 || r.Detail.Args["eta"] != int64(728_000) || r.Meter.Tone != "blue" {
		t.Errorf("the film first, by path, 42%% done at 187 fps with 12 min 8 s left: %+v %+v", r, r.Detail)
	}
	if r := rows[1]; r.Title.Name != "Harbour.Lights.S01E03.mkv" || r.Value.Args["n"] != 8.0 || r.Detail.Key != "tdarr.checking_bare" || r.Meter.Used != 8 {
		t.Errorf("the health check, which gives no speed or time: %+v %+v", r, r.Detail)
	}
	if r := rows[2]; r.Title.Key != "tdarr.waiting" || r.Value.Args["n"] != 19 {
		t.Errorf("19 files waiting, each node's queue being the same one: %+v", r)
	}
	if f := p.Figures[0]; f.Value != 2 || f.Detail.Args["n"] != 19 || f.Icon != "refresh" || len(f.Meters) != 0 || f.Adds {
		t.Errorf("two files in work and 19 waiting: %+v", f)
	}
}

// bent reads the recorded answers with each old text replaced by its new one.
func bent(t *testing.T, replace ...string) snapshot.Parts {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "api", "v2", "get-nodes.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "api", "v2"), 0o750); err != nil {
		t.Fatal(err)
	}
	answer := strings.NewReplacer(replace...).Replace(string(b))
	if err := os.WriteFile(filepath.Join(dir, "api", "v2", "get-nodes.json"), []byte(answer), 0o600); err != nil {
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
	return p
}

// With every worker idle and files waiting, the section is the queue and the
// node that is paused; with nothing waiting either, Tdarr gives no part.
func TestIdle(t *testing.T) {
	t.Parallel()
	idle := []string{`"idle": false`, `"idle": true`}
	p := bent(t, idle...)
	if len(p.Sections) != 1 || len(p.Figures) != 0 || len(p.Sections[0].Rows) != 2 {
		t.Fatalf("files waiting and a node paused, so a section of them and no figure: %+v", p)
	}
	if r := p.Sections[0].Rows[1]; r.Title.Name != "transcode-2" || r.Value.Key != "tdarr.paused" || r.Tone != "orange" {
		t.Errorf("the paused node, in orange: %+v", r)
	}
	p = bent(t, append(idle, `"healthcheckcpu": 12`, `"healthcheckcpu": 0`, `"transcodegpu": 7`, `"transcodegpu": 0`)...)
	if len(p.Sections) != 0 || len(p.Figures) != 0 {
		t.Errorf("nothing in work or waiting, no section: %+v", p)
	}
}

// What Tdarr leaves out of a file's work, the page leaves out of its row.
func TestReported(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, old, new, key string }{
		{"a file not begun", `"percentage": 41.6`, `"percentage": 0`, "tdarr.transcoding_start"},
		{"no time left", `"ETA": "0:12:08"`, `"ETA": ""`, "tdarr.transcoding_bare"},
		{"a time not written as H:MM:SS", `"ETA": "0:12:08"`, `"ETA": "soon"`, "tdarr.transcoding_bare"},
		{"no speed", `"fps": 187`, `"fps": 0`, "tdarr.transcoding_eta"},
	} {
		if got := bent(t, c.old, c.new).Sections[0].Rows[0].Detail.Key; got != c.key {
			t.Errorf("%s: %s, not %s", c.name, got, c.key)
		}
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
