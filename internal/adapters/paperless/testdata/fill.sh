#!/bin/sh
# Fills the scratch Paperless-ngx of compose.yaml with invented letters
# (ADR 0008): an inbox tag, three correspondents and ten letters as text,
# three of them left in the inbox, then a PDF cut short, which fails. It
# makes Overlook a user that can see the system's health, the statistics and
# everyone's tasks, as staff, and no document, and prints its token for
# record.sh.
set -eu
p=http://localhost:8000
auth="owner:owner-password"
post() { curl -sf -u "$auth" -H "Content-Type: application/json" "$p/api$1" -d "$2"; }
upload() { # file title
  curl -sf -u "$auth" "$p/api/documents/post_document/" -F "document=@$1" -F "title=$2" >/dev/null
}

inbox=$(post /tags/ '{"name":"Inbox","is_inbox_tag":true,"color":"#2c7be5"}' | jq .id)
post /tags/ '{"name":"Bills","color":"#e67e22"}' >/dev/null
for c in "Harbour Water" "Northgate Council" "Example Energy"; do
  post /correspondents/ "{\"name\":\"$c\"}" >/dev/null
done

letters=$(mktemp -d)
i=0
for title in "Water bill, October" "Council tax, 2026" "Energy bill, September" "Energy bill, October" \
  "Boiler service" "Car insurance renewal" "Library card" "School trip form" "Dentist reminder" "Garden waste permit"; do
  i=$((i + 1))
  printf '%s\n\nDear Owner,\n\nThis invented letter, number %s, is about: %s.\nAmount due: %s.00 GBP.\n\nYours sincerely,\nA made-up sender\n' \
    "$title" "$i" "$title" "$((i * 17))" >"$letters/letter-$i.txt"
  upload "$letters/letter-$i.txt" "$title"
done
until [ "$(curl -sf -u "$auth" "$p/api/statistics/" | jq .documents_total)" = 10 ]; do sleep 2; done

# Seven are read and filed; three wait in the inbox.
ids=$(curl -sf -u "$auth" "$p/api/documents/?ordering=id&page_size=7&fields=id" | jq -c '[.results[].id]')
post /documents/bulk_edit/ "{\"documents\":$ids,\"method\":\"remove_tag\",\"parameters\":{\"tag\":$inbox}}" >/dev/null

# A scan cut short fails.
printf '%%PDF-1.7\n1 0 obj << /Type /Catalog' >"$letters/scan.pdf"
upload "$letters/scan.pdf" "Scan from the printer"
until [ "$(curl -sf -u "$auth" "$p/api/tasks/?status=failure" | jq .count)" = 1 ]; do sleep 2; done
rm -r "$letters"

# Overlook's user, and its token.
post /users/ '{"username":"overlook","password":"overlook-password","is_active":true,"is_staff":true,"user_permissions":["view_global_statistics","view_paperlesstask"]}' >/dev/null
post /token/ '{"username":"overlook","password":"overlook-password"}' | jq -r .token
