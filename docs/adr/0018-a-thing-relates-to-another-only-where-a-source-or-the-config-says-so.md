---
status: proposed
---

# A thing relates to another only where a source or the config says so

A host, a guest, a store, a check, an app, a tool or a source, each a thing (CONTEXT.md), opens on its own in a close-up over the page it was opened from, at an address of its own: a hash, its kind first, then its source's name and the source's own identifier, such as `/servers#guest/Proxmox/105`. The close-up shows, from the snapshot alone, what its thing is and how it is, its charts, its events, the things it relates to and the way into its source. A thing relates to another only where a source says so, as Proxmox VE says which node runs each guest, or where the config does, in a `things:` list that names a thing by its address and says which check watches it, with `check:` written as an app's, or which other thing is the same machine, with `same:`. Nothing is matched by name, by address or by any other guess. A host, a guest or a store has the state of the check that watches it, by the rule an app's state follows, and what its source says of it, such as stopped, is never its state. The close-up sets the evidence side by side, a problem's beside whatever began within ten minutes of its start, and never names a cause. This amends ADR 0001: the page now opens something smaller than an area, and the config states facts about things inside a source, though still never where anything goes.

Proposed on 9 November 2026, for the road to v0.8, from a brief that asked for Overlook to become an operations workspace: entities and their relationships, an event history, incidents with a likely cause, and actions that change the homelab. Gone through question by question with the maintainer, everything that would make Overlook change something (ADR 0001), remember something (ADR 0004) or hold an opinion of its own was left out, and what remained is what the page can show from the snapshot of one thing and what it is tied to. It is decided when it is built, as step 1 of that road.

## Considered options

- **Matching by name**, Beszel's `pve1` joined to Proxmox VE's `pve1` without a word from the owner, and marked as inferred. The runner-up: it costs the owner nothing for the pair most homelabs with both have. Rejected: names repeat in a homelab, where a Beszel host called `docker` and a Proxmox VE container of the same name can be different machines, and one wrong join draws one machine's processor on another, which is a fact invented.
- **Matching by address**, a check on `immich.example.com` joined to the app linked there. Rejected: the same guess, made through a reverse proxy that serves many apps.
- **One machine read by two sources merged into one thing**, with one row and one line. Rejected: when Proxmox VE says online and Beszel says down, merging picks a winner, and that is a judgement. Either close-up shows both readings side by side. ADR 0016's store left out is narrower: there, one source repeats another's data.
- **A host's or a guest's state from its source**, stopped as down. Rejected: a guest stopped on purpose isn't down, and only a check makes a problem (ADR 0001).
- **Problems that began together grouped under a word of their own.** Rejected: _incident_ is Checkmate's word (CONTEXT.md, Stretch), and choosing which problems belong together is a judgement too. A problem's close-up shows what began near it instead.
- **A suspected cause, labelled as one.** Rejected: it would be Overlook's first opinion, wrong just when it matters, and no test can hold a heuristic to a rule.
- **Actions from the close-up**, such as restarting a guest. Rejected: they need the GET-only guard dropped, keys that write, a login and an audit trail (ADR 0001, ADR 0004, the README's No login). The close-up links to the thing in its source, where it is done, as a chore is.
- **A thing depending on another, and a backup server's backup as a guest's**, such as an app on DNS, or `vm/101` as Proxmox VE's guest 101. Not chosen for now: the first needs the owner to write every one, while runs on and watches already say what a host's outage reaches; the second isn't deterministic, since VMIDs repeat across clusters and a backup server's namespaces.
- **The joins written on each source's entry, or on the check engine's.** Rejected: two places to look, where one list names each thing by the address its close-up has, which the owner copies from the page.
- **A query or a path for the address**, `?open=` or `/thing/` served by the server. Rejected: a hash needs no route, and the page underneath stays the one it was opened over.

## Consequences

- A source's name becomes unique, since every address under it starts with it. The config refuses two sources of one name and says to give one a `name:`. Two unnamed Pi-holes, or two unnamed Proxmox VE clusters, which start today, then stop with that message, and v0.8's notes say so.
- An address is a public contract: an owner bookmarks it, shares it and gets it from MCP. Renaming a source in the config breaks every address under it, and the README says so.
- A thing `things:` names that no source gives is a warning in the log, as a link's missing check is now.
- A Docker source is one host, the one it runs on, named as the source is. Its containers run on it, as the source says, and `same:` ties it to the Proxmox VE guest or the Beszel host it is.
- ⌘K reaches every thing. Enter opens its close-up, ⌘ or Ctrl with Enter goes to it in its source, and with nothing typed ⌘K lists the problems first. It only goes places, and keeps nothing in the browser.
- An event carries the thing it is about, so that an area page lists its area's whole week and a close-up its thing's. The overview keeps its newest eight, and the week gets no page of its own.
- MCP gains a seventh tool, `close_up`, which takes the same address and gives what the close-up shows, and the other six give each thing's address, as ADR 0015 has the tools change when the page does.
- A close-up links into its source through its adapter's `link:`, as a check engine's does, since the address Overlook reads a source at often doesn't open in a browser.
- The ten minutes are a fixed value, not a setting.
- A device, such as one of UniFi's access points or NUT's UPS, isn't a thing: it stays a row until someone wants a close-up of one.
