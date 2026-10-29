package snapshot

import "math"

// The titles every adapter that reads a DNS resolver gives its parts, so
// that the page adds up every resolver's lookups, whichever kind answers
// them, such as AdGuard Home's and Pi-hole's, and names each one's
// sections by its source beside another's (CONTEXT.md, Figure).
var (
	LookupsFigure   = Text{Key: "figure.lookups"}
	LookupsChart    = Text{Key: "chart.lookups_per_hour"}
	Lookups         = Text{Key: "chart.lookups"}
	Blocked         = Text{Key: "chart.blocked"}
	ResolverSection = Text{Key: "section.resolver"}
	TopClients      = Text{Key: "section.top_clients"}
	TopDomains      = Text{Key: "section.top_domains"}
	TopBlocked      = Text{Key: "section.top_blocked"}
)

// LookupsBlocked is the key of the lookups figure's detail, the blocked
// ones today, as {blocked}.
const LookupsBlocked = "figure.lookups_blocked"

// OrNothing is a resolver's top rows, or one that says there are none yet.
func OrNothing(rows []Row) []Row {
	if len(rows) == 0 {
		return []Row{{Title: Text{Key: "top.nothing"}}}
	}
	return rows
}

// TopShare is a count of lookups and its share of all of them, as a top
// client's or domain's value.
func TopShare(n, lookups int) *Text {
	return Msg("top.share", map[string]any{"n": n, "share": PercentOf(n, lookups)})
}

// PercentOf is part of whole as a percentage, to a tenth.
func PercentOf(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return math.Round(1000*float64(part)/float64(whole)) / 10
}
