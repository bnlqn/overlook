package snapshot

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
