#!/bin/sh
# Records what Overlook reads from the scratch Audiobookshelf, with GET only,
# beside this script, by the path read and the pairs of its query that set
# it apart: the libraries, the sessions open and the newest hundred, and,
# for each library, its items with issues and, for one of podcasts, the
# episodes waiting to download. Its one argument is the API key fill.sh
# printed. A session loses its chapters, tracks and metadata, and its device
# its address, none of which is read.
set -eu
cd "$(dirname "$0")"
get() { # path query file-pairs [jq filter]
  mkdir -p "$(dirname ".$1")"
  curl -sf -H "Authorization: Bearer $key" "http://localhost:13378$1${2:+?$2}" | jq "${4:-.}" >".$1${3:+@$3}.json"
}
key=$1
slim='del(.sessions[] | .chapters, .audioTracks, .mediaMetadata, .deviceInfo.ipAddress)'
get /api/libraries include=stats include=stats
get /api/sessions/open "" "" "$slim"
get /api/sessions "itemsPerPage=100&sort=updatedAt&desc=1" "desc=1&itemsPerPage=100&sort=updatedAt" "$slim"
for id in $(jq -r '.libraries[].id' api/libraries@include=stats.json); do
  get "/api/libraries/$id/items" "filter=issues&limit=1&minified=1" filter=issues
done
for id in $(jq -r '.libraries[] | select(.mediaType == "podcast") | .id' api/libraries@include=stats.json); do
  get "/api/libraries/$id/episode-downloads" "" ""
done
