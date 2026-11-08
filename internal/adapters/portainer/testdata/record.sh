#!/bin/sh
# Records what Overlook reads from the scratch Portainer, with GET only,
# beside this script, by the path read. Its one argument is the access token
# fill.sh printed. Each environment's snapshot loses what Docker itself said
# of its engine (DockerSnapshotRaw), which names the machine, is most of the
# answer and isn't read.
set -eu
cd "$(dirname "$0")"
get() { # path [jq filter]
  mkdir -p "$(dirname "api$1")"
  curl -sf -H "X-API-Key: $key" "http://localhost:9000/api$1" | jq "${2:-.}" >"api$1.json"
}
key=$1
get /endpoints 'map(.Snapshots |= map(del(.DockerSnapshotRaw)))'
get /system/version
