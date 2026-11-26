// Package qui reads Qui, which manages qBittorrents: each one's torrents, as
// the qBittorrent adapter shows them, the rules Qui runs on it and what they
// removed this week, and Qui's version.
package qui

import (
	"cmp"
	"context"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/qbittorrent"
	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "apps"

// Adapter says what this adapter is, for the config and its test.
var Adapter = snapshot.Adapter[Config]{Key: "qui", Area: Area, Read: Read}

// Config is one Qui, under sources.qui in overlook.yaml.
type Config struct {
	Name string `yaml:"name" setup:"text,optional"`
	// URL is where Overlook reads Qui, such as http://qui:7476.
	URL string `yaml:"url" setup:"text" example:"http://qui.example.com:7476"`
	// Key is an API key that reads the instances, the torrents and the
	// automations. With its login off, Qui lets Overlook in without one.
	Key string `yaml:"key" setup:"key,optional"`

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL}
	if c.Key != "" {
		c.source.Header = http.Header{"X-API-Key": {c.Key}}
	}
	if err := c.source.Check(); err != nil {
		return err
	}
	c.Name = cmp.Or(c.Name, "Qui")
	return nil
}

// instance is one qBittorrent Qui manages.
type instance struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Active    bool   `json:"isActive"`
	Connected bool   `json:"connected"`
	Error     string `json:"connectionError"`
	// Recent are its last errors, which say why Qui can't reach it when
	// Error doesn't.
	Recent []struct {
		Message string    `json:"errorMessage"`
		At      time.Time `json:"occurredAt"`
	} `json:"recentErrors"`
	Order int `json:"sortOrder"`
}

// why is why Qui can't reach in, as Qui words it, or nothing if it doesn't
// say.
func (in instance) why() string {
	if in.Error != "" {
		return in.Error
	}
	why, at := "", time.Time{}
	for _, e := range in.Recent {
		if why == "" || e.At.After(at) {
			why, at = e.Message, e.At
		}
	}
	return why
}

// rule is one of Qui's automations on an instance.
type rule struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	DryRun  bool   `json:"dryRun"`
}

// action is one line of an instance's automation log: a torrent a rule
// removed, or, in dry run, how many it would have.
type action struct {
	Action  string `json:"action"`
	Rule    string `json:"ruleName"`
	Outcome string `json:"outcome"`
	Details struct {
		Count int `json:"count"`
	} `json:"details"`
	At time.Time `json:"createdAt"`
}

// errored asks for the torrents in error alone. Qui counts every torrent
// whatever the filter, and gives the server's state beside them.
//
// ponytail: the 50 torrents in error Qui lists first, which a homelab's fit
// in; a page of each would be needed for more.
var errored = "?limit=50&filters=" + url.QueryEscape(`{"status":["errored"]}`)

