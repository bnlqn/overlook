package main

// The contract test: each adapter turns recorded answers from its source
// (testdata/<adapter>/, by the path that was read) into Overlook's shapes,
// and validate says whether the page can draw them.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"
)

// replay answers each GET with the recorded answer at dir/<path>.json.
func replay(t *testing.T, dir string) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("%s %s: a source is only read", r.Method, r.URL.Path)
		}
		http.ServeFile(w, r, filepath.Join(dir, filepath.FromSlash(r.URL.Path))+".json")
	}))
	t.Cleanup(s.Close)
	return s
}

var placeholder = regexp.MustCompile(`\{(\w+)(?::\w+)?\}`)

// validate lists what in p the page couldn't draw.
func validate(p Parts) []string {
	var table map[string]any
	raw, _ := os.ReadFile("web/i18n/en.json")
	if err := json.Unmarshal(raw, &table); err != nil {
		return []string{"web/i18n/en.json: " + err.Error()}
	}
	var bad []string
	fail := func(format string, a ...any) { bad = append(bad, fmt.Sprintf(format, a...)) }
	text := func(where string, x Text) {
		if x.Key == "" {
			if x.Name == "" {
				fail("%s: neither a key nor a name", where)
			}
			return
		}
		entry, ok := table[x.Key]
		if !ok {
			fail("%s: %s isn't in en.json", where, x.Key)
			return
		}
		forms := []string{}
		switch e := entry.(type) {
		case string:
			forms = append(forms, e)
		case map[string]any:
			for _, f := range e {
				forms = append(forms, fmt.Sprint(f))
			}
		}
		for _, f := range forms {
			for _, m := range placeholder.FindAllStringSubmatch(f, -1) {
				if _, ok := x.Args[m[1]]; !ok {
					fail("%s: %s needs {%s}", where, x.Key, m[1])
				}
			}
		}
	}
	area := func(where, a string) {
		if !slices.Contains(Areas, a) {
			fail("%s: area %q isn't one of %v", where, a, Areas)
		}
	}
	tone := func(where, t string) {
		if !slices.Contains([]string{"", "green", "orange", "red", "blue", "gray"}, t) {
			fail("%s: tone %q", where, t)
		}
	}
	for _, c := range p.Checks {
		w := "check " + c.Key
		if c.Key == "" || c.Name == "" || c.Source == "" || c.URL == "" {
			fail("%s: needs a key, a name, a source and a url", w)
		}
		if c.Area != "" {
			area(w, c.Area)
		}
		if c.Threshold < 1 || c.Run > len(c.Results) || c.Problem != (c.Run >= c.Threshold) {
			fail("%s: run %d of %d results, threshold %d, problem %v", w, c.Run, len(c.Results), c.Threshold, c.Problem)
		}
		if !slices.IsSortedFunc(c.Results, func(a, b Result) int { return a.At.Compare(b.At) }) {
			fail("%s: results aren't oldest first", w)
		}
		if c.Since != nil && c.Run == 0 {
			fail("%s: failing since, but passing", w)
		}
		for _, o := range c.Outages {
			if o.To != nil && !o.To.After(o.From) {
				fail("%s: an outage ends before it starts", w)
			}
		}
	}
	for _, s := range p.Sections {
		w := "section " + s.Title.Key
		area(w, s.Area)
		text(w, s.Title)
		for _, r := range s.Rows {
			text(w, r.Title)
			for _, x := range []*Text{r.Value, r.Detail} {
				if x != nil {
					text(w, *x)
				}
			}
			tone(w, r.Tone)
			if r.Meter != nil && r.Meter.Total <= 0 {
				fail("%s: a meter with nothing to fill", w)
			}
		}
	}
	for _, f := range p.Figures {
		w := "figure " + f.Label.Key
		area(w, f.Area)
		text(w, f.Label)
		for _, x := range []*Text{f.Unit, f.Detail} {
			if x != nil {
				text(w, *x)
			}
		}
		tone(w, f.Tone)
		if len(f.Hours) != 0 && len(f.Hours) != 24 {
			fail("%s: %d hours, not 24", w, len(f.Hours))
		}
	}
	for _, e := range p.Events {
		w := "event " + e.Text.Key
		area(w, e.Area)
		text(w, e.Text)
		tone(w, e.Tone)
		if e.At.IsZero() {
			fail("%s: no time", w)
		}
	}
	for _, u := range p.Updates {
		if u.Name == "" || u.URL == "" {
			fail("update: needs a name and a url")
		}
	}
	return bad
}

func TestValidateCatches(t *testing.T) {
	p := Parts{Figures: []Figure{{Area: "attic", Label: Text{Key: "nope.missing"}, Hours: []float64{1, 2}}}}
	if got := validate(p); len(got) != 3 {
		t.Errorf("want the area, the key and the hours refused, got %q", got)
	}
}

func TestGatus(t *testing.T) {
	src := replay(t, "testdata/gatus")
	g := GatusConfig{Name: "Gatus", URL: src.URL, Link: "https://status.example.com/", Threshold: 3, Areas: map[string]string{"Media": "apps"}}
	p, err := readGatus(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range validate(p) {
		t.Error(b)
	}
	checks := map[string]Check{}
	for _, c := range p.Checks {
		checks[c.Key] = c
	}
	at := func(s string) time.Time { v, _ := time.Parse(time.RFC3339, s); return v }

	dns := checks["network_dns-secondary"]
	if !dns.Problem || dns.Run != 4 || dns.Why != "read udp: i/o timeout" || !dns.Since.Equal(at("2026-10-28T09:56:00Z")) {
		t.Errorf("DNS secondary should be a problem since 09:56: %+v", dns)
	}
	if len(dns.Outages) != 1 || dns.Outages[0].To != nil {
		t.Errorf("DNS secondary's problem should still be going: %+v", dns.Outages)
	}
	if dns.URL != "https://status.example.com/endpoints/network_dns-secondary" {
		t.Errorf("a check links to the check engine's own page: %s", dns.URL)
	}
	films := checks["media_films"]
	if films.Problem || films.Run != 1 || films.Area != "apps" || films.Why != "[STATUS] == 200" {
		t.Errorf("Films should be failing 1 of 3, in apps, by its condition: %+v", films)
	}
	if nas := checks["storage_nas"]; len(nas.Outages) != 1 || nas.Outages[0].To == nil || !nas.Watched.Equal(at("2026-10-20T08:00:00Z")) {
		t.Errorf("NAS was down for 20 minutes yesterday: %+v", nas.Outages)
	}
	// A minute's blip, and 20 minutes of a check run every 10, aren't problems.
	for _, key := range []string{"network_router", "storage_offsite-copy"} {
		if c := checks[key]; len(c.Outages) != 0 {
			t.Errorf("%s had no problem: %+v", key, c.Outages)
		}
	}
	if c := checks["lab_test-page"]; c.Area != "" {
		t.Errorf("a group in no area stays in none: %q", c.Area)
	}
	if c := checks["servers_hypervisor"]; c.Area != "servers" || len(c.Results) != recent || c.Results[0].MS == nil {
		t.Errorf("Hypervisor: %+v", c)
	}
}
