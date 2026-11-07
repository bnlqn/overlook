#!/bin/sh
# Fills the scratch PBS of compose.yaml (ADR 0008): a datastore, archive, on
# its tmpfs of 1 GB, and in it one backup of 600 MB of random bytes, which
# don't shrink, made as the owner. It gives Overlook a user and a token with
# the role DatastoreAudit on the datastore, as the README says, and prints
# the token's secret for record.sh.
set -eu
pbs() { docker compose -f "$(dirname "$0")/compose.yaml" exec -T pbs "$@"; }
until curl -skf -o /dev/null https://localhost:8007/; do sleep 2; done

pbs proxmox-backup-manager datastore create archive /datastore/archive >/dev/null
pbs proxmox-backup-manager user create owner@pbs --password owner-password
pbs proxmox-backup-manager acl update /datastore/archive DatastoreBackup --auth-id owner@pbs
fingerprint=$(pbs proxmox-backup-manager cert info | sed -n 's/^Fingerprint (sha256): //p')
pbs sh -c "mkdir -p /fill && head -c 600000000 /dev/urandom >/fill/workbench.img \
  && PBS_REPOSITORY=owner@pbs@localhost:archive PBS_PASSWORD=owner-password PBS_FINGERPRINT=$fingerprint \
    proxmox-backup-client backup workbench.pxar:/fill --backup-type host --backup-id workbench" >/dev/null

pbs proxmox-backup-manager user create overlook@pbs
pbs proxmox-backup-manager acl update /datastore/archive DatastoreAudit --auth-id overlook@pbs
pbs proxmox-backup-manager user generate-token overlook@pbs overlook | sed -n 's/.*"value": *"\([^"]*\)".*/\1/p'
pbs proxmox-backup-manager acl update /datastore/archive DatastoreAudit --auth-id 'overlook@pbs!overlook'
