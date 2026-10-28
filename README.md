# Overlook

One page that tells you whether your homelab is fine.

What's wrong, how each area is, what it did today. It reads the tools you
already run (Gatus, Proxmox, AdGuard Home, Immich, the arrs) and changes
nothing.

**Not released yet.** The language is in [CONTEXT.md](CONTEXT.md) and the
decisions in [docs/adr](docs/adr). This README is rewritten for v0.1.

## The road to v0.1

1. [x] The skeleton: the Go server, the strict config, the GET-only client,
       the snapshot, the translation table, the page, the Gatus adapter and
       the contract test.
2. [ ] The other adapters, one pull request each: Proxmox VE, Proxmox Backup
       Server, AdGuard Home, Immich, Radarr and Sonarr, NUT, GitHub.
3. [ ] The demo from an invented snapshot, this README, CI, and releases
       (amd64 and arm64 binaries, a multi-arch image on GHCR).
4. [ ] `v0.1.0-rc.1`, with its image public.
5. [ ] The author's homelab runs the release candidate in place of its own
       page.
6. [ ] A week of running it.
7. [ ] The repository goes public and `v0.1.0` is tagged.

v0.1 is ready when the author's homelab has run it for a week with nothing
missing, the demo is live, every adapter has its section here (its config
and the least key it needs), and CI passes.

## Adapters

### Immich

The photos, on the Apps page: what each person keeps and uploaded this week,
the space they take on Immich's disk, Immich's version and whether a newer one
is out, and the jobs queued. Today's figure is everyone's photos and videos.

```yaml
sources:
  immich:
    - url: http://immich:2283
      key: ${IMMICH_KEY}
      # name: Photos # what the page calls it, if not Immich
```

**Key:** an API key made by an admin (Account Settings, API Keys), since
Immich shows everyone's photos, the queues and each person's uploads only to
an admin. Give it only these six read permissions:

- `server.statistics`: each person's photos, videos and space
- `server.storage`: the disk the photos are on
- `server.about`: Immich's version
- `server.versionCheck`: the newest version released
- `queue.read`: the jobs queued
- `adminUser.read`: what each person uploaded this week

What each person uploaded needs Immich 3 or later; an older one leaves it out.

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) says how to set up, what every change
passes and how to add an adapter. Report a vulnerability privately, as
[SECURITY.md](SECURITY.md) explains.

## Licence

MIT.
