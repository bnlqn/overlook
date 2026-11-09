#!/bin/sh
# Fills the scratch Nginx Proxy Manager of compose.yaml with invented hosts
# (ADR 0008). Photos and Films are proxy hosts with certificates, Films'
# expiring in 12 days; Wiki points at an address nothing answers on, which
# nginx can't tell; DNS has a typo in its advanced config, which nginx
# refuses; Grafana is disabled. Blog redirects, Old answers 404, and two
# streams forward Git's SSH and WireGuard. The certificates are self-signed
# and uploaded, since Let's Encrypt needs a real domain. It makes Overlook's
# user, who isn't an admin and sees every item, with View on the hosts and
# streams, and certificates and access lists hidden: a host still brings its
# certificate's name and expiry, but no certificate's key.
set -eu
compose="docker compose -f $(dirname "$0")/compose.yaml"
c=http://localhost:8181/api
token=$(curl -sf -X POST "$c/tokens" -H "Content-Type: application/json" \
  -d '{"identity":"owner@example.com","secret":"Owner-password-1"}' | jq -r .token)
# send method path json: the owner's request, printing the answer's id.
send() {
  curl -sf -X "$1" "$c$2" -H "Authorization: Bearer $token" -H "Content-Type: application/json" ${3:+-d "$3"} | jq -r '.id? // empty'
}

user=$(send POST /users '{"name":"Overlook Reader","nickname":"Overlook","email":"overlook@example.com","roles":[],
  "is_disabled":false,"auth":{"type":"password","secret":"Overlook-password-1"}}')
send PUT "/users/$user/permissions" '{"visibility":"all","proxy_hosts":"view","redirection_hosts":"view",
  "dead_hosts":"view","streams":"view","certificates":"hidden","access_lists":"hidden"}' >/dev/null

# certificate name domain days: a self-signed certificate for domain,
# lasting days, made in the container and uploaded, printing its id.
certificate() {
  id=$(send POST /nginx/certificates "{\"provider\":\"other\",\"nice_name\":\"$1\"}")
  $compose exec -T npm sh -c "openssl req -x509 -newkey rsa:2048 -nodes -days $3 -subj '/CN=$2' \
    -keyout /tmp/key.pem -out /tmp/cert.pem 2>/dev/null &&
    curl -sf http://localhost:81/api/nginx/certificates/$id/upload -H 'Authorization: Bearer $token' \
      -F certificate=@/tmp/cert.pem -F certificate_key=@/tmp/key.pem >/dev/null && rm /tmp/key.pem /tmp/cert.pem"
  echo "$id"
}
wildcard=$(certificate Wildcard '*.example.com' 365)
films=$(certificate Films films.example.com 12)

# proxy names host port json: a proxy host for names, a JSON list, to
# host:port over HTTP, with json's own fields, printing its id.
proxy() {
  send POST /nginx/proxy-hosts "$(jq -cn --argjson n "$1" --arg h "$2" --argjson p "$3" --argjson f "$4" \
    '{domain_names:$n,forward_scheme:"http",forward_host:$h,forward_port:$p}+$f')"
}
proxy '["photos.example.com"]' immich.example.com 2283 "{\"certificate_id\":$wildcard,\"ssl_forced\":true}" >/dev/null
proxy '["films.example.com","cinema.example.com"]' jellyfin.example.com 8096 \
  "{\"certificate_id\":$films,\"ssl_forced\":true,\"allow_websocket_upgrade\":true}" >/dev/null
proxy '["wiki.example.com"]' 10.0.10.99 8080 '{}' >/dev/null
proxy '["dns.example.com"]' 10.0.10.53 80 '{"advanced_config":"proxy_read_timout 300s;"}' >/dev/null
grafana=$(proxy '["grafana.example.com"]' grafana.example.com 3000 "{\"certificate_id\":$wildcard}")
send POST "/nginx/proxy-hosts/$grafana/disable" >/dev/null

send POST /nginx/redirection-hosts '{"domain_names":["blog.example.com"],"forward_scheme":"https",
  "forward_domain_name":"www.example.com/blog","forward_http_code":301,"preserve_path":true}' >/dev/null
send POST /nginx/dead-hosts '{"domain_names":["old.example.com"]}' >/dev/null
send POST /nginx/streams '{"incoming_port":2222,"forwarding_host":"git.example.com","forwarding_port":22,
  "tcp_forwarding":true,"udp_forwarding":false}' >/dev/null
send POST /nginx/streams '{"incoming_port":51820,"forwarding_host":"10.0.10.1","forwarding_port":51820,
  "tcp_forwarding":false,"udp_forwarding":true}' >/dev/null
