// What the page says, through page() alone: the demo's three homelabs as
// they are, then small invented snapshots for what none of them shows.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { asIfNow, page } from "../js/page.js";

// A demo homelab as page() says it, at the moment it was read.
const homelab = (name) => {
  const read = JSON.parse(readFileSync(new URL(`../../demo/${name}.json`, import.meta.url), "utf8"));
  return page(read, Date.parse(read.at));
};

const NOW = Date.parse("2026-10-28T10:00:00Z");

const check = (key, area, run, problem, more = {}) => ({
  key,
  name: key,
  area,
  run,
  problem,
  threshold: 3,
  results: [{ ok: run === 0, at: "2026-10-28T09:59:00Z", ms: 12 }],
  ...more,
});

const snap = (checks, more = {}) => ({
  checks,
  sections: [],
  charts: [],
  figures: [],
  events: [],
  updates: [],
  apps: [],
  tools: [],
  chores: [],
  sources: [{ name: "Gatus", adapter: "gatus", kind: "checkengine", answered: true }],
  ...more,
});

const area = (m, name) => m.areas.find((a) => a.area === name);
const names = (parts) => parts.map((p) => p.name);

// ── The demo's three homelabs ────────────────────────────────────────────

test("the demo: one resolver down makes the network degraded, and it's named as silent", () => {
  const m = homelab("snapshot");
  assert.equal(m.homelab.state, "degraded");
  assert.deepEqual(m.homelab.headline, { key: "status.problems", args: { n: 1 } });
  assert.deepEqual(m.homelab.summary, [
    [{ key: "status.areas_degraded", args: { areas: ["network"] } }],
    [{ key: "source.no_answer", args: { names: ["Standby DNS"] } }],
  ]);
  assert.deepEqual(names(m.problems), ["Standby DNS"]);
  assert.deepEqual(
    m.areas.map((a) => [a.area, a.shown, a.state]),
    [
      ["servers", true, "up"],
      ["network", true, "degraded"],
      ["storage", true, "up"],
      ["apps", true, "up"],
    ],
  );
  assert.deepEqual(area(m, "network").silent, ["Standby DNS"]);
  const lookups = m.today.tiles.find((f) => f.label.key === "resolver.dns_lookups");
  assert.deepEqual(lookups.sources, ["Primary DNS", "Kids' DNS"], "an AdGuard Home's and a Pi-hole's add up");
  const plays = m.today.tiles.find((f) => f.label.key === "mediaserver.plays");
  assert.deepEqual(plays.sources, ["Jellyfin", "Plex"], "so do Jellyfin's and Plex's");
  assert.equal(m.events.items.length, 8, "a busy week shows its eight newest");
  assert.ok(
    m.events.items.every((e, i) => !i || m.events.items[i - 1].at >= e.at),
    "newest first",
  );
});

test("the small homelab: two checks up in Apps, quiet since its engine started watching", () => {
  const m = homelab("small");
  assert.equal(m.homelab.state, "up");
  assert.deepEqual(m.homelab.headline, { key: "status.up" });
  assert.deepEqual(m.homelab.summary, [
    [{ key: "status.quiet_since", args: { at: new Date("2026-10-12T16:40:00Z") } }],
    [
      { key: "status.checks", args: { n: 2 } },
      { key: "status.areas", args: { n: 1 } },
    ],
  ]);
  assert.deepEqual(
    m.areas.filter((a) => a.shown).map((a) => a.area),
    ["apps"],
  );
  assert.deepEqual(m.today, { tiles: [], columns: 1 });
  assert.deepEqual(m.events, { items: [], empty: { key: "events.quiet" } });
  assert.deepEqual(
    m.week.rows.map((r) => r.area),
    ["apps"],
  );
  assert.deepEqual(
    m.apps.links.map((a) => [a.link.name, a.state]),
    [
      ["Photos", "up"],
      ["Home", "up"],
    ],
  );
});

