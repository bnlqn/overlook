#!/bin/sh
# Records what Overlook reads from the scratch Uptime Kuma, with GET only,
# beside this script, by the path read: the status page fill.mjs made, and
# its monitors' results. Uptime Kuma serves those a minute at a time.
set -eu
cd "$(dirname "$0")"
mkdir -p api/status-page/heartbeat
curl -sf http://localhost:3001/api/status-page/homelab | jq . >api/status-page/homelab.json
curl -sf http://localhost:3001/api/status-page/heartbeat/homelab | jq . >api/status-page/heartbeat/homelab.json
