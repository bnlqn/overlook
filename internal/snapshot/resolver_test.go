package snapshot

import (
	"slices"
	"testing"
	"time"
)

func zone(t *testing.T, name string) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func sum(hours []float64) float64 {
	var s float64
	for _, v := range hours {
		s += v
	}
	return s
}

// A resolver that keeps its own hours, as AdGuard does, gives them timed
// back from now: today is the hours since midnight, 25 on the day the
// clocks go back, and the chart every hour it keeps.
func TestResolverHours(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 25, 23, 50, 0, 0, zone(t, "Europe/Brussels"))
	var counts []Count
	for ago := range 168 {
		counts = append(counts, Count{At: now.Add(-time.Duration(ago) * time.Hour), Lookups: 2, Blocked: 1})
	}
	p := Resolver{Counts: counts, Total: 336, Blocked: 168, Over: 7 * 24 * time.Hour, Blocking: true}.Parts("network", now)
	if len(p.Figures) != 1 || len(p.Charts) != 1 {
		t.Fatalf("a figure and a chart: %+v", p)
	}
	f := p.Figures[0]
	if f.Label.Key != "resolver.dns_lookups" || f.Area != "network" || !f.Adds || f.Value != 50 || f.Detail.Args["blocked"] != 25 {
		t.Errorf("25 hours since midnight, adding up with any resolver's: %+v %+v", f, f.Detail)
	}
	if len(f.Hours) != 24 || sum(f.Hours) != 48 || f.Within == nil || len(f.Within.Hours) != 24 || sum(f.Within.Hours) != 24 {
		t.Errorf("the last 24 hours, and the blocked ones within: %+v", f)
	}
	c := p.Charts[0]
	if c.Title.Key != "resolver.lookups_per_hour" || !c.Adds || len(c.Series) != 2 || len(c.Series[0].Hours) != 168 || sum(c.Series[0].Hours) != 336 || sum(c.Series[1].Hours) != 168 {
		t.Errorf("the week by the hour, its lookups and the blocked ones: %+v", c)
	}
}

// Counts of less than an hour, as Pi-hole's ten minutes, go in the hour
// they fall in where Overlook runs: from the half hour in a half-hour zone.
func TestResolverHalfHourZone(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 29, 11, 20, 0, 0, zone(t, "Asia/Kolkata"))
	var counts []Count
	for i := range 145 {
		counts = append(counts, Count{At: now.Add(-time.Duration(i) * 10 * time.Minute), Lookups: 1})
	}
	f := Resolver{Counts: counts, Over: 24 * time.Hour, Blocking: true}.Parts("network", now).Figures[0]
	if f.Hours[23] != 3 || f.Hours[22] != 6 || f.Value != 69 {
		t.Errorf("11:00, 11:10 and 11:20 this hour, six in the last, 69 since midnight: %v, %v", f.Hours, f.Value)
	}
	if p := (Resolver{Over: 30 * 24 * time.Hour, Blocking: true}).Parts("network", now); p.Figures != nil || p.Charts != nil || len(p.Sections) != 4 {
		t.Errorf("statistics kept by the day: its sections, but no figure and no chart: %+v", p)
	}
}

// Every resolver's section is alike: blocking, its lookups, the blocked
// ones, its upstreams and then its own rows; and its three top lists.
func TestResolverSections(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 29, 11, 20, 0, 0, time.UTC)
	var clients []Top
	for i := range 12 {
		clients = append(clients, Top{Name: "host", Address: "192.0.2.1", N: 12 - i})
	}
	r := Resolver{
		Counts: []Count{{At: now, Lookups: 1}}, Total: 200, Blocked: 63, Over: 24 * time.Hour, Blocking: true,
		Upstreams:      []Upstream{{Name: "dns.example.net", Answers: 120, Average: 16940 * time.Microsecond}},
		Clients:        clients,
		BlockedDomains: []Top{{Name: "ads.example.com", N: 50}},
		Rows:           []Row{{Title: Text{Name: "v1.0"}}},
	}
	p := r.Parts("network", now)
	titles := []string{"resolver.section", "resolver.top_clients", "resolver.top_domains", "resolver.top_blocked"}
	if len(p.Sections) != 4 || !slices.EqualFunc(p.Sections, titles, func(s Section, k string) bool { return s.Title.Key == k && s.Area == "network" }) {
		t.Fatalf("the resolver and its three top lists, as any resolver's: %+v", p.Sections)
	}
	rows := p.Sections[0].Rows
	if len(rows) != 5 || rows[0].Value.Key != "resolver.on" || rows[0].Tone != "green" || rows[4].Title.Name != "v1.0" {
		t.Fatalf("blocking, lookups, blocked, the upstream, then its own: %+v", rows)
	}
	if r := rows[2]; r.Value.Args["n"] != 31.5 || r.Detail.Args["n"] != 63 {
		t.Errorf("31.5%% blocked, 63 of them: %+v %+v", r.Value, r.Detail)
	}
	if r := rows[3]; r.Title.Name != "dns.example.net" || r.Value.Args["n"] != 120 || r.Value.Args["ms"] != 16.9 {
		t.Errorf("the upstream's answers, in ms to a tenth: %+v", r.Value)
	}
	if who := p.Sections[1].Rows; len(who) != TopRows || who[0].Detail.Name != "192.0.2.1" || who[0].Value.Args["share"] != 6.0 {
		t.Errorf("ten clients, each with its address and its share: %+v", who)
	}
	if what := p.Sections[2].Rows; len(what) != 1 || what[0].Title.Key != "resolver.nothing_yet" {
		t.Errorf("no domains yet: %+v", what)
	}
	if denied := p.Sections[3].Rows; denied[0].Value.Key != "number" || denied[0].Value.Args["n"] != 50 {
		t.Errorf("a blocked domain's lookups: %+v", denied)
	}

	for _, c := range []struct {
		r            Resolver
		value, tone  string
		figureOrange bool
	}{
		{Resolver{Blocking: true}, "resolver.on", "green", false},
		{Resolver{BackIn: 18 * time.Minute}, "resolver.paused", "orange", false},
		{Resolver{}, "resolver.off", "red", true},
	} {
		c.r.Counts = []Count{{At: now}}
		p := c.r.Parts("network", now)
		b := p.Sections[0].Rows[0]
		if b.Value.Key != c.value || b.Tone != c.tone || (p.Figures[0].Tone == "orange") != c.figureOrange {
			t.Errorf("%+v: %+v, figure %q", c.r, b, p.Figures[0].Tone)
		}
		if c.tone == "orange" && b.Detail.Args["ms"] != int64(1080000) {
			t.Errorf("paused, back on in 18 minutes: %+v", b.Detail)
		}
	}
}
