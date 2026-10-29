package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/adaptertest"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// source answers as Prometheus does, and as Alertmanager does under
// /alertmanager, from testdata: a range query by whether it reads the week
// or the last evaluations, by its step.
func source(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("%s %s: a source is only read", r.Method, r.URL.Path)
		}
		name := r.URL.Path
		if name == "/api/v1/query_range" {
			name += "/recent"
			if r.URL.Query().Get("step") == "300" {
				name = "/api/v1/query_range/week"
			}
		}
		http.ServeFile(w, r, filepath.Join("testdata", name+".json"))
	}))
	t.Cleanup(s.Close)
	return s
}

// testdata holds answers recorded from a scratch Prometheus 3.15 and
// Alertmanager 0.34 (ADR 0008): six alert rules, evaluated every 10
// seconds, in groups named Servers, Network, Storage and Apps. Disk space
// fires for a minute every other minute, and had just stopped; Packet loss
// is pending for an hour; Backup age fires, but the owner silenced it in
// Alertmanager. record.sh read them at 12:59:17.
func TestRead(t *testing.T) {
	t.Parallel()
	src := source(t)
	c := Config{URL: src.URL, Alertmanager: src.URL + "/alertmanager", Link: "https://prometheus.example.com/"}
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
	p, err := Read(context.Background(), c, time.Unix(1793278757, 0))
	if err != nil {
		t.Fatal(err)
	}
	adaptertest.Validate(t, p)

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

	if l := by["Packet loss"]; l.Problem || l.Run != snapshot.Recent || l.Threshold != 361 {
		t.Errorf("Packet loss, pending for an hour of 10-second evaluations: %+v", l)
	}
	if b := by["Backup age"]; b.Problem || b.Run != snapshot.Recent || b.Threshold != snapshot.Recent+1 || b.Why != "The offsite disk is being swapped" {
		t.Errorf("Backup age, firing but silenced, with the silence's comment: %+v", b)
	}
	if h := by["Host memory"]; h.Problem || h.Run != 0 || len(h.Outages) != 0 {
		t.Errorf("Host memory, never firing: %+v", h)
	}

	// Without Alertmanager, a silence isn't known, and Backup age is a
	// problem as Prometheus sees it.
	c.Alertmanager = ""
	if p, err = Read(context.Background(), c, time.Unix(1793278757, 0)); err != nil {
		t.Fatal(err)
	}
	for _, ch := range p.Checks {
		if ch.Name == "Backup age" && !ch.Problem {
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
