// Package uptimekuma reads Uptime Kuma, a check engine: the monitors on one
// of its status pages, each in the area its group there names, their recent
// results, and the problems those remember.
package uptimekuma

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// A monitor's result, as Uptime Kuma numbers it. Pending is a failure it
// tries again before calling the monitor down, which is when it alerts.
const (
	down        = 0
	up          = 1
	pending     = 2
	maintenance = 3
)

// Config is one Uptime Kuma, under sources.uptimekuma in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Uptime Kuma.
	URL string `yaml:"url"`
	// Page is the slug of the status page whose monitors Overlook reads,
	// such as homelab for /status/homelab.
	Page string `yaml:"page"`
	// Link is where the page sends the owner, if not URL.
	Link string `yaml:"link"`
	// Areas maps each group of the status page not named like an area to
	// the area it goes in.
	Areas map[string]string `yaml:"areas"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if c.Page == "" {
		return errors.New("page is missing: the slug of the status page to read, such as homelab")
	}
	c.Name = cmp.Or(c.Name, "Uptime Kuma")
	c.Link = cmp.Or(c.Link, c.URL)
	if err := fetch.CheckURL(c.Link); err != nil {
		return fmt.Errorf("link %w", err)
	}
	return snapshot.CheckAreas(c.Areas)
}

// beat is one result of a monitor, from the status page's heartbeats.
type beat struct {
	Status int      `json:"status"`
	Time   moment   `json:"time"`
	Ping   *float64 `json:"ping"`
}

// moment is a time as Uptime Kuma writes it, in UTC without saying so,
// such as 2026-10-29 11:03:08.690.
type moment struct{ time.Time }

func (m *moment) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	t, err := time.ParseInLocation(time.DateTime, s, time.UTC)
	m.Time = t
	return err
}

// Read reads the monitors on the status page of the Uptime Kuma at c.
func Read(ctx context.Context, c Config, _ time.Time) (snapshot.Parts, error) {
	base := strings.TrimSuffix(c.URL, "/") + "/api/status-page/"
	var page struct {
		Groups []struct {
			Name     string `json:"name"`
			Monitors []struct {
				ID   int    `json:"id"`
				Name string `json:"name"`
			} `json:"monitorList"`
		} `json:"publicGroupList"`
	}
	var beats struct {
		List map[string][]beat `json:"heartbeatList"`
	}
	if err := fetch.JSON(ctx, base+url.PathEscape(c.Page), nil, &page); err != nil {
		return snapshot.Parts{}, err
	}
	if err := fetch.JSON(ctx, base+"heartbeat/"+url.PathEscape(c.Page), nil, &beats); err != nil {
		return snapshot.Parts{}, err
	}
	checks := []snapshot.Check{}
	for _, g := range page.Groups {
		for _, m := range g.Monitors {
			key := strconv.Itoa(m.ID)
			ch := check(beats.List[key])
			ch.Key, ch.Name, ch.Source = key, m.Name, c.Name
			ch.Area = snapshot.AreaOf(g.Name, c.Areas)
			ch.URL = strings.TrimSuffix(c.Link, "/") + "/dashboard/" + key
			checks = append(checks, ch)
		}
	}
	return snapshot.Parts{Checks: checks}, nil
}

// check is what a monitor's beats, oldest first, say: its recent results,
// and from all Uptime Kuma keeps, its failures in a row and the problems
// they made. A monitor is a problem once Uptime Kuma calls it down, after
// the tries again it was given, and maintenance fails nothing.
func check(beats []beat) snapshot.Check {
	ok := func(b beat) bool { return b.Status == up || b.Status == maintenance }
	ch := snapshot.Check{Results: []snapshot.Result{}}
	for _, b := range beats[max(0, len(beats)-snapshot.Recent):] {
		ch.Results = append(ch.Results, snapshot.Result{OK: ok(b), At: b.Time.Time, MS: b.Ping})
	}

	run := 0
	for i := len(beats) - 1; i >= 0 && !ok(beats[i]); i-- {
		run++
	}
	if run > 0 {
		at := beats[len(beats)-run].Time.Time
		ch.Since = &at
	}
	ch.Problem = run > 0 && beats[len(beats)-1].Status == down

	// The stretches it was down, each from when Uptime Kuma called it so
	// to its next pass, and the tries again it took to get there: the
	// threshold, which Uptime Kuma keeps to itself.
	tries := 0
	var from *time.Time
	for i, b := range beats {
		switch {
		case b.Status == down && from == nil:
			at := b.Time.Time
			from = &at
			n := 0
			for j := i - 1; j >= 0 && beats[j].Status == pending; j-- {
				n++
			}
			tries = n
		case ok(b) && from != nil:
			to := b.Time.Time
			ch.Outages = append(ch.Outages, snapshot.Outage{From: *from, To: &to})
			from = nil
		}
	}
	if from != nil {
		ch.Outages = append(ch.Outages, snapshot.Outage{From: *from})
	}

	// ponytail: the tries again come from the last time it went down, and
	// a monitor that never did is a problem at the next failure as far as
	// the page says. If Uptime Kuma ever serves a monitor's retries to a
	// status page, read them.
	ch.Run = min(run, len(ch.Results))
	if ch.Problem {
		ch.Threshold = min(tries+1, ch.Run)
	} else {
		ch.Threshold = max(tries+1, ch.Run+1)
	}
	return ch
}
