---
status: accepted
---

# One container reads every source and holds the keys

Overlook is one container. Its server reads every source with that source's key and gives the page a snapshot, so no key and no raw answer from a source ever reaches a browser.

Decided on 28 October 2026.

## Considered options

- **Static files behind the owner's own reverse proxy**, which adds each source's key route by route. This is how the page started, in its author's homelab. Rejected: every owner would have to write those routes, and one wrong route leaks a key.
- **A server that only passes the page's reads through and adds the keys**, with the adapters running in the browser. Rejected: the raw answers would still reach the browser, every open tab would read every source again, and the shared shapes adapters give back (ADR 0001) sit better next to the keys.