test("the large homelab: a problem in every area makes every area degraded", () => {
  const m = homelab("large");
  assert.equal(m.homelab.state, "degraded");
  assert.deepEqual(m.homelab.headline, { key: "status.problems", args: { n: 4 } });
  assert.deepEqual(m.homelab.summary, [
    [{ key: "status.every_degraded" }],
    [{ key: "source.no_answer", args: { names: ["Office UPS"] } }],
  ]);
  assert.deepEqual(
    m.problems.map((c) => [c.name, c.area]),
    [
      ["Damson", "servers"],
      ["Wi-Fi garden", "network"],
      ["Offsite copy", "storage"],
      ["Books", "apps"],
    ],
  );
  assert.deepEqual(area(m, "servers").silent, ["Office UPS"], "a UPS is silent on its own area's page");
  assert.deepEqual(area(m, "network").silent, []);
  const lookups = m.today.tiles.find((f) => f.label.key === "resolver.dns_lookups");
  assert.deepEqual(lookups.lacking, ["IoT DNS"], "a resolver that answered but gave no lookups");
  assert.deepEqual(area(m, "network").charts[0].lacking, ["IoT DNS"], "on its chart too");
  assert.equal(m.apps.links.length, 25);
  assert.equal(m.apps.compact, true, "past twelve apps, the tiles go small");
  assert.equal(m.apps.columns, 4);
  const kids = m.apps.links.find((a) => a.link.name === "Kids' corner");
  assert.deepEqual([kids.check, kids.state], [undefined, "unknown"], "an app with no check");
  assert.equal(m.chores.next.name, "Check the rack's fans");
  assert.equal(m.week.rows.length, 4);
});

test("the demo's snapshots are left as they were read", () => {
  const read = JSON.parse(readFileSync(new URL("../../demo/large.json", import.meta.url), "utf8"));
  const before = JSON.stringify(read);
  page(read, Date.parse(read.at));
  assert.equal(JSON.stringify(read), before);
});

// ── The homelab and its areas ────────────────────────────────────────────

test("an area is up with no problem, degraded with one, down when all are", () => {
  const m = page(
    snap([check("dns1", "network", 0, false), check("dns2", "network", 4, true), check("nas", "storage", 3, true)]),
    NOW,
  );
  assert.equal(area(m, "network").state, "degraded");
  assert.equal(area(m, "storage").state, "down");
  assert.deepEqual(
    m.checks.map((c) => c.state),
    ["up", "down", "down"],
  );
  assert.deepEqual(names(area(m, "network").problems), ["dns2"]);
  assert.equal(m.homelab.state, "down", "an area down is the homelab down");
});

test("a check failing below its threshold is degraded, not a problem", () => {
  const m = page(snap([check("films", "apps", 1, false, { since: "2026-10-28T09:59:00Z" })]), NOW);
  const [films] = m.checks;
  assert.equal(films.state, "degraded");
  assert.ok(films.failing);
  assert.deepEqual(films.since, new Date("2026-10-28T09:59:00Z"));
  assert.equal(films.ms, 12);
  assert.equal(m.homelab.state, "up");
  assert.deepEqual(m.problems, []);
});

test("a check with no result yet is unknown, not up", () => {
  const m = page(snap([check("new", "apps", 0, false, { results: [] })]), NOW);
  assert.equal(m.checks[0].state, "unknown");
  assert.equal(area(m, "apps").ms, null);
});

test("areas not up are named by state, the worst first; all of them as every area", () => {
  const m = page(
    snap([
      check("web", "servers", 0, false),
      check("db", "servers", 3, true),
      check("dns", "network", 3, true),
      check("nas", "storage", 0, false),
      check("films", "apps", 3, true),
      check("shows", "apps", 0, false),
    ]),
    NOW,
  );
  assert.deepEqual(m.homelab.summary, [
    [{ key: "status.areas_down", args: { areas: ["network"] } }],
    [{ key: "status.areas_degraded", args: { areas: ["servers", "apps"] } }],
  ]);
  const every = page(snap([check("dns", "network", 3, true), check("nas", "storage", 3, true)]), NOW);
  assert.deepEqual(every.homelab.summary, [[{ key: "status.every_down" }]]);
});

test("an area's strip lines its checks up by time, at the pace of the most frequent", () => {
  // A check every minute and one every ten, each with its last thirty
  // results, the slow one failing once two hours ago and once ten minutes ago.
  const every = (key, minutes, failedAt = []) =>
    check(key, "storage", 0, false, {
      results: Array.from({ length: 30 }, (_, i) => {
        const ago = (29 - i) * minutes;
        return { ok: !failedAt.includes(ago), at: new Date(NOW - ago * 60e3).toISOString(), ms: 1, why: "slow" };
      }),
    });
  const { strip } = area(page(snap([every("disk", 1), every("copy", 10, [120, 10])]), NOW), "storage");
  assert.equal(strip.length, 30);
  assert.ok(
    strip.every((r) => r.n === 2),
    "half an hour, the slow check in every tick, by its result then",
  );
  assert.deepEqual(
    strip.map((r, k) => (r.ok ? -1 : k)).filter((k) => k >= 0),
    [19, 20, 21, 22, 23, 24, 25, 26, 27, 28],
    "failing from ten minutes ago until its next result passed, and not two hours ago",
  );
  assert.deepEqual(
    [strip[19].at.getTime(), strip[19].bad, strip[19].failed.map((f) => f.check.name)],
    [NOW - 10 * 60e3, 0.5, ["copy"]],
  );
  const fresh = every("new", 1);
  fresh.results = fresh.results.slice(-1);
  const alone = area(page(snap([fresh]), NOW), "storage").strip;
  assert.equal(alone.filter(Boolean).length, 1);
  assert.equal(alone[29].n, 1, "one result, its pace unknown, is the last tick");
});

