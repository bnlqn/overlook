# Security

Overlook holds a key for every source it reads, and some of those keys can do anything (ADR 0001). Its security rests on four promises:

- **It only reads.** The only HTTP client adapters get sends GET requests and refuses every other method.
- **Keys stay in the container.** The server reads every source; the browser gets a snapshot, never a key or a source's raw answer (ADR 0002). An error a source causes is shortened so that it never carries its URL.
- **A source's words can't run.** The page escapes everything in the snapshot and is served with a Content-Security-Policy that allows only its own scripts.
- **It serves nothing else of the config's folder** than `icons/` and `custom.css`.

A way to break any of these is a vulnerability.

## Supported versions

Until 1.0, only the latest release gets fixes.

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting: **Security**, then **Report a vulnerability**, on this repository. Don't open a public issue.

Say what an attacker can do, and how: the version, the config (without its keys), and the steps or a proof of concept. You'll get an answer within a week, and credit in the release that fixes it unless you'd rather not.
