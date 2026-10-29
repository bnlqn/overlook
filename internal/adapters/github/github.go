// Package github reads GitHub: the updates waiting, which are the open pull
// requests Dependabot or Renovate opened on the repositories the config
// names. On the page they are chores, never problems, and in no area.
package github

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Config is the repositories whose updates the page shows, under
// sources.github in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// Repos are the repositories read, each as owner/name.
	Repos []string `yaml:"repos"`
	// Token reads private repositories; public ones need none.
	Token string `yaml:"token"`
	// URL is the API Overlook reads, if not GitHub's own: a GitHub
	// Enterprise Server's, such as https://github.example.com/api/v3.
	URL string `yaml:"url"`
	// Bots are the authors whose pull requests are updates.
	Bots []string `yaml:"bots"`

	// last is the last read, which Read gives back for the ten minutes a
	// fetch.Cache keeps it. Without a token GitHub allows 60 requests an
	// hour, and each read is one request per repository, so ten
	// repositories hold. ponytail: one wait, with a token or without; give a
	// token a shorter one if 10 minutes ever feels slow.
	last *fetch.Cache[result]
}

// result is one read of every repository, kept even when it failed, so that
// a GitHub that refuses isn't asked again every 30 seconds.
type result struct {
	parts snapshot.Parts
	err   error
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if len(c.Repos) == 0 {
		return errors.New("repos is missing")
	}
	for _, r := range c.Repos {
		owner, name, _ := strings.Cut(r, "/")
		if owner == "" || name == "" || strings.Contains(name, "/") {
			return fmt.Errorf("repos: %q isn't owner/name", r)
		}
	}
	c.Name = cmp.Or(c.Name, "GitHub")
	c.URL = cmp.Or(c.URL, "https://api.github.com")
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if len(c.Bots) == 0 {
		c.Bots = []string{"dependabot[bot]", "renovate[bot]"}
	}
	c.last = &fetch.Cache[result]{}
	return nil
}

// pull is GitHub's answer for one pull request, from
// /repos/{owner}/{repo}/pulls.
type pull struct {
	Title   string    `json:"title"`
	HTMLURL string    `json:"html_url"`
	Created time.Time `json:"created_at"`
	User    struct {
		Login string `json:"login"`
	} `json:"user"`
}

// Read gives back the updates waiting in every repository of c, newest
// first, as its last read found them if that was under ten minutes ago,
// error and all.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	r, _ := c.last.Get(ctx, now, func(ctx context.Context) (result, error) {
		parts, err := read(ctx, c)
		return result{parts, err}, nil
	})
	return r.parts, r.err
}

func read(ctx context.Context, c Config) (snapshot.Parts, error) {
	var header http.Header
	if c.Token != "" {
		header = http.Header{"Authorization": {"Bearer " + c.Token}}
	}
	base := strings.TrimSuffix(c.URL, "/")
	pulls := make([][]pull, len(c.Repos))
	errs := make([]error, len(c.Repos))
	var wg sync.WaitGroup
	for i, r := range c.Repos {
		owner, name, _ := strings.Cut(r, "/")
		// ponytail: the first 100 open pull requests of each repository;
		// follow GitHub's Link header if one ever has more.
		u := fmt.Sprintf("%s/repos/%s/%s/pulls?state=open&per_page=100", base, url.PathEscape(owner), url.PathEscape(name))
		wg.Go(func() {
			if err := fetch.JSON(ctx, u, header, &pulls[i]); err != nil {
				errs[i] = fmt.Errorf("%s: %w", r, err)
			}
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return snapshot.Parts{}, err
	}
	var updates []snapshot.Update
	for _, p := range slices.Concat(pulls...) {
		if slices.Contains(c.Bots, p.User.Login) {
			updates = append(updates, snapshot.Update{Name: p.Title, URL: p.HTMLURL, At: p.Created})
		}
	}
	slices.SortFunc(updates, func(a, b snapshot.Update) int { return b.At.Compare(a.At) })
	return snapshot.Parts{Updates: updates}, nil
}
