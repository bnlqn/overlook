---
status: accepted
---

# The page is native modules, tested with Node's own runner and linted with Biome

The page stays plain JavaScript with no build (ADR 0003), but as native ES modules: the browser loads `web/js/*.js` as they are, and the parts that don't touch the DOM (the words, the formats, the areas' states, the HTML each shape becomes) are tested with Node's built-in test runner. Biome lints and formats the page. Node and Biome are two standalone tools a contributor runs, never dependencies: there is still no `package.json`, no `node_modules` and no npm in the repository. The modules also keep every script out of the HTML, so the page is served with a Content-Security-Policy that allows no inline script.

Decided on 28 October 2026, when the page came over from the author's homelab, untested and in one classic script.

## Considered options

- **Classic scripts, untested.** The page as it was. Rejected: the page escapes everything a source sends, writes every word through the table and decides each area's state, all with no test, and it needs inline scripts.
- **Vitest, ESLint and Prettier.** The usual tools. Rejected: each comes from npm, which ADR 0003 keeps out of the repository.
- **Deno**, which tests, lints and formats in one binary. Rejected for now: far fewer contributors have it than have Node. Moving to it later changes the tools, not the page.
- **A browser test, such as Playwright.** Rejected: it brings npm, or a headless browser driven from Go as a dependency. The page's logic is tested in Node; the page as a whole is still looked at in a browser by hand.

## Consequences

- The checks are `make check`: `go vet`, golangci-lint, Biome, `go test -race`, `node --test` and govulncheck. CONTRIBUTING.md names each tool's version.
- A module that touches the DOM at its top level can't be tested in Node. Only `shell.js`, `overview.js` and `area.js` may, and they hold no logic worth a test.
