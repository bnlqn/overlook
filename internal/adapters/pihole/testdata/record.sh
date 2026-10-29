#!/bin/sh
# Records what Overlook reads from the scratch Pi-hole beside this script,
# by the path read: one login, which it keeps as api/auth.json with its
# session replaced, then GETs only. An answer whose query sets it apart
# from another of the same path is at <path>@<query>.json.
set -eu
cd "$(dirname "$0")"
p=http://localhost:8081
mkdir -p api/stats api/dns api/info
curl -sf -X POST "$p/api/auth" -d '{"password":"owner-password"}' >login.json
sid=$(jq -r .session.sid login.json)
jq '.session.sid = "0123abcd0123abcd0123ab=" | .session.csrf = "abcd0123abcd0123abcd01="' login.json >api/auth.json && rm login.json
get() { curl -sf -H "X-FTL-SID: $sid" "$p/api$1${2:+?$2}" | jq . >"api$1${3:+@$2}.json"; }
get /stats/summary
get /history
get /dns/blocking
get /stats/top_clients count=10
get /stats/top_domains count=10
get /stats/top_domains "blocked=true&count=10" query
get /stats/upstreams
get /info/version
get /info/messages
