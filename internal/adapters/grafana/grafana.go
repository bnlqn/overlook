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
		ch := check(f.r, f.interval, f.changes)
		ch.Key, ch.Name, ch.Source = f.r.UID, f.r.Name, c.Name
		ch.Area = snapshot.AreaOf(f.folder, c.Areas)
		ch.URL = strings.TrimSuffix(c.Link, "/") + "/alerting/grafana/" + url.PathEscape(f.r.UID) + "/view"
		checks = append(checks, ch)
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

// check is what a rule's state and history say: its results at each of its
// last evaluations, its failures in a row, and its problems this week.
//
// ponytail: Grafana keeps only when a state changed, so the results at each
// evaluation are drawn from those changes at the group's interval, with no
// time taken but the last's. If Grafana ever keeps each evaluation, read
// them.
func check(r rule, interval time.Duration, history []change) snapshot.Check {
	ch := snapshot.Check{Results: []snapshot.Result{}, Threshold: 1}
	if r.LastEvaluation.Year() < 2000 {
		return ch // never evaluated yet
	}
	// The stretches it was failing and a problem: while any of its alerts
	// was, from their changes, and the one still going from its alerts now.
	type stretch struct {
		from time.Time
		to   *time.Time
	}
	var fails, problems []stretch
	var failingN, problemN int
	step := func(list *[]stretch, n *int, at time.Time, was, is bool) {
		switch {
		case !was && is:
			if *n == 0 {
				*list = append(*list, stretch{from: at})
			}
			*n++
		case was && !is && *n > 0:
			if *n--; *n == 0 {
				end := at
				(*list)[len(*list)-1].to = &end
			}
		}
	}
	for _, c := range history {
		step(&fails, &failingN, c.at, failing(c.prev), failing(c.next))
		step(&problems, &problemN, c.at, problem(c.prev), problem(c.next))
	}
	nowFailing := r.State == "firing" || r.State == "pending" || r.Health == "nodata" || r.Health == "error"
	ch.Problem = r.State == "firing" || r.Health == "nodata" || r.Health == "error"
	var since *time.Time
	for _, a := range r.Alerts {
		if failing(a.State) && (since == nil || a.ActiveAt.Before(*since)) {
			at := a.ActiveAt
			since = &at
			ch.Why = cmp.Or(r.LastError, a.Annotations["summary"])
		}
	}
	// Its state now is what it is, whatever its history says last.
	open := func(list []stretch, on bool) []stretch {
		if n := len(list); n > 0 && list[n-1].to == nil && !on {
			end := r.LastEvaluation
			list[n-1].to = &end
		}
		if on && (len(list) == 0 || list[len(list)-1].to != nil) {
			from := r.LastEvaluation
			if since != nil {
				from = *since
			}
			list = append(list, stretch{from: from})
		}
		return list
	}
	fails, problems = open(fails, nowFailing), open(problems, ch.Problem)

	in := func(list []stretch, t time.Time) bool {
		return slices.ContainsFunc(list, func(s stretch) bool { return !t.Before(s.from) && (s.to == nil || t.Before(*s.to)) })
	}
	if interval <= 0 {
		interval = time.Minute
	}
	for k := snapshot.Recent - 1; k >= 0; k-- {
		at := r.LastEvaluation.Add(-time.Duration(k) * interval)
		ch.Results = append(ch.Results, snapshot.Result{OK: !in(fails, at), At: at})
	}
	last := &ch.Results[len(ch.Results)-1]
	last.OK = !nowFailing
	if ms := r.EvaluationTime * 1000; ms > 0 {
		last.MS = &ms
	}
	if nowFailing {
		last.Why = ch.Why
	}
	for i := len(ch.Results) - 1; i >= 0 && !ch.Results[i].OK; i-- {
		ch.Run++
	}
	if ch.Run > 0 {
		ch.Since = &fails[len(fails)-1].from
	} else {
		ch.Why = ""
	}

	// How many evaluations in a row make a problem: as many as its pending
	// period holds, and the first.
	need := int(time.Duration(r.Duration*float64(time.Second))/interval) + 1
	if ch.Problem {
		ch.Threshold = max(1, min(need, ch.Run))
	} else {
		ch.Threshold = max(need, ch.Run+1)
	}
	for _, p := range problems {
		if p.to == nil || p.to.After(p.from) {
			ch.Outages = append(ch.Outages, snapshot.Outage{From: p.from, To: p.to})
		}
	}
	return ch
}
