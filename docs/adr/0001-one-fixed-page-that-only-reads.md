---
status: accepted
---

# Overlook draws one fixed page and only reads

Overlook answers the same questions for every homelab, in the same shape: what is wrong, how each of four fixed areas is (Servers, Network, Storage, Apps), and what the homelab did today and this week. Configuration says which sources fill that shape, never where anything goes on the page. Overlook changes nothing: problems come from the check engine, which also sends the alerts, so the page and the alerts can't disagree.

Decided on 28 October 2026, when the page was first opened up for other homelabs.

## Considered options

- **A page assembled from widgets**, as Homepage, Homarr, Glance and its fork Dynacat do. Rejected: that space is well served, and assembling the page throws away the opinion that makes this one worth having.
- **Areas the user names.** Rejected: every adapter would then need placing by hand, which is a widget grid again.
- **Overlook running its own checks.** Rejected: it would need alerts of its own or disagree with them, and would become a monitoring tool.

## Consequences

- "Can I move this?" and "Can I add a widget?" are answered no. A new kind of source is a new adapter.
- An adapter gives back facts in Overlook's shapes, never markup of its own.
- "Changes nothing" is enforced, not promised: the HTTP client adapters get sends only GET, and NUT only its read commands. Some keys Overlook holds can do anything (AdGuard Home has no read-only users, Radarr's and Sonarr's keys are full admin keys), so this guard is what keeps them harmless. An adapter that needs another method just to read is a design discussion.
