# Overlook

One page that tells you whether your homelab is fine.

[![The overview of a homelab that doesn't exist: one problem, Standby DNS down, so Network is degraded; today's figures; and each area's checks](docs/overview.png)](https://bnlqn.github.io/overlook/)

What's wrong, how each area of the homelab is, and what it did today and
this week, on one page that is the same for every homelab. Overlook reads
the tools you already run, such as Gatus, Proxmox, AdGuard Home, Immich,
Radarr and Sonarr, and changes nothing. [See the demo](https://bnlqn.github.io/overlook/),
drawn from a homelab that doesn't exist.

**Not released yet.** The road below says where it is. The words the
project uses are in [CONTEXT.md](CONTEXT.md) and its decisions in
[docs/adr](docs/adr).

## The road to v0.1

1. [x] The skeleton: the Go server, the strict config, the GET-only client,
       the snapshot, the translation table, the page, the Gatus adapter and
       the contract test.
2. [x] The other adapters, one pull request each: Proxmox VE, Proxmox Backup
       Server, AdGuard Home, Immich, Radarr and Sonarr, NUT, GitHub.
3. [x] The demo from an invented snapshot, this README, CI, and releases
       (amd64 and arm64 binaries, a multi-arch image on GHCR).
4. [x] `v0.1.0-rc.1`, with its image public.
5. [x] The author's homelab runs the release candidate in place of its own
       page.
6. [ ] A week of running it.
7. [ ] The repository goes public and `v0.1.0` is tagged. The demo goes
       live then: Pages set to build from GitHub Actions, and the Demo
       workflow run once by hand, since going public pushes nothing to
       `main`.

v0.1 is ready when the author's homelab has run it for a week with nothing
missing, the demo is live, every adapter has its section here (its config
and the least key it needs), and CI passes.

## What it shows

- **What's wrong**: the problems, which are the checks your check engine
  alerts on, for as long as they fail. Overlook runs no check and sends no
  alert, so the page and your alerts always agree.
- **How each area is**: Servers, Network, Storage and Apps, each up,
  degraded (a problem, but it still works, such as DNS with one of two
  resolvers down) or down, with its checks' last 30 results and a page of
  its own with what its sources add, such as a host's guests or a
  resolver's top clients.
- **Today**: a figure from each source, such as guests up, DNS lookups,
  photos or what the UPS draws.
- **This week**: each area's problems and events along the last seven days,
  and the list of what happened, as far as your sources remember it.
- **Your apps and tools**, each a link with a dot that says whether it
  answers.
- **The chores**: the maintenance you wrote down that is due or comes this
  week, and the updates waiting for your merge. A chore is never a problem.

## What it never does

- **It never changes anything.** Overlook sends a source only GET requests,
  and NUT only its read commands; the client it reads with refuses every
  other method. Some keys it holds could do anything (AdGuard Home has no
  read-only users, and Radarr's and Sonarr's keys are admin keys), and this
  is what keeps them harmless.
- **It never runs a check or sends an alert.** Your check engine does both.
- **It remembers nothing.** No database and no volume: it reads every
  source when the page asks, at most every 30 seconds, so an Overlook
  nobody looks at sends nothing into your homelab. What happened this week
  is what your sources remember.
- **It never sends a key to the browser.** The server reads every source
  and gives the page a snapshot of what it read, never a key or a source's
  own answer.
- **It never asks where to put things.** The page is fixed: your config
  says which sources fill it, never where anything goes. There are no
  widgets to arrange.
- **It never calls anywhere you didn't name.** No telemetry and no update
  check; the page's fonts and icons come from Overlook itself.

## Quick start

Overlook is one container. Put its config in a folder of its own, beside a
`compose.yaml`:

```
overlook/
  overlook.yaml
compose.yaml
.env
```

```yaml
# compose.yaml
services:
  overlook:
    image: ghcr.io/bnlqn/overlook:0.1
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./overlook:/etc/overlook:ro # the config, read-only: Overlook writes nothing
    env_file: .env                  # the keys
    environment:
      TZ: Europe/Brussels           # when today starts
```

```yaml
# overlook/overlook.yaml
sources:
  gatus:
    - url: http://gatus.example.com:8080
  proxmox:
    - url: https://pve.example.com:8006
      token: overlook@pve!overlook
      secret: ${PROXMOX_SECRET}       # from the environment
      ca: /etc/overlook/pve-root-ca.pem # copied into overlook/

apps:
  - name: Photos
    url: https://photos.example.com
    icon: immich.svg
    check: Photos
```

```sh
# .env: the keys, for your eyes only (chmod 600 .env)
PROXMOX_SECRET=00000000-0000-0000-0000-000000000000
```

Then `docker compose up -d`, and open port 8080 of that host. The log says
which sources Overlook reads, and the page names any that didn't answer.

- **Keys come from the environment.** A `${VAR}` anywhere in the config is
  read from Overlook's environment, so the config holds no secret and can
  be shared or kept in git. A `${VAR}` that isn't set stops Overlook, and
  says which.
- **The config is mounted read-only**, at `/etc/overlook`, with a CA file or
  your own icons beside it. The image runs as uid 65532, so the folder and
  its files must be readable by others, as they are by default.
- **`TZ`** says when today starts, for today's figures. Any zone works: the
  zone database is inside the binary.
- **The image** is `ghcr.io/bnlqn/overlook`, for amd64 and arm64: `0.1` for
  the newest 0.1 release, or a version such as `0.1.0`. It holds the
  binary and the CA certificates it checks HTTPS sources with, and nothing
  else.

Without Docker, each [release](https://github.com/bnlqn/overlook/releases)
has the binary for Linux on amd64 and arm64, such as for an LXC container
or a Raspberry Pi:

```bash
overlook -config /etc/overlook/overlook.yaml -listen :8080
```

## Configuring

[`overlook.example.yaml`](overlook.example.yaml) shows every source with
comments. Overlook reads the config once, at start, and refuses a key it
doesn't know, so a typo stops it rather than going unnoticed.

**Sources.** Under `sources:`, each adapter has a list, so two AdGuard Home
resolvers are two entries under `adguard:`. Each source has a `name`, which
the page uses. Each adapter gives one, such as Gatus, and two sources can't
share one, so two of a kind need names of their own.
What each adapter reads, its config and the least key it needs are under
[Adapters](#adapters) below.

**Areas.** The page has four, always the same: Servers, Network, Storage
and Apps, and an area with nothing in it isn't shown. Each adapter puts
its parts in its own area, such as Proxmox VE in Servers and AdGuard Home
in Network. A check goes in the area its group in the check engine is
named after, or the one the config gives that group, as in Gatus's
`areas:`. A check in no area still counts, and its problems still show.

**Apps and tools.** Apps are what the homelab hosts for the people who use
it; tools are what you run it with. Both are links on the page, in the
sidebar and in ⌘K:

```yaml
apps:
  - name: Photos                     # the only key besides url it needs
    url: https://photos.example.com
    icon: immich.svg                 # a file in overlook/icons/, one Overlook ships, or a URL
    check: Photos                    # the check that lights its dot
    description: The family's photos
    color: "#ED79B5"                 # tints its tile
tools:
  - name: Proxmox
    url: https://pve.example.com:8006
    icon: proxmox.svg
    check: Hypervisor
```

Overlook ships icons for a few common apps (`web/icons`); a file of the
same name in `icons/` beside the config wins over one of them.

**Chores** are the maintenance due while nothing is broken: the ones you
write down in the config, and the updates waiting for your merge, which the
[GitHub](#github) source reads.

```yaml
chores:
  - name: Swap the offsite backup disk # shown as written
    from: 2026-11-30                   # the day it's due from, as YYYY-MM-DD
    note: Bring the one at the office home
```

The page lists the chores due, from that day on, and those in the coming
week, by the viewer's own calendar; with none, it says which is next. Since
Overlook remembers nothing, a chore is done when you delete its lines, or
move its day on if it comes back, and restart Overlook, which reads its
config when it starts (`docker compose restart overlook`).

**Flags.** `-config` names the config (`overlook.yaml` by default), and
`icons/` and `custom.css` are read beside it; `-listen` is the address to
serve the page on (`:8080`).

## Adapters

Each adapter reads one kind of source. More come by pull request from
people who run them ([CONTRIBUTING.md](CONTRIBUTING.md)).

### Gatus

The check engine: every check, its last results and the problems Gatus
remembers. A check is a problem once it fails as many times in a row as
your alerts wait for, and stops being one when it passes.

```yaml
sources:
  gatus:
    - url: http://gatus:8080            # where Overlook reads Gatus
      link: https://status.example.com  # where the page sends you, if not the same
      threshold: 3                      # failures in a row that make a problem: as your alerts
      areas:                            # where a group not named like an area goes
        DNS: network
        Backups: storage
```

**Key:** none, as Gatus's API is open unless you protect it. If yours asks
for a login, as Gatus's `security.basic` does, give it as `user` and
`password`: `user: overlook` and `password: ${GATUS_PASSWORD}`.

**Areas:** a group named like an area, such as Servers or network, puts its
checks there. `areas` says where each other group goes, and a group it
doesn't name is in no area. **Threshold:** set it to your alerts'
`failure-threshold` (3 unless you changed it), so a problem on the page is
one your alerts announced.

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
is out, and the jobs queued. Today's figure is everyone's photos and videos,
with each person's share of the room they take.

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
last night backed up, each guest blue where it did, and the week each
night's backups. The last night is
the run with the most guests in the day to the newest backup, so a guest
backed up by hand after it doesn't hide it.

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
disc in the next seven days. Its home area is Apps. With Sonarr read too, the
films and shows share one tile in Today, the library, split by the room each
takes.

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
air in the next seven days. Its home area is Apps. With Radarr read too, its
shows share the library's tile with the films.

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
Overview, every resolver's added up, hour by hour. The Network page charts
them by the hour over the hours every resolver keeps, added up too, then shows
each one: whether it filters, its lookups, blocks and upstreams, and its top
clients, domains and blocked domains, over the interval its statistics keep.

```yaml
sources:
  adguard:
    - name: Primary DNS
      url: http://dns1.example.com
      user: overlook
      password: ${ADGUARD_PASSWORD}
    - name: Standby DNS
      url: http://dns2.example.com
      user: overlook
      password: ${ADGUARD_PASSWORD}
```

**Key:** a login, as `user` and `password`. AdGuard Home has no
read-only users, so that login could change anything; what keeps it
harmless is that Overlook only ever sends GET (ADR 0001). Give Overlook a
user of its own: add it under `users:` in `AdGuardHome.yaml`, with its
password hashed by `htpasswd -B -C 10 -n -b overlook <password>`, and
restart AdGuard Home.

**Today** starts at midnight in Overlook's time zone: set `TZ`, such as
`TZ=Europe/Brussels`, in its container. AdGuard Home keeps statistics by
the hour for an interval of up to 7 days; over a longer one it keeps them
by the day, in UTC, and the Overview shows no figure for it.

## No login

Overlook has no accounts and no login: anyone who can open the page sees
all of it. It never shows a key, but it does show your hosts, names and
what is wrong. Keep it on your LAN or behind your VPN, or put it behind
your reverse proxy's own authentication, such as forward auth to your
identity provider.

## Your own look

A `custom.css` beside the config is loaded after the page's own
stylesheet, so it can change any colour, size or font:

```css
/* overlook/custom.css */
:root {
  --m-accent-2: #3cad4b;   /* the accent, green */
  --m-content-max: 1400px; /* a wider page */
}
```

The page's colours, sizes and fonts are the `--m-` properties at the top
of [`web/style.css`](web/style.css). The page stays one page: custom.css
changes how it looks, not what it shows.

## Translating

The page speaks the language of your browser when someone has translated
it, and English for any word a translation lacks. A translation is one
file; [CONTRIBUTING.md](CONTRIBUTING.md#translating) says how.

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) says how to set up, what every change
passes and how to add an adapter. Report a vulnerability privately, as
[SECURITY.md](SECURITY.md) explains.

## Licence

Overlook is under the [MIT licence](LICENSE). The page ships the Inter
font, under the SIL Open Font License ([`web/fonts`](web/fonts)), and logos
of common apps, under the Apache License 2.0
([`web/icons`](web/icons)), each logo its owner's trademark.
