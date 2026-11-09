#!/bin/sh
# Records what Overlook reads from the scratch Traefik of compose.yaml, with
# GET only, beside this script, by the path read: the HTTP routers, services
# and middlewares and the certificates, each list whole on one page. It
# reads through the API's own router, as traefik.example.com, with the
# basicAuth middleware's login. The basicAuth middleware's users are left
# out, since they hold its password's hash.
set -eu
cd "$(dirname "$0")"
get() { # path [jq filter]
  mkdir -p "$(dirname "api$1")"
  curl -sf -u overlook:scratch-instance-only-not-a-password -H "Host: traefik.example.com" \
    "http://localhost:8088/api$1?per_page=10000" | jq "${2:-.}" >"api$1@per_page=10000.json"
}
get /http/routers
get /http/services
get /http/middlewares 'map(if .basicAuth then .basicAuth.users = [] else . end)'
get /certificates
