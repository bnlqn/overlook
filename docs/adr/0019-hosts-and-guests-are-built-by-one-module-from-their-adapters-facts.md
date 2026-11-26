---
status: proposed
---

# Hosts and guests are built by one module from their adapters' facts

An adapter that reads hosts or guests fills in a `snapshot.Host` or a `snapshot.Guest` for each: the identifier its source gives it that survives a restart, such as Proxmox VE's node name and VMID, Beszel's system ID or a container's name; its name; how its source says it is, in the source's words; for a guest, the host it runs on, as its source says; and for a host, its readings for the charts. One module in `internal/snapshot` turns them into Servers' rows and the hosts' charts, in words of its own, and gives each its address as a thing (ADR 0018), as ADR 0016's modules do for stores and backup runs. So a host looks the same on the page and in its close-up whichever adapter read it, and the close-up reads typed facts, never a row's tone or the values in its words. This amends ADR 0012, under which hosts "share their charts this way, but not their rows".

Proposed on 9 November 2026, with ADR 0018, for the road to v0.8, when a look at the snapshot found that only a check carries its source's own identifier: a host or a guest is a row of words, a Proxmox VE guest's VMID survives only as a value in its detail's words, and Docker reads no identifier at all. It is decided when it is built, as step 1 of that road, with Proxmox VE; Beszel and Docker move onto it in step 2.

## Considered options

- **An identifier added to the rows, events and chart lines adapters already build**, each adapter keeping its own rows. The runner-up: the smaller change. Rejected: a close-up would piece a host back together from tones and the values in its words, and Proxmox VE's and Beszel's hosts would still look different.
- **Rows left to each adapter, as ADR 0012 has them.** Rejected: that held while a row was only drawn. A close-up now reads hosts and guests, and two adapters read hosts and two read guests, which is ADR 0012's own reason for a module.
- **A container's ID as its identifier.** Rejected: it changes every time the container is recreated, as each update does, while its name stays.
- **One module for every thing at once, stores and checks among them.** Rejected: a check already carries its engine's key, and a store is already built by its module (ADR 0016). Each gains its identifier where it is.

## Consequences

- Proxmox VE's, Beszel's and Docker's hosts and guests are drawn by the module. The demo's three homelabs and the tests show Servers as it was.
- Portainer stays as it is: it lists each environment's counts, not its containers.
- A row only one adapter's hosts have goes in that adapter's own rows, after the module's, as ADR 0012 has it for a kind.
- The hosts' and guests' words go in a table of their own, `web/i18n/en/host.json`, with those `page.json` holds for them now, as the stores' are in `store.json`.
- A Proxmox VE guest's backup runs stay its node's, since Proxmox VE runs them by node. A guest's own backups wait for a backup server's backup to be tied to its guest (ADR 0018).
- Once this is decided, ADR 0012's status says that this one amends it.
