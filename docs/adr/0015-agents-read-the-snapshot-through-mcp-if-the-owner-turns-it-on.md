---
status: proposed
---

# Agents read the snapshot through MCP, if the owner turns it on

With `mcp: true` in the config, Overlook answers the Model Context Protocol at `/mcp`, at the page's own address and, like the page, with no login. It offers six tools, one for each part of the page: what's wrong, an area, today, this week, the chores, and the apps and tools. Each returns that part of the snapshot as JSON, with each message key's English words beside it and its values next to them (ADR 0006), and reads nothing the page doesn't: the same snapshot, at most 30 seconds old (ADR 0004), never a key or a source's own answer (ADR 0002). It refuses a request whose `Origin` isn't its own, so that a website its owner visits can't reach it through their browser. It is written with Go's standard library: Overlook answers only starting up, listing the tools and calling one, each request on its own, with nothing streamed. This amends ADR 0001: the page is no longer the only way to read what Overlook reads.

Proposed on 29 October 2026, for the next road, which became the road to v0.5 when v0.3.0 and v0.4.0 shipped the UniFi and Emby adapters on their own, so that an owner can ask an agent what is wrong in their homelab. It is decided when that road is written.

## Considered options

- **No MCP, with `/snapshot` documented as the way in.** Rejected: the snapshot is shaped for the page, holds message keys without their words, and comes whole, while an agent needs the words and a way to ask for one part.
- **`overlook mcp`, a process the agent starts on its own machine.** Rejected: the config and every key would have to be on that machine too.
- **The official Go SDK for MCP.** Rejected for now: a second dependency (ADR 0003) for the small part of the protocol Overlook answers. The day Overlook needs more than tools, such as resources or a login, the SDK earns its place.
- **English sentences built on the server.** Rejected: a second copy, in Go, of the page's words and how it fills them in, to keep in step with the first.
- **On by default.** Rejected: it shows no more than `/snapshot` already does, but handing the homelab's state to a model is the owner's choice to make.

## Consequences

- An agent learns what the page shows, and the model behind it sees the homelab's hosts, names and problems. The README says so where it says how to turn MCP on.
- The tools are named in CONTEXT.md's words, and change only when the page does.
- A version of MCP that this small part can't answer is a reason to look at the SDK again.
