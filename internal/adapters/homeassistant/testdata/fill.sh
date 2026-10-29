#!/bin/sh
# Fills the scratch Home Assistant of compose.yaml (ADR 0008): it answers the
# first run's questions for an owner, runs one automation, and makes
# Overlook a user of its own that isn't an administrator, with a long-lived
# token, which it prints for record.sh. Its devices are configuration.yaml's.
# Making a user and a token is only offered over Home Assistant's WebSocket,
# so those two steps are Node's, as the page's tests are.
set -eu
h=http://localhost:8123
client=$h/

# The owner, and a token to finish the first run with.
code=$(curl -sf -X POST "$h/api/onboarding/users" -H "Content-Type: application/json" \
  -d "{\"client_id\":\"$client\",\"name\":\"Owner\",\"username\":\"owner\",\"password\":\"owner-password\",\"language\":\"en-GB\"}" | jq -r .auth_code)
token=$(curl -sf -X POST "$h/auth/token" -d "grant_type=authorization_code&code=$code&client_id=$client" | jq -r .access_token)
post() { curl -sf -X POST "$h$1" -H "Authorization: Bearer $token" -H "Content-Type: application/json" -d "$2" >/dev/null; }
post /api/onboarding/core_config '{}'
post /api/onboarding/analytics '{}'
post /api/onboarding/integration "{\"client_id\":\"$client\",\"redirect_uri\":\"$client?auth_callback=1\"}"

# The porch light came on at sunset.
post /api/services/automation/trigger '{"entity_id":"automation.porch_light_at_sunset"}'

# Overlook's user, a plain user that can't change Home Assistant's config,
# and its token, made by logging in as it.
H=$h TOKEN=$token node --input-type=module -e '
const ws = new WebSocket(process.env.H.replace("http", "ws") + "/api/websocket");
let id = 0;
const ask = (msg) => new Promise((ok, no) => {
  const n = ++id;
  const on = (e) => { const m = JSON.parse(e.data); if (m.id === n) { ws.removeEventListener("message", on); m.success ? ok(m.result) : no(new Error(JSON.stringify(m.error))); } };
  ws.addEventListener("message", on);
  ws.send(JSON.stringify({ id: n, ...msg }));
});
await new Promise((ok) => ws.addEventListener("message", ok, { once: true }));
ws.send(JSON.stringify({ type: "auth", access_token: process.env.TOKEN }));
await new Promise((ok) => ws.addEventListener("message", ok, { once: true }));
const { user } = await ask({ type: "config/auth/create", name: "Overlook", group_ids: ["system-users"], local_only: false });
await ask({ type: "config/auth_provider/homeassistant/create", user_id: user.id, username: "overlook", password: "overlook-password" });
ws.close();
'
flow=$(curl -sf -X POST "$h/auth/login_flow" -H "Content-Type: application/json" \
  -d "{\"client_id\":\"$client\",\"handler\":[\"homeassistant\",null],\"redirect_uri\":\"$client\"}" | jq -r .flow_id)
code=$(curl -sf -X POST "$h/auth/login_flow/$flow" -H "Content-Type: application/json" \
  -d "{\"client_id\":\"$client\",\"username\":\"overlook\",\"password\":\"overlook-password\"}" | jq -r .result)
login=$(curl -sf -X POST "$h/auth/token" -d "grant_type=authorization_code&code=$code&client_id=$client" | jq -r .access_token)
H=$h TOKEN=$login node --input-type=module -e '
const ws = new WebSocket(process.env.H.replace("http", "ws") + "/api/websocket");
await new Promise((ok) => ws.addEventListener("message", ok, { once: true }));
ws.send(JSON.stringify({ type: "auth", access_token: process.env.TOKEN }));
await new Promise((ok) => ws.addEventListener("message", ok, { once: true }));
ws.send(JSON.stringify({ id: 1, type: "auth/long_lived_access_token", client_name: "Overlook", lifespan: 3650 }));
const m = JSON.parse((await new Promise((ok) => ws.addEventListener("message", ok, { once: true }))).data);
console.log(m.result);
ws.close();
'
