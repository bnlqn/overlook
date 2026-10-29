// Package gatus reads Gatus, the check engine: its checks, each in the area
// its group names, their recent results, and the problems its events
// remember.
package gatus

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Config is one Gatus, under sources.gatus in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Gatus.
	URL string `yaml:"url"`
	// User and Password are the login Overlook reads it with, if Gatus
	// asks for one.
	User     string `yaml:"user"`
	Password string `yaml:"password"`
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
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if (c.User == "") != (c.Password == "") {
		return errors.New("give user and password together, or neither")
	}
	c.Name = cmp.Or(c.Name, "Gatus")
	c.Link = cmp.Or(c.Link, c.URL)
	if err := fetch.CheckURL(c.Link); err != nil {
		return fmt.Errorf("link %w", err)
	}
	c.Threshold = cmp.Or(c.Threshold, 3)
	if c.Threshold < 1 {
		return errors.New("threshold must be 1 or more")
	}
	return snapshot.CheckAreas(c.Areas)
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
	login := fetch.Login(c.User, c.Password)
	var endpoints []endpoint
	if err := fetch.JSON(ctx, fmt.Sprintf("%s/api/v1/endpoints/statuses?page=1&pageSize=%d", base, snapshot.Recent), login, &endpoints); err != nil {
		return snapshot.Parts{}, err
	}
	// Each check's events, which only its own read has. A check whose read
	// fails goes without them.
	var wg sync.WaitGroup
	for i := range endpoints {
		wg.Go(func() {
			var one endpoint
			if fetch.JSON(ctx, base+"/api/v1/endpoints/"+url.PathEscape(endpoints[i].Key)+"/statuses?page=1&pageSize=1", login, &one) == nil {
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
		Key: e.Key, Name: e.Name, Area: snapshot.AreaOf(e.Group, c.Areas), Threshold: c.Threshold,
		URL: strings.TrimSuffix(c.Link, "/") + "/endpoints/" + url.PathEscape(e.Key),
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
	// Gatus lists results oldest first: the failures in a row are the newest,
	// and Gatus alerts once they reach the threshold.
	run := 0
	for i := len(ch.Results) - 1; i >= 0 && !ch.Results[i].OK; i-- {
		run++
	}
	if run > 0 {
		ch.Why = ch.Results[len(ch.Results)-1].Why
	}

	// Gatus records an event when a check's health turns: UNHEALTHY at its
	// first failure, HEALTHY at its first pass after. The stretch between
	// them alerted if it lasted Threshold results, and the one still going
	// if its failures in a row reach it. A check whose events Gatus didn't
	// give has a stretch from its first failure in a row.
	need := time.Duration(c.Threshold) * interval(ch.Results)
	var stretches []snapshot.Stretch
	for _, ev := range e.Events {
		at := ev.Timestamp
		n := len(stretches) - 1
		switch {
		case ev.Type == "START":
			ch.Watched = &at
		case ev.Type == "UNHEALTHY":
			stretches = append(stretches, snapshot.Stretch{From: at})
		case ev.Type == "HEALTHY" && n >= 0 && stretches[n].To == nil:
			stretches[n].To, stretches[n].Alerted = &at, at.Sub(stretches[n].From) >= need
		}
	}
	if n := len(stretches) - 1; run > 0 && (n < 0 || stretches[n].To != nil) {
		stretches = append(stretches, snapshot.Stretch{From: ch.Results[len(ch.Results)-run].At})
	}
	if n := len(stretches) - 1; n >= 0 && stretches[n].To == nil {
		stretches[n].Alerted = run >= c.Threshold
	}
	return snapshot.CheckOf(ch, stretches)
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
