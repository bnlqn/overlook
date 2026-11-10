#!/bin/sh
# Records what Overlook reads from the scratch Emby, with GET only, beside
# this script, by the path read. Its one argument is the API key fill.sh
# printed.
set -eu
cd "$(dirname "$0")"
get() { # path [query]
  mkdir -p "$(dirname ".$1")"
  curl -sf -H "X-Emby-Token: $key" "http://localhost:8096$1${2:+?$2}" | jq . >".$1.json"
}
key=$1
get /System/Info
get /Sessions activeWithinSeconds=960
get /Items/Counts
get /ScheduledTasks isHidden=false
get /System/ActivityLog/Entries "minDate=$(jq -rn "now - 86400 | todate")"
