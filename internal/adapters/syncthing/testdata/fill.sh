#!/bin/sh
# Fills the scratch Syncthing of compose.yaml with invented data (ADR 0008).
# home shares Photos with laptop and nas, Documents with laptop and Music,
# paused, with nas, and keeps Videos on a path it can't make. laptop fills them with invented files, a folder of home's
# Documents is made one it can't write, nas stops and misses the newest
# photos, phone asks to join home, and laptop offers it a folder, Recipes.
# Every device talks only to the others, by name: no discovery, relay or
# report leaves the machine.
set -eu
compose="docker compose -p overlook-syncthing -f $(dirname "$0")/compose.yaml"
key=scratch-instance-only-not-a-key
# api device method path [body]: Syncthing's REST API, from inside device.
api() {
  $compose exec -T "$1" curl -sf -X "$2" -H "X-API-Key: $key" -H "Content-Type: application/json" \
    "http://127.0.0.1:8384/rest$3" ${4:+-d "$4"}
}
# put device path text: a file in one of device's folders, as Syncthing's user.
put() { $compose exec -T -u 1000 "$1" sh -c "mkdir -p \"\$(dirname '$2')\" && printf '%s\n' '$3' >'$2'"; }

for d in home laptop nas phone; do
  until api "$d" GET /system/status >/dev/null 2>&1; do sleep 1; done
done
home=$(api home GET /system/status | jq -r .myID)
laptop=$(api laptop GET /system/status | jq -r .myID)
nas=$(api nas GET /system/status | jq -r .myID)
phone=$(api phone GET /system/status | jq -r .myID)

for d in home laptop nas phone; do
  api "$d" PATCH /config/options '{"globalAnnounceEnabled":false,"localAnnounceEnabled":false,"relaysEnabled":false,
    "natEnabled":false,"urAccepted":-1,"crashReportingEnabled":false}' >/dev/null
  id=$(api "$d" GET /system/status | jq -r .myID)
  api "$d" PATCH "/config/devices/$id" "{\"name\":\"$d\"}" >/dev/null
done
# device: an entry for another device, reached by its name.
device() { echo "{\"deviceID\":\"$1\",\"name\":\"$2\",\"addresses\":[\"tcp://$2:22000\"]}"; }
api home POST /config/devices "$(device "$laptop" laptop)" >/dev/null
api home POST /config/devices "$(device "$nas" nas)" >/dev/null
for d in laptop nas phone; do api "$d" POST /config/devices "$(device "$home" home)" >/dev/null; done

# folder id label paused device...: a folder shared with those devices.
folder() {
  id=$1 label=$2 paused=$3
  shift 3
  devices=$(for d in "$@"; do printf '{"deviceID":"%s"},' "$d"; done)
  echo "{\"id\":\"$id\",\"label\":\"$label\",\"path\":\"/var/syncthing/$label\",\"paused\":$paused,
    \"rescanIntervalS\":10,\"devices\":[${devices%,}]}"
}
api home POST /config/folders "$(folder photos Photos false "$laptop" "$nas")" >/dev/null
api home POST /config/folders "$(folder documents Documents false "$laptop")" >/dev/null
api home POST /config/folders "$(folder music Music true "$nas")" >/dev/null
# Videos, on a path home can't make, stops before its first scan.
api home POST /config/folders '{"id":"videos","label":"Videos","path":"/srv/videos","devices":[]}' >/dev/null
api laptop POST /config/folders "$(folder photos Photos false "$home")" >/dev/null
api laptop POST /config/folders "$(folder documents Documents false "$home")" >/dev/null
api nas POST /config/folders "$(folder photos Photos false "$home")" >/dev/null
api nas POST /config/folders "$(folder music Music false "$home")" >/dev/null

# laptop's invented files, and nas's music.
for n in 01 02 03 04 05 06; do put laptop "/var/syncthing/Photos/2026/Beach $n.jpg" "An invented photo, number $n."; done
put laptop /var/syncthing/Documents/Letters/Water.txt "An invented water bill."
put laptop /var/syncthing/Documents/Taxes/2025.txt "Invented taxes for 2025."
put nas "/var/syncthing/Music/Invented Band/Track 1.flac" "Not really music."
# have device file: whether device has synced the file.
have() { $compose exec -T "$1" test -f "$2"; }
until have home /var/syncthing/Documents/Taxes/2025.txt && have home "/var/syncthing/Photos/2026/Beach 06.jpg" &&
  have nas "/var/syncthing/Photos/2026/Beach 06.jpg"; do sleep 2; done

# Home's Taxes becomes a folder it can't write, so the next tax file fails.
$compose exec -T home sh -c 'chown root /var/syncthing/Documents/Taxes && chmod 555 /var/syncthing/Documents/Taxes'
put laptop /var/syncthing/Documents/Taxes/2026.txt "Invented taxes for 2026."
# nas stops, then laptop takes two more photos, which nas misses.
sleep 5
$compose stop nas >/dev/null 2>&1
for n in 07 08; do put laptop "/var/syncthing/Photos/2026/Beach $n.jpg" "An invented photo, number $n."; done
# laptop offers home a folder, and phone asks to join.
api laptop POST /config/folders "$(folder recipes Recipes false "$home")" >/dev/null
put laptop /var/syncthing/Recipes/Soup.txt "An onion, water, time."
until [ "$(api home GET /folder/errors?folder=documents | jq '.errors | length')" -gt 0 ] &&
  [ "$(api home GET /cluster/pending/devices | jq length)" -gt 0 ] &&
  [ "$(api home GET /cluster/pending/folders | jq length)" -gt 0 ]; do sleep 2; done
