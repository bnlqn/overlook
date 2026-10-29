#!/bin/sh
# Fills the scratch Plex and Tautulli of compose.yaml (ADR 0008): five films
# and a show of three episodes, made of test patterns, in libraries that read
# only the files; Tautulli pointed at the Plex server; four plays finished,
# then two left going, the living-room TV's as it is and Kim's phone's
# transcoded. Plex is unclaimed, so every play is its local user's. It
# prints Tautulli's API key for record.sh.
set -eu
here=$(dirname "$0")
compose="docker compose -p overlook-tautulli -f $here/compose.yaml"
p=http://localhost:32400
t=http://localhost:8181

docker run --rm -v overlook-tautulli_media:/media alpine:3.22.2 sh -c '
apk add -q ffmpeg
make() { mkdir -p "$(dirname "$1")"; ffmpeg -loglevel error -y -f lavfi -i testsrc2=duration=1200:size=320x180:rate=24 -f lavfi -i sine=duration=1200 -c:v libx264 -preset ultrafast -crf 40 -c:a aac "$1"; }
for f in "Nosferatu (1922)" "The General (1926)" "Metropolis (1927)" "Night of the Living Dead (1968)" "Sintel (2010)"; do make "/media/films/$f/$f.mkv"; done
for e in 01 02 03; do make "/media/shows/Harbour Lights/Season 01/Harbour Lights - s01e$e.mkv"; done'

curl -sf -X POST "$p/library/sections?name=Films&type=movie&agent=tv.plex.agents.none&scanner=Plex%20Movie&language=xn&location=%2Fmedia%2Ffilms" >/dev/null
curl -sf -X POST "$p/library/sections?name=Shows&type=show&agent=tv.plex.agents.none&scanner=Plex%20TV%20Series&language=xn&location=%2Fmedia%2Fshows" >/dev/null
until [ "$(curl -sf -H 'Accept: application/json' "$p/library/sections/2/all?type=4" | jq '.MediaContainer.size')" = 3 ]; do sleep 2; done

# Tautulli reads the Plex server at its address on the compose network, and
# logs a play from ten seconds on. It writes its config as it stops, so the
# config is changed while it's stopped.
id=$(curl -sf -H 'Accept: application/json' "$p/identity" | jq -r .MediaContainer.machineIdentifier)
$compose stop tautulli >/dev/null 2>&1
docker run --rm -v overlook-tautulli_tautulli:/config alpine:3.22.2 sed -i \
  -e 's|^pms_ip = .*|pms_ip = 172.31.32.10|' -e "s|^pms_identifier = .*|pms_identifier = $id|" \
  -e 's|^pms_url = .*|pms_url = http://172.31.32.10:32400|' -e 's|^pms_name = .*|pms_name = Home|' \
  -e 's|^first_run_complete = .*|first_run_complete = 1|' -e 's|^logging_ignore_interval = .*|logging_ignore_interval = 10|' \
  /config/config.ini
$compose start tautulli >/dev/null 2>&1
key=$($compose exec -T tautulli sed -n 's/^api_key = //p' /config/config.ini)
until curl -sf "$t/api/v2?apikey=$key&cmd=server_status" | jq -e '.response.data.connected' >/dev/null; do sleep 2; done

item() { curl -sf -H 'Accept: application/json' "$p/library/sections/$1/all?type=$2" | jq -r --arg t "$3" '.MediaContainer.Metadata[] | select(.title == $t) | .ratingKey'; }
# play client device item minutes-in: the client streams the item as it is,
# and says where it's got to, as a player does.
play() {
  part=$(curl -sf -H 'Accept: application/json' "$p/library/metadata/$3" | jq -r '.MediaContainer.Metadata[0].Media[0].Part[0].key')
  curl -s --limit-rate 20k -o /dev/null -H "X-Plex-Client-Identifier: $1" -H "X-Plex-Device-Name: $2" -H 'X-Plex-Product: Plex HTPC' \
    -H 'X-Plex-Platform: Linux' -H "X-Plex-Session-Identifier: $1" "$p$part" &
  timeline "$1" "$2" "$3" playing "$4"
}
timeline() { # client device item state minutes-in
  curl -sf -o /dev/null -H "X-Plex-Client-Identifier: $1" -H "X-Plex-Device-Name: $2" -H 'X-Plex-Product: Plex HTPC' -H 'X-Plex-Platform: Linux' \
    -H "X-Plex-Session-Identifier: $1" "$p/:/timeline?ratingKey=$3&key=%2Flibrary%2Fmetadata%2F$3&state=$4&time=$(($5 * 60000))&duration=1200000"
}
for f in "Nosferatu (1922)" "The General (1926)" "Night of the Living Dead (1968)"; do
  rk=$(item 1 1 "$f")
  play study-laptop "Study laptop" "$rk" 2
  sleep 20
  timeline study-laptop "Study laptop" "$rk" playing 19
  timeline study-laptop "Study laptop" "$rk" stopped 20
  sleep 5
done
rk=$(item 2 4 "Episode 1")
play study-laptop "Study laptop" "$rk" 1
sleep 20
timeline study-laptop "Study laptop" "$rk" stopped 20
sleep 5

# The two still going.
play living-room-tv "Living room TV" "$(item 1 1 "Metropolis (1927)")" 4
phone() {
  curl -sf -H 'Accept: application/json' -H 'X-Plex-Client-Identifier: kims-phone' -H "X-Plex-Device-Name: Kim's phone" \
    -H 'X-Plex-Product: Plex Web' -H 'X-Plex-Platform: Chrome' -H 'X-Plex-Session-Identifier: kims-phone' "$p$1"
}
sintel=$(item 1 1 "Sintel (2010)")
phone "/video/:/transcode/universal/start.m3u8?path=%2Flibrary%2Fmetadata%2F$sintel&mediaIndex=0&partIndex=0&protocol=hls&directPlay=0&directStream=0&location=lan&maxVideoBitrate=300&videoResolution=320x180&session=kims-phone" >/dev/null
phone "/video/:/transcode/universal/session/kims-phone/base/index.m3u8" >/dev/null
phone "/video/:/transcode/universal/session/kims-phone/base/00000.ts" >/dev/null
phone "/:/timeline?ratingKey=$sintel&key=%2Flibrary%2Fmetadata%2F$sintel&state=playing&time=300000&duration=1200000" >/dev/null
until [ "$(curl -sf "$t/api/v2?apikey=$key&cmd=get_activity" | jq -r .response.data.stream_count)" = 2 ]; do sleep 2; done
echo "$key"