test("the middle response time is of the checks answering", () => {
  const timed = (key, ms, run = 0) =>
    check(key, "apps", run, false, { results: [{ ok: !run, at: "2026-10-28T09:59:00Z", ms }] });
  const m = page(snap([timed("a", 30), timed("b", 10), timed("c", 20), timed("d", 5000, 1)]), NOW);
  assert.equal(area(m, "apps").ms, 20, "a failing check's time isn't an answer");
});

test("the quiet runs from the last problem's end, or from when the engine started watching", () => {
  const watched = "2026-10-20T08:00:00Z";
  const ended = "2026-10-27T14:20:00Z";
  const calm = page(snap([check("a", "servers", 0, false, { watched })]), NOW);
  assert.deepEqual(calm.homelab.summary[0], [{ key: "status.quiet_since", args: { at: new Date(watched) } }]);
  const once = page(
    snap([check("a", "servers", 0, false, { watched, outages: [{ from: "2026-10-27T14:00:00Z", to: ended }] })]),
    NOW,
  );
  assert.deepEqual(once.homelab.summary[0], [{ key: "status.quiet_for", args: { ms: NOW - Date.parse(ended) } }]);
});

test("the homelab names its silent sources, and an area's page those of its area and every area", () => {
  const m = page(
    snap([check("a", "servers", 0, false)], {
      sources: [
        { name: "Gatus", kind: "checkengine", answered: true },
        { name: "Uptime Kuma", kind: "checkengine", answered: false, error: "timeout" },
        { name: "Office UPS", area: "servers", answered: false },
        { name: "IoT DNS", area: "network", kind: "resolver", answered: true },
      ],
    }),
    NOW,
  );
  assert.equal(m.homelab.state, "up", "one check engine answering is enough");
  assert.deepEqual(m.homelab.summary.at(-1), [
    { key: "source.no_answer", args: { names: ["Uptime Kuma", "Office UPS"] } },
  ]);
  assert.deepEqual(area(m, "servers").silent, ["Uptime Kuma", "Office UPS"]);
  assert.deepEqual(area(m, "network").silent, ["Uptime Kuma"]);
});

test("with no check engine answering, the homelab is down and says why", () => {
  const m = page(
    snap([], {
      sources: [
        { name: "Primary DNS", area: "network", kind: "resolver", answered: false, error: "timeout" },
        { name: "Gatus", kind: "checkengine", answered: false, error: "connection refused" },
      ],
    }),
    NOW,
  );
  assert.equal(m.homelab.state, "down");
  assert.deepEqual(m.homelab.headline, { key: "source.no_answer", args: { names: ["Primary DNS", "Gatus"] } });
  assert.deepEqual(
    m.homelab.summary,
    [[{ key: "source.no_answer_why", args: { error: "connection refused" } }]],
    "the check engine's error, not the first silent source's",
  );
});

test("with no check engine, a silent resolver is named but the homelab isn't down", () => {
  const m = page(
    snap([], {
      sources: [{ name: "Primary DNS", area: "network", kind: "resolver", answered: false, error: "timeout" }],
    }),
    NOW,
  );
  assert.deepEqual(m.homelab, {
    state: "unknown",
    headline: { key: "status.no_checks" },
    summary: [[{ key: "source.no_answer", args: { names: ["Primary DNS"] } }]],
  });
});

test("a problem in a check in no area makes the homelab degraded, and the summary names it", () => {
  const mail = check("Mail", "", 3, true);
  const alone = page(snap([check("web", "servers", 0, false), mail]), NOW);
  assert.equal(alone.homelab.state, "degraded");
  assert.deepEqual(alone.homelab.summary, [[{ key: "status.checks_down", args: { names: ["Mail"] } }]]);
  const beside = page(snap([check("web", "servers", 0, false), check("dns", "network", 3, true), mail]), NOW);
  assert.deepEqual(
    beside.homelab.summary,
    [
      [{ key: "status.areas_down", args: { areas: ["network"] } }],
      [{ key: "status.checks_down", args: { names: ["Mail"] } }],
    ],
    "after the areas",
  );
  assert.equal(beside.homelab.state, "down", "an area down is still the homelab down");
});

test("with no checks and nothing silent, the homelab is unknown, not up", () => {
  const m = page(snap([]), NOW);
  assert.deepEqual(m.homelab, { state: "unknown", headline: { key: "status.no_checks" }, summary: [] });
});

