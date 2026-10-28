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

### NUT

A UPS, read from NUT's upsd over NUT's own protocol. Servers shows whether it
runs on mains or on its battery, its model and the voltage it's fed, the
battery's charge and how long it would last, and what its load draws; the
overview shows the draw and the battery.

```yaml
sources:
  nut:
    - host: nut.example.com # upsd, as host or host:port (3493 unless you say)
      ups: ups              # the UPS's name in upsd, as in its ups.conf
```

upsd needs no login for this: Overlook sends only `LIST UPS`, `LIST VAR`,
`GET VAR`, `VER` and `NETVER`, and refuses every other command in its code.
upsd must listen where Overlook can reach it, which by default it doesn't:
set `MODE=netserver` in `nut.conf`, add a `LISTEN` line for the address
Overlook reaches to `upsd.conf` (such as `LISTEN 192.0.2.13 3493`), and let
Overlook through any firewall to that port. The readings cross the network
unencrypted, as upsd sends them without TLS.

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) says how to set up, what every change
passes and how to add an adapter. Report a vulnerability privately, as
[SECURITY.md](SECURITY.md) explains.

## Licence

MIT.
