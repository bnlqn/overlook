package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
	"github.com/bnlqn/overlook/web"
)

// The invented snapshot is the contract between the server and the page (ADR
// 0003): it is the server's shapes, with no field they lack, and the page can
// draw every part of it, as it can every adapter's.
func TestSnapshot(t *testing.T) {
	t.Parallel()
	dec := json.NewDecoder(bytes.NewReader(invented))
	dec.DisallowUnknownFields()
	var s snapshot.Snapshot
	if err := dec.Decode(&s); err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, s.Parts)

	checks := map[string]bool{}
	for _, c := range s.Checks {
		checks[c.Name] = true
	}
	for _, l := range append(s.Apps, s.Tools...) {
		if l.Name == "" || !strings.HasPrefix(l.URL, "https://") || (l.Check != "" && !checks[l.Check]) {
			t.Errorf("%s: needs a name, an https:// url, and a check that is there: %+v", l.Name, l)
		}
	}
}

// The demo shows what every adapter adds to the page: each one's words, the
// check engine's checks and GitHub's updates, a problem in a degraded area, a
// source that didn't answer, and the apps and tools.
func TestSnapshotShowsEverything(t *testing.T) {
	t.Parallel()
	var s snapshot.Snapshot
	if err := json.Unmarshal(invented, &s); err != nil {
		t.Fatal(err)
	}
	if len(s.Checks) == 0 || len(s.Updates) == 0 || len(s.Apps) == 0 || len(s.Tools) == 0 {
		t.Error("no checks, updates, apps or tools")
	}
	degraded := false
	for _, c := range s.Checks {
		for _, other := range s.Checks {
			degraded = degraded || c.Problem && other.Area == c.Area && !other.Problem
		}
	}
	if !degraded {
		t.Error("no problem in an area that still works")
	}
	silent := false
	for _, src := range s.Sources {
		silent = silent || !src.Answered
	}
	if !silent {
		t.Error("no source that didn't answer")
	}

	names, err := fs.Glob(web.FS, "i18n/en/*.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		adapter := strings.TrimSuffix(path.Base(name), ".json")
		if adapter != "page" && !bytes.Contains(invented, []byte(`"key": "`+adapter+`.`)) {
			t.Errorf("nothing from %s", adapter)
		}
	}
}

func TestWrite(t *testing.T) {
	t.Parallel()
	out := t.TempDir()
	if err := write(out); err != nil {
		t.Fatal(err)
	}
	for _, page := range []string{"index.html", "servers.html", "network.html", "storage.html", "apps.html"} {
		html, err := os.ReadFile(filepath.Join(out, page))
		if err != nil || !bytes.Contains(html, []byte("<html data-demo ")) {
			t.Errorf("%s isn't marked as the demo's: %v", page, err)
		}
	}
	var words map[string]any
	raw, err := os.ReadFile(filepath.Join(out, "i18n/en.json"))
	if err != nil || json.Unmarshal(raw, &words) != nil || words["title"] != "Homelab" || words["pbs.backups"] != "Backups" {
		t.Errorf("the English table, the page's words and every adapter's: %v", err)
	}
	for _, file := range []string{"snapshot.json", "custom.css", "style.css", "js/shell.js", "icons/gatus.svg"} {
		if _, err := os.Stat(filepath.Join(out, file)); err != nil {
			t.Error(err)
		}
	}
}
