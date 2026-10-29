#!/bin/sh
# Records what Overlook reads from the scratch Seerr, with GET only, beside
# this script, by the path read. Its one argument is the API key fill.sh
# printed. A film's or a show's answer is TMDB's whole page of it, cast and
# all, so it keeps the fields Overlook reads: its title and its year.
set -eu
cd "$(dirname "$0")"
key=$1
get() { # path [jq filter]
  mkdir -p "$(dirname "api/v1$1")"
  curl -sf -H "X-Api-Key: $key" "http://localhost:5055/api/v1$1" | jq "${2:-.}" >"api/v1$(echo "$1" | cut -d'?' -f1).json"
}
get /status
get /request/count
get /issue/count
get "/request?filter=pending&sort=added&take=20"
for m in $(jq -r '.results[].media | "\(.mediaType)/\(.tmdbId)"' api/v1/request.json); do
  get "/$m" '{id, title, releaseDate, name, firstAirDate} | with_entries(select(.value != null))'
done
