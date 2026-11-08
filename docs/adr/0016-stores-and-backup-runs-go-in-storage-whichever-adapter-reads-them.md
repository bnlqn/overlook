---
status: accepted
---

# Stores and backup runs go in Storage, whichever adapter reads them

A store is one place the homelab keeps its data, with its room and its health, such as a ZFS pool, a Proxmox VE storage or a backup server's datastore. A backup run is one run that makes backups, such as a night's run of Proxmox VE's backup job. Both go in Storage whichever adapter reads them, whatever that adapter's home area, as a check goes in the area its group names. An adapter that reads stores fills in a `snapshot.Store` for each: its name, the room it takes and the room it has, and its health, as the source words it. One that reads backup runs fills in a `snapshot.BackupRun` for each: when it started and ended, how it went, and how many backups it made. One module in `internal/snapshot` turns each into Storage's rows and events, in its own words, as ADR 0012's modules do for a kind of source, so that every store and every backup run looks the same on the page, whichever source it came from. The rest of what an adapter gives stays in its home area: Proxmox VE's hosts and guests stay in Servers, while its pools, its storage and its backup runs go in Storage.

Decided on 7 November 2026, as the first half of step 3 of the road to v0.6, when selfh.st's 2025 survey showed that two thirds of homelabs run a NAS, while Storage held only what Proxmox Backup Server gives: the stores most homelabs have are read by Proxmox VE, whose home area is Servers.

## Considered options

- **A kind of source for storage**, as the road first put it. Rejected: a source has at most one kind (ADR 0012), and Proxmox VE, whose pools most homelabs have, is no storage source; nor would Unraid be, whose array issue #83 wants read.
- **Each part saying its own area**, with the fixed home area dropped. Rejected: every adapter would decide for itself where a pool goes, and two sources' pools would look different, which is what ADR 0012 ended for resolvers.
- **Stores only**, with backup runs left in their adapters' home areas. Rejected: Storage answers whether the data is safe and whether there is room, and a homelab with Proxmox VE but no backup server would find its backups in Servers.
- **Only the pools**, with Proxmox VE's storage left in Servers. Rejected: a full `local-lvm` stops guests, which is a question of room.
- **Storage filled only by sources that are storage**, such as TrueNAS or Synology DSM. Rejected: neither can be read today (README, Can't be read), and the pools most homelabs have would stay in Servers.

## Consequences

- Proxmox VE's pools, storage and backup runs move from Servers to Storage. Proxmox Backup Server reads its datastore's room as a store, and its nights become backup runs; its list of each guest's newest backup stays its own rows, after them. Two adapters read each, so each gets its module now (ADR 0012).
- "Can Unraid's array go in Storage?" is answered yes: as stores, while Unraid's host and guests go in Servers. So would TrueNAS's pools, if it can ever be read.
- A store's health and a failed backup run are red rows, never problems: only a check makes a problem (ADR 0001).
- The same night can show twice: as Proxmox VE's backup run, and as the backups the backup server keeps from it. The run says the job ran, the backup server what it holds, and nothing adds the two up.
- No figure adds stores up, since a Proxmox VE storage on a ZFS pool is the same room twice. One waits until a store can say what it sits on.
- A store that is another source's own shows once: a Proxmox VE storage on a backup server says so, and is left out while a source of that adapter answers, which shows the same datastore itself. Added on 8 November 2026, when v0.6.0-rc.1 showed one datastore twice in Storage on the maintainer's homelab.
- A source that gives stores or backup runs and doesn't answer is named in Storage too, beside its home area, so that Storage never loses them without a word.
- `adaptertest` still checks every other part an adapter gives against its home area, but not its stores and backup runs, just as it doesn't check checks.
- The stores' and the backup runs' words are in tables of their own, `web/i18n/en/store.json` and `backuprun.json`, as the kinds' are.
