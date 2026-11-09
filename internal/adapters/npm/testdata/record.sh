#!/bin/sh
# Records what Overlook reads from the scratch Nginx Proxy Manager beside
# this script, by the path read: its login, as Overlook's user, the one POST
# (ADR 0009), kept as api/tokens.json with its token replaced, then, with GET
# only, the proxy hosts, redirections, 404 hosts and streams, each with its
# certificate, and whether a newer Nginx Proxy Manager is out. It prints the
# moment it read them, which the test reads at.
set -eu
cd "$(dirname "$0")"
c=http://localhost:8181/api
mkdir -p api/nginx api/version
curl -sf -X POST "$c/tokens" -H "Content-Type: application/json" \
  -d '{"identity":"overlook@example.com","secret":"Overlook-password-1"}' >login.json
token=$(jq -r .token login.json)
jq '.token = "scratch-token-not-a-real-one"' login.json >api/tokens.json && rm login.json
get() { curl -sf -H "Authorization: Bearer $token" "$c$1${2:+?$2}" | jq . >"api$1${2:+@$2}.json"; }
date -u +%Y-%m-%dT%H:%M:%SZ
for hosts in proxy-hosts redirection-hosts dead-hosts streams; do get "/nginx/$hosts" expand=certificate; done
get /version/check
