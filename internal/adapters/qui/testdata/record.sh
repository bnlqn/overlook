#!/bin/sh
# Records what Overlook reads from the scratch Qui, with GET only, beside
# this script, by the path read. Its one argument is the key fill.sh printed.
# It first waits until Qui finds seedbox gone. qBittorrent's external
# address, which it learns from its peers, is left out of what's recorded.
# It prints the moment it recorded at, which the week's events depend on.
set -eu
cd "$(dirname "$0")"
key=$1
get() { # path [query pair] [jq filter]
  mkdir -p "$(dirname "api$1")"
  curl -sf -G -H "X-API-Key: $key" "http://localhost:7476/api$1" ${2:+--data-urlencode "$2"} | jq "${3:-.}" >"api$1.json"
}
until curl -sf -H "X-API-Key: $key" http://localhost:7476/api/instances | jq -e 'any(.[]; .connected | not)' >/dev/null; do
  sleep 1
done
get /instances '' 'map(.resolvedPublicPeerAddress = "")'
get /version
for id in $(jq '.[] | select(.isActive and .connected) | .id' api/instances.json); do
  get "/instances/$id/torrents" 'filters={"status":["errored"]}' \
    'del(.serverState.last_external_address_v4, .serverState.last_external_address_v6)'
  get "/instances/$id/automations"
  get "/instances/$id/automations/activity" 'limit=1000'
done
date -u +%Y-%m-%dT%H:%M:%SZ
