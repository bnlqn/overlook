#!/bin/sh
# Fills the scratch Jellyfin of compose.yaml with invented data (ADR 0008):
# an owner and three people, eight films and two shows made of test
# patterns, with no metadata from the internet, three people watching, one of
# them transcoded, and two failed logins. It prints the API key it made, which
# record.sh reads with.
set -eu
j=http://localhost:8096
compose="docker compose -p overlook-jellyfin -f $(dirname "$0")/compose.yaml"
client='MediaBrowser Client="fill", Device="fill", DeviceId="fill", Version="1"'
post() { curl -sf -X POST -H "Content-Type: application/json" -H "Authorization: $client${token:+, Token=\"$token\"}" "$j$1" -d "${2:-null}"; }

# The films and shows: ten minutes of a test pattern each, small enough to
# make in seconds.
$compose exec -T jellyfin sh -c '
ff=/usr/lib/jellyfin-ffmpeg/ffmpeg
make() { mkdir -p "$(dirname "$1")"; $ff -loglevel error -y -f lavfi -i testsrc2=duration=600:size=320x180:rate=24 -f lavfi -i sine=duration=600 -c:v libx264 -preset ultrafast -crf 40 -c:a aac "$1"; }
for f in "Nosferatu (1922)" "Sherlock Jr. (1924)" "The General (1926)" "Metropolis (1927)" "Night of the Living Dead (1968)" "Big Buck Bunny (2008)" "Sintel (2010)" "Tears of Steel (2012)"; do
  make "/media/films/$f/$f.mkv"
done
for e in 01 02 03 04; do make "/media/shows/Harbour Lights/Season 01/Harbour Lights S01E$e.mkv"; done
for s in 01 02; do for e in 01 02 03; do make "/media/shows/The Allotment/Season $s/The Allotment S${s}E$e.mkv"; done; done
'

# The first run's questions, and the owner.
post /Startup/Configuration '{"UICulture":"en-GB","MetadataCountryCode":"GB","PreferredMetadataLanguage":"en"}'
curl -sf "$j/Startup/User" >/dev/null
post /Startup/User '{"Name":"owner","Password":"owner-password"}'
post /Startup/RemoteAccess '{"EnableRemoteAccess":true,"EnableAutomaticPortMapping":false}'
post /Startup/Complete
token=$(post /Users/AuthenticateByName '{"Username":"owner","Pw":"owner-password"}' | jq -r .AccessToken)

# The libraries, read from the files alone.
offline='{"LibraryOptions":{"MetadataSavers":[],"TypeOptions":[
  {"Type":"Movie","MetadataFetchers":[],"ImageFetchers":[]},
  {"Type":"Series","MetadataFetchers":[],"ImageFetchers":[]},
  {"Type":"Season","MetadataFetchers":[],"ImageFetchers":[]},
  {"Type":"Episode","MetadataFetchers":[],"ImageFetchers":[]}]}}'
post "/Library/VirtualFolders?name=Films&collectionType=movies&paths=%2Fmedia%2Ffilms&refreshLibrary=false" "$offline"
post "/Library/VirtualFolders?name=Shows&collectionType=tvshows&paths=%2Fmedia%2Fshows&refreshLibrary=false" "$offline"
post /Library/Refresh
until [ "$(curl -sf -H "Authorization: $client, Token=\"$token\"" "$j/Items/Counts" | jq '.MovieCount + .EpisodeCount')" = 18 ]; do sleep 2; done

# Three people, and Overlook's key.
for name in Alex Sam Robin; do post /Users/New "{\"Name\":\"$name\",\"Password\":\"$name-password\"}" >/dev/null; done
post "/Auth/Keys?app=Overlook"
key=$(curl -sf -H "Authorization: $client, Token=\"$token\"" "$j/Auth/Keys" | jq -r '.Items[] | select(.AppName == "Overlook") | .AccessToken')

# Robin mistypes their password twice.
for _ in 1 2; do curl -s -o /dev/null -X POST -H "Content-Type: application/json" -H "Authorization: $client" "$j/Users/AuthenticateByName" -d '{"Username":"Robin","Pw":"wrong"}'; done

# Each person plays something on a device of their own: Alex a film,
# direct, Sam an episode, paused, and Robin a film transcoded.
item() { curl -sf -H "Authorization: $client, Token=\"$token\"" "$j/Items?Recursive=true&IncludeItemTypes=$1&Fields=Path" | jq -r --arg p "$2" '[.Items[] | select(.Path | contains($p))][0].Id'; }
play() { # person device film-or-episode name paused method
  device="MediaBrowser Client=\"Jellyfin Web\", Device=\"$2\", DeviceId=\"$(printf %s "$2" | tr -d " ")\", Version=\"12.4.0\""
  t=$(curl -sf -X POST -H "Content-Type: application/json" -H "Authorization: $device" "$j/Users/AuthenticateByName" -d "{\"Username\":\"$1\",\"Pw\":\"$1-password\"}" | jq -r .AccessToken)
  id=$(item "$3" "$4")
  session=$(printf %s "$2" | tr -d ' ')
  if [ "$6" = Transcode ]; then
    # A transcode starts with the playlist and its first segment, as a
    # phone's browser asks for them.
    hls="MediaSourceId=$id&PlaySessionId=$session&DeviceId=$session&VideoCodec=h264&AudioCodec=aac&MaxWidth=320&VideoBitrate=400000&AudioBitrate=96000&SegmentContainer=ts&TranscodingMaxAudioChannels=2"
    curl -sf -H "Authorization: $device, Token=\"$t\"" "$j/Videos/$id/master.m3u8?$hls" >/dev/null
    curl -sf -H "Authorization: $device, Token=\"$t\"" "$j/Videos/$id/main.m3u8?$hls" >/dev/null
    curl -sf -H "Authorization: $device, Token=\"$t\"" "$j/Videos/$id/hls1/main/0.ts?$hls&runtimeTicks=0&actualSegmentLengthTicks=30000000" >/dev/null
  fi
  curl -sf -X POST -H "Content-Type: application/json" -H "Authorization: $device, Token=\"$t\"" "$j/Sessions/Playing" \
    -d "{\"ItemId\":\"$id\",\"MediaSourceId\":\"$id\",\"PlaySessionId\":\"$session\",\"PositionTicks\":1800000000,\"CanSeek\":true,\"IsPaused\":$5,\"PlayMethod\":\"$6\"}"
}
play Alex "Living room TV" Movie "Metropolis" false DirectPlay
play Sam "Firefox" Episode "Harbour Lights S01E03" true DirectPlay
play Robin "Phone" Movie "Sintel" false Transcode

echo "$key"
