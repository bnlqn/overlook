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
2. [x] The other adapters, one pull request each: Proxmox VE, Proxmox Backup
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

### GitHub

The updates waiting: the open pull requests Dependabot or Renovate opened on
the repositories you list. They are chores, never problems.

```yaml
sources:
  github:
    - repos: [example/homelab, example/website]
      token: ${GITHUB_TOKEN} # only for private repositories
      # bots: ["dependabot[bot]", "renovate[bot]"] # whose pull requests are updates
      # url: https://github.example.com/api/v3     # a GitHub Enterprise Server's API
```

**Key:** none for public repositories. For private ones, a
[fine-grained token](https://github.com/settings/personal-access-tokens/new)
with read-only access to **Pull requests** on those repositories (GitHub adds
**Metadata** itself). Without one, GitHub answers 404 for a private
repository.

Overlook reads GitHub at most every 10 minutes, so an update can take that
long to show. Without a token GitHub allows 60 requests an hour from your
address, and each read is one request per repository: ten repositories hold,
fewer if something else at your address reads GitHub without a token.

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

### Proxmox Backup Server

Each guest's newest backup in one datastore, in Storage: when it was made,
its size, and whether it passed its check, named by its notes, which
Proxmox VE fills with the guest's name. Today shows how many guests the
last night backed up, and the week each night's backups.

```yaml
sources:
  pbs:
    - url: https://pbs.example.com:8007
      token: overlook@pbs!overlook
      secret: ${PBS_SECRET}
      datastore: backups
      ca: /etc/overlook/pbs.pem         # PBS's own certificate, copied from the server
      # server_name: pbs.example.com    # the name it's for, if the url uses another
```

**Key:** an API token with the role **DatastoreAudit** on that datastore,
which lists the backups and can't read them. A token can do no more than
its user, so both get the role. On the server:

```bash
proxmox-backup-manager user create overlook@pbs
proxmox-backup-manager acl update /datastore/backups DatastoreAudit --auth-id overlook@pbs
proxmox-backup-manager user generate-token overlook@pbs overlook
proxmox-backup-manager acl update /datastore/backups DatastoreAudit --auth-id 'overlook@pbs!overlook'
```

The third command prints the token's secret.

**Certificate:** PBS signs its own, for its own name. Copy
`/etc/proxmox-backup/proxy.pem` from the server beside Overlook and name it
as `ca`. If the `url` reaches PBS by an address its certificate doesn't
name, give the name it does as `server_name`. As for Proxmox VE, there is
no way to skip the check.

### Proxmox VE

Each host's processor, memory, uptime and version, the guests, the storage,
the ZFS pools and the last backup run, in Servers. Today shows how many
guests are up, and the week each backup run.

```yaml
sources:
  proxmox:
    - url: https://pve.example.com:8006 # any host of a cluster reads all of it
      token: overlook@pve!overlook
      secret: ${PROXMOX_SECRET}
      ca: /etc/overlook/pve-root-ca.pem  # Proxmox's own CA, copied from the host
```

**Key:** an API token of a user with the role **PVEAuditor**, which reads
everything and changes nothing. On the host:

```bash
pveum user add overlook@pve
pveum acl modify / --users overlook@pve --roles PVEAuditor
pveum user token add overlook@pve overlook --privsep 0
```

The last command prints the token's secret. The user has no password, so
the token is the only way in, and it can do no more than the user.

**Certificate:** Proxmox signs its own. Copy `/etc/pve/pve-root-ca.pem` from
the host beside Overlook and name it as `ca`: Overlook then trusts that CA
alone for this source. There is no way to skip the check, since a key sent
to whoever answers in Proxmox's place is given away. A host with a
certificate from a public CA, such as Let's Encrypt, needs no `ca`.

### Radarr

The films on disk and the room they take, the films added this week, what
Radarr is downloading and warns about, and the films out to stream or on
disc in the next seven days. Its home area is Apps.

```yaml
sources:
  radarr:
    - url: http://radarr:7878 # where Overlook reads Radarr
      key: ${RADARR_KEY}
      name: Radarr            # if you run more than one, tell them apart
```

**Key:** Radarr's API key, from Settings, General. It's the only key Radarr
has, and it can do anything, but Overlook sends it only with GET requests,
which change nothing (ADR 0001).

Radarr answers with every film at once, so Overlook reads the films at most
every 10 minutes, and a film can take that long to count. The rest is read
with every snapshot.

### Sonarr

The shows and episodes on disk and the room they take, the episodes imported
this week, what Sonarr is downloading and warns about, and the episodes that
air in the next seven days. Its home area is Apps.

```yaml
sources:
  sonarr:
    - url: http://sonarr:8989 # where Overlook reads Sonarr
      key: ${SONARR_KEY}
      name: Sonarr            # if you run more than one, tell them apart
```

**Key:** Sonarr's API key, from Settings, General. Like Radarr's, it's the
only key and it can do anything, and Overlook sends it only with GET requests
(ADR 0001).

The shows are read at most every 10 minutes, as Radarr's films are.

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

### AdGuard Home

Each resolver is its own source. Today's lookups and blocks show on the
Overview, every resolver's added up, and the Network page shows each one:
whether it filters, its lookups, blocks and upstreams, and its top
clients, domains and blocked domains, over the interval its statistics
keep.

```yaml
sources:
  adguard:
    - name: Primary DNS
      url: http://overlook:${ADGUARD_PASSWORD}@dns1.example.com
    - name: Standby DNS
      url: http://overlook:${ADGUARD_PASSWORD}@dns2.example.com
```

**Key:** a login, in the URL as `user:pass@`. AdGuard Home has no
read-only users, so that login could change anything; what keeps it
harmless is that Overlook only ever sends GET (ADR 0001). Give Overlook a
user of its own: add it under `users:` in `AdGuardHome.yaml`, with its
password hashed by `htpasswd -B -C 10 -n -b overlook <password>`, and
restart AdGuard Home.

**Today** starts at midnight in Overlook's time zone: set `TZ`, such as
`TZ=Europe/Brussels`, in its container. AdGuard Home keeps statistics by
the hour for an interval of up to 7 days; over a longer one it keeps them
by the day, in UTC, and the Overview shows no figure for it.

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) says how to set up, what every change
passes and how to add an adapter. Report a vulnerability privately, as
[SECURITY.md](SECURITY.md) explains.

## Licence

MIT.
