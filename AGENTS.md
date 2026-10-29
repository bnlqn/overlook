# Working on Overlook

Overlook is one fixed page that tells a homelab's owner what is wrong, read from the tools the homelab already runs. Three documents bind every change:

- **[CONTEXT.md](CONTEXT.md)**, the language. Name Overlook's concepts with its terms, in code, CSS, comments, docs, commits and pull requests; its _Avoid_ words are the names they never take. A source's own words (Gatus's groups and endpoints) describe that source.
- **[docs/adr/](docs/adr/)**, the decisions. A change that would contradict one needs a new ADR first, agreed with the maintainer.
- **[CONTRIBUTING.md](CONTRIBUTING.md)**, the layout, the checks and how to add an adapter.

## Conventions no config enforces

- **Scope is the road.** [README.md](README.md)'s road, under What's next, says which step is open. Work inside it, and name anything beyond it as a follow-up.
- **Plain words, short sentences, British spelling.** Comments say why, in full sentences, and describe what the page does in its own terms, never by the product it resembles.
- **Invented data only.** Hosts are `*.example.com`, names and numbers are made up, and an answer recorded from a real source has its keys, hosts and people removed before it lands in `testdata/`.
- **Overlook only reads.** Adapters reach a source only through a `fetch.Source`, under its own address (ADR 0001, ADR 0013).
- **Every word goes through the table.** The page's words live in `web/i18n/en/page.json`; an adapter returns `snapshot.Text`, its words in `web/i18n/en/<adapter>.json` (ADR 0006).
- **Dependencies stay at one.** Go's standard library and the YAML parser; Node and Biome are tools, never dependencies (ADR 0003, ADR 0007). A new dependency is the maintainer's call.
- **A deliberate shortcut is marked.** `// ponytail:` names the ceiling it has and the upgrade when it's reached.
- **An ADR records a decision that is hard to reverse, surprising and a real trade-off**, in the format of the ones there: status, title, the decision in a paragraph, when it was decided, the options considered, the consequences.

## Done

A change is done when `make check` passes and, for a change to the page, the page has been looked at in a browser with the demo's three homelabs at desktop and phone width, with screenshots in the pull request.
