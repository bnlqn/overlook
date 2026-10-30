---
status: proposed
---

# Overlook starts without a config and hands one back

When Overlook finds no config, it serves a setup page in place of the page. The setup page asks which tools the homelab runs, with each one's address and key, reads each once through the server, as its adapter would, and says what each answered. It then gives the owner `overlook.yaml` and `.env` to save beside the compose file, put together in the browser, so the server never sends a key back. Overlook writes nothing. Once a config exists, the setup page is gone, and the config is changed in its file, as before. This amends ADR 0001: until its config exists, Overlook draws a second page.

Proposed on 29 October 2026, for the next road, which became the road to v0.4 when v0.3.0 shipped the UniFi adapter on its own, so that a homelab's first page takes minutes rather than an evening of YAML. It is decided when that road is written.

## Considered options

- **A setup page that writes the config.** Rejected: it needs a writable volume, which ADR 0004 rules out, and Overlook has no login, so anyone who can open the page could change a source's address to one of their own and be sent its key, which breaks the promise in SECURITY.md that keys stay in the container.
- **A wizard in the terminal, `overlook init`,** which asks the same questions and prints the files. Rejected: the owner opens the page in a browser anyway, and a form there can say more about each tool, and show what each source answered, than a prompt can.
- **A config builder on GitHub Pages,** in the visitor's own browser. Rejected: it can't reach the homelab's sources, so it can't say whether an address or a key works.
- **Finding the homelab's tools on the network.** Rejected: Overlook never calls anywhere its owner didn't name.

## Consequences

- No config is no longer an error: Overlook serves the setup page. A config that exists but is wrong still stops it, and says why.
- Each adapter describes the fields its config takes, so that the setup page can ask for them, and links to its docs for the least key it needs.
- While there is no config, anyone who can open the page can have Overlook send a GET, with a key they typed, to an address they typed. That is no more than they could do from their own machine on the same network, and it ends when the config is saved.
- The quick start becomes: start the container with an empty config folder, open the page, save the two files it gives, and restart.
