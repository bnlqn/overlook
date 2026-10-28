// Package gatus reads Gatus, the check engine: its checks, each in the area
// its group names, their recent results, and the problems its events
// remember.
package gatus

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// recent is the results each check's strip shows: half an hour at Gatus's
// default minute.
const recent = 30

// Config is one Gatus, under sources.gatus in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Gatus. A login goes in it as user:pass@.
	URL string `yaml:"url"`
	// Link is where the page sends the owner, if not URL.
	Link string `yaml:"link"`
	// Threshold is how many failures in a row make a problem: the same
	// number as Gatus's alerts.
	Threshold int `yaml:"threshold"`
	// Areas maps each group not named like an area to the area it goes in.
	Areas map[string]string `yaml:"areas"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if c.URL == "" {
		return errors.New("url is missing")
	}
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if c.Name == "" {
		c.Name = "Gatus"
	}
	if c.Link == "" {
		u, _ := url.Parse(c.URL)
		u.User = nil
		c.Link = u.String()
	}
	if err := fetch.CheckURL(c.Link); err != nil {
		return fmt.Errorf("link %w", err)
	}
	if c.Threshold == 0 {
		c.Threshold = 3
	}
	if c.Threshold < 1 {
		return errors.New("threshold must be 1 or more")
	}
	for group, area := range c.Areas {
		if !slices.Contains(snapshot.Areas, area) {
			return fmt.Errorf("areas: %s goes to %q, not one of %s", group, area, strings.Join(snapshot.Areas, ", "))
		}
	}
	return nil
}

// endpoint is Gatus's answer for one check, from /api/v1/endpoints/statuses
// and /api/v1/endpoints/{key}/statuses.
type endpoint struct {
	Name    string   `json:"name"`
	Group   string   `json:"group"`
	Key     string   `json:"key"`
	Results []result `json:"results"`
	Events  []struct {
		Type      string    `json:"type"`
		Timestamp time.Time `json:"timestamp"`
	} `json:"events"`
}

type result struct {
	Duration         int64     `json:"duration"` // nanoseconds
	Success          bool      `json:"success"`
	Timestamp        time.Time `json:"timestamp"`
	Errors           []string  `json:"errors"`
	ConditionResults []struct {
		Condition string `json:"condition"`
		Success   bool   `json:"success"`
	} `json:"conditionResults"`
}

// Read reads every check of the Gatus at c.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	base := strings.TrimSuffix(c.URL, "/")
	var endpoints []endpoint
	if err := fetch.JSON(ctx, fmt.Sprintf("%s/api/v1/endpoints/statuses?page=1&pageSize=%d", base, recent), &endpoints); err != nil {
		return snapshot.Parts{}, err
	}
	// Each check's events, which only its own read has. A check whose read
	// fails goes without them.
	var wg sync.WaitGroup
	for i := range endpoints {
		wg.Go(func() {
			var one endpoint
			if fetch.JSON(ctx, base+"/api/v1/endpoints/"+url.PathEscape(endpoints[i].Key)+"/statuses?page=1&pageSize=1", &one) == nil {
				endpoints[i].Events = one.Events
			}
		})
	}
	wg.Wait()
	checks := make([]snapshot.Check, len(endpoints))
	for i, e := range endpoints {
		checks[i] = check(e, c)
	}
	return snapshot.Parts{Checks: checks}, nil
}

func check(e endpoint, c Config) snapshot.Check {
	ch := snapshot.Check{
		Key: e.Key, Name: e.Name, Area: area(e.Group, c.Areas), Source: c.Name, Threshold: c.Threshold,
		URL: strings.TrimSuffix(c.Link, "/") + "/endpoints/" + url.PathEscape(e.Key), Results: []snapshot.Result{},
	}
	for _, r := range e.Results {
		res := snapshot.Result{OK: r.Success, At: r.Timestamp}
		if r.Duration > 0 {
			ms := float64(r.Duration) / 1e6
			res.MS = &ms
		}
		if !r.Success {
			res.Why = r.why()
		}
		ch.Results = append(ch.Results, res)
	}
	// Gatus lists results oldest first: the failures in a row are the newest.
	for i := len(ch.Results) - 1; i >= 0 && !ch.Results[i].OK; i-- {
		ch.Run++
	}
	ch.Problem = ch.Run >= c.Threshold
	if ch.Run > 0 {
		ch.Why = ch.Results[len(ch.Results)-1].Why
	}

	// Gatus records an event when a check's health turns: UNHEALTHY at its
	// first failure, HEALTHY at its first pass after. The stretch between
	// them was a problem if it lasted Threshold results.
	stretch := time.Duration(c.Threshold) * interval(ch.Results)
	var down *time.Time
	for _, ev := range e.Events {
		at := ev.Timestamp
		switch ev.Type {
		case "START":
			ch.Watched = &at
		case "UNHEALTHY":
			down = &at
		case "HEALTHY":
			if down != nil && at.Sub(*down) >= stretch {
				ch.Outages = append(ch.Outages, snapshot.Outage{From: *down, To: &at})
			}
			down = nil
		}
	}
	if down != nil && ch.Run > 0 {
		ch.Since = down
		if ch.Problem {
			ch.Outages = append(ch.Outages, snapshot.Outage{From: *down})
		}
	}
	return ch
}

// why is a failed result's reason: its first error, or its first failed
// condition.
func (r result) why() string {
	if len(r.Errors) > 0 {
		return r.Errors[0]
	}
	for _, c := range r.ConditionResults {
		if !c.Success {
			return c.Condition
		}
	}
	return ""
}

// interval is how often Gatus runs a check, from the spacing of its results.
// ponytail: a run Gatus skipped stretches it; read the check's own interval
// if Gatus ever serves it.
func interval(results []snapshot.Result) time.Duration {
	if len(results) < 2 {
		return time.Minute
	}
	return results[len(results)-1].At.Sub(results[0].At) / time.Duration(len(results)-1)
}

// area is the area a group goes in: the config's say, or the area it's named
// like, or none.
func area(group string, areas map[string]string) string {
	if a, ok := areas[group]; ok {
		return a
	}
	if a := strings.ToLower(group); slices.Contains(snapshot.Areas, a) {
		return a
	}
	return ""
}