test("an area with no checks is unknown, not up: nothing says whether it works", () => {
  const m = page(
    snap([check("web", "servers", 0, false)], {
      sections: [{ area: "storage", title: { key: "x" }, rows: [] }],
    }),
    NOW,
  );
  const storage = area(m, "storage");
  assert.deepEqual([storage.shown, storage.state], [true, "unknown"]);
  assert.equal(m.homelab.state, "up", "and it says nothing of the homelab");
  assert.deepEqual(m.homelab.summary.at(-1)[1], { key: "status.areas", args: { n: 1 } }, "nor counts as checked");
});

test("an area with nothing in it isn't shown", () => {
  const m = page(
    snap([check("a", "servers", 0, false), check("b", "", 0, false)], {
      sections: [{ area: "storage", title: { key: "x" }, rows: [] }],
    }),
    NOW,
  );
  assert.deepEqual(
    m.areas.filter((a) => a.shown).map((a) => a.area),
    ["servers", "storage"],
  );
});

// ── Parts named by their source ──────────────────────────────────────────

test("two sources of one adapter name their parts; one alone doesn't", () => {
  const at = "2026-10-28T08:00:00Z";
  const read = snap(
    [
      { ...check("Photos", "apps", 0, false), source: "Gatus" },
      { ...check("Photos", "apps", 0, false), source: "Gatus Outside" },
      { ...check("Films", "apps", 0, false), source: "Gatus" },
    ],
    {
      sources: [
        { name: "Primary DNS", adapter: "adguard", answered: true },
        { name: "Standby DNS", adapter: "adguard", answered: false },
        { name: "Proxmox", adapter: "proxmox", answered: true },
        { name: "Old", answered: true },
        { name: "Older", answered: true },
      ],
      sections: [
        { area: "network", title: { key: "adguard.resolver" }, source: "Primary DNS", rows: [] },
        { area: "servers", title: { key: "section.hosts" }, source: "Proxmox", rows: [] },
        { area: "servers", title: { key: "section.hosts" }, source: "Beszel", rows: [] },
        { area: "servers", title: { key: "proxmox.guests" }, source: "Proxmox", rows: [] },
      ],
      charts: [
        { area: "network", title: { key: "adguard.per_hour" }, source: "Primary DNS", series: [{ hours: [1, 2] }] },
      ],
      figures: [{ area: "network", label: { key: "adguard.today" }, source: "Primary DNS", value: 1 }],
      events: [
        { at, area: "servers", text: { key: "proxmox.backup_done" }, source: "Proxmox" },
        { at, area: "servers", text: { key: "x" }, source: "Old" },
      ],
      chores: [{ name: "Dust", from: "2026-11-01", source: "Primary DNS" }],
    },
  );
  const m = page(read, NOW);
  assert.equal(area(m, "network").sections[0].by, "Primary DNS", "even with the other one silent");
  assert.deepEqual(
    m.checks.map((c) => c.by),
    ["Gatus", "Gatus Outside", undefined],
    "a check is named where another engine has one of its name",
  );
  assert.deepEqual(
    area(m, "servers").sections.map((s) => s.by),
    ["Proxmox", "Beszel", undefined],
    "a section where another source's shares its title",
  );
  assert.equal(area(m, "network").charts[0].by, "Primary DNS");
  assert.equal(m.today.tiles[0].by, "Primary DNS");
  assert.equal(m.chores.week[0].by, "Primary DNS");
  assert.deepEqual(
    m.events.items.map((e) => e.about),
    [[{ key: "area.servers" }], [{ key: "area.servers" }]],
    "sources that don't say their adapter aren't of one adapter, and Proxmox is alone",
  );
  assert.equal(read.sections[0].by, undefined, "the snapshot's own parts are left as they were");
  const alone = page({ ...read, chores: [{ name: "Dust", source: "Proxmox" }] }, NOW);
  assert.equal(alone.chores.due[0].by, undefined, "a chore no other source's shares the list with");
  const mixed = page(
    {
      ...read,
      chores: [
        { name: "Dust", source: "Chores" },
        { name: "Nosferatu", source: "Seerr" },
      ],
    },
    NOW,
  );
  assert.deepEqual(
    mixed.chores.due.map((c) => c.by),
    ["Chores", "Seerr"],
    "chores from two sources each say where they wait",
  );
});

// ── Today ────────────────────────────────────────────────────────────────

