#!/bin/sh
# Fills the scratch Pi-hole of compose.yaml with invented lookups (ADR 0008):
# it names four invented clients, denies two invented domains, then has each
# client, a container of its own on the compose network, look up a mix of
# domains, some of them denied. The living-room TV asks more than the 20
# lookups a minute it allows, so Pi-hole rate-limits it and says so.
set -eu
p=http://localhost:8081
sid=$(curl -sf -X POST "$p/api/auth" -d '{"password":"owner-password"}' | jq -r .session.sid)
api() { curl -sf -X "$1" -H "X-FTL-SID: $sid" "$p/api$2" ${3:+-d "$3"} >/dev/null; }

api PUT "/config/dns/hosts/172.31.53.11%20kims-laptop"
api PUT "/config/dns/hosts/172.31.53.12%20living-room-tv"
api PUT "/config/dns/hosts/172.31.53.13%20noors-phone"
api PUT "/config/dns/hosts/172.31.53.14%20thermostat"
api PATCH /config '{"config":{"dns":{"rateLimit":{"count":20,"interval":60}}}}'
for d in ads.example.com telemetry.example.net; do
  api POST /domains/deny/exact "{\"domain\":\"$d\",\"comment\":\"invented\"}"
done

ask() { # address domain...
  ip=$1; shift
  docker run --rm --network overlook-pihole_default --ip "$ip" busybox:1.37 \
    sh -c 'for d in "$@"; do nslookup "$d" 172.31.53.2 >/dev/null 2>&1 || true; done' sh "$@"
}
ask 172.31.53.11 photos.example.com photos.example.com mail.example.org ads.example.com docs.example.org photos.example.com news.example.net ads.example.com
ask 172.31.53.12 stream.example.net stream.example.net telemetry.example.net stream.example.net telemetry.example.net telemetry.example.net \
  stream.example.net telemetry.example.net stream.example.net stream.example.net telemetry.example.net stream.example.net
ask 172.31.53.13 mail.example.org chat.example.org ads.example.com chat.example.org photos.example.com
ask 172.31.53.14 telemetry.example.net time.example.org telemetry.example.net
