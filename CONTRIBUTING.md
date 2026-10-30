# Contributing to Overlook

Thank you for helping. Overlook answers the same questions for every homelab, in one fixed page (ADR 0001), so most contributions are an adapter for a source, a translation, or a fix.

Before you start, read [CONTEXT.md](CONTEXT.md) for the words the project uses and [docs/adr](docs/adr) for what has been decided. For anything larger than a fix, open an issue first, so nobody's work is wasted.

## How Overlook is written

Overlook is written with an AI coding agent, Claude Code. The maintainer decides, with the agent, what Overlook does and how: the road, the words in CONTEXT.md and every ADR. The agent writes the code, the tests and the docs, following [AGENTS.md](AGENTS.md). The maintainer reviews every pull request before it merges, and `make check` is the gate it passes.

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
| `internal/fetch` | the only way to read a source: a `fetch.Source` for each, which sends GET requests under its address and a login where it offers no other way in, and NUT's LIST VAR, nothing else (ADR 0001, ADR 0009, ADR 0013) |
| `internal/snapshot` | the shapes every adapter gives back, what every resolver and media server shows whichever adapter reads it (ADR 0012), and the reader that builds a snapshot on demand (ADR 0004) |
| `internal/adapters/<name>` | one adapter each, with its recorded answers in `testdata/` |
| `internal/adapters/adaptertest` | the contract test every adapter passes |
| `internal/server` | the routes, and the headers every answer carries |
| `web` | the page, embedded in the binary: HTML, `style.css`, the modules in `js/`, their tests in `test/`, the words in `i18n/`. A decision the page makes goes in `js/page.js`, with a test; `overview.js`, `area.js` and `shell.js` only write what it returns |
| `demo` | the demo: an invented snapshot with every adapter's parts, a small and a large homelab beside it, the contract the server and the page keep (ADR 0003), and what writes the page from them for GitHub Pages |

## Adding an adapter

An adapter reads the sources of one program and gives back their facts in Overlook's shapes. It comes in by pull request from someone who runs that source, who then keeps it (ADR 0005), or from the maintainer, recorded from a scratch instance (ADR 0008).

A source gets an adapter when three things hold: Overlook can read it with GET, and at most one login (ADR 0009), using a key its owner makes once and that lasts a year or more, and, if its GETs can change things too, as Tautulli's do, through a fixed list of the ones that read, which its test checks (ADR 0011); about one homelab in ten runs it, or its author does; and it shows more than whether it answers, which a check engine already says. A source that can't be read is listed under [Can't be read](README.md#cant-be-read), with why.

One adapter per pull request, with:

1. **Its package**, `internal/adapters/<name>`, with a `Config` (whose `Check` fills its defaults and refuses what can't work) and a `Read(ctx, Config, now time.Time) (snapshot.Parts, error)` that reads only through `internal/fetch`. Over HTTP, its `Check` fills a `fetch.Source` with the source's address, its key in `Header`, its `Login` where it lets Overlook in only after one (ADR 0009), and its `CA` where the system doesn't trust its certificate; `Read` then asks it for paths under that address, with `JSON`, or `Text` for an answer that isn't JSON, such as iCalendar (ADR 0013). NUT is read with `fetch.NUTVars`. It takes the time from `now`, the moment the snapshot is read, never from the clock, so every source in a snapshot agrees on what today is and its test can set it. An answer too big to read with every snapshot, such as Radarr's films, is read through a `fetch.Cache`, at most every 10 minutes. Its home area, the one its parts go in, is its `Area`; a check engine, whose checks go in every area, has none, and builds each check with `snapshot.CheckOf`, from its results and the stretches it failed, which works out whether it's a problem, since when, and its outages. Wire it into `internal/config`: a field in `Sources`, and one `add` line in `check`, which passes its `Area`, and its kind if it has one, which the page asks by: `checkengine`, `todolist`, `resolver` or `mediaserver`. A resolver's or a media server's adapter fills in its kind's facts, a `snapshot.Resolver` or a `snapshot.MediaServer`, and returns their `Parts`, which are what every source of that kind shows, so that they add up (ADR 0012); its own words are for its own rows.
2. **Recorded answers**, in its `testdata/`, by the path read, such as `testdata/api/v1/endpoints/statuses.json`, with `.ics` for an answer in iCalendar, or with `.http` for one whose status or headers matter, such as the cookie a login gives, written whole as `curl -si` prints it. A login (ADR 0009) is recorded at its path like any other answer. An answer that the query sets apart goes at `<path>@<pairs>.json`, naming only the pairs of the query that set it apart, such as Pi-hole's `api/stats/top_domains@blocked=true&count=10.json` or Prometheus's `api/v1/query_range@step=300.json`. Write a pair as the address escapes it, `%3A` for a `:`, which Windows and Go's modules refuse in a file name. Every answer is one the adapter reads: one that nothing reads fails its test. Record them from your own source, then replace every key, host, name and number that is yours. For a source you don't run, record them from a scratch instance filled with invented data, with its compose file and the steps that fill it beside them (ADR 0008).
3. **Its test**, which replays them with `adaptertest.Replay` at the moment they were recorded, checks the result with `adaptertest.Validate`, given its `Area`, and asserts what the adapter makes of them and, through the replay's `Sent()`, what it sent: its key with every read, its login once, and, for a source such as Tautulli, only the commands on its list (ADR 0011). A variation is a few edits to the recorded answers, given to `Replay` as old and new text, or, for a source that answers less, a directory of its own. A server of its own is only for what `Replay` can't play: a source that changes between reads, such as Beszel forgetting a session, a certificate, or a source that isn't HTTP, such as NUT's upsd, whose transcript is replayed through a fake upsd.
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

Versions follow [Semantic Versioning](https://semver.org) to the letter. A fix is a patch, such as `v0.3.1`. Anything new, such as an adapter, is a minor version, such as `v0.4.0`, never a patch. A change that breaks what worked, such as a config key or one of the page's promises, is a major version from 1.0 on; until then it is a minor one, and its notes say so first.

Each release goes in three steps:

1. **A pull request** brings every file that names a version up to the new one: the image in the README's quick start and its line on the image, the README's road under What's next, the commands in SECURITY.md, and any proposed ADR written for the next road.
2. **The tag**, on `main` once that pull request is merged, annotated as `Overlook v0.4.0, which …`, saying what it brings.
3. **The notes**: once the release is published, they go above GitHub's list of what was merged: what's new, how to upgrade, with anything that breaks first, and how to check the build (SECURITY.md).

To try a release without publishing anything, with Docker running:

```bash
go run github.com/goreleaser/goreleaser/v2@v2.19.0 release --snapshot --clean
```

That leaves the binaries in `dist/` and an image for each platform in Docker.

## Security

Please report a vulnerability privately, as [SECURITY.md](SECURITY.md) explains, rather than in an issue.
