// Package gitea reads Gitea, and Forgejo, which was forked from it and still
// answers alike: the updates waiting, which are the open pull requests
// Renovate opened on the repositories the config names, as GitHub's are;
// the Actions runners, in Apps, in red when one is offline; and the runs
// that failed this week, as events. The forgejo adapter is this one under
// Forgejo's name.
package gitea

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "apps"

// Adapter says what this adapter is, for the config and its test.
var Adapter = snapshot.Adapter[Config]{Key: "gitea", Area: Area, Read: Read}

// reads are the only paths the adapter sends Gitea or Forgejo, each of
// which reads. Their APIs take a GET that changes something too: Forgejo
// answers one with a runner's registration token, making one if there's
// none, and anyone with it can register a runner of their own, which then
// runs the jobs, and sees their secrets. So no other path reaches
// internal/fetch, and the test checks every path sent against this list
// (ADR 0011).
var reads = []*regexp.Regexp{
	regexp.MustCompile(`^/api/v1/repos/[^/]+/[^/]+/pulls$`),
	regexp.MustCompile(`^/api/v1/repos/[^/]+/[^/]+/actions/runs$`),
	regexp.MustCompile(`^/api/v1/admin/actions/runners$`),
}

// Config is one Gitea or Forgejo and the repositories whose updates and
// runs the page shows, under sources.gitea or sources.forgejo in
// overlook.yaml.
type Config struct {
	Name string `yaml:"name" setup:"text,optional"`
	// URL is where Overlook reads it, such as http://gitea:3000.
	URL string `yaml:"url" setup:"text" example:"http://gitea.example.com:3000"`
	// Repos are the repositories read, each as owner/name.
	Repos []string `yaml:"repos" setup:"list" example:"example/homelab example/dotfiles"`
	// Token is an admin's access token, which reads the runners and private
	// repositories; public repositories need none, and show no runners.
	Token string `yaml:"token" setup:"key,optional"`
	// Bots are the users whose pull requests are updates.
	Bots []string `yaml:"bots"`

	source fetch.Source
}

// Check fills c's defaults, under Gitea's name, and refuses what can't
// work.
func (c *Config) Check() error { return c.CheckAs("Gitea") }

// CheckAs fills c's defaults, under name, Gitea or Forgejo, and refuses
// what can't work.
func (c *Config) CheckAs(name string) error {
	c.source = fetch.Source{URL: c.URL}
	if c.Token != "" {
		c.source.Header = http.Header{"Authorization": {"token " + c.Token}}
	}
	if err := c.source.Check(); err != nil {
		return err
	}
	if len(c.Repos) == 0 {
		return errors.New("repos is missing")
	}
	for _, r := range c.Repos {
		owner, repo, _ := strings.Cut(r, "/")
		if owner == "" || repo == "" || strings.Contains(repo, "/") || slices.Contains([]string{".", ".."}, owner) || slices.Contains([]string{".", ".."}, repo) {
			return fmt.Errorf("repos: %q isn't owner/name", r)
		}
	}
	c.Name = cmp.Or(c.Name, name)
	if len(c.Bots) == 0 {
		c.Bots = []string{"renovate"}
	}
	return nil
}

// get reads the answer at path, one of reads, with query, into v.
func (c Config) get(ctx context.Context, path string, query url.Values, v any) error {
	if !slices.ContainsFunc(reads, func(r *regexp.Regexp) bool { return r.MatchString(path) }) {
		return fmt.Errorf("%s isn't a path that reads", path)
	}
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	return c.source.JSON(ctx, path, v)
}

// pull is one of a repository's open pull requests.
type pull struct {
	Title   string    `json:"title"`
	HTMLURL string    `json:"html_url"`
	Created time.Time `json:"created_at"`
	User    struct {
		Login string `json:"login"`
	} `json:"user"`
}

// run is one of a repository's Actions runs. Gitea calls its fields
// display_title, path and created_at; Forgejo, forked before Gitea renamed
// them, title, workflow_id and created.
type run struct {
	Title    string    `json:"display_title"`
	Path     string    `json:"path"`
	Created  time.Time `json:"created_at"`
	FTitle   string    `json:"title"`
	Workflow string    `json:"workflow_id"`
	FCreated time.Time `json:"created"`
}

