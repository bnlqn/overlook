---
status: accepted
---

# Adapters are compiled in, and kept by their authors

Every adapter lives in the one binary and the config switches it on; there is no plugin system. Overlook's maintainer keeps the core and the adapters for the sources they run. Anyone else's adapter comes in by pull request with recorded answers from the real source, the test that turns them into Overlook's shapes, a config example and the least key it needs, and its author is named in CODEOWNERS for it. An adapter left broken for three months with nobody to fix it is removed in a minor release.

Decided on 28 October 2026.

## Considered options

- **Go plugins.** Rejected: they must be built with the exact same toolchain and dependencies as the binary, which breaks with every release.
- **Adapters as WebAssembly modules or separate processes.** Rejected: a plugin interface is a public contract kept forever, and loading and sandboxing them is a product of its own.
- **The maintainer keeping every adapter.** Rejected: nobody can fix an adapter for a source they don't run.
