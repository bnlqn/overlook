#!/bin/sh
# Records what Overlook reads from the scratch Tautulli, with GET only and
# only the commands that read, beside this script: each command's answer at
# api/v2@cmd=<command>.json, since Tautulli answers every command at one
# path, set apart by its cmd.
# Its one argument is the API key fill.sh printed. It prints now, which the
# test reads at.
set -eu
cd "$(dirname "$0")"
key=$1
mkdir -p api
get() { curl -sf "http://localhost:8181/api/v2?apikey=$key&cmd=$1${2:+&$2}" | jq . >"api/v2@cmd=$1.json"; }
get get_activity
get server_status
get get_libraries
get get_history "after=$(date -u -v-2d +%Y-%m-%d)&length=1000"
get get_pms_update
date -u +%Y-%m-%dT%H:%M:%SZ
