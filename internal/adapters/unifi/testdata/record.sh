#!/bin/sh
# Records what Overlook reads from a UniFi OS console's first site, by the
# path read, GETs only. It keeps only the fields the adapter reads, and
# invents every ID, every device's name and every MAC address, so nothing of
# the console's own is left but its models, versions and numbers. Read the
# result before you commit it.
#
#   UNIFI=192.0.2.1 CA=unifi.pem KEY=~/.config/overlook/unifi.key ./record.sh
#
# UNIFI is the console's address, CA its certificate, which signs itself for
# unifi.local, and KEY a file that holds an API key from the Network
# application's Integrations. The key goes to curl on its input, never in
# its arguments, where other users could see it.
set -eu
cd "$(dirname "$0")"
key=$(tr -d '\r\n' <"$KEY")
get() {
	printf 'X-API-KEY: %s\n' "$key" |
		curl -sf --cacert "$CA" --connect-to "unifi.local:443:$UNIFI:443" -H @- "https://unifi.local/$1"
}
# id is the made-up ID of the nth thing, the same in jq as here.
id() { printf '00000000-0000-4000-8000-%012d' "$1"; }
v=proxy/network/integration/v1

sites=$(get "$v/sites")
real=$(echo "$sites" | jq -r '.data[0].id')
ref=$(echo "$sites" | jq -r '.data[0].internalReference')
site=$(id 1)
mkdir -p "$v/sites/$site"
echo "$sites" | jq --arg id "$site" '.count = 1 | .totalCount = 1 | .data = [.data[0] | {id: $id, internalReference, name}]' >"$v/sites.json"

get "$v/pending-devices" |
	jq '.data |= [to_entries[] | {model: .value.model, macAddress: "00:00:5e:00:53:\(10 + .key)"}]' >"$v/pending-devices.json"

devices=$(get "$v/sites/$real/devices?limit=200")
echo "$devices" | jq '
	def kind: if (.features | index("accessPoint")) then "ap"
		elif (.model | test("^(UCG|UDM|UDR|UXG|UX)")) then "gateway" else "switch" end;
	["hall", "study", "attic", "garden", "kitchen", "garage", "landing", "cellar", "porch", "loft"] as $rooms
	| .data |= [to_entries[] | .key as $n | .value | {
		id: ("00000000-0000-4000-8000-" + ("00000000000" + (100 + $n | tostring))[-12:]),
		name: (if kind == "gateway" then "gateway" else "\(kind)-\($rooms[$n % 10])" end),
		model, state, firmwareVersion, firmwareUpdatable, features, supported
	}]' >"$v/sites/$site/devices@limit=200.json"
echo "$devices" | jq -r '.data | to_entries[] | select(.value.state == "ONLINE") | "\(.key) \(.value.id)"' |
	while read -r n device; do
		d="$v/sites/$site/devices/$(id $((100 + n)))/statistics"
		mkdir -p "$d"
		get "$v/sites/$real/devices/$device/statistics/latest" | jq . >"$d/latest.json"
	done

get "$v/sites/$real/clients?limit=0" | jq . >"$v/sites/$site/clients@limit=0.json"
for type in WIRED WIRELESS; do
	q="filter=type.eq%28%27$type%27%29&limit=0"
	get "$v/sites/$real/clients?$q" | jq . >"$v/sites/$site/clients@$q.json"
done

# The older API, which nothing documents: how the WANs are, and whether the
# Network application has an update.
o=proxy/network/api/s/$ref/stat
mkdir -p "$o"
get "$o/health" | jq '.data |= [.[] | select(.subsystem == "wan" or .subsystem == "www") | {subsystem, status} +
	if .subsystem == "wan" then {"gw_system-stats": {uptime: .["gw_system-stats"].uptime}, uptime_stats}
	else {latency, uptime, speedtest_status, speedtest_lastrun, speedtest_ping, xput_down, xput_up} end]' >"$o/health.json"
get "$o/sysinfo" | jq '.data |= [.[0] | {version, update_available, update_downloaded}]' >"$o/sysinfo.json"
