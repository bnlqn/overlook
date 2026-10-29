package adguard

import (
	"context"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // Europe/Brussels, wherever the test runs

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// testdata holds two resolvers' answers, recorded at 16:36 CET on 28 October
// 2026 and scrubbed: a primary with 7 days of statistics, and a standby
// started the day before.
var recorded = time.Date(2026, 10, 28, 16, 36, 0, 0, time.FixedZone("CET", 3600))

func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	parts := func(name, dir string) snapshot.Parts {
		c := Config{Name: name, URL: src.URL + "/" + dir}
		if err := c.Check(); err != nil {
			t.Fatal(err)
		}
		p, err := read(context.Background(), c, recorded)
		if err != nil {
			t.Fatal(err)
		}
		adaptertest.Validate(t, p)
		return p
	}
	primary, standby := parts("Primary DNS", "primary"), parts("Standby DNS", "standby")

	// Since midnight is 17 of AdGuard's hours, this one included.
	for _, want := range []struct {
		p              snapshot.Parts
		lookups        float64
		blocked, first int
	}{{primary, 59146, 23650, 4856}, {standby, 1269, 26, 110}} {
		f := want.p.Figures[0]
		if f.Value != want.lookups || f.Detail.Args["blocked"] != want.blocked {
			t.Errorf("today: %v lookups, %v; want %v and %d blocked", f.Value, f.Detail.Args, want.lookups, want.blocked)
		}
		if len(f.Hours) != 24 || f.Hours[0] != float64(want.first) || f.Label.Key != snapshot.LookupsFigure.Key || f.Area != "network" || !f.Adds {
			t.Errorf("the last 24 hours, from %d: %+v", want.first, f)
		}
		if b := f.Within; b == nil || b.Label.Key != snapshot.Blocked.Key || len(b.Hours) != 24 || sumOf(b.Hours[24-17:]) != float64(want.blocked) {
			t.Errorf("the blocked ones in each of those hours, %d since midnight: %+v", want.blocked, b)
		}
	}
	if c := primary.Charts; len(c) != 1 || c[0].Title.Key != snapshot.LookupsChart.Key || !c[0].Adds || len(c[0].Series) != 2 ||
		len(c[0].Series[0].Hours) != 168 || sumOf(c[0].Series[0].Hours) != 501106 || sumOf(c[0].Series[1].Hours) != 179069 {
		t.Errorf("the resolver's week by the hour, its lookups and the blocked ones, adding up with the others': %+v", c)
	}

	if len(primary.Sections) != 4 || primary.Sections[0].Title.Key != snapshot.ResolverSection.Key {
		t.Fatalf("the resolver, then its three top lists: %+v", primary.Sections)
	}
	rows := primary.Sections[0].Rows
	if p := rows[0]; p.Value.Key != "adguard.filtering" || p.Tone != "green" {
		t.Errorf("protection: %+v", p)
	}
	if l := rows[1]; l.Value.Args["n"] != 501106 || l.Detail.Args["ms"] != int64(7*24*time.Hour/time.Millisecond) {
		t.Errorf("lookups over 7 days: %+v %+v", l.Value, l.Detail)
	}
	if b := rows[2]; b.Value.Args["n"] != 35.7 || b.Detail.Args["n"] != 179069 {
		t.Errorf("blocked: %+v %+v", b.Value, b.Detail)
	}
	if p := rows[3]; p.Value.Args["n"] != 1.6 {
		t.Errorf("processing in ms: %+v", p.Value)
	}
	if u := rows[4]; u.Title.Name != "tls://dns.example.net:853" || u.Value.Args["n"] != 10988 || u.Value.Args["ms"] != 17.7 {
		t.Errorf("the busiest upstream, with its average: %+v", u)
	}
	if v := rows[len(rows)-1]; v.Value.Name != "v0.107.79" || len(rows) != 8 {
		t.Errorf("three upstreams, then the version: %+v", rows)
	}

	clients := primary.Sections[1]
	if clients.Title.Key != snapshot.TopClients.Key || len(clients.Rows) != top {
		t.Fatalf("top clients: %+v", clients)
	}
	if c := clients.Rows[0]; c.Title.Name != "living-room-tv.lan" || c.Detail.Name != "192.0.2.10" || c.Value.Args["share"] != 17.8 {
		t.Errorf("a client AdGuard found, by its name: %+v", c)
	}
	if r := clients.Rows[7]; r.Title.Name != "192.0.2.17" || r.Detail != nil {
		t.Errorf("a client with no name goes by its address: %+v", r)
	}
	if b := standby.Sections[3].Rows; len(b) != 7 || b[0].Title.Name != "ads.example.com" || b[0].Value.Args["n"] != 14 {
		t.Errorf("the standby's 7 blocked domains: %+v", b)
	}
}

