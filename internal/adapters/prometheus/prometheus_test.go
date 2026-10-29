package prometheus

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// testdata holds answers recorded from a scratch Prometheus 3.15 and
// Alertmanager 0.34 (ADR 0008): six alert rules, evaluated every 10
// seconds, in groups named Servers, Network, Storage and Apps. Disk space
// fires for a minute every other minute, and had just stopped; Packet loss
// is pending for an hour; Backup age fires, but the owner silenced it in
// Alertmanager. record.sh read them at 15:49:15. A range query is recorded
// by its step: the week's, or the last evaluations'. Alertmanager answers
// under /alertmanager.
func TestRead(t *testing.T) {
	t.Parallel()
	src := adaptertest.Replay(t, "testdata")
	c := Config{URL: src.URL, Alertmanager: src.URL + "/alertmanager", Link: "https://prometheus.example.com/", User: "overlook", Password: "s3cret"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Unix(1793288955, 0))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)
	for _, r := range src.Sent() {
		if user, password, _ := (&http.Request{Header: r.Header}).BasicAuth(); user != "overlook" || password != "s3cret" {
			t.Errorf("%s: read without the user and password", r.URL)
		}
	}

	by := map[string]snapshot.Check{}
	for _, ch := range p.Checks {
		by[ch.Name] = ch
	}
	if len(p.Checks) != 6 || by["Disk space"].Area != "servers" || by["Packet loss"].Area != "network" ||
		by["Backup age"].Area != "storage" || by["Photos errors"].Area != "apps" {
		t.Fatalf("six rules, each in its group's area: %+v", p.Checks)
	}

	d := by["Disk space"]
	if d.Problem || d.Run != 0 || d.Threshold != 1 ||
		d.URL != "https://prometheus.example.com/alerts?search=Disk+space" || d.Key != "Servers/Disk space" {
		t.Errorf("Disk space, between its minutes of firing: %+v", d)
	}
	failed := 0
	for _, r := range d.Results {
		if !r.OK {
			failed++
		}
	}
	if len(d.Results) != snapshot.Recent || failed < 12 || failed == snapshot.Recent {
		t.Errorf("30 results, the minutes it fired among them: %d failed of %d", failed, len(d.Results))
	}
	if len(d.Outages) != 1 || d.Outages[0].To == nil {
		t.Errorf("its firing this week, to five minutes, and over: %+v", d.Outages)
	}

	pending, _ := time.Parse(time.RFC3339, "2026-10-29T15:43:59.137705404Z")
	if l := by["Packet loss"]; l.Problem || l.Run != snapshot.Recent || l.Threshold != 361 || !l.Since.Equal(pending) || len(l.Outages) != 0 {
		t.Errorf("Packet loss, pending for an hour of 10-second evaluations, since it went pending: %+v", l)
	}
	if b := by["Backup age"]; b.Problem || b.Run != snapshot.Recent || b.Threshold != 0 || b.Why != "The offsite disk is being swapped" || len(b.Outages) != 0 {
		t.Errorf("Backup age, firing but silenced, with the silence's comment, and no problem this week: %+v", b)
	}
	if h := by["Host memory"]; h.Problem || h.Run != 0 || len(h.Outages) != 0 {
		t.Errorf("Host memory, never firing: %+v", h)
	}

	// Without Alertmanager, a silence isn't known, and Backup age is a
	// problem as Prometheus sees it, still going since it fired.
	c.Alertmanager = ""
	if p, err = Read(context.Background(), c, time.Unix(1793288955, 0)); err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)
	for _, ch := range p.Checks {
		if ch.Name == "Backup age" && (!ch.Problem || ch.Threshold != 1 || len(ch.Outages) != 1 || ch.Outages[0].To != nil) {
			t.Errorf("Backup age without Alertmanager: %+v", ch)
		}
	}
}

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://prometheus:9090"}
	if err := c.Check(); err != nil || c.Name != "Prometheus" || c.Link != c.URL {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{}, "url is missing"},
		{Config{URL: "prometheus:9090"}, "url needs"},
		{Config{URL: "http://prometheus:9090", Alertmanager: "alertmanager"}, "alertmanager needs"},
		{Config{URL: "http://prometheus:9090", User: "overlook"}, "together"},
		{Config{URL: "http://prometheus:9090", Areas: map[string]string{"DNS": "attic"}}, "attic"},
	} {
		if err := bad.c.Check(); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}
