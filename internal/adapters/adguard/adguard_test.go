package adguard

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

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
		c := Config{Name: name, URL: src.URL + "/" + dir, User: "overlook", Password: "s3cret"}
		if err := c.Check(); err != nil {
			t.Fatal(err)
		}
		p, err := Read(context.Background(), c, recorded)
		if err != nil {
			t.Fatal(err)
		}
		adaptertest.Validate(t, Area, p)
		return p
	}
	primary, standby := parts("Primary DNS", "primary"), parts("Standby DNS", "standby")
	for _, r := range src.Sent() {
		if user, password, _ := (&http.Request{Header: r.Header}).BasicAuth(); user != "overlook" || password != "s3cret" {
			t.Errorf("%s: read without the user and password", r.URL)
		}
	}

	// Its hours timed back from now: since midnight is 17 of them, this
	// one included.
	for _, want := range []struct {
		p              snapshot.Parts
		lookups        float64
		blocked, first int
	}{{primary, 59146, 23650, 4856}, {standby, 1269, 26, 110}} {
		f := want.p.Figures[0]
		if f.Value != want.lookups || f.Detail.Args["blocked"] != want.blocked {
			t.Errorf("today: %v lookups, %v; want %v and %d blocked", f.Value, f.Detail.Args, want.lookups, want.blocked)
		}
		if f.Hours[0] != float64(want.first) || sumOf(f.Within.Hours[24-17:]) != float64(want.blocked) {
			t.Errorf("the last 24 hours, from %d, and the blocked ones within: %+v", want.first, f)
		}
	}
	if c := primary.Charts; len(c) != 1 || len(c[0].Series[0].Hours) != 168 || sumOf(c[0].Series[0].Hours) != 501106 || sumOf(c[0].Series[1].Hours) != 179069 {
		t.Errorf("the resolver's week by the hour, its lookups and the blocked ones: %+v", c)
	}

	rows := primary.Sections[0].Rows
	if len(rows) != 8 || rows[0].Value.Key != "resolver.on" {
		t.Fatalf("filtering, lookups, blocked, three upstreams, processing and the version: %+v", rows)
	}
	if l := rows[1]; l.Value.Args["n"] != 501106 || l.Detail.Args["ms"] != int64(7*24*time.Hour/time.Millisecond) {
		t.Errorf("lookups over 7 days: %+v %+v", l.Value, l.Detail)
	}
	if b := rows[2]; b.Detail.Args["n"] != 179069 {
		t.Errorf("blocked: %+v %+v", b.Value, b.Detail)
	}
	if u := rows[3]; u.Title.Name != "tls://dns.example.net:853" || u.Value.Args["n"] != 10988 || u.Value.Args["ms"] != 17.7 {
		t.Errorf("the busiest upstream, with its average: %+v", u)
	}
	if p := rows[6]; p.Value.Args["n"] != 1.6 {
		t.Errorf("processing in ms: %+v", p.Value)
	}
	if v := rows[7]; v.Value.Name != "v0.107.79" {
		t.Errorf("the version: %+v", v)
	}

	clients := primary.Sections[1].Rows
	if c := clients[0]; c.Title.Name != "living-room-tv.lan" || c.Detail.Name != "192.0.2.10" || c.Value.Args["share"] != 17.8 {
		t.Errorf("a client AdGuard found, by its name: %+v", c)
	}
	if r := clients[7]; r.Title.Name != "192.0.2.17" || r.Detail != nil {
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

// Statistics kept by the day have no hours, so no counts: AdGuard's days
// are UTC's.
func TestByDay(t *testing.T) {
	t.Parallel()
	byDay := stats{TimeUnits: "days", ByUnit: make([]int, 30), BlockedBy: make([]int, 30)}
	if r := resolver(byDay, status{}, clients{}, recorded); r.Counts != nil || r.Over != 30*24*time.Hour {
		t.Errorf("no counts, over 30 days: %+v", r)
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