func sumOf(hours []float64) float64 {
	total := 0.0
	for _, v := range hours {
		total += v
	}
	return total
}

func TestClientNames(t *testing.T) {
	t.Parallel()
	cl := clients{
		Found: []found{{"desk-lamp.lan", "192.0.2.20"}},
		Named: []named{{"Tablet", []string{"192.0.2.20", "198.51.100.0/24"}}},
	}
	if names := clientNames(cl); names["192.0.2.20"] != "Tablet" || len(names) != 1 {
		t.Errorf("the owner's name beats the one AdGuard found, and a range names no one: %v", names)
	}
}

func TestToday(t *testing.T) {
	t.Parallel()
	brussels, err := time.LoadLocation("Europe/Brussels")
	if err != nil {
		t.Fatal(err)
	}
	ones := stats{TimeUnits: "hours", ByUnit: make([]int, 168), BlockedBy: make([]int, 168)}
	for i := range ones.ByUnit {
		ones.ByUnit[i] = 1
	}
	for _, c := range []struct {
		at   time.Time
		want float64
	}{
		{time.Date(2026, 10, 28, 0, 10, 0, 0, brussels), 1},
		{time.Date(2026, 10, 28, 23, 50, 0, 0, brussels), 24},
		// The clocks went back at 3:00, so the day had 25 hours.
		{time.Date(2026, 10, 25, 23, 50, 0, 0, brussels), 25},
	} {
		if f := today(ones, c.at); f[0].Value != c.want {
			t.Errorf("at %s: %v hours since midnight, want %v", c.at, f[0].Value, c.want)
		}
	}
	byDay := stats{TimeUnits: "days", ByUnit: make([]int, 30), BlockedBy: make([]int, 30)}
	if f := today(byDay, recorded); f != nil {
		t.Errorf("statistics by the day have no today: %+v", f)
	}
	if c := hourly(byDay); c != nil {
		t.Errorf("nor any hours: %+v", c)
	}
}

func TestProtection(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		st        status
		key, tone string
		pausedFor any
	}{
		{status{Filtering: true}, "adguard.filtering", "green", nil},
		{status{PausedFor: 25 * 60e3}, "adguard.paused", "orange", int64(25 * 60e3)},
		{status{}, "adguard.off", "red", nil},
	} {
		p := sections(stats{}, c.st, clients{})[0].Rows[0]
		var back any
		if p.Detail != nil {
			back = p.Detail.Args["ms"]
		}
		if p.Value.Key != c.key || p.Tone != c.tone || back != c.pausedFor {
			t.Errorf("%+v: %+v", c.st, p)
		}
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://dns.example.com", User: "overlook", Password: "s3cret"}
	if err := c.Check(); err != nil || c.Name != "AdGuard Home" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{}, "url is missing"},
		{Config{URL: "dns.example.com"}, "url needs"},
		{Config{URL: "http://overlook:s3cret@dns.example.com"}, "url can't hold a login"},
		{Config{URL: "http://dns.example.com", User: "overlook"}, "user and password together"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
