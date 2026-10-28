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

### What Overlook reads

**Source**:
One running tool Overlook reads, at its address and with its key, such as a
Proxmox host or an AdGuard Home resolver.
_Avoid_: Service, integration, endpoint

**Adapter**:
The part of Overlook that reads one kind of source and gives back its facts
in Overlook's own shapes. Two AdGuard Home resolvers are two sources read by
one adapter. Each adapter has a fixed home area.
_Avoid_: Widget, plugin, integration

**Check engine**:
The source that runs the homelab's checks and sends its alerts, such as
Gatus. Overlook never runs a check or sends an alert itself.
_Avoid_: Monitor, uptime tool

**Check**:
One test the check engine repeats on a schedule, each result passing or
failing.
_Avoid_: Monitor, probe, endpoint

**Snapshot**:
Every source's facts as Overlook read them at one moment. The page is drawn
from it.
_Avoid_: State, cache

### What the page shows

**Area**:
One of the four fixed parts of a homelab: Servers, Network, Storage and Apps.
An area with nothing in it isn't shown.
_Avoid_: Group, section, category

**Problem**:
A check that has failed long enough for the check engine to alert on it. It
lasts exactly as long as the check keeps failing and is never dismissed.
_Avoid_: Alert (the push that announces a problem), incident, issue

**Degraded**:
An area that still works while it has a problem, such as DNS with one of two
resolvers down. An area that doesn't work is down; one with no problem is up.
_Avoid_: Warning, partial outage

**Quiet**:
The stretch since the homelab's last problem ended.
_Avoid_: Uptime, streak

**Section**:
A titled list of rows an adapter adds to its home area's page, such as the
guests on a Proxmox host.
_Avoid_: Card, panel, widget

**Chart**:
What a source counts by the hour, over as many hours as it keeps, drawn on
its adapter's home area's page, such as a resolver's lookups and the blocked
ones. Several sources' charts of the same count are drawn as one.
_Avoid_: Graph, plot, time series

**Figure**:
A number an adapter reports for today, such as lookups blocked or photos
uploaded, sometimes with the hours that make it up or the shares it splits
into, such as each person's photos. Figures of one whole from several
sources, such as films and shows in the library, share a tile.
_Avoid_: Stat, metric, KPI

**Event**:
Something the homelab did this week, as a source remembers it, such as a
night's backups or films added.
_Avoid_: Activity, log entry

**Chore**:
Maintenance due while nothing is broken: a dated chore the owner wrote down,
or an update waiting to be merged. A chore is never a problem.
_Avoid_: Todo, task

**App**:
A service the homelab hosts for the people who use it, such as the photos or
the films. The page links to each app and shows whether it answers.
_Avoid_: Service, bookmark

**Tool**:
Something the owner uses to run or watch the homelab, such as Proxmox or the
check engine. The page links to each tool like an app.
_Avoid_: Admin app, service
