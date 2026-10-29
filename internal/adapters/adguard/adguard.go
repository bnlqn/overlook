// Package adguard reads AdGuard Home, a resolver: its lookups and blocks,
// whether it is filtering, and its statistics, with its clients' names, as
// any resolver's (snapshot.Resolver).
package adguard

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"math"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "network"

// Config is one AdGuard Home, under sources.adguard in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads AdGuard Home.
	URL string `yaml:"url"`
	// User and Password are the login Overlook reads it with.
	User     string `yaml:"user"`
	Password string `yaml:"password"`

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL, Header: fetch.Basic(c.User, c.Password)}
	if err := c.source.Check(); err != nil {
		return err
	}
	if (c.User == "") != (c.Password == "") {
		return errors.New("give user and password together, or neither")
	}
	c.Name = cmp.Or(c.Name, "AdGuard Home")
	return nil
}

// stats is AdGuard Home's /control/stats: its counts over the interval it
// keeps, by the hour up to 7 days and by the day beyond, the current one
// last; and its top lists, biggest first, each entry an object of one name.
type stats struct {
	TimeUnits  string           `json:"time_units"`
	Lookups    int              `json:"num_dns_queries"`
	Blocked    int              `json:"num_blocked_filtering"`
	Processing float64          `json:"avg_processing_time"` // seconds
	ByUnit     []int            `json:"dns_queries"`
	BlockedBy  []int            `json:"blocked_filtering"`
	Clients    []map[string]int `json:"top_clients"`
	Domains    []map[string]int `json:"top_queried_domains"`
	BlockedTop []map[string]int `json:"top_blocked_domains"`
	Upstreams  []map[string]int `json:"top_upstreams_responses"`
	// UpstreamTimes are each upstream's average answer, in seconds.
	UpstreamTimes []map[string]float64 `json:"top_upstreams_avg_time"`
}

// status is AdGuard Home's /control/status.
type status struct {
	Version   string `json:"version"`
	Filtering bool   `json:"protection_enabled"`
	// PausedFor is how long until filtering comes back on, in
	// milliseconds, when the owner paused it for a while.
	PausedFor int64 `json:"protection_disabled_duration"`
}

// clients is AdGuard Home's /control/clients: the ones the owner named, each
// by its addresses, and the ones it found itself, such as by reverse DNS.
type clients struct {
	Named []named `json:"clients"`
	Found []found `json:"auto_clients"`
}

type named struct {
	Name string   `json:"name"`
	IDs  []string `json:"ids"`
}

type found struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

// Read reads the AdGuard Home at c as at now, whose time zone says when
// today started.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var s stats
	var st status
	var cl clients
	for _, r := range []struct {
		path string
		v    any
	}{{"/control/stats", &s}, {"/control/status", &st}, {"/control/clients", &cl}} {
		if err := c.source.JSON(ctx, r.path, r.v); err != nil {
			return snapshot.Parts{}, err
		}
	}
	return resolver(s, st, cl, now).Parts(Area, now), nil
}

// resolver is what AdGuard Home's statistics, status and clients say, as at
// now, over the interval its statistics keep.
func resolver(s stats, st status, cl clients, now time.Time) snapshot.Resolver {
	unit := time.Hour
	if s.TimeUnits == "days" {
		unit = 24 * time.Hour
	}
	r := snapshot.Resolver{
		Total: s.Lookups, Blocked: s.Blocked, Over: time.Duration(len(s.ByUnit)) * unit,
		Blocking: st.Filtering, BackIn: time.Duration(st.PausedFor) * time.Millisecond,
	}
	// Its hours end with the current one, so each is timed back from now.
	// Statistics kept by the day have none: AdGuard's days are UTC's.
	//
	// ponytail: AdGuard's hours are UTC's, so in a half-hour zone each one
	// counted is half in the hour before the one it is put in. AdGuard
	// can't be asked for others; split each in two if it matters there.
	if s.TimeUnits == "hours" && len(s.BlockedBy) == len(s.ByUnit) {
		for i, n := range s.ByUnit {
			ago := time.Duration(len(s.ByUnit)-1-i) * time.Hour
			r.Counts = append(r.Counts, snapshot.Count{At: now.Add(-ago), Lookups: n, Blocked: s.BlockedBy[i]})
		}
	}
	times := map[string]float64{}
	for _, t := range s.UpstreamTimes {
		maps.Copy(times, t)
	}
	for _, e := range entries(s.Upstreams) {
		r.Upstreams = append(r.Upstreams, snapshot.Upstream{Name: e.name, Answers: e.n, Average: time.Duration(times[e.name] * float64(time.Second))})
	}
	names := clientNames(cl)
	for _, e := range entries(s.Clients) {
		t := snapshot.Top{Name: e.name, N: e.n}
		if name, ok := names[e.name]; ok {
			t.Name, t.Address = name, e.name
		}
		r.Clients = append(r.Clients, t)
	}
	for _, e := range entries(s.Domains) {
		r.Domains = append(r.Domains, snapshot.Top{Name: e.name, N: e.n})
	}
	for _, e := range entries(s.BlockedTop) {
		r.BlockedDomains = append(r.BlockedDomains, snapshot.Top{Name: e.name, N: e.n})
	}
	r.Rows = []snapshot.Row{
		{Title: snapshot.Text{Key: "adguard.processing"}, Value: snapshot.Msg("adguard.ms", map[string]any{"n": ms(s.Processing)})},
		{Title: snapshot.Text{Key: "adguard.version"}, Value: &snapshot.Text{Name: st.Version}},
	}
	return r
}

type entry struct {
	name string
	n    int
}

// entries are a top list's, each entry an object of one name.
func entries(list []map[string]int) []entry {
	var out []entry
	for _, item := range list {
		for name, n := range item {
			out = append(out, entry{name, n})
		}
	}
	return out
}

// clientNames are the clients' names by address: the ones AdGuard found,
// under the ones the owner gave it. An address range names no one address.
func clientNames(cl clients) map[string]string {
	names := map[string]string{}
	for _, f := range cl.Found {
		if _, ok := names[f.IP]; f.Name != "" && !ok {
			names[f.IP] = f.Name
		}
	}
	for _, n := range cl.Named {
		for _, id := range n.IDs {
			if !strings.Contains(id, "/") {
				names[id] = n.Name
			}
		}
	}
	return names
}

// ms is seconds as milliseconds, to a tenth.
func ms(seconds float64) float64 { return math.Round(seconds*1e4) / 10 }
