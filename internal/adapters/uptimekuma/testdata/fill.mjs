// Fills the scratch Uptime Kuma of compose.yaml with invented monitors
// (ADR 0008), on a status page called homelab whose groups are named like
// Overlook's areas, then keeps two of them pushed for eleven minutes, so
// that every monitor has half an hour of results at its 20 seconds. Uptime
// Kuma is set up only over socket.io, which this speaks over Node's own
// WebSocket, as the page's tests run on Node.
//
//   node fill.mjs

const kuma = "http://localhost:3001";
const ws = new WebSocket(`${kuma.replace("http", "ws")}/socket.io/?EIO=4&transport=websocket`);
let id = 0;
const waiting = new Map();
ws.addEventListener("message", ({ data }) => {
  if (data === "2") return ws.send("3"); // Engine.IO's ping
  const ack = /^43(\d+)(.*)$/s.exec(data);
  if (ack) {
    waiting.get(Number(ack[1]))(JSON.parse(ack[2])[0]);
    waiting.delete(Number(ack[1]));
  }
});
const emit = (event, ...args) =>
  new Promise((ok, no) => {
    const n = ++id;
    waiting.set(n, (r) => (r && r.ok === false ? no(new Error(`${event}: ${r.msg}`)) : ok(r)));
    ws.send(`42${n}${JSON.stringify([event, ...args])}`);
  });
await new Promise((ok) => ws.addEventListener("open", ok));
ws.send("40");
await new Promise((ok) => setTimeout(ok, 500));

await emit("setup", "owner", "owner-password-1");
await emit("login", { username: "owner", password: "owner-password-1", token: "" });

// Every monitor checks every 20 seconds. Those that check Uptime Kuma's own
// page pass; those that check a port nothing listens on fail, Requests
// after 3 retries and Books after 60, so it is still retrying; VPN and
// Recipes pass or fail as they are pushed.
const base = {
  interval: 20,
  retryInterval: 20,
  maxretries: 0,
  method: "GET",
  accepted_statuscodes: ["200-299"],
  notificationIDList: {},
  kafkaProducerBrokers: [],
  kafkaProducerSaslOptions: {},
  conditions: [],
  rabbitmqNodes: [],
  timeout: 16,
  active: true,
};
const up = "http://127.0.0.1:3001/";
const down = "http://127.0.0.1:1/";
const monitors = {
  Servers: [
    { name: "Hypervisor", type: "http", url: up },
    { name: "Backup server", type: "http", url: up },
  ],
  Network: [
    { name: "Router", type: "http", url: up },
    { name: "Internet", type: "http", url: up },
    { name: "VPN", type: "push", pushToken: "vpn0123456789abc" },
  ],
  Apps: [
    { name: "Photos", type: "http", url: up },
    { name: "Recipes", type: "push", pushToken: "recipes0123456789" },
    { name: "Requests", type: "http", url: down, maxretries: 3 },
    { name: "Books", type: "http", url: down, maxretries: 60 },
  ],
  Outside: [{ name: "Website", type: "http", url: up }],
};
const groups = [];
for (const [name, list] of Object.entries(monitors)) {
  const monitorList = [];
  for (const m of list) {
    const { monitorID } = await emit("add", { ...base, ...m });
    monitorList.push({ id: monitorID });
  }
  groups.push({ name, monitorList });
}
await emit("addStatusPage", "Homelab", "homelab");
await emit(
  "saveStatusPage",
  "homelab",
  {
    slug: "homelab",
    title: "Homelab",
    description: "",
    theme: "auto",
    autoRefreshInterval: 300,
    showTags: false,
    showPoweredBy: true,
    showOnlyLastHeartbeat: false,
    showCertificateExpiry: false,
    analyticsType: null,
    domainNameList: [],
  },
  "/icon.svg",
  groups,
);
ws.close();

// VPN passes for seven minutes, then fails; Recipes fails for two minutes
// from the fourth, then passes again.
const push = (token, status, msg) => fetch(`${kuma}/api/push/${token}?status=${status}&msg=${encodeURIComponent(msg)}&ping=`);
for (let s = 0; s < 660; s += 20) {
  await push("vpn0123456789abc", s < 420 ? "up" : "down", s < 420 ? "OK" : "handshake timed out");
  const recipesDown = s >= 240 && s < 360;
  await push("recipes0123456789", recipesDown ? "down" : "up", recipesDown ? "database is locked" : "OK");
  await new Promise((ok) => setTimeout(ok, 20_000));
}
