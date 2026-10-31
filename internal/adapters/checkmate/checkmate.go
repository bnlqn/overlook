// Package checkmate reads Checkmate, a check engine: each monitor it checks
// is a check, in the area its group names, with its recent results and the
// problems its incidents remember.
package checkmate

import (
	"cmp"
	"context"
	"encoding/json"
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

// Config is one Checkmate, under sources.checkmate in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Checkmate, such as http://checkmate:52345.
	URL string `yaml:"url"`
	// User and Password are a Checkmate user's email and password: one with
	// the role user, which reads every monitor and changes none. Checkmate
	// lets Overlook in only with a login, one POST (ADR 0009).
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	// Link is where the page sends the owner, if not URL.
	Link string `yaml:"link"`
	// Areas maps each group of monitors not named like an area to the area
	// it goes in.
	Areas map[string]string `yaml:"areas"`

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL, Login: &fetch.Login{
		Path: "/api/v1/auth/login", Credentials: map[string]string{"email": c.User, "password": c.Password}, Take: token,
	}}
	if err := c.source.Check(); err != nil {
		return err
	}
	if c.User == "" || c.Password == "" {
		return errors.New("user and password are missing: Checkmate lets Overlook in only with a login")
	}
	c.Name = cmp.Or(c.Name, "Checkmate")
	c.Link = cmp.Or(c.Link, c.URL)
	if err := fetch.CheckURL(c.Link); err != nil {
		return fmt.Errorf("link %w", err)
	}
	return snapshot.CheckAreas(c.Areas)
}

// token takes the session from a login's answer: the token Checkmate gives,
// which it lets last 99 days unless its owner says otherwise, sent back as a
// bearer's.
func token(_ http.Header, body []byte) (http.Header, error) {
	var a struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &a); err != nil || a.Data.Token == "" {
		return nil, errors.New("no token in the answer")
	}
	return http.Header{"Authorization": {"Bearer " + a.Data.Token}}, nil
}

// monitor is one of Checkmate's monitors, with its recent checks, oldest
// first.
type monitor struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Type  string `json:"type"`
	Group string `json:"group"`
	// Status is up, down, breached, initializing, maintenance or paused.
	Status   string `json:"status"`
	IsActive bool   `json:"isActive"`
	// Checkmate calls a monitor down once Threshold percent of its last
	// Size checks failed.
	Size      int       `json:"statusWindowSize"`
	Threshold int       `json:"statusWindowThreshold"`
	CreatedAt time.Time `json:"createdAt"`
	Checks    []result  `json:"recentChecks"`
}

