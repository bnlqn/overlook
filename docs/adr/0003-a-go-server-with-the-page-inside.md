---
status: accepted
---

# The server is Go, and the page is plain HTML, CSS and JavaScript inside it

The server is written in Go and ships as one static binary with the page embedded in it. The page has no framework and no build: it fetches the snapshot and draws it. Go's standard library covers everything the server does (reading sources over HTTP and NUT's TCP protocol, timeouts, JSON, serving files), so the only dependency is a YAML parser and there is no npm anywhere in the repository. That keeps the supply chain of a service that holds keys small, lets the same binary run in a tiny image, on a Raspberry Pi or straight in an LXC container, and lets code written today still build years later with little upkeep.

Decided on 28 October 2026.

## Considered options

- **TypeScript on Node.** The runner-up: the largest pool of contributors, and the page's first loaders were already JavaScript. Rejected: a runtime in every image, npm's churn and supply chain beside the keys, and no code the page and the server would really share.
- **Rust.** Rejected: the fewest people who could write an adapter, for a footprint Go already gets close to.
- **Bun or Deno.** Rejected: larger binaries and younger runtimes, for little gain over Go.
- **A framework for the page.** Rejected: one fixed page drawn from one snapshot doesn't need one, and it would bring a build and npm back.

## Consequences

- The snapshot is shaped in Go and drawn in JavaScript. The demo's invented snapshot is also the contract test every adapter's output is checked against.
