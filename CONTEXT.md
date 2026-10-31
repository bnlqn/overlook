# Overlook

A page that tells a homelab's owner what is wrong, how each area of the
homelab is, and what it did today and this week. It reads the tools the
homelab already runs and changes nothing.

## Language

**Homelab**:
The machines, network and services someone hosts for themselves, which
Overlook looks over.
_Avoid_: Lab, infrastructure, stack

**Owner**:
Whoever runs the homelab and reads Overlook. Overlook has no accounts, so
every viewer is an owner.
_Avoid_: User (the people an app serves are its users), admin

**Host**:
One machine the homelab runs on, physical or virtual, that a source reports
on as a whole, such as a Proxmox host, a NAS or a VM that runs containers.
The Servers area holds the hosts.
_Avoid_: Server (the area), node, system, box

**Guest**:
A virtual machine or container a host runs. A guest can be a host too, when
a source reports on it as a whole, as Beszel does for a VM.
_Avoid_: Workload, instance

### What Overlook reads

**Source**:
One running tool Overlook reads, at its address and with its key or login,
such as a Proxmox host or an AdGuard Home resolver.
_Avoid_: Service, integration, endpoint

**Adapter**:
The part of Overlook that reads the sources of one program, such as AdGuard
Home, and gives back their facts in Overlook's own shapes. Two AdGuard Home
resolvers are two sources read by one adapter. Each adapter has a fixed home
area, but a check engine's checks go in every area, and chores go in none.
_Avoid_: Widget, plugin, integration

**Kind**:
What a source is for, whichever adapter reads it: a check engine, a to-do
list, a resolver or a media server. A source has at most one.
_Avoid_: Type, category, role

**Check engine**:
A source that runs the homelab's checks and sends its alerts, such as Gatus,
Uptime Kuma, Grafana or Prometheus. A homelab may have more than one.
Overlook never runs a check or sends an alert itself.
_Avoid_: Monitor, uptime tool

**Check**:
One test a check engine repeats on a schedule, each result passing or
failing, such as a Gatus endpoint, an Uptime Kuma monitor or a Grafana alert
rule.
_Avoid_: Monitor, probe, endpoint

**To-do list**:
A source that keeps the chores the owner writes down, such as a list on
Radicale or Nextcloud. Overlook never marks a chore done itself.
_Avoid_: Calendar, task list

**Resolver**:
A source that answers the homelab's DNS lookups and blocks the ones its
lists name, such as AdGuard Home or Pi-hole. A homelab may have more than
one, and their lookups add up.
_Avoid_: DNS server, ad blocker

**Media server**:
A source that plays the homelab's films, shows and music, or tells what one
plays, such as Jellyfin, Emby, or Plex through Tautulli. Their plays add up.
_Avoid_: Streaming server, Plex (for all of them)

**Snapshot**:
Every source's facts as Overlook read them at one moment. The page is drawn
from it.
_Avoid_: State, cache

### What the page shows

**Page**:
What Overlook draws for a homelab, in the same shape for every one: what is
wrong, how each area is, and what the homelab did today and this week, with
a page of its own for each area. The config says which sources fill it,
never where anything goes.
_Avoid_: Dashboard, start page, homepage

**Area**:
One of the four fixed parts of a homelab: Servers, Network, Storage and Apps.
An area with nothing in it isn't shown.
_Avoid_: Group, section, category

**Problem**:
A check that has failed long enough for its check engine to alert on it. It
lasts exactly as long as the check keeps failing, from its first failure to
its next pass, and is never dismissed.
_Avoid_: Alert (the push that announces a problem), incident, issue

**Degraded**:
An area that still works while it has a problem, such as DNS with one of two
resolvers down. An area that doesn't work is down, one whose checks have no
problem is up, and one with no checks is unknown, since nothing says whether
it works. The homelab is down when an area is down or it has check engines
and none answered, degraded with any other problem, one in a check in no
area too, unknown with no checks, and up otherwise.
_Avoid_: Warning, partial outage

**Quiet**:
The stretch since the homelab's last problem ended.
_Avoid_: Uptime, streak

**Section**:
A titled list of rows an adapter adds to its home area's page, such as the
guests on a Proxmox host.
_Avoid_: Card, panel, widget

**Chart**:
What a source counts or measures by the hour, over as many hours as it
keeps, drawn on its adapter's home area's page, such as a resolver's lookups
and the blocked ones, or a host's processor. Several sources' charts of the
same count are added up into one, and those of the same measure share one, a
line each, both over the hours they all keep.
_Avoid_: Graph, plot, time series

**Figure**:
A number an adapter reports for today, such as lookups blocked or photos
uploaded, sometimes with the hours that make it up or the shares it splits
into, such as each person's photos. Figures of one count from several
sources, such as two hosts' guests, are added up into one, split by source,
its hours over those they all keep; figures of one whole, such as films and
shows in the library, share a tile.
_Avoid_: Stat, metric, KPI

**Event**:
Something the homelab did this week, as a source remembers it, such as a
night's backups or films added.
_Avoid_: Activity, log entry

**Chore**:
Something the owner does while nothing is broken: one they keep on a to-do
list, an update waiting to be merged or to be installed from inside its tool,
a battery to change, a device waiting to be adopted into the tool that
runs it, or a request waiting for approval, such as a film someone asked
for in Seerr.
It is done in the source it waits in, never on the page, and is never a
problem.
_Avoid_: Todo, task, clear (for done)

**App**:
A service the homelab hosts for the people who use it, such as the photos or
the films. The page links to each app and shows whether it answers.
_Avoid_: Service, bookmark

**Tool**:
Something the owner uses to run or watch the homelab, such as Proxmox or the
check engine. The page links to each tool like an app.
_Avoid_: Admin app, service
