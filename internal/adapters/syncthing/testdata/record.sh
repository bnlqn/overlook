#!/bin/sh
# Records what Overlook reads from home, the scratch Syncthing's device that
# compose.yaml opens, with GET only, beside this script, by the path read:
# each folder's state but a paused one's, the failures of a folder that has
# them, and each other device's completion. The upgrade's answer is recorded
# whole, since the Docker image, built without upgrades, refuses it.
set -eu
cd "$(dirname "$0")"
key=scratch-instance-only-not-a-key
get() { # path [file]
  file=${2:-${1#/}}
  mkdir -p "$(dirname "$file")"
  curl -sf -H "X-API-Key: $key" "http://localhost:8384$1" | jq . >"$file.json"
}
get /rest/system/version
get /rest/system/error
get /rest/system/connections
get /rest/stats/device
get /rest/config/folders
get /rest/config/devices
get /rest/cluster/pending/devices
get /rest/cluster/pending/folders
curl -si -H "X-API-Key: $key" http://localhost:8384/rest/system/upgrade | sed '/^Date:/d; /^Expires:/d' >rest/system/upgrade.http
for f in $(jq -r '.[] | select(.paused | not) | .id' rest/config/folders.json); do
  get "/rest/db/status?folder=$f" "rest/db/status@folder=$f"
  if [ "$(jq .pullErrors "rest/db/status@folder=$f.json")" -gt 0 ]; then
    get "/rest/folder/errors?folder=$f" "rest/folder/errors@folder=$f"
  fi
done
for d in $(jq -r '.connections | keys[]' rest/system/connections.json); do
  get "/rest/db/completion?device=$d" "rest/db/completion@device=$d"
done
