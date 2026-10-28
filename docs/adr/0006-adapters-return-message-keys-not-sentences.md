---
status: accepted
---

# Adapters return message keys, never sentences

Overlook can be translated. Every word an adapter adds to the page (a figure's label, a section's title, an event's line) is a message key with its values, such as `pbs.backed_up` with `n: 4`, and each adapter brings its English strings with it. The page picks the strings for the viewer's browser language, falls back to English for any it lacks, and uses the browser's own plural rules, so the server needs no translation library. Each translation is one file, kept by its author like an adapter (ADR 0005), and a translation that lags behind never blocks a release. Names from sources and from the config (guests, checks, apps, chores) are shown as written.

Decided on 28 October 2026, before the first adapter, because adapters that return English sentences could only be made translatable by changing every one of them.

## Considered options

- **English only.** Rejected: nothing about the page needs it, and a missing string can simply fall back to English.
- **Adapters returning English text, translated later.** Rejected: the change would touch every adapter, including ones other people keep.
