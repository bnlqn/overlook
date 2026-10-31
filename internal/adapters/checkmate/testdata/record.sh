#!/bin/sh
# Records what Overlook reads from the scratch Checkmate beside this script,
# by the path read: its login, as Overlook's user, the one POST (ADR 0009),
# then, with GET only, every monitor with its recent checks, each hardware
# monitor's own last 30, which the list leaves out, and the open incidents
# and the others of the last month. It prints the moment it read them,
# which the test reads at.
set -eu
cd "$(dirname "$0")"
c=http://localhost:52345/api/v1
mkdir -p api/v1/auth api/v1/monitors/team api/v1/incidents api/v1/checks
curl -sf -X POST "$c/auth/login" -H "Content-Type: application/json" \
  -d '{"email":"overlook@example.com","password":"Overlook-password-1"}' | jq . >api/v1/auth/login.json
token=$(jq -r .data.token api/v1/auth/login.json)
get() { curl -sf -H "Authorization: Bearer $token" "$c$1" | jq .; }
date -u +%Y-%m-%dT%H:%M:%SZ
get /monitors/team/with-checks >api/v1/monitors/team/with-checks.json
for id in $(jq -r '.data.monitors[] | select(.type == "hardware" and .isActive) | .id' api/v1/monitors/team/with-checks.json); do
  get "/checks/$id?type=hardware&sortOrder=desc&dateRange=day&page=0&rowsPerPage=30" >"api/v1/checks/$id.json"
done
get "/incidents/team?status=true&sortOrder=desc&page=0&rowsPerPage=1000" >"api/v1/incidents/team@status=true.json"
get "/incidents/team?status=false&dateRange=month&sortOrder=desc&page=0&rowsPerPage=1000" >"api/v1/incidents/team@status=false.json"
