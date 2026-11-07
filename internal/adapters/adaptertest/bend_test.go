package adaptertest

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// The odd answers made of recorded ones: in one item of each list, each
// field set to null and left out, the item null, and a time in another
// format, each made to the first item it changes, and none to an answer
// that isn't JSON.
func TestBends(t *testing.T) {
	t.Parallel()
	all := bends(map[string]string{
		"api/checks.json": `{"version": 3, "checks": [{"name": "dns", "at": null}, {"name": "photos", "at": "2026-03-16T16:24:44Z", "tags": ["a"]}]}`,
		"api/login.http":  "HTTP/1.1 200 OK\r\nSet-Cookie: SID=0123abcd\r\n\r\n{\"ok\": [1]}",
		"chores.ics":      "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n",
	})
	var got []string
	for _, b := range all {
		got = append(got, b.name+" "+b.how)
	}
	want := []string{
		"api/checks.json .checks[*] null",
		"api/checks.json .checks[*].at left out",
		"api/checks.json .checks[*].name null",
		"api/checks.json .checks[*].name left out",
		"api/checks.json .checks[*].at null",
		"api/checks.json .checks[*].at in another format",
		"api/checks.json .checks[*].tags null",
		"api/checks.json .checks[*].tags left out",
		"api/checks.json .checks[*].tags[*] null",
		"api/login.http .ok[*] null",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("bends:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Each is written from the answer as recorded, whatever was written
	// before it.
	for range 2 {
		if text := all[5].text(); text != `{"checks":[{"at":null,"name":"dns"},{"at":"2026-03-16 16:24:44","name":"photos","tags":["a"]}],"version":3}` {
			t.Errorf("a time in another format: %s", text)
		}
		if text := all[9].text(); text != "HTTP/1.1 200 OK\r\nSet-Cookie: SID=0123abcd\r\n\r\n{\"ok\":[null]}" {
			t.Errorf("a whole answer, its head kept: %q", text)
		}
	}
}

// checks is a small adapter's answers: two checks, and each one's state at
// its time, at a path of its own.
var checks = map[string]string{
	"api/checks.json": `{"checks": [{"name": "dns", "at": "2026-03-16T16:24:44Z"}, {"name": "photos", "at": "2026-03-16T16:25:44Z"}]}`,
	"api/checks/dns@at=2026-03-16T16%3A24%3A44Z.json":    `{"up": true}`,
	"api/checks/photos@at=2026-03-16T16%3A25%3A44Z.json": `{"up": false}`,
}

// Bend passes an adapter that an odd item costs only that item: one that
// reads through fetch, which leaves out a check it can't read, and that
// leaves out a check with no name, whose state it can't read. A check whose
// time is null asks for its state at a time no answer was recorded for, and
// gets the one recorded at that path.
func TestBend(t *testing.T) {
	t.Parallel()
	Bend(t, recorded(t, checks), func(address string) (snapshot.Parts, error) {
		src := fetch.Source{URL: address}
		if err := src.Check(); err != nil {
			return snapshot.Parts{}, err
		}
		var list struct {
			Checks []struct {
				Name string    `json:"name"`
				At   time.Time `json:"at"`
			} `json:"checks"`
		}
		if err := src.JSON(context.Background(), "/api/checks", &list); err != nil {
			return snapshot.Parts{}, err
		}
		var p snapshot.Parts
		for _, c := range list.Checks {
			if c.Name == "" {
				continue
			}
			var state struct{ Up bool }
			at := url.Values{"at": {c.At.Format(time.RFC3339)}}
			if err := src.JSON(context.Background(), "/api/checks/"+c.Name+"?"+at.Encode(), &state); err != nil {
				return snapshot.Parts{}, err
			}
			p.Chores = append(p.Chores, snapshot.Chore{Name: c.Name})
		}
		return p, nil
	})
}

// Bend fails the test of an adapter that one odd item takes off the page, as
// one entity's time took Home Assistant (#102): one that reads its answer
// with json.Unmarshal, which fails it for one time in another format, and
// that a check with no name panics.
func TestBendFails(t *testing.T) {
	if os.Getenv("OVERLOOK_BEND_FAILS") == "1" {
		Bend(t, recorded(t, checks), func(url string) (snapshot.Parts, error) {
			src := fetch.Source{URL: url}
			if err := src.Check(); err != nil {
				return snapshot.Parts{}, err
			}
			body, err := src.Text(context.Background(), "/api/checks")
			if err != nil {
				return snapshot.Parts{}, err
			}
			var list struct {
				Checks []struct {
					Name *string   `json:"name"`
					At   time.Time `json:"at"`
				} `json:"checks"`
			}
			if err := json.Unmarshal(body, &list); err != nil {
				return snapshot.Parts{}, err
			}
			var p snapshot.Parts
			for _, c := range list.Checks {
				p.Chores = append(p.Chores, snapshot.Chore{Name: *c.Name})
			}
			return p, nil
		})
		return
	}
	t.Parallel()
	// The test runs itself, so that the failures are its own and not this
	// test's.
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestBendFails$") //nolint:gosec // the test's own binary
	cmd.Env = append(os.Environ(), "OVERLOOK_BEND_FAILS=1")
	out, err := cmd.CombinedOutput()
	for _, want := range []string{
		"FAIL: TestBendFails/api/checks.json_.checks[*].at_in_another_format",
		`gives back nothing for its source: parsing time "2026-03-16 16:24:44"`,
		"FAIL: TestBendFails/api/checks.json_.checks[*].name_null",
		"gives back nothing for its source: panic: runtime error: invalid memory address",
	} {
		if err == nil || !bytes.Contains(out, []byte(want)) {
			t.Errorf("%v, and no %q in:\n%s", err, want, out)
		}
	}
}

// Every adapter whose answers are JSON runs Bend in its test, so that one
// added later is held to it from the start.
func TestEveryAdapterBends(t *testing.T) {
	t.Parallel()
	answers, err := filepath.Glob("../*/testdata")
	if err != nil || len(answers) == 0 {
		t.Fatalf("no adapter's answers: %v", err)
	}
	for _, dir := range answers {
		inJSON := false
		err := filepath.WalkDir(dir, func(name string, _ fs.DirEntry, err error) error {
			inJSON = inJSON || filepath.Ext(name) == ".json"
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		tests, err := filepath.Glob(filepath.Join(filepath.Dir(dir), "*_test.go"))
		if err != nil {
			t.Fatal(err)
		}
		bends := slices.ContainsFunc(tests, func(name string) bool {
			b, err := os.ReadFile(name) //nolint:gosec // the repository's own tests
			return err == nil && bytes.Contains(b, []byte("adaptertest.Bend("))
		})
		if inJSON && !bends {
			t.Errorf("%s: its answers are JSON, and its test doesn't run adaptertest.Bend", filepath.Base(filepath.Dir(dir)))
		}
	}
}
