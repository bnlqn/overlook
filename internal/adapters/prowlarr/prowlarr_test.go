package prowlarr

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/adapters/servarr"
)

// testdata holds answers recorded from Prowlarr 2.7 with its one indexer
// renamed and its key replaced, and three invented beside it in the same
// shape: Openshelf failing since the night before, as its status and health
// say, and Ferrovia disabled.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{Config: servarr.Config{URL: src.URL, Key: "0123abcd"}}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := read(context.Background(), c, time.Date(2026, 10, 28, 9, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	if len(p.Sections) != 1 || p.Sections[0].Title.Key != "prowlarr.indexers" {
		t.Fatalf("one section, the indexers: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 5 {
		t.Fatalf("four indexers and a warning: %+v", rows)
	}
	if r := rows[0]; r.Title.Name != "Ferrovia" || r.Value.Key != "prowlarr.disabled" || r.Tone != "gray" || r.Detail != nil {
		t.Errorf("Ferrovia, disabled, first by name: %+v", r)
	}
	if r := rows[1]; r.Title.Name != "Lantern" || r.Value.Key != "prowlarr.enabled" || r.Tone != "" ||
		r.Detail.Args["n"] != 1036 || r.Detail.Args["grabs"] != 3 || r.Detail.Args["ms"] != 91.0 {
		t.Errorf("Lantern: 24 searches and 1,012 RSS queries, 3 grabbed, in 91 ms: %+v %+v", r, r.Detail)
	}
	if r := rows[3]; r.Title.Name != "Openshelf" || r.Value.Key != "prowlarr.failing" || r.Tone != "red" ||
		r.Detail.Args["at"] != time.Date(2026, 10, 27, 23, 5, 13, 0, time.UTC) {
		t.Errorf("Openshelf, failing since 23:05: %+v %+v", r, r.Detail)
	}
	if r := rows[4]; r.Title.Name != "Prowlarr" || r.Tone != "orange" || !strings.Contains(r.Detail.Name, "Openshelf") {
		t.Errorf("Prowlarr's warning, as it words it: %+v", r)
	}
}
