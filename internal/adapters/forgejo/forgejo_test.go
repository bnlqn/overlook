package forgejo

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/adapters/gitea"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// recorded is a moment just after testdata was recorded.
var recorded = time.Date(2026, 11, 7, 23, 55, 0, 0, time.UTC)

// testdata holds answers recorded from a scratch Forgejo 17.0.1 on 7
// November 2026, filled as the gitea adapter's scratch Gitea is (compose.yaml,
// fill.sh and record.sh beside them). Forgejo gives its runners as a list,
// and its runs' fields their older names.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{gitea.Config{URL: src.URL, Repos: []string{"owner/homelab", "owner/website"}, Token: "0123abcd"}}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if c.Name != "Forgejo" {
		t.Errorf("named Forgejo: %q", c.Name)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Adapter, p)

	// Only these paths, each one of the gitea adapter's reads, which its
	// test checks.
	paths := []string{
		"/api/v1/repos/owner/homelab/pulls", "/api/v1/repos/owner/homelab/actions/runs",
		"/api/v1/repos/owner/website/pulls", "/api/v1/repos/owner/website/actions/runs",
		"/api/v1/admin/actions/runners",
	}
	for _, r := range src.Sent() {
		if r.Header.Get("Authorization") != "token 0123abcd" || !slices.Contains(paths, r.URL.Path) {
			t.Errorf("sent %s, without the token or not a read", r.URL)
		}
	}

	if len(p.Updates) != 3 {
		t.Errorf("renovate's three pull requests: %+v", p.Updates)
	}
	if len(p.Events) != 2 {
		t.Fatalf("two runs that failed this week: %+v", p.Events)
	}
	if e := p.Events[0]; e.Text.Args["workflow"] != "lint.yaml" || e.Text.Args["title"] != "Deploy the stacks" ||
		!e.At.Equal(time.Date(2026, 11, 7, 23, 51, 6, 0, time.UTC)) {
		t.Errorf("the newest failed run, from Forgejo's own fields: %+v", e)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 2 || rows[0].Value.Key != "gitea.online" || rows[0].Detail.Name != "v13.2.1" ||
		rows[1].Value.Key != "gitea.offline" || rows[1].Tone != "red" {
		t.Errorf("rack-runner idle, so online, with its version, and spare-runner offline: %+v", rows)
	}
}

// An odd item in one of its answers costs a row, never the source.
func TestBend(t *testing.T) {
	t.Parallel()
	adaptertest.Bend(t, "testdata", func(url string) (snapshot.Parts, error) {
		c := Config{gitea.Config{URL: url, Repos: []string{"owner/homelab", "owner/website"}, Token: "0123abcd"}}
		if err := c.Check(); err != nil {
			return snapshot.Parts{}, err
		}
		return Read(context.Background(), c, recorded)
	})
}
