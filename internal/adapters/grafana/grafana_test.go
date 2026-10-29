package grafana

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// testdata holds answers recorded from a scratch Grafana 13.3 (ADR 0008),
// which testdata/fill.sh fills: six alert rules over Grafana's own test
// data, evaluated every 10 seconds, in folders named Servers, Network,
// Storage and Apps. Disk space fired at 12:10, 12:12 and 12:14, for a
// minute each, and is firing; Packet loss has been pending since 12:10,
// for an hour; Offsite sync gets no data. record.sh read them at 12:14:22.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Token: "glsa_0123abcd", Link: "https://grafana.example.com/"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := read(context.Background(), c, time.UnixMilli(1793276062000))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	by := map[string]snapshot.Check{}
	var names []string
	for _, ch := range p.Checks {
		by[ch.Name] = ch
		names = append(names, ch.Name+" "+ch.Area)
	}
	if len(p.Checks) != 6 || by["Disk space"].Area != "servers" || by["Packet loss"].Area != "network" || by["Offsite sync"].Area != "storage" {
		t.Fatalf("six rules, each in its folder's area: %v", names)
	}

	at := func(clock string) time.Time {
		v, _ := time.Parse(time.RFC3339, "2026-10-29T"+clock+"Z")
		return v
	}
	d := by["Disk space"]
	if !d.Problem || d.Run != 2 || d.Threshold != 1 || !d.Since.Equal(at("12:14:10")) || d.Why != "Disk space is over 90" ||
		d.URL != "https://grafana.example.com/alerting/grafana/disk-space/view" {
		t.Errorf("Disk space, firing for two evaluations: %+v", d)
	}
	if len(d.Results) != snapshot.Recent || !d.Results[snapshot.Recent-1].At.Equal(at("12:14:20")) || d.Results[snapshot.Recent-1].MS == nil {
		t.Errorf("30 results, 10 seconds apart, to its last evaluation: %+v", d.Results)
	}
	failed := 0
	for _, r := range d.Results {
		if !r.OK {
			failed++
		}
	}
	if failed != 14 {
		t.Errorf("six evaluations each of the two past minutes and two now: %d", failed)
	}
	if len(d.Outages) != 3 || !d.Outages[0].From.Equal(at("12:10:10")) || !d.Outages[0].To.Equal(at("12:11:10")) || d.Outages[2].To != nil {
		t.Errorf("two outages this week, and the one still going: %+v", d.Outages)
	}

	if l := by["Packet loss"]; l.Problem || l.Run == 0 || l.Threshold != 361 || len(l.Outages) != 0 {
		t.Errorf("Packet loss, pending for an hour of 10-second evaluations: %+v", l)
	}
	if o := by["Offsite sync"]; !o.Problem || o.Run == 0 {
		t.Errorf("Offsite sync, with no data, which Grafana alerts on: %+v", o)
	}
	if h := by["Hypervisor memory"]; h.Problem || h.Run != 0 || h.Threshold != 1 || h.Since != nil {
		t.Errorf("Hypervisor memory, normal: %+v", h)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://grafana:3000", Token: "glsa_0123abcd"}
	if err := c.Check(); err != nil || c.Name != "Grafana" || c.Link != c.URL {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Token: "t"}, "url is missing"},
		{Config{URL: "grafana:3000", Token: "t"}, "url needs"},
		{Config{URL: "http://grafana:3000"}, "token is missing"},
		{Config{URL: "http://grafana:3000", Token: "t", Areas: map[string]string{"DNS": "attic"}}, "attic"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
