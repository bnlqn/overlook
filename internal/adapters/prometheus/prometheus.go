// Package prometheus reads Prometheus, a check engine, with its
// Alertmanager: each alerting rule is a check, in the area its group names,
// with its last results from the history Prometheus keeps of its alerts,
// and a problem while it fires, unless Alertmanager silences it.
package prometheus

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// weekStep is how finely the week's problems are read: every five minutes,
// which a week of pending and firing alerts holds in a few thousand samples.
//
// ponytail: a problem's start and end are known to five minutes, but for
// the one still going, which starts when its alert went pending. If that
// shows on the week, read the week at the rules' interval.
const weekStep = 5 * time.Minute

// Config is one Prometheus, under sources.prometheus in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Prometheus, such as http://prometheus:9090.
	URL string `yaml:"url"`
	// Alertmanager is where Overlook reads the Alertmanager Prometheus sends
	// its alerts to, if it does, such as http://alertmanager:9093.
	Alertmanager string `yaml:"alertmanager"`
	// User and Password are the login both are read with, if they ask for
	// one.
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	// Link is where the page sends the owner, if not URL.
	Link string `yaml:"link"`
	// Areas maps each rule group not named like an area to the area it
	// goes in.
	Areas map[string]string `yaml:"areas"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if c.Alertmanager != "" {
		if err := fetch.CheckURL(c.Alertmanager); err != nil {
			return fmt.Errorf("alertmanager %w", err)
		}
	}
	if (c.User == "") != (c.Password == "") {
		return errors.New("give user and password together, or neither")
	}
	c.Name = cmp.Or(c.Name, "Prometheus")
	c.Link = cmp.Or(c.Link, c.URL)
	if err := fetch.CheckURL(c.Link); err != nil {
		return fmt.Errorf("link %w", err)
	}
	return snapshot.CheckAreas(c.Areas)
}

func (c Config) get(ctx context.Context, base, path string, v any) error {
	return fetch.JSON(ctx, strings.TrimSuffix(base, "/")+path, fetch.Login(c.User, c.Password), v)
}

type rule struct {
	Name           string            `json:"name"`
	Type           string            `json:"type"`
	State          string            `json:"state"` // inactive, pending or firing
	Health         string            `json:"health"`
	LastError      string            `json:"lastError"`
	Duration       float64           `json:"duration"` // seconds pending before firing
	LastEvaluation time.Time         `json:"lastEvaluation"`
	EvaluationTime float64           `json:"evaluationTime"`
	Annotations    map[string]string `json:"annotations"`
	Alerts         []struct {
		State       string            `json:"state"`
		ActiveAt    time.Time         `json:"activeAt"`
		Annotations map[string]string `json:"annotations"`
	} `json:"alerts"`
}

// series is a range query's answer: each alert name's samples, as
// [seconds, "value"].
type series struct {
	Data struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Values [][2]any          `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// times are the moments of each alert name's samples, in seconds, or only
// of those in state, if it's given.
func (s series) times(state string) map[string][]float64 {
	out := map[string][]float64{}
	for _, r := range s.Data.Result {
		if state != "" && r.Metric["alertstate"] != state {
			continue
		}
		for _, v := range r.Values {
			if t, ok := v[0].(float64); ok {
				out[r.Metric["alertname"]] = append(out[r.Metric["alertname"]], t)
			}
		}
	}
	for name := range out {
		slices.Sort(out[name])
		out[name] = slices.Compact(out[name])
	}
	return out
}

func (c Config) rangeOf(ctx context.Context, query string, from, to time.Time, step time.Duration) (series, error) {
	q := url.Values{
		"query": {query},
		"start": {strconv.FormatInt(from.Unix(), 10)}, "end": {strconv.FormatInt(to.Unix(), 10)},
		"step": {strconv.FormatFloat(step.Seconds(), 'f', -1, 64)},
	}
	var s series
	err := c.get(ctx, c.URL, "/api/v1/query_range?"+q.Encode(), &s)
	return s, err
}

