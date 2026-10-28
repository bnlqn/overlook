---
status: accepted
---

# Overlook remembers nothing

Overlook has no database and no volume. Every fact on the page comes from a source at the moment the page asks: a snapshot older than 30 seconds is read again, every source in parallel, so an Overlook nobody looks at sends nothing into the homelab. History, such as when the last problem ended or what happened this week, is only shown where a source keeps it (the check engine's events, a backup server's list). A fact no source remembers isn't shown.

Decided on 28 October 2026.

## Considered options

- **Reading every source on a timer and keeping the results**, which would allow charts and history of Overlook's own. Rejected: it needs a volume, migrations and retention, and turns Overlook into a store of what every tool already stores.

## Consequences

- Deleting or replacing the container loses nothing, and upgrading never migrates data.
- "Can Overlook chart this over a month?" is answered by whether the source keeps a month.
