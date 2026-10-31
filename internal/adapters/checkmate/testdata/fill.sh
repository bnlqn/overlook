#!/bin/sh
# Fills the scratch Checkmate of compose.yaml with invented monitors
# (ADR 0008), in groups named Servers, Network, Storage and Media, checked
# every 20 seconds. Standby DNS checks a port nothing listens on, so it is
# down from the start; Offsite copy fails for a hundred seconds and
# recovers; Films, sent to a page the agent doesn't have, fails its last
# three checks of the five in a row its window of 10 at 50% needs; NAS
# agent, a hardware monitor read from the Capture agent, is breached once
# its memory is over 1% for five checks; Old wiki is paused. It makes the
# owner, and Overlook's user, with the role user, which reads and changes
# nothing. It takes ten and a half minutes.
set -eu
c=http://localhost:52345/api/v1
post() { curl -sf -X "$1" "$c$2" -H "Content-Type: application/json" ${token:+-H "Authorization: Bearer $token"} -d "$3"; }

token=
token=$(post POST /auth/register '{"user":{"firstName":"Sam","lastName":"Owner","email":"owner@example.com","password":"Owner-password-1"}}' | jq -r .data.token)
post POST /auth/users '{"firstName":"Overlook","lastName":"Reader","email":"overlook@example.com","password":"Overlook-password-1","role":["user"]}' >/dev/null

# monitor name group type json: makes a monitor checked every 20 seconds,
# with json's own fields, and prints its id.
monitor() {
  post POST /monitors "$(jq -cn --arg n "$1" --arg g "$2" --arg t "$3" --argjson f "$4" '{name:$n,group:$g,type:$t,interval:20000}+$f')" | jq -r .data.id
}
up='{"url":"http://localhost:52345/"}'
down='{"url":"http://127.0.0.1:1/"}'
agent='{"url":"http://capture:59232/api/v1/metrics","secret":"scratch-capture-secret"}'
monitor Hypervisor Servers http "$up" >/dev/null
monitor "Hypervisor agent" Servers hardware "$agent" >/dev/null
monitor "NAS agent" Servers hardware "$(echo "$agent" | jq -c '.memoryAlertThreshold=1')" >/dev/null
monitor Router Network ping '{"url":"127.0.0.1"}' >/dev/null
monitor "Standby DNS" Network port '{"url":"127.0.0.1","port":53}' >/dev/null
monitor "Backup target" Storage port '{"url":"mongodb","port":27017}' >/dev/null
offsite=$(monitor "Offsite copy" Storage http "$up")
monitor Photos Media http "$up" >/dev/null
films=$(monitor Films Media http "$(echo "$up" | jq -c '.statusWindowSize=10|.statusWindowThreshold=50')")
monitor "Test page" "" http "$up" >/dev/null
wiki=$(monitor "Old wiki" Apps http "$up")
post POST "/monitors/pause/$wiki" '{}' >/dev/null

sleep 180
post PATCH "/monitors/$offsite" "$down" >/dev/null
sleep 100
post PATCH "/monitors/$offsite" "$up" >/dev/null
sleep 300
# An edit sets the status window back to its default unless it's given.
post PATCH "/monitors/$films" '{"url":"http://capture:59232/films","statusWindowSize":10,"statusWindowThreshold":50}' >/dev/null
sleep 50
