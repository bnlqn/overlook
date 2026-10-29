package nextcloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// testdata holds answers recorded from a scratch Nextcloud 35.0.2 (ADR 0008),
// which testdata/fill.sh fills: three invented people and the owner, their
// files and Nextcloud's own, one share by link and one between two of them,
// and every app up to date.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Token: "serverinfo-0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	rows := p.Sections[0].Rows
	if len(rows) != 5 {
		t.Fatalf("files, shares, free space, the database and the version: %+v", rows)
	}
	if r := rows[0]; r.Value.Args["n"] != 309 || r.Detail.Args["n"] != 4 || r.Detail.Args["active"] != 4 {
		t.Errorf("309 files of 4 people, all active today: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[1]; r.Value.Args["n"] != 2 || r.Detail.Args["n"] != 1 || r.Detail.Args["open"] != 1 {
		t.Errorf("2 shares, one by link with no password: %+v", r.Detail)
	}
	if r := rows[3]; r.Detail.Name != "sqlite3" {
		t.Errorf("the database: %+v", r)
	}
	if r := rows[4]; r.Value.Name != "35.0.2" || r.Tone != "" {
		t.Errorf("the version, up to date: %+v", r)
	}
	if len(p.Chores) != 0 {
		t.Errorf("no app waiting, as PHP's []: %+v", p.Chores)
	}
	if f := p.Figures[0]; f.Value != 309 || f.Detail.Args["n"] != 4 {
		t.Errorf("the files, and who used it today: %+v", f)
	}
}

// source answers as testdata does, with each of edits made to the recorded
// answers, as old and new.
func source(t *testing.T, edits ...string) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		answer, err := os.ReadFile(filepath.Join("testdata", r.URL.Path+".json"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		body := string(answer)
		for i := 0; i+1 < len(edits); i += 2 {
			body = strings.ReplaceAll(body, edits[i], edits[i+1])
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

// An app waiting for its update is a chore, done on Nextcloud's page of
// updates, and a newer Nextcloud is said beside its version.
func TestUpdates(t *testing.T) {
	t.Parallel()
	src := source(t,
		`"app_updates": []`, `"app_updates": {"deck": "1.17.2", "calendar": "6.2.1"}`,
		`"available": false`, `"available": true, "available_version": "35.0.3"`,
	)
	c := Config{URL: src.URL, Token: "serverinfo-0123abcd", Link: "https://cloud.example.com"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)
	if len(p.Chores) != 2 || p.Chores[0].Name != "calendar" || p.Chores[0].Note != "→ 6.2.1" ||
		p.Chores[0].URL != "https://cloud.example.com/settings/apps/updates" {
		t.Errorf("two apps waiting, by name: %+v", p.Chores)
	}
	if r := p.Sections[0].Rows[4]; r.Tone != "orange" || r.Value.Args["version"] != "35.0.3" {
		t.Errorf("35.0.3 is out: %+v", r)
	}
}

// In maintenance, Nextcloud answers only its status, and the page says so.
func TestMaintenance(t *testing.T) {
	t.Parallel()
	src := source(t, `"maintenance": false`, `"maintenance": true`)
	c := Config{URL: src.URL, Token: "serverinfo-0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if rows := p.Sections[0].Rows; len(rows) != 1 || rows[0].Tone != "red" || rows[0].Value.Key != "nextcloud.maintenance" {
		t.Errorf("in maintenance, in red: %+v", rows)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://nextcloud", Token: "serverinfo-0123abcd"}
	if err := c.Check(); err != nil || c.Name != "Nextcloud" || c.Link != c.URL {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Token: "t"}, "url is missing"},
		{Config{URL: "nextcloud", Token: "t"}, "url needs"},
		{Config{URL: "http://nextcloud"}, "token is missing"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
