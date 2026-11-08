#!/bin/sh
# Fills the scratch Portainer of compose.yaml with invented data (ADR 0008):
# its two Docker engines as environments, the first running a few containers,
# one unhealthy, one that exited with an error and one done with its job, and
# a stack of two, and the second stopped once Portainer has seen it. It makes
# Overlook a user of its own, which isn't an administrator, with both
# environments, and prints that user's access token for record.sh.
set -eu
p=http://localhost:9000
compose="docker compose -p overlook-portainer -f $(dirname "$0")/compose.yaml"
json() { curl -sf -H "Authorization: Bearer $jwt" -H "Content-Type: application/json" "$p/api$1" -X "$2" -d "$3"; }

until curl -sf "$p/api/system/status" >/dev/null; do sleep 1; done
for engine in media backups; do
  until $compose exec -T "$engine" docker info >/dev/null 2>&1; do sleep 1; done
done
curl -sf "$p/api/users/admin/init" -H "Content-Type: application/json" \
  -d '{"Username":"owner","Password":"owner-password-1"}' >/dev/null
jwt=$(curl -sf "$p/api/auth" -H "Content-Type: application/json" \
  -d '{"username":"owner","password":"owner-password-1"}' | jq -r .jwt)

for engine in media backups; do
  curl -sf "$p/api/endpoints" -H "Authorization: Bearer $jwt" \
    -F Name="$engine" -F EndpointCreationType=1 -F URL="tcp://$engine:2375" >/dev/null
done

# The media engine's containers, started on it directly.
run() { $compose exec -T media docker run -q "$@" >/dev/null; }
run -d --name wiki --restart unless-stopped nginx:1.29-alpine
run -d --name feeds --health-cmd false --health-interval 5s --health-retries 1 nginx:1.29-alpine
run -d --name cache redis:8.2-alpine
run --name nightly-sync busybox:1.37 sh -c 'exit 3' || true
run --name db-migrate busybox:1.37 true
# A stack Portainer deploys, of two containers.
json "/stacks/create/standalone/string?endpointId=1" POST "$(jq -n '{Name: "recipes",
  StackFileContent: "services:\n  web:\n    image: nginx:1.29-alpine\n  db:\n    image: redis:8.2-alpine\n"}')" >/dev/null
# The backups engine runs two, until it stops.
$compose exec -T backups docker run -q -d --name restic-rest nginx:1.29-alpine >/dev/null
$compose exec -T backups docker run -q -d --name offsite-sync busybox:1.37 sleep 1d >/dev/null

# Overlook's user, with both environments, and no role, as Portainer CE has
# none that only reads.
user=$(json /users POST '{"Username":"overlook","Password":"overlook-password-1","Role":2}' | jq .Id)
for e in 1 2; do
  json "/endpoints/$e" PUT "{\"UserAccessPolicies\":{\"$user\":{\"RoleId\":0}}}" >/dev/null
done

# Portainer sees both engines as they are, then the backups engine stops, and
# its next look finds it down.
sleep 6 # until feeds fails its health check
json /endpoints/snapshot POST '' >/dev/null
$compose stop backups >/dev/null 2>&1
json /endpoints/snapshot POST '' >/dev/null

# The user's access token, made as itself, which asks for its password.
jwt=$(curl -sf "$p/api/auth" -H "Content-Type: application/json" \
  -d '{"username":"overlook","password":"overlook-password-1"}' | jq -r .jwt)
json "/users/$user/tokens" POST '{"description":"overlook","password":"overlook-password-1"}' | jq -r .rawAPIKey
