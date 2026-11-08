#!/bin/sh
# Records what Overlook reads from the scratch Gitea, with GET only, beside
# this script, by the path read. Its one argument is the token fill.sh
# printed.
set -eu
cd "$(dirname "$0")"
get() { # path query
  mkdir -p "$(dirname "api/v1$1")"
  curl -sf -H "Authorization: token $token" "http://localhost:3010/api/v1$1${2:+?$2}" | jq . >"api/v1$1${2:+@$2}.json"
}
token=$1
get /admin/actions/runners
for repo in homelab website; do
  get "/repos/owner/$repo/pulls" "limit=50&state=open"
  get "/repos/owner/$repo/actions/runs" "limit=50&status=failure"
done
