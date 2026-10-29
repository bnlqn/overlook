#!/bin/sh
# Records what Overlook reads from the scratch Home Assistant, with GET only,
# beside this script, by the path read. Its one argument is the token fill.sh
# printed. The states lose the people and where they are, which the demo
# devices name, and the cameras' and players' tokens.
set -eu
cd "$(dirname "$0")"
get() { # path [jq filter]
  mkdir -p "$(dirname ".$1")"
  curl -sf -H "Authorization: Bearer $token" "http://localhost:8123$1" | jq "${2:-.}" >".$1.json"
}
token=$1
get /api/config
get /api/states '[.[] | select(.entity_id | test("^(person|device_tracker|image_processing)\\.") | not)]
  | walk(if type == "object" and has("access_token") then .access_token = "0123abcd" else . end)
  | walk(if type == "string" then sub("token=[0-9a-f]+"; "token=0123abcd") else . end)'
