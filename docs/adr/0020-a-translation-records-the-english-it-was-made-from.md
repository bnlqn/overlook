---
status: accepted
---

# A translation records the English it was made from

Each translation's messages are recorded with the English each was made from: `web/test/translated.json` holds, for every language and every key its file has, the first eight hex digits of the SHA-256 of that key's English, as written in `web/i18n/en/*.json`. `make translated` writes it from the files as they are. The page's tests read it, and fail when a translation has a message whose English has changed since, naming the language and the key, so that a translation never says what English no longer does. Whoever changes English then updates each translation of it, or deletes the key from those it can't update, so that it shows in English, and runs `make translated`. A translation may still lack keys, which show in English, so a new adapter never has to come in ten languages; the tests print each language's share of English's keys, so that a lag shows without blocking anything. A change of the maintainer's that adds or rewords English updates every translation in the same pull request, as AGENTS.md says. This amends ADR 0006: a translation that lags never blocks a release, but one that has gone wrong blocks the pull request that made it wrong, until it is updated or its key removed.

Proposed on 10 November 2026, the day ten translations came in, when the question was how they stay true to English as it changes. Decided the same day, before it was built.

## Considered options

- **Nothing recorded, as ADR 0006 has it.** Rejected: the tests already catch a key English dropped, a placeholder or markup changed, but not English reworded under the same key, and then a translation says what the page no longer means, which is worse than English.
- **Every translation required to have every key.** Rejected: an adapter's author would have to bring it in ten languages, or the maintainer would have to translate every outside pull request before it could merge, and it contradicts ADR 0006 outright.
- **A key changed whenever its English changes in meaning,** so that the old translation no longer matches and the message falls back to English. Rejected: it needs no tooling, but it rests on everyone judging what counts as a change of meaning, and it misses a quiet rewording.
- **The English recorded inside each translation file,** beside each message. Rejected: the files translators edit would double in size and carry English they must not change.
- **The record in `web/i18n`,** beside the translations. Rejected: the server embeds that folder and the demo publishes it, for a file only the tests read.
- **The record of both the English and the translation,** so that a translation changed since its English did counts as updated without a command. Rejected: a translation edited for any other reason would pass for updated, and the command is one step, whose diff a reviewer reads.
- **A scheduled workflow that opens an issue for each language's missing keys.** Rejected: the share the tests print shows the same, without issues to keep.

## Consequences

- Any change to an English message, a typo's fix too, fails the page's tests until every translation of it is updated, or deleted, or found still right, and `make translated` is run. The diff of `web/test/translated.json` shows a reviewer which keys were recorded again, and for which languages.
- An outside contributor who can't translate deletes the stale lines, which shows those messages in English until someone translates them again.
- `make translated` records whatever the files hold, so running it without looking would hide a stale message. The pull request's review is the check on that, as it is for `go.sum`.
- The page's tests print, for each language, how many of English's keys it has, such as `de: 605 of 612`.
- AGENTS.md gains the convention, and CONTRIBUTING.md's Translating says what to do when English changes.
- ADR 0006's status says that this one amends it.
