---
status: accepted
---

# An adapter reads a source only under its own address

An adapter gives `internal/fetch` each of its sources once, as a `fetch.Source`: its address, its key or its login (ADR 0009), and the CA its certificate is signed by, if the system doesn't trust it. It then reads the source by path, and a path must be empty, for the address itself, or begin with `/`, so that no read, and no key, goes anywhere but the source's own address. Until now each read took a whole URL and the key with it, so nothing but review kept a key from going to an address an adapter built from a source's answer. This amends ADR 0001: `fetch` is still the only way to a source, and each source is now reached only where its owner put it.

Decided on 29 October 2026, in a review of Overlook's design before step 9 of the road to v0.2 opens the repository to other adapters' authors.

## Considered options

- **Whole URLs, as before**, read by `fetch.JSON`, `Client.JSON` or `Session.JSON`. Rejected: three ways to read for an author to choose between, each adapter joining its address and path and setting its key by hand, and a key that goes wherever its URL does.
- **A source as an interface adapters take, with a stand-in in memory for tests.** Rejected: only one would run outside the tests, and adapter tests would then skip the GET-only guard, the redirect rule and the login, which are what keep keys harmless. Tests read a real `fetch.Source` over HTTP, from `adaptertest.Replay`.
- **A key in the address, added by `fetch` to every read.** Rejected: it invites keys where proxies log them. Tautulli, which takes its key only there, builds its address itself, after checking its fixed list of commands (ADR 0011).

## Consequences

- An adapter's author learns one way to read: fill a `fetch.Source` in `Check`, then ask it for paths. Pi-hole and qBittorrent read the same way with a login as without one.
- An address in a source's answer, such as the next page in GitHub's Link header, is read only as a path under the source's address, and only if it is under it.
- A key goes with every read of its source, Nextcloud's to its status too, which is harmless: it is the same source.
- Two addresses an adapter reads, such as Prometheus and its Alertmanager, are two sources.