test("figures that add up and share a label key are one tile, hour by hour, their details' numbers too", () => {
  const hours = (n) => Array(24).fill(n);
  const dns = (source, value, blocked, more = {}) => ({
    area: "network",
    icon: "shield",
    label: { key: "adguard.today" },
    adds: true,
    source,
    value,
    hours: hours(value / 24),
    within: { label: { key: "adguard.blocked" }, hours: hours(1) },
    detail: { key: "adguard.today_blocked", args: { blocked } },
    ...more,
  });
  const photos = { area: "apps", icon: "photos", label: { key: "immich.photos" }, value: 9 };
  const figures = [dns("Primary", 240, 12), photos, dns("Secondary", 48, 2)];
  const sources = [
    { name: "Primary", adapter: "adguard", answered: true },
    { name: "Secondary", adapter: "adguard", answered: true },
  ];
  const [both, alone] = page(snap([], { figures, sources }), NOW).today.tiles;
  assert.equal(both.value, 288);
  assert.deepEqual(both.hours, hours(12));
  assert.deepEqual(both.within, { label: { key: "adguard.blocked" }, hours: hours(2) });
  assert.deepEqual(both.detail.args, { blocked: 14 });
  assert.deepEqual(both.shares, [
    { label: { name: "Primary" }, value: 240 },
    { label: { name: "Secondary" }, value: 48 },
  ]);
  assert.equal(both.by, undefined, "the tile is both sources', so it names neither");
  assert.equal(alone.value, 9);
  assert.equal(figures[0].value, 240, "the snapshot's own figures are left as they were");
  assert.deepEqual(figures[0].detail.args, { blocked: 12 });
  assert.deepEqual(figures[0].within.hours, hours(1));
  const [lone] = page(
    snap([], { figures: [dns("Primary", 240, 12), dns("Secondary", 48, 2, { hours: undefined })], sources }),
    NOW,
  ).today.tiles;
  assert.deepEqual(lone.hours, [], "hours only one resolver has would mislead");
  assert.equal(lone.within, undefined, "and so would the part within them");
  const day = (n) => Array.from({ length: n }, (_, i) => i);
  const [short] = page(
    snap([], {
      figures: [
        dns("Primary", 240, 12, { hours: day(24), within: { hours: day(24) } }),
        dns("Secondary", 48, 2, { hours: day(6), within: { hours: day(6) } }),
      ],
      sources,
    }),
    NOW,
  ).today.tiles;
  assert.deepEqual(
    short.hours,
    [18, 20, 22, 24, 26, 28],
    "over the six hours both keep, the latest last, as charts are",
  );
  assert.deepEqual(short.within.hours, short.hours);
  const ups = (value) => ({ area: "servers", icon: "plug", label: { key: "nut.ups" }, value });
  assert.equal(
    page(snap([], { figures: [ups(80), ups(120)] }), NOW).today.tiles.length,
    2,
    "two UPSes' draws stay apart unless they say they add up",
  );
});

test("two hosts' guests are one tile: their units, details and meters added, split by source", () => {
  const guests = (source, up, of, cpu, tone) => ({
    area: "servers",
    label: { key: "proxmox.guests" },
    adds: true,
    source,
    value: up,
    unit: { key: "proxmox.of_up", args: { n: of } },
    detail: { key: "proxmox.hosts_online", args: { online: 1, n: 1 } },
    meters: [{ label: { key: "proxmox.cpu_meter" }, used: cpu, total: 8 }],
    tone,
  });
  const tiles = (...figures) => page(snap([], { figures }), NOW).today.tiles;
  const [tile] = tiles(guests("pve1", 7, 8, 2, "orange"), guests("pve2", 5, 5, 4));
  assert.equal(tile.value, 12);
  assert.deepEqual(tile.unit.args, { n: 13 });
  assert.deepEqual(tile.detail.args, { online: 2, n: 2 });
  assert.deepEqual(tile.meters, [{ label: { key: "proxmox.cpu_meter" }, used: 6, total: 16 }]);
  assert.equal(tile.tone, "orange", "a guest down on either host shows");
  const [one] = tiles(guests("pve1", 7, 8, 2));
  assert.equal(one.shares, undefined, "one source alone isn't split");
  const [lost] = tiles(guests("pve1", 7, 8, 2), { ...guests("pve2", 5, 5, 4), meters: undefined });
  assert.equal(lost.meters, undefined, "meters only one host has would mislead");
});

