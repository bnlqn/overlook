package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
)

// testdata holds answers recorded from GitHub on 28 October 2026 and
// scrubbed: example/homelab, with two pull requests from Dependabot, and
// example/website, with one from Renovate and one from a person.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Repos: []string{"example/website", "example/homelab"}}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

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
	c := Config{URL: src.URL, Repos: []string{"example/homelab"}}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if p, err := Read(ctx, c); err != nil || len(p.Updates) != 2 {
		t.Fatalf("read %+v, %v", p, err)
	}
	src.Close()
	if p, err := Read(ctx, c); err != nil || len(p.Updates) != 2 {
		t.Errorf("within 10 minutes, the last answer: %+v, %v", p, err)
	}
	c.last.at = c.last.at.Add(-every)
	if _, err := Read(ctx, c); err == nil || !strings.Contains(err.Error(), "example/homelab: ") {
		t.Errorf("after 10 minutes, GitHub is read again, and an error names its repository: %v", err)
	}
	c.URL = adaptertest.Replay(t, "testdata").URL
	if _, err := Read(ctx, c); err == nil {
		t.Error("within 10 minutes of a refusal, the refusal")
	}
}

func TestReadToken(t *testing.T) {
	t.Parallel()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer github_pat_example" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(s.Close)
	c := Config{URL: s.URL, Repos: []string{"example/private"}, Token: "github_pat_example"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), c); err != nil {
		t.Errorf("a private repository is read with the token: %v", err)
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
