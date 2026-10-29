# Contributing to Overlook

Thank you for helping. Overlook answers the same questions for every homelab, in one fixed page (ADR 0001), so most contributions are an adapter for a source, a translation, or a fix.

Before you start, read [CONTEXT.md](CONTEXT.md) for the words the project uses and [docs/adr](docs/adr) for what has been decided. For anything larger than a fix, open an issue first, so nobody's work is wasted.

## Setting up

You need:

| Tool | Version | For |
| --- | --- | --- |
| [Go](https://go.dev/dl/) | 1.27 or later | the server |
| [Node.js](https://nodejs.org/) | 22.12 or later | the page's tests, with its own runner (no npm) |
| [Biome](https://biomejs.dev/guides/manual-installation/) | 2.6.3 | linting and formatting the page |
| [golangci-lint](https://golangci-lint.run/welcome/install/) | 2.15.0 | linting and formatting the Go code |

Then:

```bash
make check
```

That runs every check a pull request passes, in the order CI does: `go vet`, golangci-lint, Biome, the Go tests with the race detector, the page's tests, and govulncheck. `make fmt` formats everything the way the linters expect.

To see the page with every adapter's parts in it, run the demo, which draws it from an invented snapshot and reads no source:

```bash
go run ./demo -serve localhost:8000
```

and open <http://localhost:8000>. The demo serves two more homelabs, each from an invented snapshot of its own: a small one, at <http://localhost:8000/small/>, and a large one, at the most the page is made for, at <http://localhost:8000/large/>. Every change to the page is looked at with all three. To see it read your own sources, give the server a config with only those, starting from `overlook.example.yaml`:

```bash
go run ./cmd/overlook -config path/to/overlook.yaml
```

and open <http://localhost:8080>.

## Where things are

| Path | What it holds |
| --- | --- |
| `cmd/overlook` | the binary: flags, the server's start and its graceful stop |
| `internal/config` | `overlook.yaml`: strict loading, `${VAR}`s, every adapter's config |
| `internal/fetch` | the only way to read a source: GET requests, a login where a source offers no other way in, and NUT's LIST VAR, nothing else (ADR 0001, ADR 0009) |
| `internal/snapshot` | the shapes every adapter gives back, and the reader that builds a snapshot on demand (ADR 0004) |
| `internal/adapters/<name>` | one adapter each, with its recorded answers in `testdata/` |
| `internal/adapters/adaptertest` | the contract test every adapter passes |
| `internal/server` | the routes, and the headers every answer carries |
| `web` | the page, embedded in the binary: HTML, `style.css`, the modules in `js/`, their tests in `test/`, the words in `i18n/` |
| `demo` | the demo: an invented snapshot with every adapter's parts, a small and a large homelab beside it, the contract the server and the page keep (ADR 0003), and what writes the page from them for GitHub Pages |

## Adding an adapter

An adapter reads one kind of source and gives back its facts in Overlook's shapes. It comes in by pull request from someone who runs that source, who then keeps it (ADR 0005), or from the maintainer, recorded from a scratch instance (ADR 0008).

A source gets an adapter when three things hold: Overlook can read it with GET, and at most one login (ADR 0009), using a key its owner makes once and that lasts a year or more; about one homelab in ten runs it, or its author does; and it shows more than whether it answers, which a check engine already says. A source that can't be read is listed under [Can't be read](README.md#cant-be-read), with why.

One adapter per pull request, with:

1. **Its package**, `internal/adapters/<name>`, with a `Config` (whose `Check` fills its defaults and refuses what can't work) and a `Read(ctx, Config) (snapshot.Parts, error)` that reads only through `internal/fetch`: `fetch.JSON` over HTTP, `fetch.Text` for an answer that isn't JSON, such as iCalendar, a `fetch.Session` for a source that lets Overlook in only after a login (ADR 0009), or `fetch.NUTVars` from NUT's upsd. An answer too big to read with every snapshot, such as Radarr's films, is read through a `fetch.Cache`, at most every 10 minutes. Its home area, the one its parts go in, is its `Area`; a check engine, whose checks go in every area, has none. Wire it into `internal/config`: a field in `Sources`, and one `add` line in `check`, which passes its `Area`.
2. **Recorded answers**, in its `testdata/`, by the path read, such as `testdata/api/v1/endpoints/statuses.json`, or with `.ics` for an answer in iCalendar. An answer whose query sets it apart from another of the same path goes at `<path>@<query>.json`, such as Pi-hole's `api/stats/top_domains@blocked=true&count=10.json`. Record them from your own source, then replace every key, host, name and number that is yours. For a source you don't run, record them from a scratch instance filled with invented data, with its compose file and the steps that fill it beside them (ADR 0008).
3. **Its test**, which replays them with `adaptertest.Replay` (NUT's, which isn't HTTP, replays a transcript of upsd through a fake upsd in its own test), checks the result with `adaptertest.Validate`, and asserts what the adapter makes of them.
4. **Its words**, in `web/i18n/en/<name>.json`, each key starting with `<name>.`. Every word the adapter adds to the page is a key and its values (ADR 0006); names from the source are shown as written.
5. **Its config**, in `overlook.example.yaml`, and the least key it needs, in its section of the README.
6. **Its author**, in `.github/CODEOWNERS`, beside its folder and its words.
7. **Its parts in the demo**, `demo/snapshot.json`, after the parts of the adapters before it in `check`, with names and numbers made up. The demo's test fails until a word of the adapter's is there.

## Translating

A translation is one file, `web/i18n/<language>.json`, such as `fr.json`, with any of the keys of `web/i18n/en/*.json`. A key it lacks shows in English, so a translation can be partial and never blocks a release (ADR 0006). A message with plurals takes the forms of [the language's plural rules](https://www.unicode.org/cldr/charts/latest/supplemental/language_plural_rules.html), such as `{"one": "…", "other": "…"}`.

## Recording a decision

A decision that is hard to reverse, surprising to a newcomer and the result of a real trade-off gets an ADR in `docs/adr`, numbered after the last, in the format of the ones there. Anything less belongs in the pull request's description.

## Pull requests

- Keep each to one change, and describe what it changes and why.
- `make check` passes.
- A change to the page is looked at with the demo's three homelabs, and comes with screenshots at desktop and phone width.
- Commit messages are a plain sentence, then a body that says why.

By contributing, you agree that your contribution is licensed under the project's [MIT licence](LICENSE).

## Releasing

The maintainer tags a commit of `main` as `v` and its version, such as `v0.1.0`, and pushes the tag. The release workflow runs `make check`, then [GoReleaser](https://goreleaser.com) builds the Linux binaries for amd64 and arm64, publishes them as the tag's release, and pushes one image for both to `ghcr.io/bnlqn/overlook` (`.goreleaser.yaml`, `Dockerfile`). A tag with a suffix, such as `v0.1.0-rc.1`, is a release candidate, and the image's `latest` stays where it was.

To try a release without publishing anything, with Docker running:

```bash
go run github.com/goreleaser/goreleaser/v2@v2.19.0 release --snapshot --clean
```

That leaves the binaries in `dist/` and an image for each platform in Docker.

## Security

Please report a vulnerability privately, as [SECURITY.md](SECURITY.md) explains, rather than in an issue.
