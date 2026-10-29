package uptimekuma

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// testdata holds a status page recorded from a scratch Uptime Kuma 2.5.8
// (ADR 0008), which testdata/fill.mjs fills: ten monitors every 20 seconds
// for eleven minutes, in groups named Servers, Network, Apps and Outside.
// Requests went down after three tries again, Books is still trying again
// after 34, VPN went down at 11:07 when its pushes failed, and Recipes was
// down for two minutes, came back, and went down again as the pushes
// stopped. The rest pass.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Page: "homelab", Link: "https://uptime.example.com/"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, "", p)

	by := map[string]snapshot.Check{}
	var names []string
	for _, ch := range p.Checks {
		by[ch.Name] = ch
		names = append(names, ch.Name+" "+ch.Area)
	}
	if got := strings.Join(names, ", "); got != "Hypervisor servers, Backup server servers, Router network, Internet network, VPN network, "+
		"Photos apps, Recipes apps, Requests apps, Books apps, Website " {
		t.Errorf("each monitor in its group's area, Outside in none: %s", got)
	}

	if h := by["Hypervisor"]; len(h.Results) != snapshot.Recent || h.Run != 0 || h.Problem || h.Threshold != 0 || h.Results[0].MS == nil ||
		h.URL != "https://uptime.example.com/dashboard/1" || h.Key != "1" {
		t.Errorf("Hypervisor, passing, its last 30 results, never down, so with no threshold Uptime Kuma says: %+v", h)
	}
	at := func(s string) time.Time {
		v, _ := time.Parse(time.DateTime, s)
		return v
	}
	if r := by["Requests"]; !r.Problem || r.Run != snapshot.Recent || r.Threshold != 4 || !r.Since.Equal(at("2026-10-29 11:03:08.793")) ||
		len(r.Outages) != 1 || r.Outages[0].To != nil || !r.Outages[0].From.Equal(*r.Since) {
		t.Errorf("Requests, down after three tries again, a problem since its first: %+v", r)
	}
	if b := by["Books"]; b.Problem || b.Run != snapshot.Recent || b.Threshold != 0 || len(b.Outages) != 0 {
		t.Errorf("Books, still trying again, not yet a problem, with no threshold Uptime Kuma says: %+v", b)
	}
	if v := by["VPN"]; !v.Problem || v.Run != 25 || v.Threshold != 1 || len(v.Outages) != 1 || v.Outages[0].To != nil {
		t.Errorf("VPN, down since its pushes failed: %+v", v)
	}
	if r := by["Recipes"]; !r.Problem || r.Run != 1 || len(r.Outages) != 2 || r.Outages[0].To == nil || r.Outages[1].To != nil {
		t.Errorf("Recipes, down for two minutes, then again: %+v", r)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://uptime-kuma:3001", Page: "homelab"}
	if err := c.Check(); err != nil || c.Name != "Uptime Kuma" || c.Link != c.URL {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Page: "homelab"}, "url is missing"},
		{Config{URL: "uptime-kuma:3001", Page: "homelab"}, "url needs"},
		{Config{URL: "http://uptime-kuma:3001"}, "page is missing"},
		{Config{URL: "http://uptime-kuma:3001", Page: "homelab", Areas: map[string]string{"DNS": "attic"}}, "attic"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
