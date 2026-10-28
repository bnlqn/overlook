# Contributing to Overlook

Thank you for helping. Overlook answers the same questions for every homelab, in one fixed page (ADR 0001), so most contributions are an adapter for a source, a translation, or a fix.

Before you start, read [CONTEXT.md](CONTEXT.md) for the words the project uses and [docs/adr](docs/adr) for what has been decided. For anything larger than a fix, open an issue first, so nobody's work is wasted.

## Setting up

You need:

| Tool | Version | For |
| --- | --- | --- |
| [Go](https://go.dev/dl/) | 1.25 or later | the server |
| [Node.js](https://nodejs.org/) | 22.12 or later | the page's tests, with its own runner (no npm) |
| [Biome](https://biomejs.dev/guides/manual-installation/) | 2.6.3 | linting and formatting the page |
| [golangci-lint](https://golangci-lint.run/welcome/install/) | 2.15.0 | linting and formatting the Go code |

Then:

```bash
make check
```

That runs every check a pull request passes, in the order CI will: `go vet`, golangci-lint, Biome, the Go tests with the race detector, the page's tests, and govulncheck. `make fmt` formats everything the way the linters expect.

To see the page, point the example config at a Gatus you run:

```bash
GATUS_URL=http://gatus:8080 go run ./cmd/overlook -config overlook.example.yaml
```

and open <http://localhost:8080>.

## Where things are

| Path | What it holds |
| --- | --- |
| `cmd/overlook` | the binary: flags, the server's start and its graceful stop |
| `internal/config` | `overlook.yaml`: strict loading, `${VAR}`s, every adapter's config |
| `internal/fetch` | the only way to read a source: GET requests, nothing else (ADR 0001) |
| `internal/snapshot` | the shapes every adapter gives back, and the reader that builds a snapshot on demand (ADR 0004) |
| `internal/adapters/<name>` | one adapter each, with its recorded answers in `testdata/` |
| `internal/adapters/adaptertest` | the contract test every adapter passes |
| `internal/server` | the routes, and the headers every answer carries |
| `web` | the page, embedded in the binary: HTML, `style.css`, the modules in `js/`, their tests in `test/`, the words in `i18n/` |

## Adding an adapter

An adapter reads one kind of source and gives back its facts in Overlook's shapes. It comes in by pull request from someone who runs that source, who then keeps it (ADR 0005). One adapter per pull request, with:

1. **Its package**, `internal/adapters/<name>`, with a `Config` (whose `Check` fills its defaults and refuses what can't work) and a `Read(ctx, Config) (snapshot.Parts, error)` that reads only through `internal/fetch`. Wire it into `internal/config`: a field in `Sources`, its `Check` in `check`, its `Read` in `Readers`.
2. **Recorded answers**, in its `testdata/`, by the path read, such as `testdata/api/v1/endpoints/statuses.json`. Record them from your own source, then replace every key, host, name and number that is yours.
3. **Its test**, which replays them with `adaptertest.Replay`, checks the result with `adaptertest.Validate`, and asserts what the adapter makes of them.
4. **Its words**, in `web/i18n/en/<name>.json`, each key starting with `<name>.`. Every word the adapter adds to the page is a key and its values (ADR 0006); names from the source are shown as written.
5. **Its config**, in `overlook.example.yaml`, and the least key it needs, in its section of the README.
6. **Its author**, in `.github/CODEOWNERS`, beside its folder and its words.

## Translating

A translation is one file, `web/i18n/<language>.json`, such as `fr.json`, with any of the keys of `web/i18n/en/*.json`. A key it lacks shows in English, so a translation can be partial and never blocks a release (ADR 0006). A message with plurals takes the forms of [the language's plural rules](https://www.unicode.org/cldr/charts/latest/supplemental/language_plural_rules.html), such as `{"one": "…", "other": "…"}`.

## Recording a decision

A decision that is hard to reverse, surprising to a newcomer and the result of a real trade-off gets an ADR in `docs/adr`, numbered after the last, in the format of the ones there. Anything less belongs in the pull request's description.

## Pull requests

- Keep each to one change, and describe what it changes and why.
- `make check` passes.
- A change to the page comes with screenshots at desktop and phone width.
- Commit messages are a plain sentence, then a body that says why.

By contributing, you agree that your contribution is licensed under the project's [MIT licence](LICENSE).

## Security

Please report a vulnerability privately, as [SECURITY.md](SECURITY.md) explains, rather than in an issue.
