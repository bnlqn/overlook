package gitea

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// recorded is a moment just after testdata was recorded.
var recorded = time.Date(2026, 11, 7, 23, 45, 0, 0, time.UTC)

// testdata holds answers recorded from a scratch Gitea 28.3.0 on 7 November
// 2026 (compose.yaml, fill.sh and record.sh beside them): owner/homelab and
// owner/website, with three pull requests from renovate and one from kim, a
// person; two runs of homelab's lint workflow that failed; and two runners,
// spare-runner offline.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Repos: []string{"owner/homelab", "owner/website"}, Token: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Area, p)
	sent(t, src.Sent(), "token 0123abcd")

	var names []string
	for _, u := range p.Updates {
		names = append(names, u.Name)
	}
	if !slices.Contains(names, "fix(deps): update dependency astro to v7.2.1") || len(names) != 3 || slices.Contains(names, "Add the garden page") {
		t.Errorf("renovate's three pull requests, and not kim's: %q", names)
	}
	if u := p.Updates[0]; !strings.HasPrefix(u.URL, "http://localhost:3010/owner/") || u.At.IsZero() {
		t.Errorf("an update links to its pull request, from when it was opened: %+v", u)
	}

	if len(p.Events) != 2 {
		t.Fatalf("two runs that failed this week: %+v", p.Events)
	}
	e := p.Events[0]
	if e.Tone != "red" || e.Area != Area || e.Text.Args["workflow"] != "lint.yaml" || e.Text.Args["repo"] != "owner/homelab" ||
		e.Text.Args["title"] != "Deploy the stacks" || !e.At.Equal(time.Date(2026, 11, 7, 23, 40, 36, 0, time.UTC)) {
		t.Errorf("the newest failed run, in red, by its workflow, its repository and its commit: %+v", e)
	}

	rows := p.Sections[0].Rows
	if len(rows) != 2 || rows[0].Title.Name != "rack-runner" || rows[0].Value.Key != "gitea.online" ||
		rows[1].Title.Name != "spare-runner" || rows[1].Value.Key != "gitea.offline" || rows[1].Tone != "red" {
		t.Errorf("rack-runner online, spare-runner offline in red: %+v", rows)
	}

	// A week later, the runs are older than this week.
	p, err = Read(context.Background(), c, recorded.AddDate(0, 0, 8))
	if err != nil || len(p.Events) != 0 {
		t.Errorf("no run failed this week: %+v, %v", p.Events, err)
	}

	// Without a token, the runners aren't read.
	c.Token = ""
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	before := len(src.Sent())
	if p, err = Read(context.Background(), c, recorded); err != nil || len(p.Sections) != 0 || len(p.Updates) != 3 {
		t.Errorf("without a token, the updates and no runners: %+v, %v", p, err)
	}
	for _, r := range src.Sent()[before:] {
		if strings.Contains(r.URL.Path, "/admin/") || r.Header.Get("Authorization") != "" {
			t.Errorf("read %s, or with a token, without one", r.URL)
		}
	}
}

// sent fails t for each request in all not sent with authorization, or to a
// path not in reads, and for a path that changes something that reads would
// let through.
func sent(t *testing.T, all []adaptertest.Request, authorization string) {
	t.Helper()
	for _, r := range all {
		if r.Header.Get("Authorization") != authorization {
			t.Errorf("%s: read without the token", r.URL)
		}
		if !slices.ContainsFunc(reads, func(re *regexp.Regexp) bool { return re.MatchString(r.URL.Path) }) {
			t.Errorf("%s: not one of reads", r.URL)
		}
	}
	for _, path := range []string{
		"/api/v1/admin/runners/registration-token",
		"/api/v1/admin/actions/runners/registration-token",
		"/api/v1/repos/owner/homelab/actions/runners/registration-token",
		"/api/v1/user/actions/runners/registration-token",
		"/api/v1/repos/owner/homelab/actions/runs/1/rerun",
	} {
		if slices.ContainsFunc(reads, func(re *regexp.Regexp) bool { return re.MatchString(path) }) {
			t.Errorf("reads lets %s through", path)
		}
	}
}

// A path that isn't one of reads never reaches Gitea.
func TestGetRefuses(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, t.TempDir())
	c := Config{URL: src.URL, Repos: []string{"owner/homelab"}, Token: "0123abcd"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	var v any
	if err := c.get(context.Background(), "/api/v1/admin/runners/registration-token", nil, &v); err == nil || !strings.Contains(err.Error(), "isn't a path that reads") {
		t.Errorf("the registration token refused: %v", err)
	}
	if len(src.Sent()) != 0 {
		t.Errorf("sent %+v", src.Sent())
	}
}

// A runner running a job, Gitea's busy and Forgejo's active, and one
// disabled.
func TestRows(t *testing.T) {
	t.Parallel()
	rows := rows(runners{{Name: "b", Status: "online", Busy: true}, {Name: "a", Status: "active"}, {Name: "c", Status: "online", Disabled: true}})
	p := snapshot.Parts{Sections: []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "gitea.runners"}, Rows: rows}}}
	adaptertest.Validate(t, Area, p)
	if rows[0].Value.Key != "gitea.busy" || rows[1].Value.Key != "gitea.busy" || rows[2].Value.Key != "gitea.disabled" {
		t.Errorf("two running a job, one disabled: %+v", rows)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://gitea:3000", Repos: []string{"owner/homelab"}}
	if err := c.Check(); err != nil || c.Name != "Gitea" || !slices.Equal(c.Bots, []string{"renovate"}) {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Repos: []string{"owner/homelab"}}, "url is missing"},
		{Config{URL: "http://gitea:3000"}, "repos is missing"},
		{Config{URL: "http://gitea:3000", Repos: []string{"homelab"}}, `"homelab" isn't owner/name`},
		{Config{URL: "http://gitea:3000", Repos: []string{"owner/homelab/pulls"}}, "isn't owner/name"},
		{Config{URL: "http://gitea:3000", Repos: []string{"../admin"}}, "isn't owner/name"},
		{Config{URL: "http://gitea:3000", Repos: []string{"owner/.."}}, "isn't owner/name"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}

// An odd item in one of its answers costs a row, never the source.
func TestBend(t *testing.T) {
	t.Parallel()
	adaptertest.Bend(t, "testdata", func(url string) (snapshot.Parts, error) {
		c := Config{URL: url, Repos: []string{"owner/homelab", "owner/website"}, Token: "0123abcd"}
		if err := c.Check(); err != nil {
			return snapshot.Parts{}, err
		}
		return Read(context.Background(), c, recorded)
	})
}
