package gatus

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// testdata holds invented answers, shaped like Gatus's: twelve checks around
// 10:00 UTC on 28 October 2026.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Link: "https://status.example.com/", Areas: map[string]string{"Media": "apps"}}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

	checks := map[string]snapshot.Check{}
	for _, ch := range p.Checks {
		checks[ch.Key] = ch
	}
	at := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}

	dns := checks["network_dns-secondary"]
	if !dns.Problem || dns.Run != 4 || dns.Why != "read udp: i/o timeout" || dns.Since == nil || !dns.Since.Equal(at("2026-10-28T09:56:00Z")) {
		t.Errorf("DNS secondary is a problem since 09:56: %+v", dns)
	}
	if len(dns.Outages) != 1 || dns.Outages[0].To != nil {
		t.Errorf("DNS secondary's problem is still going: %+v", dns.Outages)
	}
	if dns.URL != "https://status.example.com/endpoints/network_dns-secondary" {
		t.Errorf("a check links to its page in Gatus: %s", dns.URL)
	}
	if films := checks["media_films"]; films.Problem || films.Run != 1 || films.Area != "apps" || films.Why != "[STATUS] == 200" {
		t.Errorf("Films is failing 1 of 3, in apps by the config, by its condition: %+v", films)
	}
	if nas := checks["storage_nas"]; len(nas.Outages) != 1 || nas.Outages[0].To == nil || nas.Watched == nil || !nas.Watched.Equal(at("2026-10-20T08:00:00Z")) {
		t.Errorf("NAS was down for 20 minutes yesterday: %+v", nas.Outages)
	}
	// A minute's blip, and 20 minutes of a check run every 10, weren't problems.
	for _, key := range []string{"network_router", "storage_offsite-copy"} {
		if ch := checks[key]; len(ch.Outages) != 0 {
			t.Errorf("%s had no problem: %+v", key, ch.Outages)
		}
	}
	if ch := checks["lab_test-page"]; ch.Area != "" {
		t.Errorf("a group in no area stays in none: %q", ch.Area)
	}
	if ch := checks["servers_hypervisor"]; ch.Area != "servers" || len(ch.Results) != snapshot.Recent || ch.Results[0].MS == nil {
		t.Errorf("Hypervisor: %+v", ch)
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "https://status.example.com", User: "overlook", Password: "s3cret"}
	if err := c.Check(); err != nil || c.Name != "Gatus" || c.Threshold != 3 || c.Link != "https://status.example.com" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{}, "url is missing"},
		{Config{URL: "status.example.com"}, "url needs"},
		{Config{URL: "https://overlook:s3cret@status.example.com"}, "url can't hold a login"},
		{Config{URL: "http://gatus", Password: "s3cret"}, "user and password together"},
		{Config{URL: "http://gatus", Link: "https://overlook:s3cret@status.example.com"}, "link can't hold a login"},
		{Config{URL: "http://gatus", Threshold: -1}, "threshold"},
		{Config{URL: "http://gatus", Areas: map[string]string{"DNS": "attic"}}, "attic"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
