---
status: proposed
---

# A state is decided in Go and carried in the snapshot

Each check's state, each area's and the homelab's, the sources each area names, and the check that says whether each app or tool answers, and so its state, are decided once, in `internal/snapshot`. The Reader decides them as the last step of every read, and the snapshot carries them: a check is up, failing, down or unknown, and an area and the homelab are up, degraded, down or unknown (CONTEXT.md, State). The page words them and MCP gives them, and neither decides them again. None of these rules needs the moment the page is viewed or its language: only their words do, such as how long the homelab has been quiet, so the words stay in the page (ADR 0006). This follows ADR 0003, the snapshot shaped in Go and drawn in JavaScript, and amends ADR 0007: the page's other decisions stay in `js/page.js`, tested with Node, but the states are tested in Go.

Proposed on 8 November 2026, from an architecture review of that day, for the road to v0.7, as its seventh step, after the overview's and the area page's decisions moved into `page()` the same day. Go already decided whether a check is a problem, and `page.js` decided the states made from it. MCP, built as step 5 of the road to v0.6 (ADR 0015), then copied two of those rules, the sources an area names and the check a link names, and said the area's in its tools' descriptions, and it could not give the homelab's state, the one the page leads with, without copying a third. CONTEXT.md's rule for Degraded had changed twice by then, each time in `page.js` alone.

## Considered options

- **The states left in `page.js`**, with MCP describing them, until the rule for Degraded next changes. The runner-up: nothing differed between the page and MCP, and the page's every decision stayed in one module. Rejected: the rules were already split between Go and JavaScript, Go held copies of two of them already, and waiting for the page and MCP to differ is waiting for a bug no test catches.
- **The states beside the snapshot**, from a function that `/snapshot` and MCP each call, with the snapshot left as the facts read. Rejected: two callers to keep in step, when the snapshot is already what both read.
- **Quiet's start, and the adding up of figures and charts, decided in Go too.** Rejected for now: Quiet is a moment the page's headline words, and an agent has the outages from `this_week`; the adding up is shaped for the page's tiles, and an agent loses nothing by reading each source's figures. Either can move the day an agent needs it.
- **A reason beside the homelab's state**, such as why it is down. Rejected: down with no checks can only mean that no check engine answered, since only a check engine gives checks, and a test in Go holds to that.
- **The demo deciding its homelabs' states as it writes them**, with Node's tests reading what it wrote. Rejected: `node --test` would have to wait for `go run ./demo`.

## Consequences

- The snapshot gains a state for each check, each app and each tool; the four areas, each with its state and the names of the sources it names; and the homelab's state. Each app's and tool's `engine` names the check engine whose check decided it: the config's, or else the worst one's.
- `page.js` reads the states rather than working them out: `checkState`, the area's and the homelab's state, which sources an area names and `linkCheck` go. It still says what each means, and when the homelab is down with no checks, it says that no check engine answered.
- MCP's `problems` gives the homelab's state and each area's, `area` its area's state and sources as the snapshot has them, and `apps_and_tools` each link's state and its one check. The tools' descriptions lose the rules, and the instructions say once what each state means. The tools give what the page already shows, so they still change only when the page does (ADR 0015).
- The log's warning about a link whose check two check engines share uses the same choice as the page.
- The states' tests move from `web/test/page.test.js` to Go, as a table. The page's tests give `page()` snapshots with their states, and test what it says and draws. The demo's three homelabs carry their states in their JSON, which those tests read as it is, and the demo's test fails when a state there isn't the one Go decides.
- CONTRIBUTING's rule becomes: a state, and which checks and sources make it, is decided in `internal/snapshot`, with a Go test; every other decision the page makes goes in `js/page.js`, with a test. A contributor now tells two homes apart, and that sentence says which.
- A change to how a state is decided is made in one Go function and its test, and in CONTEXT.md, never in the page and MCP both.
- `snapshot.SourceState`, which says whether a source answered, is renamed `SourceRead`, since a state is how a check, an app, a tool, an area or the homelab is.
- Once accepted, ADR 0007's status says that this one amends it.
