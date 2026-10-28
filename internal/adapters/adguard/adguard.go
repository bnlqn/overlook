// Package adguard reads AdGuard Home, a DNS resolver: today's lookups and
// blocks, whether it is filtering, and its statistics, with its clients'
// names. Two resolvers are two sources, each with its own sections, and the
// page adds their figures up (snapshot.Figure).
package adguard

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// top is the rows each top list shows.
const top = 10

// Config is one AdGuard Home, under sources.adguard in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads AdGuard Home, its login in it as
	// user:pass@.
	URL string `yaml:"url"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if c.URL == "" {
		return errors.New("url is missing")
	}
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if c.Name == "" {
		c.Name = "AdGuard Home"
	}
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

// Read reads the AdGuard Home at c.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	return read(ctx, c, time.Now())
}

// read reads the AdGuard Home at c as at now, whose time zone says when
// today started.
func read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	base := strings.TrimSuffix(c.URL, "/") + "/control/"
	var s stats
	var st status
	var cl clients
	for _, r := range []struct {
		path string
		v    any
	}{{"stats", &s}, {"status", &st}, {"clients", &cl}} {
		if err := fetch.JSON(ctx, base+r.path, nil, r.v); err != nil {
			return snapshot.Parts{}, err
		}
	}
	return snapshot.Parts{Figures: today(s, now), Sections: sections(c, s, st, cl)}, nil
}

// today is the lookups since midnight where Overlook runs, with the blocked
// ones, and the last 24 hours. Statistics kept by the day have no today:
// AdGuard's days are UTC's.
func today(s stats, now time.Time) []snapshot.Figure {
	if s.TimeUnits != "hours" || len(s.ByUnit) < 24 || len(s.BlockedBy) < 24 {
		return nil
	}
	// The hours since midnight, this one included: 23 or 25 on the days the
	// clocks change.
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	since := int(now.Sub(midnight)/time.Hour) + 1
	hours := make([]float64, 24)
	for i, n := range s.ByUnit[len(s.ByUnit)-24:] {
		hours[i] = float64(n)
	}
	return []snapshot.Figure{{
		Area: "network", Icon: "shield", Label: snapshot.Text{Key: "adguard.today"}, Adds: true,
		Value: float64(sum(s.ByUnit, since)), Hours: hours,
		Detail: msg("adguard.today_blocked", args{"blocked": sum(s.BlockedBy, since)}),
	}}
}

// sum adds up the last n counts.
func sum(counts []int, n int) int {
	total := 0
	for _, v := range counts[max(0, len(counts)-n):] {
		total += v
	}
	return total
}

// sections are the resolver itself, then its top clients, domains and
// blocked domains, over the interval its statistics keep.
func sections(c Config, s stats, st status, cl clients) []snapshot.Section {
	unit := time.Hour
	if s.TimeUnits == "days" {
		unit = 24 * time.Hour
	}
	interval := time.Duration(len(s.ByUnit)) * unit

	protection := snapshot.Row{Title: *msg("adguard.protection", nil), Value: msg("adguard.filtering", nil), Tone: "green"}
	switch {
	case st.Filtering:
	case st.PausedFor > 0:
		protection.Value, protection.Tone = msg("adguard.paused", nil), "orange"
		protection.Detail = msg("adguard.back_in", args{"ms": st.PausedFor})
	default:
		protection.Value, protection.Tone = msg("adguard.off", nil), "red"
	}
	resolver := []snapshot.Row{
		protection,
		{
			Title: *msg("adguard.lookups", nil), Value: msg("adguard.n", args{"n": s.Lookups}),
			Detail: msg("adguard.over", args{"ms": interval.Milliseconds()}),
		},
		{
			Title: *msg("adguard.blocked", nil), Value: msg("adguard.percent", args{"n": percent(s.Blocked, s.Lookups)}),
			Detail: msg("adguard.count", args{"n": s.Blocked}),
		},
		{Title: *msg("adguard.processing", nil), Value: msg("adguard.ms", args{"n": ms(s.Processing)})},
	}
	// The upstreams say how the resolver resolves, so they sit with it.
	times := map[string]float64{}
	for _, t := range s.UpstreamTimes {
		maps.Copy(times, t)
	}
	for _, e := range entries(s.Upstreams) {
		resolver = append(resolver, snapshot.Row{
			Title: snapshot.Text{Name: e.name}, Detail: msg("adguard.upstream", nil),
			Value: msg("adguard.answers", args{"n": e.n, "ms": ms(times[e.name])}),
		})
	}
	resolver = append(resolver, snapshot.Row{Title: *msg("adguard.version", nil), Value: &snapshot.Text{Name: st.Version}})

	names := clientNames(cl)
	var who, what, blocked []snapshot.Row
	for _, e := range entries(s.Clients) {
		r := snapshot.Row{Title: snapshot.Text{Name: e.name}, Value: share(e.n, s.Lookups)}
		if name, ok := names[e.name]; ok {
			r.Title, r.Detail = snapshot.Text{Name: name}, &snapshot.Text{Name: e.name}
		}
		who = append(who, r)
	}
	for _, e := range entries(s.Domains) {
		what = append(what, snapshot.Row{Title: snapshot.Text{Name: e.name}, Value: share(e.n, s.Lookups)})
	}
	for _, e := range entries(s.BlockedTop) {
		blocked = append(blocked, snapshot.Row{Title: snapshot.Text{Name: e.name}, Value: msg("adguard.n", args{"n": e.n})})
	}

	on := args{"name": c.Name}
	return []snapshot.Section{
		{Area: "network", Title: snapshot.Text{Name: c.Name}, Rows: resolver},
		{Area: "network", Title: *msg("adguard.top_clients", on), Rows: orNothing(who)},
		{Area: "network", Title: *msg("adguard.top_domains", on), Rows: orNothing(what)},
		{Area: "network", Title: *msg("adguard.top_blocked", on), Rows: orNothing(blocked)},
	}
}

type entry struct {
	name string
	n    int
}

// entries are a top list's first ten.
func entries(list []map[string]int) []entry {
	var out []entry
	for _, item := range list[:min(top, len(list))] {
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

// orNothing is rows, or one that says there are none yet.
func orNothing(rows []snapshot.Row) []snapshot.Row {
	if len(rows) == 0 {
		return []snapshot.Row{{Title: *msg("adguard.nothing", nil)}}
	}
	return rows
}

type args = map[string]any

func msg(key string, a args) *snapshot.Text { return &snapshot.Text{Key: key, Args: a} }

// share is a count and its share of every lookup.
func share(n, lookups int) *snapshot.Text {
	return msg("adguard.share", args{"n": n, "share": percent(n, lookups)})
}

// percent is part of whole as a percentage, to a tenth.
func percent(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return math.Round(1000*float64(part)/float64(whole)) / 10
}

// ms is seconds as milliseconds, to a tenth.
func ms(seconds float64) float64 { return math.Round(seconds*1e4) / 10 }
