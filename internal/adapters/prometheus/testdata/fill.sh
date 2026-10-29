#!/bin/sh
# Fills the scratch Alertmanager of compose.yaml (ADR 0008): the owner
# silences Backup age for a day, while the offsite disk is swapped. Then it
# waits four minutes, so that Prometheus's history of its alerts holds Disk
# space firing twice. Prometheus's rules are rules.yml's.
set -eu
until curl -sf -o /dev/null http://localhost:9090/-/ready && curl -sf -o /dev/null http://localhost:9093/-/ready; do sleep 2; done
now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
tomorrow=$(date -u -v+1d +%Y-%m-%dT%H:%M:%SZ)
curl -sf -X POST -H "Content-Type: application/json" http://localhost:9093/api/v2/silences -d "{
  \"matchers\": [{\"name\": \"alertname\", \"value\": \"Backup age\", \"isRegex\": false, \"isEqual\": true}],
  \"startsAt\": \"$now\", \"endsAt\": \"$tomorrow\", \"createdBy\": \"owner\",
  \"comment\": \"The offsite disk is being swapped\"}" >/dev/null
sleep 240
