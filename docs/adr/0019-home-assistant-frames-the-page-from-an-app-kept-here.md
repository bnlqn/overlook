---
status: accepted
---

# Home Assistant frames the page, from an app kept here

Overlook installs inside Home Assistant OS as an app, which Home Assistant called an add-on until 2026, and the app is kept in this repository: `repository.yaml` at its root, where Home Assistant's app store looks when it is given the repository's address, and the app's files in `homeassistant/`, beside Unraid's template. The app runs the released image, `ghcr.io/bnlqn/overlook`, at the version its `config.yaml` names, with its own folder from Home Assistant mounted read-only at `/etc/overlook`, where the image reads its config. It opens in Home Assistant's sidebar through ingress: Home Assistant serves the page from its own address, under a path of its own, inside a frame of its own page, behind its own login. The page's addresses are all relative already, and Home Assistant passes on the browser's `Host`, so `/setup` and `/mcp` go on refusing a request whose `Origin` isn't their own without trusting any header a proxy adds. One word changes: the Content-Security-Policy says `frame-ancestors 'self'` in place of `'none'`, so that a page at Overlook's own address may frame it, and still no other. The app's port stays closed until its owner opens it, for MCP or for a browser outside Home Assistant.

Proposed on 9 November 2026, as the first step of the road to v0.7, once ingress, tried behind a proxy that does what Home Assistant's does, drew the page under its path but found it blocked by `frame-ancestors 'none'`. Decided the same day, before it was built.

## Considered options

- **A repository of its own for the app**, such as `bnlqn/overlook-homeassistant`. Rejected: a second repository to bring to each release, and the road keeps the app beside Unraid's template. What keeping it here costs: Home Assistant clones the whole of Overlook, 6 MB, and reads every `config.json` in it as an app's, so the Home Assistant adapter's recorded `testdata/api/config.json` adds a line to the Supervisor's log saying it isn't one.
- **A branch of its own**, which the store takes as `https://github.com/bnlqn/overlook#homeassistant`. Rejected: a branch to bring to each release, which nobody browsing the repository sees.
- **The image built on the owner's machine**, from a Dockerfile in the app's folder, as Home Assistant does when `image` is left out. Rejected: it is slower to install and update, and it isn't the image GitHub attests (SECURITY.md).
- **Reading `X-Ingress-Path`** to write the page's addresses whole, or a `<base>`. Rejected: they are relative already, as the demo's small and large homelabs, each served from a folder, show.
- **Trusting `X-Forwarded-Host`** for the `Origin` checks. Not needed: Home Assistant passes `Host` on, and whoever reaches the port could send that header too.
- **`frame-ancestors 'self'` only on what comes through ingress**, from 172.30.32.2 or with `X-Ingress-Path`. Rejected: code for nothing, since a page at Overlook's own address can already read all it serves, and a page at any other address still can't frame it.
- **Answering only Home Assistant, at 172.30.32.2**, as its guide for apps advises, so that an app's own login can't be gone around. Rejected: Overlook has no login (README, No login), and the port an owner opens for MCP would answer nothing.
- **No ingress: a port open from the start and Home Assistant's Open Web UI button.** Rejected: Overlook wouldn't open in the sidebar, and every machine on the network could read the page, with no login.
- **The config in Home Assistant's form**, the app's Configuration tab, in place of `overlook.yaml`. Rejected: a second way to write the config, beside the file and the setup page that hands it back (ADR 0014).

## Consequences

- An owner adds `https://github.com/bnlqn/overlook` to the app store, installs Overlook and opens it from the sidebar, which shows the setup page. They save its two files in the app's folder, which Home Assistant names after the repository's address and the app, such as `app_configs/0a1b2c3d_overlook`, through the Samba share or a terminal app, and restart the app (`homeassistant/DOCS.md`). The setup page says where, as it does on Unraid, since Home Assistant sets `SUPERVISOR_TOKEN` in every app.
- Moving the app out of this repository later would move its folder too, and its updates: each owner would add the new repository, install it again and move their files. So it stays here.
- The app first names v0.7.0, the first release whose page Home Assistant can frame, so until that is published, installing the app fails to pull its image.
- Each release takes one more step: once its image is on GHCR, a pull request brings the app's `version` to it. Home Assistant offers an update when that changes and pulls that tag, so it never offers one it can't download, and a release candidate never reaches it.
- Inside Home Assistant, Overlook is behind Home Assistant's login, and only its administrators see it in the sidebar. With the port closed, the setup page's gap that ADR 0014 accepts can't be reached: a site can't point its name at an address Home Assistant keeps to itself.
- An agent reaches MCP only through the port, since it can't log in to Home Assistant's ingress. With the port open, Overlook answers anyone who reaches it, as everywhere else. Other apps on Home Assistant's network can reach its page, as containers on one network can, and read what `/snapshot` gives, never a key.
- Wherever it runs, a page at Overlook's own address may frame it. Behind a proxy that serves several apps under one host, each under a path, each may frame the others, which could read them already.
- The app hasn't been installed in a real Home Assistant OS until the maintainer installs it.
