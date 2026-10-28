---
status: accepted
---

# Chores come from a to-do list the owner keeps

The chores the owner writes down are read from a to-do list they already keep, as the iCalendar file a CalDAV server such as Radicale, Nextcloud or Baïkal serves by GET. The owner adds, changes and marks chores done there, in any app that speaks CalDAV or through the server's own API, and the page shows what the list says the next time it reads it. Overlook never marks a chore done, and its config holds no chores.

Decided on 29 October 2026, before v0.1, when changing a chore still meant editing the config and restarting Overlook.

## Considered options

- **Overlook keeping the chores and changing them from the page.** Rejected: it needs a volume (ADR 0004) and requests that write, which ADR 0001 and ADR 0009 never allow, and, with no login, it would let anyone who reaches the page, or any site its viewer opens, change them.
- **The config, read again when it changes.** Rejected: a chore is still YAML on the host, with no app to mark it done from.
- **Chores in both the config and a to-do list.** Rejected: two places for one thing, one of them still needing a restart.

## Consequences

- "Can I tick a chore off on the page?" is answered no: mark it done on the list, and the page follows.
- An owner with no CalDAV server runs one, such as Radicale, to have chores.
- A to-do list that can't be read with a GET, such as Home Assistant's, whose items come only from a POST that isn't a login, isn't read.
- A chore that comes back is the to-do app's business: it moves the chore's due day on when it's marked done, and Overlook reads only that day.
