package config

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// TestScratch holds the overlook.yaml beside each scratch instance's
// compose file (ADR 0008), with which the weekly recording reads what it
// recorded, to a config that loads and reads only the adapter of its
// folder, so that a config key renamed breaks it here rather than a week
// later.
func TestScratch(t *testing.T) {
	t.Setenv("URL", "http://localhost")
	composes, err := filepath.Glob("../adapters/*/testdata/compose.yaml")
	if err != nil || len(composes) == 0 {
		t.Fatal("no scratch instance's compose file", err)
	}
	for _, compose := range composes {
		testdata := filepath.Dir(compose)
		c, err := Load(filepath.Join(testdata, "overlook.yaml"))
		if err != nil {
			t.Error(err)
			continue
		}
		adapter := filepath.Base(filepath.Dir(testdata))
		if r := c.Readers(); len(r) == 0 || slices.ContainsFunc(r, func(s snapshot.Source) bool { return s.Adapter != adapter }) {
			t.Errorf("%s: reads %+v, not %s alone", testdata, r, adapter)
		}
	}
}

// TestRecorded reads what the weekly recording (.github/workflows/record.yml)
// has just recorded from the scratch instance of the adapter that
// $OVERLOOK_RECORDED names, through the overlook.yaml beside it, and checks
// what the adapter makes of it as the adapter's own test does. It reads at
// the moment record.sh printed, $OVERLOOK_RECORDED_AT, for answers that
// depend on it, or else now. Without $OVERLOOK_RECORDED it is skipped, since
// make check has no scratch instance.
func TestRecorded(t *testing.T) {
	adapter := os.Getenv("OVERLOOK_RECORDED")
	if adapter == "" {
		t.Skip("only the weekly recording records a scratch instance")
	}
	now := time.Now()
	if at := os.Getenv("OVERLOOK_RECORDED_AT"); at != "" {
		var err error
		if now, err = time.Parse(time.RFC3339, at); err != nil {
			t.Fatal(err)
		}
	}
	testdata := filepath.Join("..", "adapters", adapter, "testdata")
	t.Setenv("URL", adaptertest.Replay(t, testdata).URL)
	c, err := Load(filepath.Join(testdata, "overlook.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range c.Readers() {
		p, err := s.Read(context.Background(), now)
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		// Validate asks only what the adapter says it is, which the source
		// carries.
		adaptertest.Validate(t, snapshot.Adapter[struct{}]{Key: s.Adapter, Area: s.Area, Kind: s.Kind, Storage: s.Storage}, p)
	}
}
