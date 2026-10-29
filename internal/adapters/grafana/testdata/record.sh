#!/bin/sh
# Records what Overlook reads from the scratch Grafana, with GET only, beside
# this script, by the path read: its alert rules, and each one's state
# history over the week before now, at <path>@<query>.json. Its one argument
# is the token fill.sh printed. It prints now, in milliseconds, which the
# test reads at.
set -eu
cd "$(dirname "$0")"
g=http://localhost:3000
get() { curl -sf -H "Authorization: Bearer $token" "$g$1${2:+?$2}" | jq .; }
token=$1
now=$(($(date +%s) * 1000))
from=$((now - 7 * 24 * 3600 * 1000))
mkdir -p api/prometheus/grafana/api/v1 api/v1/rules
get /api/prometheus/grafana/api/v1/rules >api/prometheus/grafana/api/v1/rules.json
for uid in $(jq -r '.data.groups[].rules[].uid' api/prometheus/grafana/api/v1/rules.json); do
  get /api/v1/rules/history "from=$from&ruleUID=$uid" >"api/v1/rules/history@from=$from&ruleUID=$uid.json"
done
echo "$now"
