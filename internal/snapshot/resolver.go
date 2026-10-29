package snapshot

import (
	"math"
	"time"
)

// TopRows is how many rows a resolver's top list shows. An adapter whose
// source can be asked for fewer asks for no more.
const TopRows = 10

// Resolver is what an adapter reads from a resolver (CONTEXT.md), which
// Parts makes the figure, chart and sections every resolver gives, so that
// two resolvers' lookups add up, whichever adapters read them (ADR 0012).
type Resolver struct {
	// Counts are its lookups and the blocked ones over Over, each at a
	// moment of the time it counts, such as the start of Pi-hole's ten
	// minutes. With none, as for statistics kept by the day, it gives no
	// figure and no chart.
	Counts []Count
	// Total and Blocked are its lookups and the blocked ones over Over,
	// as long as its statistics keep them.
	Total, Blocked int
	Over           time.Duration
	// Blocking says whether it blocks. When it doesn't, BackIn is how long
	// until it does again, if the owner paused it for a while.
	Blocking bool
	BackIn   time.Duration
	// Upstreams are the servers it asks, the busiest first, of which it
	// shows as many as a top list.
	Upstreams []Upstream
	// Clients, Domains and BlockedDomains are its top lists, the most
	// first.
	Clients, Domains, BlockedDomains []Top
	// Rows are its own, after the upstreams, such as its version.
	Rows []Row
}

// Count is a resolver's lookups and the blocked ones in a stretch of time.
type Count struct {
	At               time.Time
	Lookups, Blocked int
}

// Upstream is a server a resolver asks, how many lookups it answered, and
// how long it took on average.
type Upstream struct {
	Name    string
	Answers int
	Average time.Duration
}

// Top is one of a top list: a name, the address under it if it has one,
// and its lookups.
type Top struct {
	Name, Address string
	N             int
}

// Parts are the resolver's parts in area, as at now: its lookups today with
// their last 24 hours, the chart of every hour it keeps, its section and its
// three top lists.
func (r Resolver) Parts(area string, now time.Time) Parts {
	blocking := Row{Title: Text{Key: "resolver.blocking"}, Value: Msg("resolver.on", nil), Tone: "green"}
	switch {
	case r.Blocking:
	case r.BackIn > 0:
		blocking.Value, blocking.Tone = Msg("resolver.paused", nil), "orange"
		blocking.Detail = Msg("resolver.back_in", map[string]any{"ms": r.BackIn.Milliseconds()})
	default:
		blocking.Value, blocking.Tone = Msg("resolver.off", nil), "red"
	}
	rows := []Row{
		blocking,
		{
			Title: Text{Key: "resolver.lookups"}, Value: Number(r.Total),
			Detail: Msg("resolver.over", map[string]any{"ms": r.Over.Milliseconds()}),
		},
		{
			Title: Text{Key: "resolver.blocked"}, Value: Percent(percentOf(r.Blocked, r.Total)),
			Detail: Msg("resolver.count", map[string]any{"n": r.Blocked}),
		},
	}
	// The upstreams say how it resolves, so they sit with it.
	for _, u := range r.Upstreams[:min(TopRows, len(r.Upstreams))] {
		rows = append(rows, Row{
			Title: Text{Name: u.Name}, Detail: Msg("resolver.upstream", nil),
			Value: Msg("resolver.answers", map[string]any{"n": u.Answers, "ms": math.Round(float64(u.Average)/1e5) / 10}),
		})
	}
	rows = append(rows, r.Rows...)

	share := func(n int) *Text {
		return Msg("resolver.share", map[string]any{"n": n, "share": percentOf(n, r.Total)})
	}
	top := func(list []Top, value func(int) *Text) []Row {
		var out []Row
		for _, t := range list[:min(TopRows, len(list))] {
			row := Row{Title: Text{Name: t.Name}, Value: value(t.N)}
			if t.Address != "" {
				row.Detail = &Text{Name: t.Address}
			}
			out = append(out, row)
		}
		if len(out) == 0 {
			return []Row{{Title: Text{Key: "resolver.nothing_yet"}}}
		}
		return out
	}
	// The same titles as any resolver's, so that the page names each by
	// its source beside another's.
	p := Parts{Sections: []Section{
		{Area: area, Title: Text{Key: "resolver.section"}, Rows: rows},
		{Area: area, Title: Text{Key: "resolver.top_clients"}, Rows: top(r.Clients, share)},
		{Area: area, Title: Text{Key: "resolver.top_domains"}, Rows: top(r.Domains, share)},
		{Area: area, Title: Text{Key: "resolver.top_blocked"}, Rows: top(r.BlockedDomains, Number[int])},
	}}
	if len(r.Counts) == 0 {
		return p
	}

	kept := int(r.Over / time.Hour)
	lookups, blocked := newTally(now, max(kept, 24)), newTally(now, max(kept, 24))
	for _, c := range r.Counts {
		lookups.add(c.At, c.Lookups)
		blocked.add(c.At, c.Blocked)
	}
	last := func(t *tally, n int) []float64 { return t.hours[len(t.hours)-n:] }
	p.Figures = []Figure{{
		Area: area, Icon: "shield", Label: Text{Key: "resolver.dns_lookups"}, Adds: true,
		Value: float64(lookups.today), Hours: last(lookups, 24),
		Within: &Series{Label: Text{Key: "resolver.blocked"}, Hours: last(blocked, 24)},
		Detail: Msg("resolver.lookups_blocked", map[string]any{"blocked": blocked.today}),
		Tone:   orange(rows),
	}}
	if kept >= 2 {
		p.Charts = []Chart{{
			Area: area, Title: Text{Key: "resolver.lookups_per_hour"}, Adds: true,
			Series: []Series{
				{Label: Text{Key: "resolver.lookups"}, Hours: last(lookups, kept)},
				{Label: Text{Key: "resolver.blocked"}, Hours: last(blocked, kept)},
			},
		}}
	}
	return p
}

// percentOf is part of whole as a percentage, to a tenth.
func percentOf(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return math.Round(1000*float64(part)/float64(whole)) / 10
}