test("a figure added up from some sources of its kind names those that answered but gave none", () => {
  const dns = (name, adapter, answered = true) => ({ name, adapter, kind: "resolver", answered });
  const lookups = (source) => ({
    area: "network",
    label: { key: "resolver.dns_lookups" },
    adds: true,
    source,
    value: 1,
  });
  const sources = [
    { name: "Gatus", adapter: "gatus", kind: "checkengine", answered: true },
    dns("Primary", "adguard"),
    dns("Secondary", "adguard"),
    dns("Kids", "pihole"),
    dns("Old", "adguard", false),
    { name: "Proxmox", adapter: "proxmox", answered: true },
    { name: "Immich", adapter: "immich", answered: true },
  ];
  const tile = (...figures) => page(snap([], { sources, figures }), NOW).today.tiles[0];
  assert.deepEqual(
    tile(lookups("Primary"), lookups("Secondary")).lacking,
    ["Kids"],
    "a Pi-hole beside AdGuard Homes; one that didn't answer is said elsewhere",
  );
  assert.deepEqual(tile(lookups("Kids"), lookups("Primary")).lacking, ["Secondary"]);
  assert.deepEqual(tile(lookups("Primary"), lookups("Secondary"), lookups("Kids")).lacking, []);
  const guests = { area: "servers", label: { key: "proxmox.guests" }, adds: true, source: "Proxmox", value: 3 };
  assert.deepEqual(tile(guests).lacking, [], "a source with no kind lacks none");
});

test("figures that share a join key are one tile, the first's number first; alone, each stays", () => {
  const library = { key: "servarr.library" };
  const films = {
    area: "apps",
    label: { key: "radarr.films" },
    value: 412,
    unit: { key: "radarr.unit", args: { n: 412 } },
    detail: { key: "radarr.this_week", args: { n: 3 } },
    join: library,
    part: { label: { key: "radarr.films" }, value: 2600, tone: "orange" },
  };
  const shows = {
    ...films,
    label: { key: "sonarr.shows" },
    value: 38,
    part: { label: { key: "sonarr.shows" }, value: 1300 },
  };
  const photos = { area: "apps", label: { key: "immich.photos" }, value: 9 };
  const tiles = (...figures) => page(snap([], { figures }), NOW).today.tiles;
  const [tile, other] = tiles(films, photos, shows);
  assert.equal(tile.label, library);
  assert.equal(tile.value, 412, "the films' number leads");
  assert.equal(tile.detail, undefined, "each one's line moves to its part");
  assert.deepEqual(names(tile.also), [undefined], "the shows follow, as they are");
  assert.equal(tile.also[0].value, 38);
  assert.deepEqual(
    tile.shares.map((s) => s.value),
    [2600, 1300],
  );
  assert.equal(tile.wide, false, "two figures fit one tile");
  assert.equal(other.label.key, "immich.photos");
  assert.deepEqual(
    tiles(films, photos).map((f) => f.label.key),
    ["radarr.films", "immich.photos"],
    "films alone keep their own tile",
  );
  const [twin] = tiles({ ...films, by: "Radarr" }, { ...films, by: "Radarr 4K" });
  assert.deepEqual(
    twin.shares.map((s) => s.by),
    ["Radarr", "Radarr 4K"],
    "two Radarrs' parts say whose they are",
  );
  const [wide] = tiles(films, shows, { ...shows, value: 7 });
  assert.equal(wide.wide, true, "three figures make a tile two columns wide");
});

test("tiles take as many columns as make their rows even, a wide one counting two", () => {
  const figure = (i) => ({ area: "apps", label: { key: `f${i}` }, value: i });
  const columns = (n) => page(snap([], { figures: Array.from({ length: n }, (_, i) => figure(i)) }), NOW).today.columns;
  assert.equal(columns(8), 4, "8 tiles in rows of 4, not 6 and 2");
  assert.equal(columns(6), 6);
  assert.equal(columns(7), 4);
  assert.equal(columns(25), 5);
  assert.equal(columns(0), 1);
});

test("tiles whose parts take the colours each start two on from the one before", () => {
  const part = (key, value) => ({
    area: "apps",
    label: { key },
    value,
    join: { key },
    part: { label: { key }, value },
  });
  const toned = {
    area: "apps",
    label: { key: "pbs" },
    value: 1,
    shares: [{ label: { name: "a" }, value: 1, tone: "green" }],
  };
  const tiles = page(snap([], { figures: [part("a", 1), part("a", 2), toned, part("b", 1), part("b", 2)] }), NOW).today
    .tiles;
  assert.deepEqual(
    tiles.map((f) => f.start),
    [0, 0, 2],
    "a tile whose shares all have a tone takes none",
  );
});

// ── The areas' charts ────────────────────────────────────────────────────

