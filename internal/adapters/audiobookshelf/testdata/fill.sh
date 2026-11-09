#!/bin/sh
# Fills the scratch Audiobookshelf of compose.yaml with invented data (ADR
# 0008): an owner, two people, kim and sam, and Overlook's own admin, with
# its API key; Audiobooks, six public-domain books whose chapters are half an
# hour of silence, made here, one of them gone from the disk since its scan;
# Podcasts, two invented podcasts of the same silence, one with three
# episodes asked of a host that never answers, so that one downloads and two
# wait; what kim and sam listened to today and sam yesterday, and kim and
# the owner listening now. It prints the API key, which record.sh reads with.
set -eu
a=http://localhost:13378
compose="docker compose -p overlook-audiobookshelf -f $(dirname "$0")/compose.yaml"
# api method path [body]: Audiobookshelf's API, as whoever's token $token is.
api() { curl -sf -X "$1" -H "Authorization: Bearer $token" -H "Content-Type: application/json" "$a$2" ${3:+-d "$3"}; }
# login name: that person's token.
login() { curl -sf -X POST -H "Content-Type: application/json" "$a/login" -d "{\"username\":\"$1\",\"password\":\"$1-password\"}" | jq -r .user.accessToken; }
# total library query: how many of the library's items fit the query.
total() { api GET "/api/libraries/$1/items?limit=1&minified=1${2:+&$2}" | jq .total; }
# item library title: the id of the library's item of that title.
item() { api GET "/api/libraries/$1/items?minified=1" | jq -r --arg t "$2" '.results[] | select(.media.metadata.title == $t) | .id'; }
ms() { echo $(($(date +%s) * 1000 - $1 * 60000)); } # ms minutes: that many minutes ago, in milliseconds

until curl -sf "$a/healthcheck" >/dev/null; do sleep 1; done

# The books and episodes: chapters of half an hour of silence, copies of
# one made in seconds.
$compose exec -T audiobookshelf sh -c '
ffmpeg -loglevel error -y -f lavfi -i anullsrc=r=8000:cl=mono -t 1800 -c:a libmp3lame -q:a 9 /tmp/half-hour.mp3
book() { # author title chapters
  mkdir -p "/audiobooks/$1/$2"
  i=1; while [ $i -le $3 ]; do cp /tmp/half-hour.mp3 "/audiobooks/$1/$2/$(printf %02d $i).mp3"; i=$((i + 1)); done
}
book "Jane Austen" "Pride and Prejudice" 23
book "Arthur Conan Doyle" "The Hound of the Baskervilles" 13
book "H. G. Wells" "The Time Machine" 7
book "Mary Shelley" "Frankenstein" 17
book "Jules Verne" "Around the World in Eighty Days" 14
book "Lewis Carroll" "Alice'"'"'s Adventures in Wonderland" 5
for e in "01 Digging In" "02 Leeks in November" "03 The Shed Roof" "04 Seed Swap"; do
  mkdir -p "/podcasts/The Allotment Hour" && cp /tmp/half-hour.mp3 "/podcasts/The Allotment Hour/$e.mp3"
done
for e in "01 First Light" "02 Cold Joints"; do
  mkdir -p "/podcasts/Soldering at Night" && cp /tmp/half-hour.mp3 "/podcasts/Soldering at Night/$e.mp3"
done
chown -R 1000:1000 /audiobooks /podcasts
'

# The owner, then the libraries, read from the files alone.
curl -sf -X POST -H "Content-Type: application/json" "$a/init" -d '{"newRoot":{"username":"owner","password":"owner-password"}}' >/dev/null
token=$(login owner)
books=$(api POST /api/libraries '{"name":"Audiobooks","mediaType":"book","folders":[{"fullPath":"/audiobooks"}],"settings":{"skipMatchingMediaWithAsin":true,"skipMatchingMediaWithIsbn":true}}' | jq -r .id)
podcasts=$(api POST /api/libraries '{"name":"Podcasts","mediaType":"podcast","folders":[{"fullPath":"/podcasts"}],"settings":{"podcastSearchRegion":"gb"}}' | jq -r .id)
api POST "/api/libraries/$books/scan" >/dev/null
api POST "/api/libraries/$podcasts/scan" >/dev/null
until [ "$(total "$books")" = 6 ] && [ "$(total "$podcasts")" = 2 ]; do sleep 2; done

# Alice goes from the disk, and the next scan finds it missing.
$compose exec -T audiobookshelf rm -r "/audiobooks/Lewis Carroll"
api POST "/api/libraries/$books/scan" >/dev/null
until [ "$(total "$books" filter=issues)" = 1 ]; do sleep 2; done