// Read reads every alerting rule of the Prometheus at c.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	return read(ctx, c, time.Now())
}

func read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var rules struct {
		Data struct {
			Groups []struct {
				Name     string  `json:"name"`
				Interval float64 `json:"interval"` // seconds
				Rules    []rule  `json:"rules"`
			} `json:"groups"`
		} `json:"data"`
	}
	if err := c.get(ctx, c.URL, "/api/v1/rules?type=alert", &rules); err != nil {
		return snapshot.Parts{}, err
	}
	// The week, by alert name, for this week's problems: each five minutes,
	// whether it was pending or fired in them.
	week, err := c.rangeOf(ctx, `max by (alertname, alertstate) (max_over_time(ALERTS[5m]))`, now.Add(-7*24*time.Hour), now, weekStep)
	if err != nil {
		return snapshot.Parts{}, err
	}
	failingWeek, firing := week.times(""), week.times("firing")
	silenced, err := c.silenced(ctx)
	if err != nil {
		return snapshot.Parts{}, err
	}

	// The rules' last evaluations, on one grid to the newest of them, each
	// failing where an alert of its rule was pending or firing then, as
	// Prometheus's ALERTS series remembers: one query for each interval the
	// groups are evaluated at.
	var last time.Time
	for _, g := range rules.Data.Groups {
		for _, r := range g.Rules {
			if r.LastEvaluation.After(last) {
				last = r.LastEvaluation
			}
		}
	}
	failing := map[time.Duration]map[string][]float64{}
	checks := []snapshot.Check{}
	for _, g := range rules.Data.Groups {
		interval := time.Duration(g.Interval * float64(time.Second))
		if interval <= 0 {
			interval = time.Minute
		}
		if _, ok := failing[interval]; !ok && last.Year() > 2000 {
			s, err := c.rangeOf(ctx, "max by (alertname) (ALERTS)", last.Add(-(snapshot.Recent-1)*interval), last, interval)
			if err != nil {
				return snapshot.Parts{}, err
			}
			failing[interval] = s.times("")
		}
		for _, r := range g.Rules {
			if r.Type != "" && r.Type != "alerting" {
				continue
			}
			why, quiet := silenced[r.Name]
			checks = append(checks, check(snapshot.Check{
				Key: g.Name + "/" + r.Name, Name: r.Name, Area: snapshot.AreaOf(g.Name, c.Areas),
				URL: strings.TrimSuffix(c.Link, "/") + "/alerts?" + url.Values{"search": {r.Name}}.Encode(),
			}, r, last, interval, failing[interval][r.Name], stretchesOf(failingWeek[r.Name], firing[r.Name]), quiet, why))
		}
	}
	return snapshot.Parts{Checks: checks}, nil
}

// silence is one of Alertmanager's, with why, as its comment words it.
type silence struct {
	ID      string `json:"id"`
	Comment string `json:"comment"`
}

// silenced is, for each alert name all of whose alerts Alertmanager keeps
// quiet, by a silence or as inhibited by another, why, as the silence's
// comment words it. With no Alertmanager, none is.
func (c Config) silenced(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	if c.Alertmanager == "" {
		return out, nil
	}
	var alerts []struct {
		Labels map[string]string `json:"labels"`
		Status struct {
			State       string   `json:"state"` // active or suppressed
			SilencedBy  []string `json:"silencedBy"`
			InhibitedBy []string `json:"inhibitedBy"`
		} `json:"status"`
	}
	var silences []silence
	if err := c.get(ctx, c.Alertmanager, "/api/v2/alerts", &alerts); err != nil {
		return nil, err
	}
	if err := c.get(ctx, c.Alertmanager, "/api/v2/silences", &silences); err != nil {
		return nil, err
	}
	loud := map[string]bool{}
	for _, a := range alerts {
		name := a.Labels["alertname"]
		if a.Status.State != "suppressed" {
			loud[name] = true
			continue
		}
		why := ""
		for _, id := range a.Status.SilencedBy {
			if i := slices.IndexFunc(silences, func(s silence) bool { return s.ID == id }); i >= 0 {
				why = cmp.Or(why, silences[i].Comment)
			}
		}
		out[name] = cmp.Or(out[name], why)
	}
	for name := range loud {
		delete(out, name)
	}
	return out, nil
}

