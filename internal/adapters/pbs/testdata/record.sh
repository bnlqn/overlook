#!/bin/sh
# Records the datastore's status from the scratch PBS, with GET only, beside
# this script, by the path read: the room the datastore takes and has. Its
# one argument is the secret fill.sh printed. The backups in
# api2/json/admin/datastore/archive/snapshots.json came from a real PBS, and
# this leaves them as they are, but records the scratch one's where they are
# missing, as in the weekly recording, which starts with no answers.
set -eu
cd "$(dirname "$0")"
secret=$1
get() { # name
  curl -skf -H "Authorization: PBSAPIToken=overlook@pbs!overlook:$secret" \
    "https://localhost:8007/api2/json/admin/datastore/archive/$1" | jq . >"api2/json/admin/datastore/archive/$1.json"
}
mkdir -p api2/json/admin/datastore/archive
get status
[ -f api2/json/admin/datastore/archive/snapshots.json ] || get snapshots
