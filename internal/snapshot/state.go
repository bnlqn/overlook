package snapshot

import "slices"

// Area is one of the four areas as the snapshot carries it: its state and
// the names of the sources it names, those that answered too.
type Area struct {
	Area  string `json:"area"`
	State string `json:"state"`
	// Sources are its adapters' sources, those of every area, such as a
	// check engine, and in Storage those that give stores or backup runs,
	// whatever their home area (ADR 0016), so that Storage never loses them
	// without a word.
	Sources []string `json:"sources"`
}

// Homelab is the homelab as the snapshot carries it.
type Homelab struct {
	State string `json:"state"`
}

// Decide is snap with the state of each check, app, tool, area and the
// homelab decided from its facts (CONTEXT.md, State), the sources each area
// names, and the check engine whose check decided each app and tool. The
// Reader decides them as the last step of every read, and the page and MCP
// only read them, so that neither decides one again (ADR 0017). snap is
// left as it was.
func Decide(snap Snapshot) Snapshot {
	snap.Checks = slices.Clone(snap.Checks)
	for i, c := range snap.Checks {
		snap.Checks[i].State = checkState(c)
	}
	snap.Apps = linked(snap.Apps, snap.Checks)
	snap.Tools = linked(snap.Tools, snap.Checks)
	snap.Areas = make([]Area, 0, len(Areas))
	for _, area := range Areas {
		snap.Areas = append(snap.Areas, areaOf(area, snap))
	}
	snap.Homelab = Homelab{State: homelabState(snap)}
	return snap
}

// checkState is a check's state (CONTEXT.md, Check): down while it is a
// problem, failing while it fails short of one, up when its last result
// passed, and unknown with no result yet.
func checkState(c Check) string {
	switch {
	case c.Problem:
		return "down"
	case c.Run > 0:
		return "failing"
	case len(c.Results) > 0:
		return "up"
	}
	return "unknown"
}

// worse ranks a check's states, so that a link names the worst of the checks
// of its name.
var worse = map[string]int{"up": 0, "unknown": 1, "failing": 2, "down": 3}

// linked is links, each with the state of the check that says whether it
// answers and the check engine that check is in: the one of its name in the
// check engine the config names, or, with none named, the worst of those of
// its name, the first of them where they tie, so that a check in two check
// engines never hides a problem. With no such check, it's unknown.
func linked(links []Link, checks []Check) []Link {
	out := make([]Link, 0, len(links))
	for _, l := range links {
		var by *Check
		for i, c := range checks {
			if l.Check != "" && c.Name == l.Check && (l.Engine == "" || c.Source == l.Engine) &&
				(by == nil || worse[c.State] > worse[by.State]) {
				by = &checks[i]
			}
		}
		l.State = "unknown"
		if by != nil {
			l.State, l.Engine = by.State, by.Source
		}
		out = append(out, l)
	}
	return out
}

// areaOf is an area's state (CONTEXT.md, Degraded), down when every check in
// it is a problem, degraded with any, up with none, and unknown with no
// checks, since nothing says whether it works; and the sources it names.
func areaOf(area string, snap Snapshot) Area {
	checks, problems := 0, 0
	for _, c := range snap.Checks {
		if c.Area == area {
			checks++
			if c.Problem {
				problems++
			}
		}
	}
	a := Area{Area: area, State: "degraded", Sources: []string{}}
	switch {
	case checks == 0:
		a.State = "unknown"
	case problems == 0:
		a.State = "up"
	case problems == checks:
		a.State = "down"
	}
	for _, s := range snap.Sources {
		if s.Area == "" || s.Area == area || s.Storage && area == "storage" {
			a.Sources = append(a.Sources, s.Name)
		}
	}
	return a
}

// homelabState is the homelab's state (CONTEXT.md, Degraded): down when an
// area is down or it has check engines and none answered, degraded with any
// other problem, one in a check in no area too, unknown with no checks, and
// up otherwise. Only a check engine gives checks, so down with none can only
// mean that no check engine answered.
func homelabState(snap Snapshot) string {
	engines, answered := false, false
	for _, s := range snap.Sources {
		if s.Kind == "checkengine" {
			engines, answered = true, answered || s.Answered
		}
	}
	switch {
	case engines && !answered:
		return "down"
	case len(snap.Checks) == 0:
		return "unknown"
	case !slices.ContainsFunc(snap.Checks, func(c Check) bool { return c.Problem }):
		return "up"
	case slices.ContainsFunc(snap.Areas, func(a Area) bool { return a.State == "down" }):
		return "down"
	}
	return "degraded"
}
