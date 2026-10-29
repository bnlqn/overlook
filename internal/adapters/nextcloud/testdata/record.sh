#!/bin/sh
# Records what Overlook reads from the scratch Nextcloud, with GET only,
# beside this script, by the path read: its status, which needs no key, and
# serverinfo's answer, with the token fill.sh printed, its one argument. The
# answer loses the PHP extensions and caches, which say nothing about
# Nextcloud's health and are most of its size.
set -eu
cd "$(dirname "$0")"
n=http://localhost:8082
mkdir -p ocs/v2.php/apps/serverinfo/api/v1
curl -sf "$n/status.php" | jq . >status.php.json
curl -sf -H "NC-Token: $1" "$n/ocs/v2.php/apps/serverinfo/api/v1/info?format=json&skipApps=false&skipUpdate=false" \
  | jq 'del(.ocs.data.server.php.extensions, .ocs.data.server.php.opcache, .ocs.data.server.php.apcu)' >ocs/v2.php/apps/serverinfo/api/v1/info.json