// Read reads the Qui at c, which Check has filled: the torrents of each
// instance it manages that isn't disabled, in its order, or, in red, one Qui
// can't reach; Qui's version and each instance's rules; and what the rules
// removed this week.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var instances []instance
	if err := c.source.JSON(ctx, "/api/instances", &instances); err != nil {
		return snapshot.Parts{}, err
	}
	var version struct {
		Version string `json:"version"`
		Latest  string `json:"latestVersion"`
		Newer   bool   `json:"updateAvailable"`
	}
	if err := c.source.JSON(ctx, "/api/version", &version); err != nil {
		return snapshot.Parts{}, err
	}

	// A disabled instance is one Qui doesn't read; one with no id can't be
	// read through it.
	instances = slices.DeleteFunc(instances, func(in instance) bool { return !in.Active || in.ID == 0 })
	slices.SortStableFunc(instances, func(a, b instance) int { return cmp.Compare(a.Order, b.Order) })
	var p snapshot.Parts
	var read []qbittorrent.Torrents
	var rows []snapshot.Row
	unreached := false
	for _, in := range instances {
		// One qBittorrent's torrents are titled as the qBittorrent adapter
		// titles them, so that the page names each by its source beside a
		// qBittorrent read on its own; several are each named.
		s := snapshot.Section{Area: Area, Title: snapshot.Text{Key: "qbittorrent.torrents"}}
		if len(instances) > 1 {
			s.Title = snapshot.Text{Key: "qui.torrents_on", Args: map[string]any{"name": in.Name}}
		}
		if !in.Connected {
			unreached = true
			r := snapshot.Row{Title: snapshot.Text{Key: "qbittorrent.connection"}, Value: snapshot.Msg("qui.unreached", nil), Tone: "red"}
			if why := in.why(); why != "" {
				r.Detail = &snapshot.Text{Name: why}
			}
			s.Rows = []snapshot.Row{r}
			p.Sections = append(p.Sections, s)
			continue
		}
		at := "/api/instances/" + strconv.Itoa(in.ID)
		var list struct {
			Torrents []qbittorrent.Torrent `json:"torrents"`
			Counts   struct {
				Status map[string]int `json:"status"`
			} `json:"counts"`
			Server struct {
				Connection string `json:"connection_status"`
				Down       int64  `json:"dl_info_speed"`
				Up         int64  `json:"up_info_speed"`
				Free       int64  `json:"free_space_on_disk"`
			} `json:"serverState"`
		}
		var rules []rule
		var log []action
		if err := c.source.JSON(ctx, at+"/torrents"+errored, &list); err != nil {
			return snapshot.Parts{}, err
		}
		if err := c.source.JSON(ctx, at+"/automations", &rules); err != nil {
			return snapshot.Parts{}, err
		}
		if err := c.source.JSON(ctx, at+"/automations/activity?limit=1000", &log); err != nil {
			return snapshot.Parts{}, err
		}
		// Qui counts downloading and seeding as qBittorrent's own filters
		// do, so they're the same numbers the qBittorrent adapter shows.
		t := qbittorrent.Torrents{
			Downloading: list.Counts.Status["downloading"], Seeding: list.Counts.Status["seeding"],
			Down: list.Server.Down, Up: list.Server.Up, Free: list.Server.Free,
			Connection: list.Server.Connection, Errored: list.Torrents,
		}
		read = append(read, t)
		s.Rows = t.Rows()
		for _, r := range rules {
			row := snapshot.Row{
				Title: snapshot.Text{Name: r.Name}, Value: snapshot.Msg("qui.on", nil),
				Detail: snapshot.Msg("qui.rule_on", map[string]any{"name": in.Name}),
			}
			switch {
			case !r.Enabled:
				row.Value, row.Tone = snapshot.Msg("qui.off", nil), "gray"
			case r.DryRun:
				row.Value, row.Tone = snapshot.Msg("qui.dry_run", nil), "gray"
			}
			rows = append(rows, row)
		}
		p.Sections = append(p.Sections, s)
		p.Events = append(p.Events, removed(log, now)...)
	}

	newer := ""
	if version.Newer {
		newer = version.Latest
	}
	p.Sections = append(p.Sections, snapshot.Section{
		Area: Area, Title: snapshot.Text{Key: "qui.title"}, Rows: append([]snapshot.Row{snapshot.VersionRow(version.Version, newer)}, rows...),
	})
	if len(read) > 0 || unreached {
		p.Figures = []snapshot.Figure{qbittorrent.Figure(unreached, read...)}
	}
	return p, nil
}

// removed is what log says the rules removed in the week to now, what they
// would have removed in dry run, and what they failed to remove: one event
// for each rule, outcome and day, at the newest of them. Qui doesn't say
// which rule a dry run's line is from, so those of a day are one event.
// A rule's other actions, such as tagging, change nothing that can't be
// changed back, and aren't events.
//
// ponytail: the 1,000 newest lines of each instance's log, which a week of
// a homelab's rules fits in; a rule that removes more needs the log paged.
func removed(log []action, now time.Time) []snapshot.Event {
	type group struct{ key, rule, day string }
	var order []group
	count, newest := map[group]int{}, map[group]time.Time{}
	for _, a := range log {
		if a.At.After(now) || !a.At.After(now.AddDate(0, 0, -7)) {
			continue
		}
		g := group{rule: a.Rule, day: a.At.In(now.Location()).Format(time.DateOnly)}
		switch {
		case a.Action == "delete_failed":
			g.key = "qui.remove_failed"
		case !strings.HasPrefix(a.Action, "deleted_"):
			continue
		case a.Outcome == "success":
			g.key = "qui.removed"
		case a.Outcome == "dry-run":
			g.key, g.rule = "qui.would_remove", ""
		default:
			continue
		}
		if _, ok := count[g]; !ok {
			order = append(order, g)
		}
		count[g] += max(a.Details.Count, 1)
		if a.At.After(newest[g]) {
			newest[g] = a.At
		}
	}
	events := make([]snapshot.Event, 0, len(order))
	for _, g := range order {
		e := snapshot.Event{At: newest[g], Area: Area, Icon: "download", Text: *snapshot.Msg(g.key, map[string]any{"n": count[g]})}
		switch g.key {
		case "qui.remove_failed":
			e.Tone, e.Text.Args["rule"] = "red", g.rule
		case "qui.removed":
			e.Text.Args["rule"] = g.rule
		case "qui.would_remove":
			e.Tone = "gray"
		}
		events = append(events, e)
	}
	return events
}
