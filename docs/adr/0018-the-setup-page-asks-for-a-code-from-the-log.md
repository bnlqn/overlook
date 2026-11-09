---
status: accepted
---

# The setup page asks for a code from the log

While there is no config, Overlook makes a setup code as it starts, with `crypto/rand`: twelve characters of base32's 32, the letters A to Z and the digits 2 to 7, which leave out 0, 1 and 8 so that none is taken for O, I or B. That is 60 bits, written in three groups of four, such as `K7QM-X3RD-NW5P`. It writes the code in its log once, on the line that says there is no config, and keeps it in memory only, a new one each start (ADR 0004). The setup page asks for the code before anything else, and shows the adapters once Overlook has taken it. It keeps the code in the tab's memory, as it keeps the keys, and sends it with every try: typed once, and asked for again only after a reload, which starts the sources again too, or when a try is refused because Overlook has restarted with a new code. A try without the right code is refused before anything is read, compared in constant time, in any case and with or without its dashes. The `Origin` check stays, before it. Wrong codes are not slowed: at 10,000 guesses a second, more than a browser sends, half the codes take 1.8 million years. A website can then no longer have Overlook read an address in the homelab, not even one that points its own name at Overlook's address, because it can't read Overlook's log. This amends ADR 0014, which accepted that gap.

Proposed on 9 November 2026, as the fifth step of the road to v0.7. Decided the same day, before it was built.

## Considered options

- **The gap accepted, as ADR 0014 did,** for as long as there is no config. Rejected: closing it costs the owner one paste, and it was the only way around the setup page's promises that SECURITY.md named.
- **Eight characters, 40 bits.** The runner-up: four fewer to type. Rejected: at 10,000 guesses a second, half the codes take under two years, which is out of reach but not hopeless. Twelve, typed once, are.
- **Wrong codes slowed,** one at a time with a pause, which would let the code be shorter. Rejected: the length already puts guessing out of reach, and the pause is state shared between requests, for no gain.
- **The setup page locked after so many wrong codes,** until a restart. Rejected: any website could then lock the owner out of their own setup page.
- **Each wrong code logged.** Rejected: a website guessing would fill the log and push the code out of sight.
- **The code traded for a cookie,** so that a reload doesn't ask for it again. Rejected: a reload already starts the sources again, and the server would have to remember sessions, or sign them.
- **The code typed with each try.** Rejected: one step for every source, where one for the setup page does.
- **The code in an `Authorization` header.** Rejected: an owner who puts a login in front of Overlook, as the README suggests for a page with none, already sends one there.
- **A code the owner chooses, in the environment.** Rejected: one more thing to set before the first page, a field more on Unraid's template, and a code chosen by hand is weaker.
- **The code in a file in the config's folder.** Rejected: Overlook writes nothing, and the folder is mounted read-only (ADR 0004).
- **Only the names Overlook answers to,** the usual cure for DNS rebinding. Rejected: there is no config yet to give them, and Overlook can't tell its owner's name for it from a website's.

## Consequences

- The first start gains a step: the code from Overlook's log, such as with `docker compose logs overlook`, or from the container's log on Unraid's Docker page. The setup page says where, the README's quick start does too, and Unraid's template names the step.
- SECURITY.md loses the gap it accepted: the setup page reads an address only for someone who can read Overlook's log, who already has the host.
- MCP's gap stays, as ADR 0015 accepts it: it reads no more than `/snapshot` already gives.
- CONTEXT.md gains the setup code, which is never called a password, a token or a key, since a source has those.
- A test refuses a try without the code, with a wrong one and from another `Origin`, and takes one with the right code, typed in either case and without its dashes.
- ADR 0014's status says that this one amends it.
