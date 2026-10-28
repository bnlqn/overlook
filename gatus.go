package main

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// The Gatus adapter: the check engine's checks, each in the area its group
// names, their recent results, and the problems Gatus remembers.

// The results each check's strip shows: half an hour at Gatus's minute.
const recent = 30

type gatusEndpoint struct {
	Name    string `json:"name"`
	Group   string `json:"group"`
	Key     string `json:"key"`
	Results []struct {
		Duration         int64     `json:"duration"` // nanoseconds
		Success          bool      `json:"success"`
		Timestamp        time.Time `json:"timestamp"`
		Errors           []string  `json:"errors"`
		ConditionResults []struct {
			Condition string `json:"condition"`
			Success   bool   `json:"success"`
		} `json:"conditionResults"`
	} `json:"results"`
	Events []struct {
		Type      string    `json:"type"`
		Timestamp time.Time `json:"timestamp"`
	} `json:"events"`
}

func readGatus(ctx context.Context, g GatusConfig) (Parts, error) {
	base := strings.TrimSuffix(g.URL, "/")
	var endpoints []gatusEndpoint
	if err := getJSON(ctx, fmt.Sprintf("%s/api/v1/endpoints/statuses?page=1&pageSize=%d", base, recent), &endpoints); err != nil {
		return Parts{}, err
	}
	// Each check's events, which only its own read has. A check whose read
	// fails goes without them.
	var wg sync.WaitGroup
	for i := range endpoints {
		wg.Go(func() {
			var one gatusEndpoint
			if getJSON(ctx, base+"/api/v1/endpoints/"+url.PathEscape(endpoints[i].Key)+"/statuses?page=1&pageSize=1", &one) == nil {
				endpoints[i].Events = one.Events
			}
		})
	}
	wg.Wait()
	checks := make([]Check, len(endpoints))
	for i, e := range endpoints {
		checks[i] = gatusCheck(e, g)
	}
	return Parts{Checks: checks}, nil
}

func gatusCheck(e gatusEndpoint, g GatusConfig) Check {
	c := Check{
		Key: e.Key, Name: e.Name, Area: gatusArea(e.Group, g.Areas), Source: g.Name, Threshold: g.Threshold,
		URL: strings.TrimSuffix(g.Link, "/") + "/endpoints/" + url.PathEscape(e.Key), Results: []Result{},
	}
	for _, r := range e.Results {
		res := Result{OK: r.Success, At: r.Timestamp}
		if r.Duration > 0 {
			ms := float64(r.Duration) / 1e6
			res.MS = &ms
		}
		if !r.Success && len(r.Errors) > 0 {
			res.Why = r.Errors[0]
		} else if !r.Success {
			for _, cr := range r.ConditionResults {
				if !cr.Success {
					res.Why = cr.Condition
					break
				}
			}
		}
		c.Results = append(c.Results, res)
	}
	// Gatus lists results oldest first: the failures in a row are the newest.
	for i := len(c.Results) - 1; i >= 0 && !c.Results[i].OK; i-- {
		c.Run++
	}
	c.Problem = c.Run >= g.Threshold
	if c.Run > 0 {
		c.Why = c.Results[len(c.Results)-1].Why
	}

	// Gatus records an event when a check's health turns: UNHEALTHY at its
	// first failure, HEALTHY at its first pass after. The stretch between
	// them was a problem if it lasted Threshold results.
	stretch := time.Duration(g.Threshold) * interval(c.Results)
	var down *time.Time
	for _, ev := range e.Events {
		t := ev.Timestamp
		switch ev.Type {
		case "START":
			c.Watched = &t
		case "UNHEALTHY":
			down = &t
		case "HEALTHY":
			if down != nil && t.Sub(*down) >= stretch {
				c.Outages = append(c.Outages, Outage{From: *down, To: &t})
			}
			down = nil
		}
	}
	if down != nil && c.Run > 0 {
		c.Since = down
		if c.Problem {
			c.Outages = append(c.Outages, Outage{From: *down})
		}
	}
	return c
}

// interval is how often Gatus runs a check, from the spacing of its results.
// ponytail: a run Gatus skipped stretches it; read the check's own interval
// if Gatus ever serves it.
func interval(results []Result) time.Duration {
	if len(results) < 2 {
		return time.Minute
	}
	return results[len(results)-1].At.Sub(results[0].At) / time.Duration(len(results)-1)
}

// gatusArea is the area a group goes to: the config's say, or the area it's
// named like. A check in no area still counts, and still shows as a problem.
func gatusArea(group string, areas map[string]string) string {
	if a, ok := areas[group]; ok {
		return a
	}
	if a := strings.ToLower(group); slices.Contains(Areas, a) {
		return a
	}
	return ""
}