test("charts that add up and share a title key are one, over the hours they all have", () => {
  const chart = (source, n, v, more = {}) => ({
    area: "network",
    title: { key: "adguard.per_hour" },
    adds: true,
    source,
    series: [
      { label: { key: "adguard.lookups" }, hours: Array(n).fill(v) },
      { label: { key: "adguard.blocked" }, hours: Array(n).fill(1) },
    ],
    ...more,
  });
  const charts = [chart("Primary", 168, 10), chart("Standby", 24, 4), chart("Other", 24, 9, { adds: false })];
  const [both, apart] = area(page(snap([], { charts }), NOW), "network").charts;
  assert.equal(both.series[0].hours.length, 24, "a week and a day make a day");
  assert.deepEqual(both.series[0].hours, Array(24).fill(14));
  assert.deepEqual(both.series[1].hours, Array(24).fill(2));
  assert.deepEqual(
    both.sources.map((s) => [s.name, s.hours.length, s.hours[0]]),
    [
      ["Primary", 24, 10],
      ["Standby", 24, 4],
    ],
  );
  assert.equal(apart.source, "Other", "one that doesn't add up stays apart");
  assert.equal(charts[0].series[0].hours.length, 168, "the snapshot's own charts are left as they were");
  const [alone] = area(page(snap([], { charts: [chart("Primary", 168, 10)] }), NOW), "network").charts;
  assert.equal(alone.series[0].hours.length, 168);
  assert.deepEqual(names(alone.sources), ["Primary"]);
});

test("levels from several sources are one chart, their lines side by side over the hours they all have", () => {
  const cpu = (source, lines, n) => ({
    area: "servers",
    title: { key: "chart.processor" },
    measures: true,
    source,
    series: lines.map((name, k) => ({ label: { name }, hours: Array.from({ length: n }, () => 10 * (k + 1)) })),
  });
  const [one] = area(
    page(snap([], { charts: [cpu("Cluster", ["alder", "birch"], 5), cpu("Vault", ["vault"], 3)] }), NOW),
    "servers",
  ).charts;
  assert.deepEqual(
    one.series.map((s) => [s.label.name, s.hours.length]),
    [
      ["alder", 3],
      ["birch", 3],
      ["vault", 3],
    ],
  );
  assert.equal(one.series[1].hours[0], 20, "never added up");
  assert.deepEqual(names(one.sources), ["Cluster", "Vault"]);
  assert.equal(one.by, undefined);
});

// ── The week ─────────────────────────────────────────────────────────────

test("this week's events are newest first, at most eight, outages and updates among them", () => {
  const hoursAgo = (h) => new Date(NOW - h * 3600e3).toISOString();
  const read = snap(
    [
      check("dns", "network", 0, false, {
        outages: [
          { from: hoursAgo(30), to: hoursAgo(29) },
          { from: hoursAgo(24 * 9), to: hoursAgo(24 * 8) },
        ],
      }),
    ],
    {
      events: [{ at: hoursAgo(2), area: "storage", icon: "archive", text: { key: "pbs.backed_up" } }],
      updates: [{ at: hoursAgo(1), name: "Photos 3.4" }],
    },
  );
  const { items } = page(read, NOW).events;
  assert.deepEqual(
    items.map((e) => [e.text.key, e.mark]),
    [
      ["events.update", { icon: "pr", tone: "green" }],
      ["pbs.backed_up", { icon: "archive", tone: "blue" }],
      ["events.back", { state: "up" }],
      ["events.down", { state: "down" }],
    ],
    "an outage over a week ago isn't this week's",
  );
  assert.deepEqual(items[2].text.args, { name: "dns", ms: 3600e3 });
  assert.deepEqual(items[2].about, [{ key: "area.network" }]);
  const many = page(
    snap([], { updates: Array.from({ length: 12 }, (_, i) => ({ at: hoursAgo(i), name: `u${i}` })) }),
    NOW,
  );
  assert.equal(many.events.items.length, 8);
  const silent = page(snap([], { sources: [{ name: "Gatus", answered: false }] }), NOW);
  assert.deepEqual(
    silent.events,
    { items: [], empty: { key: "source.no_answer", args: { names: ["Gatus"] } } },
    "with a source silent, the week may not have been quiet",
  );
});

