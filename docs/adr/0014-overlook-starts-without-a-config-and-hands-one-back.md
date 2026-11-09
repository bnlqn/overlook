---
status: accepted, amended by ADR 0018
---

# Overlook starts without a config and hands one back

When there is no `overlook.yaml`, Overlook serves the setup page in place of the page. The setup page offers every adapter, grouped as the README lists them, and asks for each source only what it needs to be read: its address, its key or login, the extras it can't be read without, such as Uptime Kuma's status page, and, if the owner wants them, its name and its CA. Each adapter marks those fields with a `setup:` tag on its config, and an `example:` tag gives an invented value, such as the usual address. The setup page tries each source once through the server, with a POST to `/setup` that goes through the config's own checks and then one read, and says what it found, in the page's words: so many checks, problems, sections, charts, figures, events and chores. A source that fails says what to do about it, by the kind of failure `fetch.Why` tells apart, such as a certificate signed by a CA the system doesn't trust, or `localhost`, which in a container is the container itself. It then puts together, in the browser, so that the server never sends a key back, `overlook.yaml` for the sources that answered, each key in it a `${VAR}`, and the `.env` that holds the keys. The owner saves both in the config's folder and restarts Overlook, which then reads `.env` from beside the config, the environment winning over it, and the setup page, still open, turns into the page. Overlook writes nothing. Once the config exists, the setup page is gone, and the config is changed in its file, as before. This amends ADR 0001: until its config exists, Overlook draws a second page.

Proposed on 29 October 2026, and decided on 2 November 2026, as the first step of the road to v0.5, when a request to list Overlook in Unraid's Community Applications showed what the first start costs without it: a container that stops at once, and keys typed into a form one at a time.

## Considered options

- **A setup page that writes the config.** Rejected: it needs a writable volume, which ADR 0004 rules out, and Overlook has no login, so anyone who can open the page could change a source's address to one of their own and be sent its key, which breaks the promise in SECURITY.md that keys stay in the container.
- **A wizard in the terminal, `overlook init`,** which asks the same questions and prints the files. Rejected: the owner opens the page in a browser anyway, and a form there can say more about each tool, and show what each source answered, than a prompt can.
- **A config builder on GitHub Pages,** in the visitor's own browser. Rejected: it can't reach the homelab's sources, so it can't say whether an address or a key works.
- **Finding the homelab's tools on the network.** Rejected: Overlook never calls anywhere its owner didn't name.
- **`.env` beside the compose file, as Compose reads it.** Rejected: Unraid and `docker run` have no compose file, so their owners would copy each key into the container's variables by hand, and a changed `env_file` takes `docker compose up -d`, not a restart.
- **Keys written into `overlook.yaml`.** Rejected: the config could no longer be shared or kept in git.
- **Asking every field, `areas`, `link` and `threshold` among them.** Rejected: they shape the page once it reads, and the file the setup page hands back links each source's README section, which describes them.
- **Each adapter listing its fields in Go, beside its config.** Rejected: a second list of the same fields, to keep in step by hand. The tags sit on the fields they are about, and a test fails on one it can't read.
- **A pasted CA, handed back as a third file.** Rejected: `fetch` would take a certificate's content as well as its path. The owner copies the CA into the folder first, as the README already says.
- **Overlook noticing the saved files by itself.** Rejected: it reads its config once, at start, and a restart is one command or one click.
- **A code from the log, asked for before the first try.** Rejected for now: it would close the gap below, at the cost of a step before the first page, for a gap that lasts only until the config is saved.

## Consequences

- No config is no longer an error: Overlook serves the setup page. A config that exists but is wrong, an empty one too, still stops it, and says why.
- The quick start becomes: start the container with an empty config folder, open the page, save the two files it gives, and restart. The compose file needs no `env_file`.
- `.env` sits in the config's folder, readable by Overlook's user, and stays out of git when the folder is in it. Overlook serves nothing of it.
- An adapter's author tags the fields its sources need, and its README section's heading is its name on the setup page, which a test checks.
- While there is no config, anyone who can open the page can have Overlook send a GET, with a key they typed, to an address they typed. That is no more than they could do from their own machine on the same network, and it ends when the config is saved. The try refuses a request whose `Origin` isn't Overlook's own, so an ordinary website can't do it through the owner's browser, but that doesn't stop DNS rebinding: a site that points its own name at Overlook could have it read addresses inside the homelab, and learn what each answered, counted, or why it didn't. SECURITY.md says so.
- The demo has no setup page: it shows a homelab already set up, and a try needs a server.
