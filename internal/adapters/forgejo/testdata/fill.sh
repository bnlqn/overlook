#!/bin/sh
# Fills the scratch Forgejo of compose.yaml with invented data (ADR 0008): an
# admin, owner, with two repositories, homelab and website; renovate, a user
# named like Renovate, with three pull requests open on them, and kim, a
# person, with one; two Actions runners, spare stopped once it has
# registered; and a workflow on homelab that fails, beside one that passes.
# It prints an access token of owner's, scoped read:repository and
# read:admin, for record.sh.
set -eu
cd "$(dirname "$0")"
g=http://localhost:3011
compose="docker compose -p overlook-forgejo -f compose.yaml"
# as user curl...: the API, logged in as that user.
as() {
  user=$1
  shift
  curl -sf -u "$user:$user-password-1" -H "Content-Type: application/json" "$@"
}
# file user repo branch path message text: a new file, on a new branch if
# it isn't main.
file() {
  branch=$(if [ "$3" != main ]; then echo ",\"new_branch\":\"$3\""; fi)
  as "$1" -X POST "$g/api/v1/repos/owner/$2/contents/$4" \
    -d "{\"message\":\"$5\",\"content\":\"$(printf '%s\n' "$6" | base64 | tr -d '\n')\"$branch}" >/dev/null
}
# pull user repo branch title: a pull request from branch into main.
pull() { as "$1" -X POST "$g/api/v1/repos/owner/$2/pulls" -d "{\"head\":\"$3\",\"base\":\"main\",\"title\":\"$4\"}" >/dev/null; }

until curl -sf "$g/api/v1/version" >/dev/null; do sleep 1; done
for user in owner renovate kim; do
  admin=$(if [ $user = owner ]; then echo --admin; fi)
  $compose exec -T -u git forgejo forgejo admin user create $admin --username $user --password $user-password-1 \
    --email $user@example.com --must-change-password=false >/dev/null
done
for repo in homelab website; do
  as owner -X POST "$g/api/v1/user/repos" -d "{\"name\":\"$repo\",\"auto_init\":true,\"default_branch\":\"main\"}" >/dev/null
  for user in renovate kim; do
    as owner -X PUT "$g/api/v1/repos/owner/$repo/collaborators/$user" -d '{"permission":"write"}' >/dev/null
  done
done

# The runners register; spare stops at once, and stays offline.
RUNNER_TOKEN=$($compose exec -T -u git forgejo forgejo actions generate-runner-token | tail -1)
export RUNNER_TOKEN
$compose up -d runner spare 2>/dev/null
until [ "$(as owner "$g/api/v1/admin/actions/runners" | jq length)" = 2 ]; do sleep 1; done
$compose stop spare 2>/dev/null

# The pull requests: Renovate's updates, and a person's.
file renovate homelab renovate/immich-server stacks/photos/version "Update immich-server" "v3.5.0"
pull renovate homelab renovate/immich-server "chore(deps): update ghcr.io/immich-app/immich-server docker tag to v3.5.0"
file renovate homelab renovate/gatus stacks/status/version "Update gatus" "v5.31.0"
pull renovate homelab renovate/gatus "chore(deps): update twinproduction/gatus docker tag to v5.31.0"
file renovate website renovate/astro package.version "Update astro" "7.2.1"
pull renovate website renovate/astro "fix(deps): update dependency astro to v7.2.1"
file kim website kim/garden garden.md "Add the garden page" "The garden, invented."
pull kim website kim/garden "Add the garden page"

# The workflows: one that fails, one that passes.
file owner homelab main .forgejo/workflows/lint.yaml "Lint the stacks" "on: [push]
jobs:
  lint:
    runs-on: host
    steps:
      - run: echo 'stacks/photos: an invented mistake' && exit 1
        shell: sh"
file owner homelab main .forgejo/workflows/deploy.yaml "Deploy the stacks" "on: [push]
jobs:
  deploy:
    runs-on: host
    steps:
      - run: echo 'Deployed, in a manner of speaking.'
        shell: sh"
until [ "$(as owner "$g/api/v1/repos/owner/homelab/actions/runs?status=failure" | jq '.total_count')" -ge 1 ] &&
  [ "$(as owner "$g/api/v1/repos/owner/homelab/actions/runs?status=success" | jq '.total_count')" -ge 1 ]; do sleep 2; done

# Forgejo calls a runner offline a minute after it last called.
until [ "$(as owner "$g/api/v1/admin/actions/runners" | jq -r '.[] | select(.name == "spare-runner") | .status')" = offline ]; do
  sleep 5
done
as owner -X POST "$g/api/v1/users/owner/tokens" -d '{"name":"overlook","scopes":["read:repository","read:admin"]}' | jq -r .sha1
