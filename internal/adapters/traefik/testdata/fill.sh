#!/bin/sh
# Fills the scratch Traefik of compose.yaml with invented data (ADR 0008):
# a self-signed certificate for photos.example.com, good for a year, and
# one for films.example.com that expires in 10 days, then waits for films'
# health check to find its second server down.
set -eu
cd "$(dirname "$0")"
compose="docker compose -f compose.yaml"
dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
# cert name days: a self-signed certificate for name.example.com.
cert() {
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days "$2" \
    -subj "/CN=$1.example.com" -addext "subjectAltName=DNS:$1.example.com" \
    -keyout "$dir/$1.key" -out "$dir/$1.pem" 2>/dev/null
}
cert photos 365
cert films 10
for f in "$dir"/*; do $compose cp "$f" traefik:/certs/ >/dev/null 2>&1; done
# Traefik reads the certificates when it starts.
$compose restart traefik >/dev/null 2>&1
api() { curl -sf -u overlook:scratch-instance-only-not-a-password -H "Host: traefik.example.com" "http://localhost:8088/api$1"; }
until [ "$(api /http/services/films@file 2>/dev/null | jq -r '.serverStatus["http://films-standby"]')" = DOWN ]; do sleep 1; done
