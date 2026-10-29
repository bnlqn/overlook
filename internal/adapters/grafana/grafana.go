// Package grafana reads Grafana's alerting, a check engine: each alert rule
// Grafana manages is a check, in the area its folder names, with its recent
// results and the problems its state history remembers.
package grafana

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// week is how far back the state history is read, for this week's problems.
const week = 7 * 24 * time.Hour

// Config is one Grafana, under sources.grafana in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Grafana, such as http://grafana:3000.
	URL string `yaml:"url"`
	// Token is a service account's, with the Viewer role.
	Token string `yaml:"token"`
	// Link is where the page sends the owner, if not URL.
	Link string `yaml:"link"`
	// Areas maps each folder of alert rules not named like an area to the
	// area it goes in.
	Areas map[string]string `yaml:"areas"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if c.Token == "" {
		return errors.New("token is missing: a service account's, with the Viewer role")
	}
	c.Name = cmp.Or(c.Name, "Grafana")
	c.Link = cmp.Or(c.Link, c.URL)
	if err := fetch.CheckURL(c.Link); err != nil {
		return fmt.Errorf("link %w", err)
	}
	return snapshot.CheckAreas(c.Areas)
}

// rule is one of Grafana's alert rules, as its Prometheus-like API gives
// it: firing, pending or inactive, and its health, ok, nodata or error.
type rule struct {
	UID            string    `json:"uid"`
	Name           string    `json:"name"`
	State          string    `json:"state"`
	Health         string    `json:"health"`
	LastError      string    `json:"lastError"`
	Duration       float64   `json:"duration"` // seconds pending before firing
	LastEvaluation time.Time `json:"lastEvaluation"`
	EvaluationTime float64   `json:"evaluationTime"` // seconds
	Alerts         []struct {
		State       string            `json:"state"`
		ActiveAt    time.Time         `json:"activeAt"`
		Annotations map[string]string `json:"annotations"`
	} `json:"alerts"`
}

// history is a rule's state history, as a frame of columns: when each of
// its alerts changed, from what to what.
type history struct {
	Data struct {
		Values [][]any `json:"values"`
	} `json:"data"`
}

// change is one change of an alert's state.
type change struct {
	at         time.Time
	prev, next string
}

func (c Config) get(ctx context.Context, path string, v any) error {
	return fetch.JSON(ctx, strings.TrimSuffix(c.URL, "/")+path, http.Header{"Authorization": {"Bearer " + c.Token}}, v)
}

// Read reads every alert rule Grafana manages, at c.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	return read(ctx, c, time.Now())
}

func read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var rules struct {
		Data struct {
			Groups []struct {
				Folder   string  `json:"file"`
				Interval float64 `json:"interval"` // seconds
				Rules    []rule  `json:"rules"`
			} `json:"groups"`
		} `json:"data"`
	}
	if err := c.get(ctx, "/api/prometheus/grafana/api/v1/rules", &rules); err != nil {
		return snapshot.Parts{}, err
	}
	type found struct {
		r        rule
		folder   string
		interval time.Duration
		changes  []change
	}
	var all []found
	for _, g := range rules.Data.Groups {
		for _, r := range g.Rules {
			all = append(all, found{r: r, folder: g.Folder, interval: time.Duration(g.Interval * float64(time.Second))})
		}
	}
	// Each rule's state history, which only its own read has. A rule whose
	// read fails goes without its past.
	from := url.Values{"from": {strconv.FormatInt(now.Add(-week).UnixMilli(), 10)}}
	var wg sync.WaitGroup
	for i := range all {
		wg.Go(func() {
			var h history
			q := url.Values{"ruleUID": {all[i].r.UID}, "from": from["from"]}
			if c.get(ctx, "/api/v1/rules/history?"+q.Encode(), &h) == nil {
				all[i].changes = changes(h)
			}
		})
	}
	wg.Wait()
	checks := []snapshot.Check{}
	for _, f := range all {
		checks = append(checks, check(snapshot.Check{
			Key: f.r.UID, Name: f.r.Name, Area: snapshot.AreaOf(f.folder, c.Areas),
			URL: strings.TrimSuffix(c.Link, "/") + "/alerting/grafana/" + url.PathEscape(f.r.UID) + "/view",
		}, f.r, f.interval, f.changes))
	}
	return snapshot.Parts{Checks: checks}, nil
}

// changes are a history's changes, oldest first. Grafana writes their times
// in microseconds.
func changes(h history) []change {
	v := h.Data.Values
	if len(v) < 4 {
		return nil
	}
	var out []change
	for i := range min(len(v[0]), len(v[2]), len(v[3])) {
		t, ok := v[0][i].(float64)
		prev, _ := v[2][i].(string)
		next, _ := v[3][i].(string)
		if ok {
			out = append(out, change{at: time.UnixMicro(int64(t)), prev: prev, next: next})
		}
	}
	slices.SortStableFunc(out, func(a, b change) int { return a.at.Compare(b.at) })
	return out
}

// An alert's state, as its history words it, such as Alerting or
// Normal (Updated): failing while pending, and a problem once it fires, or
// while its query gives no data or an error, which Grafana alerts on too.
func failing(state string) bool {
	return problem(state) || strings.HasPrefix(state, "Pending")
}

func problem(state string) bool {
	return strings.HasPrefix(state, "Alerting") || strings.HasPrefix(state, "NoData") || strings.HasPrefix(state, "Error")
}

// check is ch as a rule's state and history make it: its results at each of
// its last evaluations, drawn from its changes at the group's interval, and
// the stretches it failed this week, each while any of its alerts was
// failing, alerted if one was a problem in it. Its state now is what it is,
// whatever its history says last.
func check(ch snapshot.Check, r rule, interval time.Duration, history []change) snapshot.Check {
	ch.Threshold = snapshot.ThresholdOf(time.Duration(r.Duration*float64(time.Second)), interval)
	var stretches []snapshot.Stretch
	failingN := 0
	for _, c := range history {
		n := len(stretches) - 1
		switch was, is := failing(c.prev), failing(c.next); {
		case !was && is:
			if failingN == 0 {
				stretches = append(stretches, snapshot.Stretch{From: c.at})
				n++
			}
			failingN++
		case was && !is && failingN > 0:
			if failingN--; failingN == 0 {
				at := c.at
				stretches[n].To = &at
			}
		}
		if problem(c.next) && n >= 0 {
			stretches[n].Alerted = true
		}
	}

	nowFailing := r.State == "firing" || r.State == "pending" || r.Health == "nodata" || r.Health == "error"
	alerting := r.State == "firing" || r.Health == "nodata" || r.Health == "error"
	var since *time.Time
	for _, a := range r.Alerts {
		if failing(a.State) && (since == nil || a.ActiveAt.Before(*since)) {
			since = &a.ActiveAt
			ch.Why = cmp.Or(r.LastError, a.Annotations["summary"])
		}
	}
	n := len(stretches) - 1
	switch {
	case n >= 0 && stretches[n].To == nil && !nowFailing:
		at := r.LastEvaluation
		stretches[n].To = &at
	case nowFailing && (n < 0 || stretches[n].To != nil):
		stretches = append(stretches, snapshot.Stretch{From: *cmp.Or(since, &r.LastEvaluation)})
		n++
	}
	if nowFailing {
		stretches[n].Alerted = alerting
	}

	ch.Results = snapshot.Grid(r.LastEvaluation, interval, stretches)
	if len(ch.Results) == 0 {
		return snapshot.CheckOf(ch, nil)
	}
	last := &ch.Results[len(ch.Results)-1]
	last.OK = !nowFailing
	if ms := r.EvaluationTime * 1000; ms > 0 {
		last.MS = &ms
	}
	if nowFailing {
		last.Why = ch.Why
	} else {
		ch.Why = ""
	}
	return snapshot.CheckOf(ch, stretches)
}
