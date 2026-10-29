#!/bin/sh
# Records what Overlook reads from the scratch Paperless-ngx, with GET only and
# at the API's version 10, beside this script, by the path read. Its one
# argument is the token fill.sh printed. The status loses the machine's own
# names: the address of Celery's worker and the host's kernel.
set -eu
cd "$(dirname "$0")"
get() { # path query [jq filter]
  mkdir -p "$(dirname "api$1")"
  curl -sf -H "Authorization: Token $token" -H "Accept: application/json; version=10" "http://localhost:8000/api$1/${2:+?$2}" \
    | jq "${3:-.}" >"api$1.json"
}
token=$1
get /status "" '.server_os = "Linux-6.12.0-aarch64-with-glibc2.41" | .tasks.celery_url = "celery@paperless"'
get /statistics
get /tasks "status=failure&acknowledged=false&ordering=-date_created&page_size=10"