// stretchesOf are the week's stretches from its samples, each of which says
// a rule was pending or fired in the five minutes up to it: a stretch runs
// from five minutes before its first sample to its last, and alerted if it
// fired in one of them.
func stretchesOf(failing, firing []float64) []snapshot.Stretch {
	var out []snapshot.Stretch
	for i, t := range failing {
		at := time.Unix(int64(t), 0)
		if i == 0 || t-failing[i-1] > 1.5*weekStep.Seconds() {
			out = append(out, snapshot.Stretch{From: at.Add(-weekStep)})
		}
		s := &out[len(out)-1]
		_, fired := slices.BinarySearch(firing, t)
		s.To, s.Alerted = &at, s.Alerted || fired
	}
	return out
}

// check is ch as a rule's state, its recent alerts and its week make it: its
// last results, failing where an alert of it was pending or firing then, and
// the stretches it failed this week, the one still going from when its
// first alert went pending. A rule Alertmanager keeps quiet still fails,
// with why it's quiet, but its stretch now hasn't alerted: Alertmanager
// sends no alert for it, and it has no threshold to reach.
//
// ponytail: a stretch Alertmanager kept quiet before now has alerted, as
// Overlook reads only the silences now. If that shows on the week, read the
// silences that ended too.
func check(ch snapshot.Check, r rule, last time.Time, interval time.Duration, recent []float64, week []snapshot.Stretch, quiet bool, why string) snapshot.Check {
	ch.Threshold = snapshot.ThresholdOf(time.Duration(r.Duration*float64(time.Second)), interval)
	if quiet {
		ch.Threshold = 0
	}
	// Each sample of the last evaluations is the evaluation nearest it.
	var evaluations []snapshot.Stretch
	for _, t := range recent {
		from := time.Unix(int64(t), 0).Add(-interval / 2)
		to := from.Add(interval)
		evaluations = append(evaluations, snapshot.Stretch{From: from, To: &to})
	}
	ch.Results = snapshot.Grid(last, interval, evaluations)
	if len(ch.Results) == 0 {
		return snapshot.CheckOf(ch, nil)
	}
	// Its last result is its state now, whatever the series has caught.
	now := &ch.Results[len(ch.Results)-1]
	now.OK = r.State == "inactive"
	if ms := r.EvaluationTime * 1000; ms > 0 {
		now.MS = &ms
	}
	if now.OK {
		return snapshot.CheckOf(ch, week)
	}

	var since *time.Time
	for _, a := range r.Alerts {
		if (a.State == "pending" || a.State == "firing") && (since == nil || a.ActiveAt.Before(*since)) {
			since = &a.ActiveAt
			ch.Why = cmp.Or(a.Annotations["summary"], r.Annotations["summary"], r.LastError)
		}
	}
	if quiet && why != "" {
		ch.Why = why
	}
	now.Why = ch.Why
	// The week's stretch that started within five minutes of this one is
	// this one, as the week can tell; one that started before ends where
	// this one starts.
	from := *cmp.Or(since, &last)
	if n := len(week) - 1; n >= 0 && !week[n].From.Before(from.Add(-weekStep)) {
		week = week[:n]
	}
	if n := len(week) - 1; n >= 0 && week[n].To.After(from) {
		week[n].To = &from
	}
	return snapshot.CheckOf(ch, append(week, snapshot.Stretch{From: from, Alerted: r.State == "firing" && !quiet}))
}
