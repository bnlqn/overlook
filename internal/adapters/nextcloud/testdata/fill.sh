#!/bin/sh
# Fills the scratch Nextcloud of compose.yaml with invented data (ADR 0008):
# three people, each with a few files of their own, a link share and a share
# between two of them, and runs its background jobs once. It gives
# serverinfo a token, which reads its answer and nothing else, and prints it
# for record.sh.
set -eu
n=http://localhost:8082
compose="docker compose -p overlook-nextcloud -f $(dirname "$0")/compose.yaml"
occ() { $compose exec -T -u www-data nextcloud php occ "$@"; }
for who in Kim Noor Sam; do
  user=$(echo "$who" | tr '[:upper:]' '[:lower:]')
  $compose exec -T -u www-data -e OC_PASS="$user-password-1" nextcloud \
    php occ user:add --password-from-env --display-name="$who" "$user" >/dev/null
done

# Each one's files, put over WebDAV as themselves.
put() { curl -sf -u "$1:$1-password-1" -X PUT "$n/remote.php/dav/files/$1/$(echo "$2" | sed 's/ /%20/g')" --data-binary "$3" >/dev/null; }
mkcol() { curl -sf -u "$1:$1-password-1" -X MKCOL "$n/remote.php/dav/files/$1/$2" >/dev/null; }
for who in kim noor sam; do mkcol "$who" Notes; done
put kim "Notes/Shopping.md" "- bread
- oranges"
put kim "Notes/Holiday ideas.md" "A week by the sea, somewhere invented."
put kim "Budget 2026.csv" "month,spent
January,812"
put noor "Notes/Reading list.md" "Three invented novels."
put noor "Recipes.md" "Soup: an onion, water, time."
put sam "Notes/Garden.md" "Plant the beans in April."
put sam "Football rota.txt" "Saturdays: Sam, Noor."

# Kim shares a folder with a link, and Noor her recipes with Sam.
curl -sf -u kim:kim-password-1 -H "OCS-APIRequest: true" -X POST "$n/ocs/v2.php/apps/files_sharing/api/v1/shares" \
  -d path=/Notes -d shareType=3 >/dev/null
curl -sf -u noor:noor-password-1 -H "OCS-APIRequest: true" -X POST "$n/ocs/v2.php/apps/files_sharing/api/v1/shares" \
  -d path=/Recipes.md -d shareType=0 -d shareWith=sam >/dev/null

# Its background jobs count the files, as cron does every five minutes.
$compose exec -T -u www-data nextcloud php -f cron.php

token=serverinfo-$(od -An -N12 -tx1 /dev/urandom | tr -d ' \n')
occ config:app:set serverinfo token --value="$token" >/dev/null
echo "$token"
