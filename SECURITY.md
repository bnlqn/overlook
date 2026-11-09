# Security

Overlook holds a key for every source it reads, and some of those keys can do anything (ADR 0001). Its security rests on four promises:

- **It only reads.** The only HTTP client adapters get sends GET requests, and a login where a source offers no other way in, and refuses every other request (ADR 0009).
- **Keys stay in the container.** The server reads every source; the browser gets a snapshot, never a key or a source's raw answer (ADR 0002), and so does an agent, through MCP, if the owner turns it on (ADR 0015). An error a source causes is shortened so that it never carries its URL.
- **A source's words can't run.** The page escapes everything in the snapshot and is served with a Content-Security-Policy that allows only its own scripts.
- **It serves nothing else of the config's folder** than `icons/` and `custom.css`.

A way to break any of these is a vulnerability.

While there is no config, Overlook serves the setup page in place of the page (ADR 0014), and it can be asked to read an address with a key, both typed into it, and say what it found, counted, or why it found nothing. It never sends the key back: the files it hands over are put together in the browser. It asks first for the setup code it wrote in its log as it started, a new one each start, and refuses a request without it, or whose `Origin` isn't its own (ADR 0018). So only someone who can read its log can have it read: a website can't, through its owner's browser, not even one that points its own name at Overlook's address (DNS rebinding).

With `mcp: true`, Overlook answers MCP at `/mcp`, with no login, as the page has none, and its tools read only the snapshot the page is drawn from. It refuses a request whose `Origin` isn't its own, so a website can't read the homelab through its owner's browser. A website that points its own name at Overlook's address, DNS rebinding, gets around that, and here it is accepted: it reads no more than `/snapshot` already gives it.

## Checking a release

From v0.2.0 on, GitHub attests every release: a signed record that the binaries and the image were built from this repository by its release workflow, and from which commit. Before you give Overlook your keys, check what you run:

```bash
gh attestation verify overlook_0.6.0_linux_amd64.tar.gz --repo bnlqn/overlook
gh attestation verify oci://ghcr.io/bnlqn/overlook:0.6.0 --repo bnlqn/overlook
```

Each archive also comes with an SBOM, its name followed by `.sbom.json`, which lists everything built into the binary.

## Supported versions

Until 1.0, only the latest release gets fixes.

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting: **Security**, then **Report a vulnerability**, on this repository. Don't open a public issue.

Say what an attacker can do, and how: the version, the config (without its keys), and the steps or a proof of concept. You'll get an answer within a week, and credit in the release that fixes it unless you'd rather not.
