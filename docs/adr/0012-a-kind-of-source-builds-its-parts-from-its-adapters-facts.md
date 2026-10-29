---
status: accepted
---

# A kind of source builds its parts from its adapters' facts

A resolver's parts and a media server's are built by one module for each kind, in `internal/snapshot`. The adapter reads its source and fills in the kind's facts: its lookups or plays with their times, what plays now, its upstreams and top lists, and rows of its own. The module gives back the kind's figure, chart and sections, in the kind's words, which are kept in a table of their own. So AdGuard Home's and Pi-hole's lookups always add up, because neither adapter can give them in any other shape, and every resolver looks the same on the page. Inside these modules, and in the hosts' charts, there is one rule for the hour: counts and levels go by the hour where Overlook runs, the last one being this hour so far, and today starts at midnight there. A source that keeps its own hours, as AdGuard Home does, gives them as they are.

Decided on 29 October 2026, after an architecture review found two things: adding up rested on titles and fields that only a comment asked for, and the same code for the hour was pasted into three adapters. The decision came before step 9 of the road to v0.2 opens the repository to other people's adapters.

## Considered options

- **Each adapter building its own parts from shared titles, as before.** Rejected: two figures add up only if both give the same label, 24 hours, a part within and a detail of the same key. A third resolver's adapter that missed one would be added up wrongly, without a word.
- **Functions for the figure and the chart that add up, with the sections left to each adapter.** Rejected: the rows every resolver shows, and their words, would stay copied in every adapter, and a third would copy them again.
- **A package for each kind, as `servarr` is for its three adapters.** Rejected: the rule for the hour, which the hosts' charts share, would still live in `internal/snapshot`.
- **UTC hours everywhere, as the hosts' charts used.** Rejected: in a half-hour zone, the page's hours and today would be half an hour off.

## Consequences

- Whoever writes a resolver's or a media server's adapter keeps that adapter (ADR 0005), but the kind's module is the maintainer's. A row every resolver should show is added there, once, for all of them. A row only one source has goes in that adapter's own rows, after the kind's.
- A fact a source can't give is left out, and so is its row.
- A kind's figure turns orange when any row of its section is red, such as blocking turned off.
- Each source tells the page its kind, so that the page can say when a source of that kind answered but gave none of what it adds up. A source has at most one kind: `checkengine`, `todolist`, `resolver` or `mediaserver`.
- The kinds' words are in `web/i18n/en/resolver.json` and `mediaserver.json`, not in the adapters' tables. A translation brings them as it brings an adapter's.
- A kind gets a module only once two adapters read it, because only then does anything vary behind it. Hosts, read by Proxmox and Beszel, share their charts this way, but not their rows.
- AdGuard Home's own hours are half an hour off in a half-hour zone.
