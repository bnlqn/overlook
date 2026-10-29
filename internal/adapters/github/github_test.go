package github

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// recorded is a moment on the day testdata was recorded: GitHub's adapter
// uses it only to keep its answer for ten minutes.
var recorded = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

// testdata holds answers recorded from GitHub on 28 October 2026 and
// scrubbed: example/homelab, with two pull requests from Dependabot, and
// example/website, with one from Renovate and one from a person.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Repos: []string{"example/website", "example/homelab"}, Token: "github_pat_example"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, "", p)
	for _, r := range src.Sent() {
		if r.Header.Get("Authorization") != "Bearer github_pat_example" {
			t.Errorf("%s: read without the token", r.URL)
		}
	}

	var names []string
	for _, u := range p.Updates {
		names = append(names, u.Name)
	}
	want := []string{
		"Media: Bump linuxserver/prowlarr from 2.6.5 to 2.7.0 in /stacks/media in the linuxserver group",
		"Platform: Bump python from 3.13.17-alpine3.23 to 3.15.1-alpine3.23 in /stacks/platform",
		"chore(deps): update pnpm to v12",
	}
	if !slices.Equal(names, want) {
		t.Errorf("the bots' pull requests, newest first, and not the person's: %q", names)
	}
	u := p.Updates[0]
	if u.URL != "https://github.com/example/homelab/pull/23" || !u.At.Equal(time.Date(2026, 10, 28, 7, 59, 30, 0, time.UTC)) {
		t.Errorf("an update links to its pull request, from when it was opened: %+v", u)
	}
}

// GitHub is read at most every 10 minutes, and a refusal is kept as long as
// an answer.
func TestReadEvery(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Repos: []string{"example/website", "example/homelab"}}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{recorded, recorded.Add(9 * time.Minute)} {
		if p, err := Read(ctx, c, at); err != nil || len(p.Updates) != 3 {
			t.Fatalf("read %+v, %v", p, err)
		}
	}
	if n := len(src.Sent()); n != 2 {
		t.Errorf("within 10 minutes, the last answer: %d reads of two repositories", n)
	}
	if _, err := Read(ctx, c, recorded.Add(10*time.Minute)); err != nil || len(src.Sent()) != 4 {
		t.Errorf("after 10 minutes, GitHub is read again: %d reads, %v", len(src.Sent()), err)
	}

	// A GitHub that answers nothing.
	src = adaptertest.Replay(t, t.TempDir())
	c.URL = src.URL
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{recorded, recorded.Add(9 * time.Minute)} {
		if _, err := Read(ctx, c, at); err == nil || !strings.Contains(err.Error(), "example/homelab: HTTP 404") {
			t.Errorf("a refusal names its repository: %v", err)
		}
	}
	if n := len(src.Sent()); n != 2 {
		t.Errorf("within 10 minutes of a refusal, the refusal: %d reads of two repositories", n)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{Repos: []string{"example/homelab"}}
	if err := c.Check(); err != nil || c.Name != "GitHub" || c.URL != "https://api.github.com" ||
		!slices.Equal(c.Bots, []string{"dependabot[bot]", "renovate[bot]"}) {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{}, "repos is missing"},
		{Config{Repos: []string{"homelab"}}, `"homelab" isn't owner/name`},
		{Config{Repos: []string{"example/homelab/pulls"}}, "isn't owner/name"},
		{Config{Repos: []string{"/homelab"}}, "isn't owner/name"},
		{Config{Repos: []string{"example/homelab"}, URL: "api.github.com"}, "url needs"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