test("the week runs from midnight six days ago to now, each area's outages cut to it", () => {
  const now = new Date(2026, 9, 28, 12, 0);
  const start = new Date(2026, 9, 22);
  const at = (d, h) => new Date(2026, 9, d, h).toISOString();
  const read = snap(
    [
      check("nas", "storage", 3, true, {
        outages: [{ from: at(20, 0), to: at(23, 0) }, { from: at(28, 11) }],
      }),
      check("web", "servers", 0, false, { outages: [{ from: at(10, 0), to: at(11, 0) }] }),
    ],
    {
      events: [
        { at: at(27, 2), area: "storage", text: { key: "pbs.backed_up" } },
        {
          at: new Date(2026, 9, 27, 2, 30).toISOString(),
          area: "storage",
          text: { key: "pbs.backed_up" },
          tone: "red",
        },
        { at: at(26, 2), area: "storage", text: { key: "pbs.backed_up" } },
      ],
    },
  );
  const { days, rows } = page(read, +now).week;
  assert.equal(days.length, 7);
  assert.deepEqual(days[0], { at: start, left: 0 });
  assert.ok(days.every((d, i) => !i || d.left > days[i - 1].left) && days[6].left < 1);
  const [servers, storage] = rows;
  assert.deepEqual(servers.outages, [], "an outage before the week isn't on it");
  assert.equal(storage.outages.length, 2);
  const [cut, ongoing] = storage.outages;
  assert.equal(cut.left, 0, "one that began before the week starts with it");
  assert.equal(cut.ms, 3 * 864e5, "and still lasted as long as it did");
  assert.equal(ongoing.to, null);
  assert.equal(ongoing.left + ongoing.width, 1, "one still going runs to now");
  assert.equal(storage.down, 864e5 + 3600e3, "down in all, within the week");
  assert.deepEqual(
    storage.events.map((e) => [e.events.length, e.bad]),
    [
      [2, true],
      [1, false],
    ],
    "events in the same hour are one, bad when any is",
  );
});

// ── The apps, the tools and the chores ───────────────────────────────────

test("a link's check is in the engine it names, or the worst of its name", () => {
  const photos = (source, problem) => ({ ...check("Photos", "apps", problem ? 3 : 0, problem), source });
  const read = snap([photos("Gatus", false), photos("Gatus Outside", true)], {
    apps: [{ name: "Photos", check: "Photos" }, { name: "Films", check: "Films" }, { name: "Notes" }],
    tools: [{ name: "Gatus", check: "Photos", engine: "Gatus" }],
  });
  const m = page(read, NOW);
  const [either, unknown, none] = m.apps.links;
  assert.equal(either.check.source, "Gatus Outside", "a problem in either shows");
  assert.equal(either.state, "down");
  assert.equal(m.tools[0].check.source, "Gatus");
  assert.equal(m.tools[0].state, "up");
  assert.equal(unknown.check, undefined);
  assert.equal(unknown.state, "unknown", "a check no engine has");
  assert.equal(none.check, undefined, "a link with no check");
  assert.deepEqual([m.apps.compact, m.apps.columns], [false, 3]);
});

test("chores group by the viewer's today: due, with no day too, this week, and the next after it", () => {
  const chores = [
    { name: "later", from: "2026-11-20" },
    { name: "tomorrow", from: "2026-10-29" },
    { name: "today", from: "2026-10-28" },
    { name: "in six days", from: "2026-11-03" },
    { name: "in a week", from: "2026-11-04" },
    { name: "overdue", from: "2026-10-01" },
    { name: "whenever" },
  ];
  const by = page(snap([], { chores }), +new Date(2026, 9, 28, 23, 59)).chores;
  assert.deepEqual(names(by.due), ["overdue", "today", "whenever"]);
  assert.deepEqual(names(by.week), ["tomorrow", "in six days"]);
  assert.equal(by.next.name, "in a week");
  assert.equal(+by.next.from, +new Date(2026, 10, 4), "a day is midnight where the page is viewed");
  assert.equal(page(snap([]), NOW).chores.next, null);
});

// ── The demo ─────────────────────────────────────────────────────────────

test("the demo's snapshot moves to now, every time in it, a text's too, and nothing else", () => {
  const read = {
    at: "2026-10-28T09:30:00Z",
    checks: [
      { name: "2026", since: "2026-10-28T09:04:40.5+01:00", results: [{ at: "2026-10-28T09:29:40Z", ms: 1.2 }] },
    ],
    events: [
      {
        at: "2026-10-28T00:30:00Z",
        text: { key: "proxmox.backup_done", args: { at: "2026-10-28T00:30:00Z", ms: 512000 } },
      },
    ],
  };
  const now = Date.parse("2026-11-02T15:00:00Z");
  const moved = asIfNow(read, now);
  assert.equal(moved.at, "2026-11-02T15:00:00.000Z");
  assert.equal(moved.checks[0].since, "2026-11-02T13:34:40.500Z");
  assert.equal(moved.checks[0].results[0].at, "2026-11-02T14:59:40.000Z");
  assert.equal(moved.events[0].text.args.at, "2026-11-02T06:00:00.000Z");
  assert.equal(moved.events[0].text.args.ms, 512000);
  assert.equal(moved.checks[0].name, "2026");
  assert.equal(
    asIfNow({ ...read, chores: [{ from: "2026-10-27" }] }, now).chores[0].from,
    "2026-11-01",
    "a day, by whole days",
  );
  assert.equal(read.at, "2026-10-28T09:30:00Z", "the snapshot read stays as it was");
});
