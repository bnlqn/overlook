package snapshot

import (
	"slices"
	"testing"
	"time"
)

// check is a check named name in area, of the check engine Gatus, whose last
// result passed, or failed run times in a row, a problem or not.
func check(name, area string, run int, problem bool) Check {
	return Check{Name: name, Area: area, Source: "Gatus", Run: run, Problem: problem, Results: []Result{{OK: run == 0, At: time.Unix(0, 0)}}}
}

var gatus = SourceRead{Name: "Gatus", Adapter: "gatus", Kind: "checkengine", Answered: true}

func TestCheckState(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		why   string
		check Check
		want  string
	}{
		{"its last result passed", check("a", "apps", 0, false), "up"},
		{"it fails short of a problem", check("a", "apps", 2, false), "failing"},
		{"it is a problem", check("a", "apps", 3, true), "down"},
		{"it has no result yet", Check{Name: "a", Area: "apps"}, "unknown"},
	} {
		if got := Decide(Snapshot{Parts: Parts{Checks: []Check{tc.check}}}).Checks[0].State; got != tc.want {
			t.Errorf("%s: %s, want %s", tc.why, got, tc.want)
		}
	}
}

func TestAreaState(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		why    string
		checks []Check
		want   string
	}{
		{"it has no checks: nothing says whether it works", nil, "unknown"},
		{"it has only checks of other areas", []Check{check("a", "apps", 3, true), check("b", "", 3, true)}, "unknown"},
		{"no check is a problem", []Check{check("a", "network", 0, false), check("b", "network", 1, false)}, "up"},
		{"one check of two is a problem", []Check{check("a", "network", 3, true), check("b", "network", 0, false)}, "degraded"},
		{"every check is a problem", []Check{check("a", "network", 3, true), check("b", "network", 4, true)}, "down"},
	} {
		got := Decide(Snapshot{Sources: []SourceRead{gatus}, Parts: Parts{Checks: tc.checks}}).Areas
		if len(got) != len(Areas) || got[1].Area != "network" || got[1].State != tc.want {
			t.Errorf("%s: %+v, want network %s", tc.why, got, tc.want)
		}
	}
}

// An area names its adapters' sources, those of every area, and in Storage
// those that give stores or backup runs, answered or not.
func TestAreaSources(t *testing.T) {
	t.Parallel()
	snap := Decide(Snapshot{Sources: []SourceRead{
		gatus,
		{Name: "AdGuard", Adapter: "adguard", Area: "network", Kind: "resolver"},
		{Name: "PVE", Adapter: "proxmox", Area: "servers", Storage: true, Answered: true},
		{Name: "TrueNAS", Adapter: "truenas", Area: "storage", Answered: true},
	}})
	want := map[string][]string{
		"servers": {"Gatus", "PVE"},
		"network": {"Gatus", "AdGuard"},
		"storage": {"Gatus", "PVE", "TrueNAS"},
		"apps":    {"Gatus"},
	}
	for _, a := range snap.Areas {
		if !slices.Equal(a.Sources, want[a.Area]) {
			t.Errorf("%s names %v, want %v", a.Area, a.Sources, want[a.Area])
		}
	}
	if empty := Decide(Snapshot{}).Areas[0].Sources; empty == nil {
		t.Error("an area with no sources names an empty list, never null")
	}
}

func TestHomelabState(t *testing.T) {
	t.Parallel()
	silent := SourceRead{Name: "Gatus", Adapter: "gatus", Kind: "checkengine", Error: "timeout"}
	resolver := SourceRead{Name: "AdGuard", Adapter: "adguard", Area: "network", Kind: "resolver"}
	for _, tc := range []struct {
		why     string
		sources []SourceRead
		checks  []Check
		want    string
	}{
		{"it has check engines and none answered", []SourceRead{silent}, nil, "down"},
		{"one check engine of two answered", []SourceRead{silent, gatus}, []Check{check("a", "apps", 0, false)}, "up"},
		{"it has no checks", []SourceRead{gatus}, nil, "unknown"},
		{"it has no check engine, and a resolver didn't answer", []SourceRead{resolver}, nil, "unknown"},
		{"no check is a problem", []SourceRead{gatus}, []Check{check("a", "apps", 0, false), check("b", "", 2, false)}, "up"},
		{"an area is degraded", []SourceRead{gatus}, []Check{check("a", "apps", 3, true), check("b", "apps", 0, false)}, "degraded"},
		{"a check in no area is a problem", []SourceRead{gatus}, []Check{check("a", "apps", 0, false), check("b", "", 3, true)}, "degraded"},
		{"an area is down", []SourceRead{gatus}, []Check{check("a", "apps", 3, true), check("b", "network", 0, false), check("c", "network", 3, true)}, "down"},
	} {
		if got := Decide(Snapshot{Sources: tc.sources, Parts: Parts{Checks: tc.checks}}).Homelab.State; got != tc.want {
			t.Errorf("%s: %s, want %s", tc.why, got, tc.want)
		}
	}
}

func TestLinkState(t *testing.T) {
	t.Parallel()
	in := func(engine string, c Check) Check { c.Source = engine; return c }
	checks := []Check{
		in("Gatus", check("Photos", "apps", 0, false)),
		in("Gatus Outside", check("Photos", "apps", 3, true)),
		in("Gatus", check("Films", "apps", 1, false)),
		in("Gatus Outside", check("Films", "apps", 2, false)),
	}
	for _, tc := range []struct {
		why                string
		link               Link
		state, wantsEngine string
	}{
		{"it names no check", Link{Name: "Home"}, "unknown", ""},
		{"no check engine has a check of its name", Link{Name: "Wiki", Check: "Wiki"}, "unknown", ""},
		{"two check engines share its check's name", Link{Name: "Photos", Check: "Photos"}, "down", "Gatus Outside"},
		{"they tie, so the first", Link{Name: "Films", Check: "Films"}, "failing", "Gatus"},
		{"the config names the check engine", Link{Name: "Photos", Check: "Photos", Engine: "Gatus"}, "up", "Gatus"},
		{"the check engine it names hasn't the check", Link{Name: "Wiki", Check: "Wiki", Engine: "Gatus"}, "unknown", "Gatus"},
	} {
		got := Decide(Snapshot{Parts: Parts{Checks: checks}, Apps: []Link{tc.link}, Tools: []Link{tc.link}})
		for _, l := range []Link{got.Apps[0], got.Tools[0]} {
			if l.State != tc.state || l.Engine != tc.wantsEngine {
				t.Errorf("%s: %s in %q, want %s in %q", tc.why, l.State, l.Engine, tc.state, tc.wantsEngine)
			}
		}
	}
}

// Deciding leaves the snapshot it's given as it was, so the demo's test can
// set what it reads beside what Decide makes of it.
func TestDecideLeavesSnapshot(t *testing.T) {
	t.Parallel()
	snap := Snapshot{Parts: Parts{Checks: []Check{check("a", "apps", 3, true)}}, Apps: []Link{{Name: "A", Check: "a"}}}
	Decide(snap)
	if snap.Checks[0].State != "" || snap.Apps[0].State != "" || snap.Apps[0].Engine != "" || snap.Areas != nil {
		t.Errorf("Decide changed what it was given: %+v", snap)
	}
}
