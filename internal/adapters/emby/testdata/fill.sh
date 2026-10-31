#!/bin/sh
# Fills the scratch Emby of compose.yaml with invented data (ADR 0008): an
# owner and three people, eight films and two shows made of test patterns,
# with no metadata from the internet, three people watching, one of them
# transcoded, and two failed logins. It prints the API key it made, which
# record.sh reads with.
set -eu
j=http://localhost:8096
compose="docker compose -p overlook-emby -f $(dirname "$0")/compose.yaml"
client='Emby Client="fill", Device="fill", DeviceId="fill", Version="1"'
post() { curl -sf -X POST -H "Content-Type: application/json" -H "X-Emby-Authorization: $client" ${token:+-H "X-Emby-Token: $token"} "$j$1" -d "${2:-null}"; }

until curl -sf "$j/System/Info/Public" >/dev/null; do sleep 2; done

# The films and shows: ten minutes of a test pattern each, small enough to
# make in seconds.
$compose exec -T emby sh -c '
make() { mkdir -p "$(dirname "$1")"; /bin/ffmpeg -loglevel error -y -f lavfi -i testsrc2=duration=600:size=320x180:rate=24 -f lavfi -i sine=duration=600 -c:v libx264 -preset ultrafast -crf 40 -c:a aac "$1"; }
for f in "Nosferatu (1922)" "Sherlock Jr. (1924)" "The General (1926)" "Metropolis (1927)" "Night of the Living Dead (1968)" "Big Buck Bunny (2008)" "Sintel (2010)" "Tears of Steel (2012)"; do
  make "/media/films/$f/$f.mkv"
done
for e in 01 02 03 04; do make "/media/shows/Harbour Lights/Season 01/Harbour Lights S01E$e.mkv"; done
for s in 01 02; do for e in 01 02 03; do make "/media/shows/The Allotment/Season $s/The Allotment S${s}E$e.mkv"; done; done
'

# The first run's questions, and the owner.
post /Startup/Configuration '{"UICulture":"en-GB","MetadataCountryCode":"GB","PreferredMetadataLanguage":"en"}'
curl -sf -H "X-Emby-Authorization: $client" "$j/Startup/User" >/dev/null
post /Startup/User '{"Name":"owner","Password":"owner-password"}' >/dev/null
post /Startup/Complete
token=$(post /Users/AuthenticateByName '{"Username":"owner","Pw":"owner-password"}' | jq -r .AccessToken)

# The libraries, read from the files alone.
offline='{"LibraryOptions":{"EnableInternetProviders":false,"MetadataSavers":[],"TypeOptions":[
  {"Type":"Movie","MetadataFetchers":[],"ImageFetchers":[]},
  {"Type":"Series","MetadataFetchers":[],"ImageFetchers":[]},
  {"Type":"Season","MetadataFetchers":[],"ImageFetchers":[]},
  {"Type":"Episode","MetadataFetchers":[],"ImageFetchers":[]}]}}'
post "/Library/VirtualFolders?name=Films&collectionType=movies&paths=%2Fmedia%2Ffilms&refreshLibrary=false" "$offline"
post "/Library/VirtualFolders?name=Shows&collectionType=tvshows&paths=%2Fmedia%2Fshows&refreshLibrary=false" "$offline"
post /Library/Refresh
until [ "$(curl -sf -H "X-Emby-Token: $token" "$j/Items/Counts" | jq '.MovieCount + .EpisodeCount')" = 18 ]; do sleep 2; done

# Three people, and Overlook's key.
for name in Alex Sam Robin; do
  id=$(post /Users/New "{\"Name\":\"$name\"}" | jq -r .Id)
  post "/Users/$id/Password" "{\"Id\":\"$id\",\"CurrentPw\":\"\",\"NewPw\":\"$name-password\"}"
done
curl -sf -X POST -H "X-Emby-Token: $token" "$j/Auth/Keys?App=Overlook"
key=$(curl -sf -H "X-Emby-Token: $token" "$j/Auth/Keys" | jq -r '.Items[] | select(.AppName == "Overlook") | .AccessToken')

# Robin mistypes their password twice, on their phone.
phone='Emby Client="Emby Web", Device="Phone", DeviceId="Phone", Version="4.10.2.0"'
for _ in 1 2; do curl -s -o /dev/null -X POST -H "Content-Type: application/json" -H "X-Emby-Authorization: $phone" "$j/Users/AuthenticateByName" -d '{"Username":"Robin","Pw":"wrong"}'; done

# Each person plays something on a device of their own: Alex a film,
# direct, Sam an episode, paused, and Robin a film transcoded. Emby works
# out the play method itself, so Robin's transcode is started for real, with
# the playlist and its first segment, as a phone's browser asks for them.
item() { curl -sf -H "X-Emby-Token: $token" "$j/Items?Recursive=true&IncludeItemTypes=$1&Fields=Path" | jq -r --arg p "$2" '[.Items[] | select(.Path | contains($p))][0].Id'; }
play() { # person device film-or-episode name paused method
  device="Emby Client=\"Emby Web\", Device=\"$2\", DeviceId=\"$(printf %s "$2" | tr -d " ")\", Version=\"4.10.2.0\""
  t=$(curl -sf -X POST -H "Content-Type: application/json" -H "X-Emby-Authorization: $device" "$j/Users/AuthenticateByName" -d "{\"Username\":\"$1\",\"Pw\":\"$1-password\"}" | jq -r .AccessToken)
  id=$(item "$3" "$4")
  ps=$(printf %s "$2" | tr -d ' ')
  if [ "$6" = Transcode ]; then
    info=$(curl -sf -X POST -H "Content-Type: application/json" -H "X-Emby-Authorization: $device" -H "X-Emby-Token: $t" "$j/Items/$id/PlaybackInfo?MaxStreamingBitrate=400000&StartTimeTicks=0&IsPlayback=true" \
      -d '{"DeviceProfile":{"MaxStreamingBitrate":400000,"DirectPlayProfiles":[],"TranscodingProfiles":[{"Container":"ts","Type":"Video","VideoCodec":"h264","AudioCodec":"aac","Protocol":"hls","Context":"Streaming","MaxAudioChannels":"2"}]}}')
    ps=$(echo "$info" | jq -r .PlaySessionId)
    main=$(curl -sf -H "X-Emby-Authorization: $device" "$j$(echo "$info" | jq -r '.MediaSources[0].TranscodingUrl')" | grep -v '^#' | head -1)
    segment=$(curl -sf -H "X-Emby-Authorization: $device" "$j/videos/$id/$main" | grep -v '^#' | head -1)
    curl -sf -o /dev/null -H "X-Emby-Authorization: $device" "$j/videos/$id/$segment"
  fi
  curl -sf -X POST -H "Content-Type: application/json" -H "X-Emby-Authorization: $device" -H "X-Emby-Token: $t" "$j/Sessions/Playing" \
    -d "{\"ItemId\":\"$id\",\"MediaSourceId\":\"mediasource_$id\",\"PlaySessionId\":\"$ps\",\"PositionTicks\":1800000000,\"CanSeek\":true,\"IsPaused\":$5,\"PlayMethod\":\"$6\"}"
}
play Alex "Living room TV" Movie "Metropolis" false DirectPlay
play Sam "Firefox" Episode "Harbour Lights S01E03" true DirectPlay
play Robin "Phone" Movie "Sintel" false Transcode

echo "$key"
