package unifi

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// testdata holds answers recorded by record.sh from the author's UniFi
// Cloud Gateway Fiber, on UniFi OS 5.1.34 with Network 10.6.109, on 30
// October 2026 at 09:44:57 UTC, with every ID, device name and MAC address
// made up: one site, seven devices all online, 27 clients, nothing waiting,
// a WAN up for 31 hours and a second one with nothing plugged in.
var recorded = time.Date(2026, 10, 30, 9, 44, 57, 0, time.UTC)

// replay reads the recorded console, with edits made to its answers, and
// checks what it sent.
func replay(t *testing.T, edits ...string) snapshot.Parts {
	t.Helper()
	src := adaptertest.Replay(t, "testdata", edits...)
	c := Config{URL: src.URL, Key: "0123abcd", Link: "https://unifi.example.com/"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, recorded)
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, Area, p)
	for _, r := range src.Sent() {
		if r.Header.Get("X-Api-Key") != "0123abcd" {
			t.Errorf("%s: read without the key", r.URL)
		}
	}
	return p
}

func TestRead(t *testing.T) {
	t.Parallel()
	p := replay(t)

	if len(p.Sections) != 2 || p.Sections[0].Title.Key != "unifi.internet" || p.Sections[1].Title.Key != "unifi.devices" {
		t.Fatalf("the internet, then the devices: %+v", p.Sections)
	}
	wans := p.Sections[0].Rows
	if len(wans) != 2 {
		t.Fatalf("two WANs and no speed test, none having run: %+v", wans)
	}
	if w := wans[0]; w.Title.Name != "WAN" || w.Tone != "" || w.Value.Args["ms"] != int64(111848000) ||
		w.Detail.Key != "unifi.wan_detail" || w.Detail.Args["availability"] != 99.8 || w.Detail.Args["latency"] != 3.0 || w.Detail.Args["period"] != int64(86400000) {
		t.Errorf("WAN, up 31 hours, 99.8%% of the last day, at 3 ms: %+v %+v %+v", w, w.Value, w.Detail)
	}
	if w := wans[1]; w.Title.Name != "WAN2" || w.Tone != "gray" || w.Detail.Key != "unifi.down_since_start" {
		t.Errorf("WAN2, with nothing plugged in, down since the gateway started, in grey: %+v %+v", w, w.Detail)
	}

	var names []string
	for _, r := range p.Sections[1].Rows {
		names = append(names, r.Title.Name)
		if r.Tone != "" || r.Value.Key != "unifi.up_for" || len(r.Meters) != 2 || r.Detail.Args["model"] == "" {
			t.Errorf("%s: online, with its uptime, processor, memory and model: %+v", r.Title.Name, r)
		}
	}
	if strings.Join(names, ", ") != "ap-attic, ap-garden, ap-kitchen, ap-study, gateway, switch-garage, switch-hall" {
		t.Errorf("the seven devices, by name: %v", names)
	}
	if r := p.Sections[1].Rows[4]; r.Value.Args["ms"] != int64(111922000) || r.Meters[0].Used != 22.6 || r.Meters[1].Used != 52.4 ||
		r.Detail.Args["model"] != "UCG Fiber" || r.Detail.Args["firmware"] != "5.1.34" {
		t.Errorf("the gateway, on UniFi OS 5.1.34: %+v %+v %+v", r.Value, r.Meters, r.Detail)
	}

	if len(p.Figures) != 1 {
		t.Fatalf("one figure: %+v", p.Figures)
	}
	f := p.Figures[0]
	if f.Value != 27 || !f.Adds || f.Tone != "" || f.Detail.Args["online"] != 7 || f.Detail.Args["n"] != 7 {
		t.Errorf("27 clients, all seven devices online, and WAN2 doesn't turn it orange: %+v %+v", f, f.Detail)
	}
	if len(f.Shares) != 2 || f.Shares[0].Label.Key != "unifi.wired" || f.Shares[0].Value != 13 || f.Shares[1].Value != 14 {
		t.Errorf("13 wired and 14 wireless, and none on the VPN: %+v", f.Shares)
	}
	if len(p.Chores) != 0 {
		t.Errorf("nothing waiting: %+v", p.Chores)
	}
}

func TestReadChores(t *testing.T) {
	t.Parallel()
	p := replay(t,
		"\"6.7.57\",\n      \"firmwareUpdatable\": false", "\"6.7.57\",\n      \"firmwareUpdatable\": true",
		"\"totalCount\": 0,\n  \"data\": []", "\"totalCount\": 1,\n  \"data\": [{\"model\": \"U7 Pro\", \"macAddress\": \"00:00:5e:00:53:10\"}]",
		"\"update_available\": false", "\"update_available\": true",
	)
	want := []snapshot.Chore{
		{Name: "Network", Note: "10.6.109 →", URL: "https://unifi.example.com/network/default/settings/updates"},
		{Name: "U7 Pro", Note: "00:00:5e:00:53:10", URL: "https://unifi.example.com/network/default/devices"},
		{Name: "ap-study", Note: "6.7.57 →", URL: "https://unifi.example.com/network/default/devices"},
	}
	if len(p.Chores) != len(want) {
		t.Fatalf("the application's update, a device to adopt and one to update: %+v", p.Chores)
	}
	for i, ch := range p.Chores {
		if ch != want[i] {
			t.Errorf("chore %d: got %+v, want %+v", i, ch, want[i])
		}
	}
}

