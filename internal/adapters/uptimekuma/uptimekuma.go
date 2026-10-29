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

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL}
	if err := c.source.Check(); err != nil {
		return err
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
	const api = "/api/status-page/"
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
	if err := c.source.JSON(ctx, api+url.PathEscape(c.Page), &page); err != nil {
		return snapshot.Parts{}, err
	}
	if err := c.source.JSON(ctx, api+"heartbeat/"+url.PathEscape(c.Page), &beats); err != nil {
		return snapshot.Parts{}, err
	}
	checks := []snapshot.Check{}
	for _, g := range page.Groups {
		for _, m := range g.Monitors {
			key := strconv.Itoa(m.ID)
			checks = append(checks, check(snapshot.Check{
				Key: key, Name: m.Name, Area: snapshot.AreaOf(g.Name, c.Areas),
				URL: strings.TrimSuffix(c.Link, "/") + "/dashboard/" + key,
			}, beats.List[key]))
		}
	}
	return snapshot.Parts{Checks: checks}, nil
}

// check is ch as a monitor's beats, oldest first, make it: its results, and
// the stretches it failed, each from its first failing beat to its next
// pass, alerted once Uptime Kuma called it down, after the tries again it
// was given, and the one still going only while it's down now. Maintenance
// fails nothing.
func check(ch snapshot.Check, beats []beat) snapshot.Check {
	ok := func(b beat) bool { return b.Status == up || b.Status == maintenance }
	var stretches []snapshot.Stretch
	failed := 0 // the last stretch's failures
	for _, b := range beats {
		ch.Results = append(ch.Results, snapshot.Result{OK: ok(b), At: b.Time.Time, MS: b.Ping})
		n := len(stretches) - 1
		switch {
		case !ok(b) && (n < 0 || stretches[n].To != nil):
			stretches = append(stretches, snapshot.Stretch{From: b.Time.Time})
			failed, n = 0, n+1
		case ok(b) && n >= 0 && stretches[n].To == nil:
			to := b.Time.Time
			stretches[n].To = &to
		}
		if ok(b) {
			continue
		}
		// The tries again it took to go down, and the first, are its
		// threshold, which Uptime Kuma keeps to itself.
		failed++
		if b.Status == down && !stretches[n].Alerted {
			stretches[n].Alerted, ch.Threshold = true, failed
		}
	}

	// ponytail: the tries again come from the last time it went down, and
	// one that never did, or tries again more than it did then, has none.
	// If Uptime Kuma ever serves a monitor's retries to a status page, read
	// them.
	if n := len(stretches) - 1; n >= 0 && stretches[n].To == nil {
		stretches[n].Alerted = beats[len(beats)-1].Status == down
		if !stretches[n].Alerted && failed >= ch.Threshold {
			ch.Threshold = 0
		}
	}
	return snapshot.CheckOf(ch, stretches)
}
