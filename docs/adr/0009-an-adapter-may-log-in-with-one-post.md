---
status: accepted
---

# An adapter may log in with one POST

Overlook sends a source only GET requests, with one exception: an adapter whose source offers no other way in may declare one login path, and `internal/fetch` sends that path a single POST carrying only the credentials from the config. The session it gets back is kept in memory and sent with GETs until the source refuses it, and only then is it fetched again. Any other POST, and any POST to another path, is refused as before. This amends ADR 0001, whose guard is what keeps Overlook's keys harmless, so that Pi-hole, which 37% of homelabs run and which gives a session only for a POST, can be read.

Decided on 28 October 2026, when the most-run homelab tools were checked for what Overlook could read.

## Considered options

- **GET only**, as ADR 0001 has it. Rejected: Pi-hole, qBittorrent before 5.2 and others stay unreadable, and an owner whose DNS is Pi-hole sees an empty Network.
- **The owner logs in by hand and gives Overlook the session.** Rejected: Pi-hole forgets a session left idle for 30 minutes, so the page would go dark whenever nobody looked at it.
- **Any POST an adapter asks for.** Rejected: the guard would become a promise again, not a rule.

## Consequences

- The promise becomes "only GET, and a login where a source offers no other way in". A login changes nothing in the homelab but a session.
- The session lives in memory only, so Overlook still remembers nothing (ADR 0004), and an Overlook read every 30 seconds logs in once, not every time, so it doesn't fill the source's list of sessions.
- A key that can do anything is still a reason to leave a source out: Vaultwarden's admin token could be read with a login, but it can delete every user, for little more than a version on the page.
