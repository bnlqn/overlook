---
status: proposed
---

# An adapter may read a source that also changes things on GET, through a fixed list of reads

Tautulli's API takes every command as a GET with its one key in the address, reads such as `get_activity` and changes such as `delete_all_history`, `restart` and `update` alike. For such a source, the GET-only client of ADR 0001 doesn't keep the key harmless, so its adapter names, in one list in its code, the only commands it sends, each of them a read, refuses any other before it reaches `internal/fetch`, and its test checks every command it sends against that list. This amends ADR 0001 for these sources alone: "changes nothing" is still enforced by the client for every other source, and for these it rests on that list, which a pull request that adds to it has to show reads.

Proposed on 29 October 2026, with the Tautulli adapter, which step 5 of the road to v0.2 names as how Overlook reads Plex.

## Considered options

- **Plex's own API, with its token**, whose writes are PUT, POST and DELETE, so the GET-only client would keep it harmless. Not chosen for now: the token is the owner's whole Plex account, its other servers and its friends included, where Tautulli's key reaches only Tautulli, and Plex keeps no history of plays a page could chart.
- **Leaving Tautulli out**, under Can't be read, as Backrest and Deluge are. Rejected if this is accepted: Plex is the media server most homelabs run, and an owner who runs it sees nothing of it.
- **A guard in `internal/fetch` for Tautulli's commands.** Rejected: the client would learn one source's API, and a second source like it would add another.

## Consequences

- The README's Tautulli section says that its key can change Tautulli, even through GET, and that what keeps it harmless is the adapter's list.
- Tautulli takes its key only in the address, so a proxy in front of it may log the key; the README says to let Overlook reach Tautulli directly.
- A source like it gets an adapter only with a list of the same kind, and the same test.
