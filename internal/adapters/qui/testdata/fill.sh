#!/bin/sh
# Fills the scratch Qui of compose.yaml with invented data (ADR 0008). Each
# qBittorrent lets Qui in without a login and, once fill.sh has set it, talks
# to no one else. home seeds invented films and an episode, keeps one stopped,
# and one whose file is moved away, as when a disk isn't mounted, and waits
# for two that no peer has. Qui gets an owner, a key that only reads, and
# both qBittorrents. Its rule Finished films removes the two films in done,
# and Old episodes, in dry run, only says it would remove the episode. Then
# seedbox stops, so that Qui can't reach it. It prints the key for record.sh.
set -eu
cd "$(dirname "$0")"
compose="docker compose -p overlook-qui -f compose.yaml"
q=http://localhost:7476

# login qbittorrent: a session, with the password qBittorrent made up for
# this start, as its log gives it.
login() {
  until $compose logs "$1" | grep -q 'temporary password'; do sleep 1; done
  password=$($compose logs "$1" | sed -n 's/.*temporary password is provided for this session: //p' | tail -n 1 | tr -d '\r')
  until $compose exec -T "$1" curl -sf -c /tmp/session -d "username=admin&password=$password" \
    http://127.0.0.1:8080/api/v2/auth/login >/dev/null; do sleep 1; done
}
# qbt qbittorrent path [curl args]: qBittorrent's API, from inside it.
qbt() {
  c=$1 p=$2
  shift 2
  $compose exec -T "$c" curl -sf -b /tmp/session "http://127.0.0.1:8080/api/v2$p" "$@"
}

until curl -sf -o /dev/null "$q/health"; do sleep 1; done
for c in home seedbox; do
  login "$c"
  qbt "$c" /app/setPreferences --data-urlencode 'json={"dht":false,"pex":false,"lsd":false,"upnp":false,
    "bypass_auth_subnet_whitelist_enabled":true,"bypass_auth_subnet_whitelist":"0.0.0.0/0","save_path":"/downloads"}'
done

# seed path category: an invented file on home's disk, made into a torrent
# it seeds, in category; prints its hash.
seed() {
  $compose exec -T home sh -c "mkdir -p \"\$(dirname '$1')\" && head -c 4194304 /dev/urandom >'$1' && chown -R qbtUser:qbtUser /downloads"
  qbt home /torrentcreator/addTask --data-urlencode "sourcePath=$1" -d startSeeding=true -d private=true >/dev/null
  until qbt home /torrents/info | jq -e --arg n "$(basename "$1")" 'any(.[]; .name == $n)' >/dev/null; do sleep 1; done
  hash=$(qbt home /torrents/info | jq -r --arg n "$(basename "$1")" '.[] | select(.name == $n) | .hash')
  qbt home /torrents/setCategory -d "hashes=$hash" --data-urlencode "category=$2"
  echo "$hash"
}
for c in done films tv; do qbt home /torrents/createCategory -d "category=$c"; done
seed /downloads/films/Nosferatu.1922.1080p.mkv done >/dev/null
seed /downloads/films/Metropolis.1927.1080p.mkv done >/dev/null
seed /downloads/films/Safety.Last.1923.1080p.mkv films >/dev/null
seed /downloads/tv/Harbour.Lights.S01E01.mkv tv >/dev/null
seed /downloads/films/The.General.1926.1080p.mkv films >/dev/null
stopped=$(seed /downloads/films/Sherlock.Jr.1924.1080p.mkv films)
qbt home /torrents/stop -d "hashes=$stopped"
# The General's file moved away: qBittorrent finds it missing as it starts.
$compose exec -T home mv /downloads/films/The.General.1926.1080p.mkv /tmp/
$compose restart home >/dev/null 2>&1
login home
# Two no peer has, by hashes made up, which wait for their metadata.
for m in 'a1b2c3d4e5f60718293a4b5c6d7e8f9012345678&dn=The.Kid.1921.1080p' \
  'b2c3d4e5f60718293a4b5c6d7e8f9012345678a1&dn=Harbour.Lights.S01E02'; do
  qbt home /torrents/add --data-urlencode "urls=magnet:?xt=urn:btih:$m" >/dev/null
done

# Qui: its owner, signed in, and the two qBittorrents, with no password. A
# signed-in change must say it comes from a script, as Qui's own page does.
jar=$(mktemp)
as() { # method path [body]
  curl -sf -b "$jar" -c "$jar" -X "$1" -H "Content-Type: application/json" -H "X-Requested-With: XMLHttpRequest" \
    "$q/api$2" ${3:+-d "$3"}
}
as POST /auth/setup '{"username":"owner","password":"owner-password"}' >/dev/null
as POST /auth/login '{"username":"owner","password":"owner-password"}' >/dev/null
home=$(as POST /instances '{"name":"Home","host":"http://home:8080","username":"","password":""}' | jq .id)
as POST /instances '{"name":"Seedbox","host":"http://seedbox:8080","username":"","password":""}' >/dev/null
until as GET /instances | jq -e 'all(.[]; .connected)' >/dev/null; do sleep 1; done

# Its rules, on home, once Qui sees its eight torrents settled after its
# start, since a rule runs as soon as it's made: one removes the films in
# done, and the other, in dry run, only says what it would remove.
until as GET "/instances/$home/torrents?limit=1" | jq -e '.counts.total == 8 and .counts.status.checking == 0' >/dev/null; do
  sleep 1
done
rule() { # name category dry-run
  as POST "/instances/$home/automations" "{\"name\":\"$1\",\"trackerPattern\":\"*\",\"enabled\":true,\"dryRun\":$3,
    \"conditions\":{\"schemaVersion\":\"1\",\"delete\":{\"enabled\":true,\"mode\":\"deleteWithFiles\",
    \"condition\":{\"field\":\"CATEGORY\",\"operator\":\"EQUAL\",\"value\":\"$2\"}}}}" >/dev/null
}
rule "Finished films" done false
rule "Old episodes" tv true
until as GET "/instances/$home/automations/activity" | jq -e '[.[] | select(.outcome == "success")] | length == 2' >/dev/null &&
  as GET "/instances/$home/automations/activity" | jq -e 'any(.[]; .outcome == "dry-run")' >/dev/null; do
  sleep 1
done

# The key Overlook reads with, which reads the parts it shows and changes
# nothing.
key=$(as POST /api-keys '{"name":"Overlook","permissions":[{"area":"instances","level":"read"},
  {"area":"torrents","level":"read"},{"area":"automations","level":"read"}]}' | jq -r .key)
rm "$jar"

$compose stop seedbox >/dev/null 2>&1
echo "$key"
