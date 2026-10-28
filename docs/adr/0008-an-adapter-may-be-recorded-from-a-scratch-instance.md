---
status: accepted
---

# An adapter may be recorded from a scratch instance

An adapter for a source nobody on the project runs may be recorded from a scratch instance: the source started from a compose file kept in the adapter's `testdata/`, filled with invented data, and its answers recorded from it. The maintainer keeps such an adapter until someone who runs the source takes it over, records it again from their own and is named in CODEOWNERS for it. This amends ADR 0005, which lets in only adapters recorded from a source someone runs, so that the sources most homelabs run needn't wait for a contributor before Overlook can read them.

Decided on 28 October 2026, when the page was made to fit homelabs larger and more varied than its author's.

## Considered options

- **Waiting for someone who runs it**, as ADR 0005 has it. Rejected for the common sources: an owner who finds their check engine or media server missing leaves, and nobody arrives to write its adapter.
- **Writing it from the source's API documentation alone.** Rejected: an answer written from a spec is a guess at the source, and the test would prove only that the adapter agrees with the guess.

## Consequences

- A scratch instance shows what the source answers, not how a real homelab uses it, so such an adapter can miss what only real data brings, such as a thousand requests or a rule in error. Its section of the README says it was recorded from a scratch instance until someone who runs it takes it over.
- Anyone can record its answers again: the compose file, and the steps that fill it with invented data, sit beside them.