// result is one of a monitor's checks: whether it reached what it checks,
// how long it took, and Checkmate's words for how it went.
type result struct {
	Status    bool      `json:"status"`
	MS        float64   `json:"responseTime"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

// incident is a stretch Checkmate called a monitor down, or breached, and
// alerted on it, which has no end while it lasts.
type incident struct {
	ID        string     `json:"id"`
	MonitorID string     `json:"monitorId"`
	StartTime time.Time  `json:"startTime"`
	EndTime   *time.Time `json:"endTime"`
	// StatusCode is breach, for a monitor whose thresholds were crossed, and
	// Message then says which.
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

// breach is the status code Checkmate gives an incident that a hardware or
// Docker monitor's thresholds opened, rather than a failed check.
const breach = 9999

// pages are where Checkmate's own links show a monitor of each type; every
// other type is under uptime.
var pages = map[string]string{"hardware": "infrastructure", "pagespeed": "pagespeed", "docker": "docker/host"}

// Read reads every monitor Checkmate checks, at c.
func Read(ctx context.Context, c Config, _ time.Time) (snapshot.Parts, error) {
	var list struct {
		Data struct {
			Monitors []monitor `json:"monitors"`
		} `json:"data"`
	}
	if err := c.source.JSON(ctx, "/api/v1/monitors/team/with-checks", &list); err != nil {
		return snapshot.Parts{}, err
	}
	// A paused monitor is neither checked nor alerted on.
	monitors := slices.DeleteFunc(list.Data.Monitors, func(m monitor) bool { return !m.IsActive })

	// The incidents still open, however old, and the others, as Checkmate's
	// own list of them asks. Checkmate dates an incident by when it opened,
	// so those of the last month hold every one that ended this week, but
	// for one longer. An incident that closes between the two reads is in
	// both, and the second, closed, is the one kept.
	// ponytail: an incident open for over a month and closed this week is
	// lost, and a month of more than 1000 loses its oldest; page through
	// them if a homelab ever has that many.
	byID := map[string]incident{}
	for _, q := range []string{"status=true", "status=false&dateRange=month"} {
		var page struct {
			Data struct {
				Incidents []incident `json:"incidents"`
			} `json:"data"`
		}
		if err := c.source.JSON(ctx, "/api/v1/incidents/team?"+q+"&sortOrder=desc&page=0&rowsPerPage=1000", &page); err != nil {
			return snapshot.Parts{}, err
		}
		for _, in := range page.Data.Incidents {
			byID[in.ID] = in
		}
	}
	incidents := map[string][]incident{}
	for _, in := range byID {
		incidents[in.MonitorID] = append(incidents[in.MonitorID], in)
	}

	// The list gives a hardware monitor only its newest check, so each
	// one's last results take a read of their own, of the last day, which
	// holds 30 at any interval up to 48 minutes. One whose read fails keeps
	// its newest.
	var wg sync.WaitGroup
	for i := range monitors {
		if monitors[i].Type != "hardware" {
			continue
		}
		wg.Go(func() {
			var page struct {
				Data struct {
					Checks []result `json:"checks"`
				} `json:"data"`
			}
			q := fmt.Sprintf("?type=hardware&sortOrder=desc&dateRange=day&page=0&rowsPerPage=%d", snapshot.Recent)
			if c.source.JSON(ctx, "/api/v1/checks/"+url.PathEscape(monitors[i].ID)+q, &page) == nil && len(page.Data.Checks) > 0 {
				slices.Reverse(page.Data.Checks)
				monitors[i].Checks = page.Data.Checks
			}
		})
	}
	wg.Wait()

	checks := []snapshot.Check{}
	for _, m := range monitors {
		checks = append(checks, check(snapshot.Check{
			Key: m.ID, Name: m.Name, Area: snapshot.AreaOf(m.Group, c.Areas),
			URL: strings.TrimSuffix(c.Link, "/") + "/" + cmp.Or(pages[m.Type], "uptime") + "/" + url.PathEscape(m.ID),
		}, m, incidents[m.ID]))
	}
	return snapshot.Parts{Checks: checks}, nil
}

// check is ch as a monitor and its incidents make it. A result fails when
// its check did, or while a breach was open, when Checkmate checked it
// fine but found it over its thresholds. Each run of failures in the
// results is a stretch, from its first failure to its next pass, alerted if
// an incident overlaps it; an incident before the first result is a
// stretch of its own. The one still going has alerted only while Checkmate
// calls the monitor down or breached.
func check(ch snapshot.Check, m monitor, incidents []incident) snapshot.Check {
	ch.Threshold, ch.Watched = threshold(m), &m.CreatedAt
	slices.SortFunc(incidents, func(a, b incident) int { return a.StartTime.Compare(b.StartTime) })
	// Checkmate leaves an incident open when a maintenance window or a pause
	// takes its monitor out of down or breached, and nothing closes it
	// after, so one is still open only while the monitor is down or
	// breached, and ended where it began otherwise. A breach someone
	// resolved by hand goes on while Checkmate calls the monitor breached.
	alerting := m.Status == "down" || m.Status == "breached"
	for i := range incidents {
		if in := &incidents[i]; in.EndTime == nil && !alerting {
			in.EndTime = &in.StartTime
		}
	}
	for i := len(incidents) - 1; i >= 0 && m.Status == "breached"; i-- {
		if incidents[i].StatusCode == breach {
			incidents[i].EndTime = nil
			break
		}
	}
	for _, r := range m.Checks {
		res := snapshot.Result{OK: r.Status, At: r.CreatedAt}
		if r.MS > 0 {
			res.MS = &r.MS
		}
		if !r.Status {
			res.Why = r.Message
		} else if i := slices.IndexFunc(incidents, func(in incident) bool { return in.StatusCode == breach && covers(in, r.CreatedAt) }); i >= 0 {
			res.OK, res.Why = false, incidents[i].Message
		}
		ch.Results = append(ch.Results, res)
	}

	var stretches []snapshot.Stretch
	for _, in := range incidents {
		if len(ch.Results) > 0 && !in.StartTime.Before(ch.Results[0].At) {
			break
		}
		s := snapshot.Stretch{From: in.StartTime, To: in.EndTime, Alerted: true}
		// One that lasted into the results ends where they say it did.
		if len(ch.Results) > 0 && (s.To == nil || s.To.After(ch.Results[0].At)) {
			s.To = nil
		}
		stretches = append(stretches, s)
	}
	for _, r := range ch.Results {
		n := len(stretches) - 1
		going := n >= 0 && stretches[n].To == nil
		switch {
		case !r.OK && !going:
			stretches = append(stretches, snapshot.Stretch{From: r.At})
		case r.OK && going:
			stretches[n].To = &r.At
		}
	}
	for i := range stretches {
		s := &stretches[i]
		s.Alerted = s.Alerted || slices.ContainsFunc(incidents, func(in incident) bool {
			return (s.To == nil || !in.StartTime.After(*s.To)) && (in.EndTime == nil || !in.EndTime.Before(s.From))
		})
	}
	if n := len(stretches) - 1; n >= 0 && stretches[n].To == nil {
		stretches[n].Alerted = alerting
	}

	if n := len(ch.Results); n > 0 && !ch.Results[n-1].OK {
		ch.Why = ch.Results[n-1].Why
	}
	// A monitor Checkmate doesn't call down however many checks it failed,
	// as one put in a maintenance window while it failed, has no threshold
	// to speak of.
	if ch = snapshot.CheckOf(ch, stretches); !ch.Problem && ch.Run >= ch.Threshold {
		ch.Threshold = 0
	}
	return ch
}

// covers says whether in was open at t.
func covers(in incident, t time.Time) bool {
	return !t.Before(in.StartTime) && (in.EndTime == nil || t.Before(*in.EndTime))
}

// threshold is how many failures in a row make Checkmate call m down: the
// share of its status window its threshold names, rounded up, so 3 of 5 at
// 60%. A heartbeat monitor's window is its last beat alone.
func threshold(m monitor) int {
	if m.Type == "heartbeat" {
		return 1
	}
	return max(1, (m.Size*m.Threshold+99)/100)
}