func TestReadWANDown(t *testing.T) {
	t.Parallel()
	p := replay(t,
		"\"downtime\": 111865", "\"downtime\": 600",
		"\"speedtest_lastrun\": 0", "\"speedtest_lastrun\": 1793340000",
		"\"speedtest_ping\": 0,", "\"speedtest_ping\": 4,",
		"\"xput_down\": 0.0", "\"xput_down\": 912.4",
		"\"xput_up\": 0.0", "\"xput_up\": 488.6",
	)
	wans := p.Sections[0].Rows
	if len(wans) != 3 {
		t.Fatalf("two WANs and the speed test: %+v", wans)
	}
	if w := wans[1]; w.Tone != "red" || w.Detail.Key != "unifi.down_since" || w.Detail.Args["at"] != recorded.Add(-10*time.Minute) {
		t.Errorf("WAN2, down ten minutes ago after the gateway started, in red: %+v %+v", w, w.Detail)
	}
	if s := wans[2]; s.Title.Key != "unifi.speed_test" || s.Value.Args["down"] != 912.0 || s.Value.Args["up"] != 489.0 ||
		s.Detail.Args["at"] != time.Unix(1793340000, 0) || s.Detail.Args["ping"] != 4.0 {
		t.Errorf("the last speed test, after the WANs: %+v %+v %+v", s, s.Value, s.Detail)
	}
	if f := p.Figures[0]; f.Tone != "orange" {
		t.Errorf("a WAN down turns the figure orange: %+v", f)
	}
}

// The older API's reads, which no document promises, leave out only their
// own rows when an upgrade moves or changes them.
func TestReadOlderAPIGone(t *testing.T) {
	t.Parallel()
	p := replay(t,
		"\"subsystem\": \"wan\"", "\"subsystem\": \"wan\"}",
		"\"update_available\": false", "\"update_available\": \"no\"",
	)
	if len(p.Sections) != 1 || p.Sections[0].Title.Key != "unifi.devices" || len(p.Sections[0].Rows) != 7 {
		t.Errorf("the devices, without the internet: %+v", p.Sections)
	}
	if len(p.Figures) != 1 || p.Figures[0].Value != 27 || len(p.Chores) != 0 {
		t.Errorf("the clients still, and no chore: %+v %+v", p.Figures, p.Chores)
	}
}

func TestPartsSitesAndStates(t *testing.T) {
	t.Parallel()
	// The only WAN, down since the gateway started a day ago.
	down := int64(86000)
	w := line{Subsystem: "wan", WANs: map[string]wan{"WAN": {Downtime: &down}}}
	w.Gateway.Uptime = "86400"
	h := &health{Data: []line{w}}

	p := parts([]read{
		{
			site: site{Name: "Home", Ref: "default"}, health: h, clients: 3, wired: 3,
			devices: []device{{ID: "1", Name: "ap-hall", Model: "U6 Lite", State: "OFFLINE"}},
		},
		{
			site: site{Name: "Cabin", Ref: "cabin"}, clients: 2, wireless: 1,
			devices: []device{{ID: "2", Name: "ap-loft", Model: "U6 Mesh", State: "CONNECTION_INTERRUPTED"}, {ID: "3", Name: "sw", State: "SOMETHING_NEW"}},
		},
	}, nil, nil, "https://unifi.example.com", recorded)
	adaptertest.Validate(t, Area, p)

	var titles []string
	for _, s := range p.Sections {
		titles = append(titles, s.Title.Key+" "+s.Title.Args["site"].(string))
	}
	if strings.Join(titles, ", ") != "unifi.internet_at Home, unifi.devices_at Home, unifi.devices_at Cabin" {
		t.Errorf("each site's sections, named by it: %v", titles)
	}
	if w := p.Sections[0].Rows[0]; w.Tone != "red" || w.Detail.Key != "unifi.down_since" {
		t.Errorf("the only WAN, down since the gateway started, is red: %+v", w)
	}
	rows := slices.Concat(p.Sections[1].Rows, p.Sections[2].Rows)
	for i, want := range []string{"red unifi.offline", "orange unifi.interrupted", "gray "} {
		if r := rows[i]; r.Tone+" "+r.Value.Key != want {
			t.Errorf("%s: want %s, got %+v", r.Title.Name, want, r)
		}
	}
	if f := p.Figures[0]; f.Value != 5 || f.Tone != "orange" || f.Detail.Args["online"] != 0 || f.Detail.Args["n"] != 3 || len(f.Shares) != 3 {
		t.Errorf("five clients, three wired, one wireless and one on the VPN, and no device online: %+v", f)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "https://192.0.2.1", Key: "0123abcd"}
	if err := c.Check(); err != nil || c.Name != "UniFi" || c.Link != c.URL {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Key: "0123abcd"}, "url is missing"},
		{Config{URL: "https://192.0.2.1"}, "key is missing"},
		{Config{URL: "https://192.0.2.1", Key: "0123abcd", ServerName: "unifi.local"}, "server_name needs"},
		{Config{URL: "https://192.0.2.1", Key: "0123abcd", Link: "unifi"}, "link needs"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
