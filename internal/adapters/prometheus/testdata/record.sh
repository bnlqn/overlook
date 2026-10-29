#!/bin/sh
# Records what Overlook reads from the scratch Prometheus and Alertmanager,
# with GET only, beside this script: the alerting rules, the two queries of
# the ALERTS series the adapter sends, by their step, as
# query_range@step=10.json (the last 30 evaluations) and
# query_range@step=300.json (the week's pending and firing),
# and Alertmanager's alerts and silences under alertmanager/. It prints now,
# which the test reads at.
set -eu
cd "$(dirname "$0")"
p=http://localhost:9090
mkdir -p api/v1 alertmanager/api/v2
curl -sf "$p/api/v1/rules?type=alert" | jq . >api/v1/rules.json
now=$(date +%s)
last=$(jq -r '[.data.groups[].rules[].lastEvaluation] | max | sub("\\.[0-9]+"; "") | fromdateiso8601' api/v1/rules.json)
curl -sf -G "$p/api/v1/query_range" --data-urlencode 'query=max by (alertname) (ALERTS)' \
  -d "start=$((last - 290))" -d "end=$last" -d step=10 | jq . >api/v1/query_range@step=10.json
curl -sf -G "$p/api/v1/query_range" --data-urlencode 'query=max by (alertname, alertstate) (max_over_time(ALERTS[5m]))' \
  -d "start=$((now - 7 * 24 * 3600))" -d "end=$now" -d step=300 | jq . >api/v1/query_range@step=300.json
curl -sf http://localhost:9093/api/v2/alerts | jq . >alertmanager/api/v2/alerts.json
curl -sf http://localhost:9093/api/v2/silences | jq . >alertmanager/api/v2/silences.json
echo "$now"
