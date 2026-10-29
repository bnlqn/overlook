#!/bin/sh
# Fills the scratch Grafana of compose.yaml with invented alert rules
# (ADR 0008), each over a number from Grafana's own test data, in folders
# named Servers, Network, Storage and Apps, evaluated every 10 seconds.
# Disk space fires and clears every other minute; Packet loss is pending
# for an hour; Offsite sync gets no data; the rest are normal. It
# makes Overlook a service account with the Viewer role and prints its
# token for record.sh.
set -eu
g=http://localhost:3000
api() { curl -sf -u owner:owner-password -H "Content-Type: application/json" -H "X-Disable-Provenance: true" -X "$1" "$g/api$2" -d "$3"; }

ds=$(api POST /datasources '{"name":"Test data","type":"grafana-testdata-datasource","access":"proxy"}' | jq -r .datasource.uid)
folder() { api POST /folders "{\"title\":\"$1\"}" | jq -r .uid; }
servers=$(folder Servers)
network=$(folder Network)
storage=$(folder Storage)
apps=$(folder Apps)

# rule uid title folder group threshold for model: fires while the test
# data model gives a number over threshold, for as long as for.
rule() {
  api POST /v1/provisioning/alert-rules "{
    \"uid\":\"$1\",\"title\":\"$2\",\"folderUID\":\"$3\",\"ruleGroup\":\"$4\",\"orgID\":1,
    \"condition\":\"C\",\"for\":\"$6\",\"noDataState\":\"NoData\",\"execErrState\":\"Error\",
    \"annotations\":{\"summary\":\"$2 is over $5\"},
    \"data\":[
      {\"refId\":\"A\",\"datasourceUid\":\"$ds\",\"relativeTimeRange\":{\"from\":600,\"to\":0},\"model\":$7},
      {\"refId\":\"B\",\"datasourceUid\":\"__expr__\",\"model\":{\"refId\":\"B\",\"type\":\"reduce\",\"expression\":\"A\",\"reducer\":\"last\"}},
      {\"refId\":\"C\",\"datasourceUid\":\"__expr__\",\"model\":{\"refId\":\"C\",\"type\":\"threshold\",\"expression\":\"B\",
       \"conditions\":[{\"evaluator\":{\"type\":\"gt\",\"params\":[$5]}}]}}]}" >/dev/null
}
# A number that stays as it is, and one that is high for a minute and low
# for the next, as Grafana's test data pulses.
steady() { echo "{\"refId\":\"A\",\"scenarioId\":\"csv_metric_values\",\"stringInput\":\"$1\"}"; }
pulse='{"refId":"A","scenarioId":"predictable_pulse","pulseWave":{"timeStep":60,"onCount":1,"offCount":1,"onValue":96,"offValue":72}}'
rule disk-space "Disk space" "$servers" hosts 90 0s "$pulse"
rule hypervisor-memory "Hypervisor memory" "$servers" hosts 90 0s "$(steady 41)"
rule packet-loss "Packet loss" "$network" links 5 1h "$(steady 7)"
rule router-temperature "Router temperature" "$network" links 70 0s "$(steady 12)"
rule offsite-sync "Offsite sync" "$storage" backups 0 0s '{"refId":"A","scenarioId":"no_data_points"}'
rule photos-errors "Photos errors" "$apps" services 10 0s "$(steady 3)"
for f in "$servers hosts" "$network links" "$storage backups" "$apps services"; do
  set -- $f
  rules=$(curl -sf -u owner:owner-password "$g/api/v1/provisioning/folder/$1/rule-groups/$2" | jq -c '.rules')
  api PUT "/v1/provisioning/folder/$1/rule-groups/$2" "{\"interval\":10,\"rules\":$rules}" >/dev/null
done

# Four minutes, so that Disk space has fired and cleared twice.
sleep 240

# Overlook's service account, a Viewer, and its token.
sa=$(api POST /serviceaccounts '{"name":"Overlook","role":"Viewer"}' | jq .id)
api POST "/serviceaccounts/$sa/tokens" '{"name":"overlook"}' | jq -r .key