// runner is one Actions runner. Gitea says whether it's online and busy;
// Forgejo whether it's idle, active or offline, and its version.
type runner struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Busy     bool   `json:"busy"`
	Disabled bool   `json:"disabled"`
	Version  string `json:"version"`
}

// runners are /api/v1/admin/actions/runners: Gitea's in an object, Forgejo's
// a list.
type runners []runner

func (rs *runners) UnmarshalJSON(b []byte) error {
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("[")) {
		return json.Unmarshal(b, (*[]runner)(rs))
	}
	var page struct {
		Runners []runner `json:"runners"`
	}
	err := json.Unmarshal(b, &page)
	*rs = page.Runners
	return err
}

// Read gives back the updates waiting in every repository of c, newest
// first, the runs that failed there this week, and, with a token, the
// runners.
//
// ponytail: the first 50 open pull requests and failed runs of each
// repository, read one after another; follow the pages if one ever has
// more.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var p snapshot.Parts
	week := now.AddDate(0, 0, -7)
	for _, r := range c.Repos {
		owner, repo, _ := strings.Cut(r, "/")
		at := "/api/v1/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
		var pulls []pull
		if err := c.get(ctx, at+"/pulls", url.Values{"state": {"open"}, "limit": {"50"}}, &pulls); err != nil {
			return snapshot.Parts{}, fmt.Errorf("%s: %w", r, err)
		}
		for _, pr := range pulls {
			if slices.Contains(c.Bots, pr.User.Login) && pr.Title != "" && pr.HTMLURL != "" {
				p.Updates = append(p.Updates, snapshot.Update{Name: pr.Title, URL: pr.HTMLURL, At: pr.Created})
			}
		}
		var runs struct {
			Runs []run `json:"workflow_runs"`
		}
		if err := c.get(ctx, at+"/actions/runs", url.Values{"status": {"failure"}, "limit": {"50"}}, &runs); err != nil {
			return snapshot.Parts{}, fmt.Errorf("%s: %w", r, err)
		}
		for _, run := range runs.Runs {
			created := cmp.Or(run.Created, run.FCreated)
			if created.Before(week) {
				continue
			}
			workflow, _, _ := strings.Cut(cmp.Or(run.Workflow, run.Path), "@")
			p.Events = append(p.Events, snapshot.Event{
				At: created, Area: Area, Icon: "pr", Tone: "red",
				Text: snapshot.Text{Key: "gitea.run_failed", Args: map[string]any{
					"workflow": workflow, "repo": r, "title": cmp.Or(run.Title, run.FTitle),
				}},
			})
		}
	}
	slices.SortFunc(p.Updates, func(a, b snapshot.Update) int { return b.At.Compare(a.At) })
	slices.SortFunc(p.Events, func(a, b snapshot.Event) int { return b.At.Compare(a.At) })

	// The runners, only an admin's token reads.
	if c.Token != "" {
		var all runners
		if err := c.get(ctx, "/api/v1/admin/actions/runners", nil, &all); err != nil {
			return snapshot.Parts{}, fmt.Errorf("runners: %w", err)
		}
		p.Sections = []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "gitea.runners"}, Rows: rows(all)}}
	}
	return p, nil
}

// rows are a row for each runner, by name: offline in red, disabled,
// running a job, or online, with its version where it says.
func rows(all runners) []snapshot.Row {
	slices.SortFunc(all, func(a, b runner) int { return cmp.Compare(a.Name, b.Name) })
	rows := []snapshot.Row{}
	for _, r := range all {
		if r.Name == "" {
			continue
		}
		row := snapshot.Row{Title: snapshot.Text{Name: r.Name}}
		switch {
		case r.Status == "offline":
			row.Value, row.Tone = snapshot.Msg("gitea.offline", nil), "red"
		case r.Disabled:
			row.Value, row.Tone = snapshot.Msg("gitea.disabled", nil), "gray"
		case r.Busy || r.Status == "active":
			row.Value, row.Tone = snapshot.Msg("gitea.busy", nil), "blue"
		default:
			row.Value = snapshot.Msg("gitea.online", nil)
		}
		if r.Version != "" {
			row.Detail = &snapshot.Text{Name: r.Version}
		}
		rows = append(rows, row)
	}
	return rows
}