# Two people, and Overlook's own admin with its key, which never expires.
for name in kim sam overlook; do
  type=$(if [ $name = overlook ]; then echo admin; else echo user; fi)
  api POST /api/users "{\"username\":\"$name\",\"password\":\"$name-password\",\"type\":\"$type\",\"isActive\":true}" >/dev/null
done
overlook=$(api GET /api/users | jq -r '.users[] | select(.username == "overlook") | .id')
key=$(api POST /api/api-keys "{\"name\":\"Overlook\",\"userId\":\"$overlook\",\"isActive\":true}" | jq -r .apiKey.apiKey)

pride=$(item "$books" "Pride and Prejudice")
hound=$(item "$books" "The Hound of the Baskervilles")
frankenstein=$(item "$books" "Frankenstein")
machine=$(item "$books" "The Time Machine")
allotment=$(item "$podcasts" "The Allotment Hour")
soldering=$(item "$podcasts" "Soldering at Night")
leeks=$(api GET "/api/items/$allotment?expanded=1" | jq -r '.media.episodes[] | select(.title | contains("Leeks")) | .id')

# What each listened to, as their phone tells Audiobookshelf once it's
# back online: kim an hour of a book today, sam half an hour of an episode
# today, and two hours of a book yesterday.
# session id item episode title seconds started-minutes-ago updated-minutes-ago
session() {
  jq -n --arg id "play_local_$1" --arg item "$2" --arg episode "$3" --arg title "$4" --argjson s "$5" \
    --argjson started "$(ms "$6")" --argjson updated "$(ms "$7")" '{
      id: $id, libraryItemId: $item, episodeId: (if $episode == "" then null else $episode end),
      mediaType: (if $episode == "" then "book" else "podcast" end), displayTitle: $title, playMethod: 3,
      mediaPlayer: "exoplayer", timeListening: $s, startTime: 0, currentTime: $s, startedAt: $started,
      updatedAt: $updated, date: ($updated / 1000 | strftime("%Y-%m-%d")), dayOfWeek: ($updated / 1000 | strftime("%A"))}'
}
phone() { echo "{\"deviceId\":\"$1-phone\",\"manufacturer\":\"$2\",\"model\":\"$3\",\"clientName\":\"Abs Android\"}"; }
token=$(login kim)
api POST /api/session/local-all "{\"deviceInfo\":$(phone kim Fairphone FP5),\"sessions\":[$(session kim1 "$pride" "" "Pride and Prejudice" 3600 80 20)]}" >/dev/null
token=$(login sam)
api POST /api/session/local-all "{\"deviceInfo\":$(phone sam Google "Pixel 9"),\"sessions\":[
  $(session sam1 "$allotment" "$leeks" "02 Leeks in November" 1800 60 30),
  $(session sam2 "$hound" "" "The Hound of the Baskervilles" 7200 1680 1560)]}" >/dev/null

# Listening now: kim a book on their phone, ten minutes in, and the owner
# another in a browser.
token=$(login kim)
id=$(api POST "/api/items/$frankenstein/play" "{\"deviceInfo\":$(phone kim Fairphone FP5),\"mediaPlayer\":\"exoplayer\",\"forceDirectPlay\":true}" | jq -r .id)
api POST "/api/session/$id/sync" '{"currentTime":600,"timeListened":600}' >/dev/null
token=$(login owner)
id=$(curl -sf -X POST -H "Authorization: Bearer $token" -H "Content-Type: application/json" \
  -H "User-Agent: Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0" "$a/api/items/$machine/play" \
  -d '{"deviceInfo":{"deviceId":"owner-browser","clientName":"Abs Web"},"mediaPlayer":"html5","forceDirectPlay":true}' | jq -r .id)
api POST "/api/session/$id/sync" '{"currentTime":300,"timeListened":300}' >/dev/null

# Three new episodes of Soldering at Night, asked of a host that never
# answers: the first downloads, the other two wait.
# episode n title: one of its feed's episodes.
episode() { echo "{\"title\":\"$2\",\"episode\":\"$1\",\"enclosure\":{\"url\":\"http://podcasts.example.com/soldering/$1.mp3\",\"type\":\"audio/mpeg\",\"length\":\"28800000\"},\"publishedAt\":$(ms $((60 * 24 * (6 - $1))))}"; }
api POST "/api/podcasts/$soldering/download-episodes" "[$(episode 3 "Flux, and Why"),$(episode 4 "The Tip Is Not the Iron"),$(episode 5 "Desoldering Braid")]" >/dev/null
until [ "$(api GET "/api/libraries/$podcasts/episode-downloads" | jq '.queue | length')" = 2 ]; do sleep 1; done

echo "$key"
