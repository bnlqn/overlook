import assert from "node:assert/strict";
import { test } from "node:test";
import {
  areaState,
  asIfNow,
  busiest,
  byState,
  checkState,
  choresBy,
  evenColumns,
  fold,
  joinCharts,
  joinFigures,
  linkCheck,
  missing,
  named,
  quietSince,
  readChecks,
  shown,
  silent,
  together,
} from "../js/checks.js";

const check = (key, area, run, problem, more = {}) => ({
  key,
  name: key,
  area,
  run,
  problem,
  threshold: 3,
  results: [{ ok: run === 0, at: "2026-10-28T10:00:00Z", ms: 12 }],
  ...more,
});

const snap = (checks, more = {}) => ({
  checks,
  sections: [],
  charts: [],
  figures: [],
  events: [],
  updates: [],
  sources: [{ name: "Gatus", answered: true }],
  ...more,
});

test("an area is up with no problem, degraded with one, down when all are", () => {
  const checks = readChecks(
    snap([check("dns1", "network", 0, false), check("dns2", "network", 4, true), check("nas", "storage", 3, true)]),
  );
  assert.equal(areaState("network", checks).state, "degraded");
  assert.equal(areaState("storage", checks).state, "down");
  assert.equal(areaState("servers", checks).state, "up");
  assert.deepEqual(
    checks.map((c) => checkState(c)),
    ["up", "down", "down"],
  );
});

test("a check failing below its threshold is degraded, not a problem", () => {
  const [films] = readChecks(snap([check("films", "apps", 1, false, { since: "2026-10-28T09:59:00Z" })]));
  assert.equal(checkState(films), "degraded");
  assert.ok(films.failing);
  assert.deepEqual(films.since, new Date("2026-10-28T09:59:00Z"));
  assert.equal(films.ms, 12);
});

test("an area's page names its own silent sources and those of every area", () => {
  const snap = {
    sources: [
      { name: "Gatus", answered: false },
      { name: "Office UPS", area: "servers", answered: false },
      { name: "IoT DNS", area: "network", answered: true },
    ],
  };
  assert.deepEqual(silent(snap), ["Gatus", "Office UPS"], "the overview names them all");
  assert.deepEqual(silent(snap, "servers"), ["Gatus", "Office UPS"]);
  assert.deepEqual(silent(snap, "network"), ["Gatus"]);
});

test("a source of the same adapter that answered but gave none is missing", () => {
  const dns = (name, answered = true) => ({ name, adapter: "adguard", answered });
  const snap = {
    sources: [
      dns("Primary"),
      dns("Secondary"),
      dns("IoT"),
      dns("Old", false),
      { name: "Gatus", adapter: "gatus", answered: true },
    ],
  };
  assert.deepEqual(missing(snap, ["Primary", "Secondary"]), ["IoT"], "one that didn't answer is said elsewhere");
  assert.deepEqual(missing(snap, ["Primary", "Secondary", "IoT"]), []);
});

test("areas not up group by state, the worst first", () => {
  const a = (area, state) => ({ area, state });
  assert.deepEqual(
    byState([a("servers", "degraded"), a("network", "down"), a("storage", "up"), a("apps", "degraded")]),
    [
      { state: "down", areas: ["network"] },
      { state: "degraded", areas: ["servers", "apps"] },
    ],
  );
});

test("tiles take as many columns as make their rows even", () => {
  assert.equal(evenColumns(8, 6), 4);
  assert.equal(evenColumns(6, 6), 6);
  assert.equal(evenColumns(7, 6), 4);
  assert.equal(evenColumns(2, 3), 2);
  assert.equal(evenColumns(25, 6), 5);
  assert.equal(evenColumns(0, 6), 1);
});

test("a check with no result yet is unknown, not up", () => {
  assert.equal(checkState({ problem: false, failing: false, recent: [] }), "unknown");
  assert.equal(checkState({ problem: false, failing: false, recent: [{ ok: true }] }), "up");
  assert.equal(checkState({ problem: true, failing: true, recent: [] }), "down", "a problem is a problem");
});

test("a link's check is in the engine it names, or the worst of its name", () => {
  const photos = (source, problem) => ({
    name: "Photos",
    source,
    problem,
    failing: problem,
    recent: [{ ok: !problem }],
  });
  const checks = [photos("Gatus", false), photos("Gatus Outside", true)];
  assert.equal(linkCheck({ check: "Photos" }, checks).source, "Gatus Outside", "a problem in either shows");
  assert.equal(linkCheck({ check: "Photos", engine: "Gatus" }, checks).source, "Gatus");
  assert.equal(linkCheck({ check: "Films" }, checks), undefined);
});

test("the quiet runs from the last problem's end, or from when the engine started watching", () => {
  const watched = "2026-10-20T08:00:00Z";
  const ended = "2026-10-27T14:20:00Z";
  const calm = readChecks(snap([check("a", "servers", 0, false, { watched })]));
  assert.deepEqual(quietSince(calm), { since: new Date(watched), problemFree: true });
  const once = readChecks(
    snap([check("a", "servers", 0, false, { watched, outages: [{ from: "2026-10-27T14:00:00Z", to: ended }] })]),
  );
  assert.deepEqual(quietSince(once), { since: new Date(ended), problemFree: false });
});

test("an area with nothing in it isn't shown", () => {
  const s = snap([check("a", "servers", 0, false), check("b", "", 0, false)], {
    sections: [{ area: "storage", title: { key: "x" }, rows: [] }],
    sources: [
      { name: "Gatus", answered: true },
      { name: "Proxmox", answered: false },
    ],
  });
  assert.deepEqual(shown(s), ["servers", "storage"]);
  assert.deepEqual(silent(s), ["Proxmox"]);
});

test("figures that add up and share a label key are one, hour by hour, their details' numbers too", () => {
  const hours = (n) => Array(24).fill(n);
  const dns = (value, blocked, more = {}) => ({
    area: "network",
    icon: "shield",
    label: { key: "adguard.today" },
    adds: true,
    value,
    hours: hours(value / 24),
    within: { label: { key: "adguard.blocked" }, hours: hours(1) },
    detail: { key: "adguard.today_blocked", args: { blocked } },
    ...more,
  });
  const photos = { area: "apps", icon: "photos", label: { key: "immich.photos" }, value: 9 };
  const figures = [dns(240, 12), photos, dns(48, 2)];
  const [both, alone] = together(figures);
  assert.equal(both.value, 288);
  assert.deepEqual(both.hours, hours(12));
  assert.deepEqual(both.within, { label: { key: "adguard.blocked" }, hours: hours(2) });
  assert.deepEqual(both.detail.args, { blocked: 14 });
  assert.deepEqual(alone, { ...photos, hours: [] });
  assert.equal(figures[0].value, 240, "the snapshot's own figures are left as they were");
  assert.deepEqual(figures[0].detail.args, { blocked: 12 });
  assert.deepEqual(figures[0].within.hours, hours(1));
  const [lone] = together([dns(240, 12), dns(48, 2, { hours: undefined })]);
  assert.deepEqual(lone.hours, [], "hours only one resolver has would mislead");
  assert.equal(lone.within, undefined, "and so would the part within them");
  const ups = (value) => ({ area: "servers", icon: "plug", label: { key: "nut.ups" }, value });
  assert.equal(together([ups(80), ups(120)]).length, 2, "two UPSes' draws stay apart unless they say they add up");
});

test("two sources of one adapter name their parts; one of a kind doesn't", () => {
  const snap = {
    sources: [
      { name: "Primary DNS", adapter: "adguard", answered: true },
      { name: "Standby DNS", adapter: "adguard", answered: false },
      { name: "Proxmox", adapter: "proxmox", answered: true },
      { name: "Old", answered: true },
      { name: "Older", answered: true },
    ],
    checks: [
      { name: "Photos", source: "Gatus" },
      { name: "Photos", source: "Gatus Outside" },
      { name: "Films", source: "Gatus" },
    ],
    sections: [
      { title: { key: "adguard.resolver" }, source: "Primary DNS" },
      { title: { key: "section.hosts" }, source: "Proxmox" },
      { title: { key: "section.hosts" }, source: "Beszel" },
      { title: { key: "proxmox.guests" }, source: "Proxmox" },
    ],
    charts: [{ title: { key: "adguard.per_hour" }, source: "Primary DNS" }],
    figures: [{ label: { key: "adguard.today" }, source: "Primary DNS" }],
    events: [{ text: { key: "proxmox.backup_done" }, source: "Proxmox" }, { source: "Old" }],
    chores: [{ name: "Dust", from: "2026-11-01", source: "Primary DNS" }],
  };
  const out = named(snap);
  assert.equal(out.sections[0].by, "Primary DNS", "even with the other one silent");
  assert.equal(out.checks[1].by, "Gatus Outside", "a check whose name another engine shares");
  assert.equal(out.checks[2].by, undefined, "a check no other engine has");
  assert.equal(out.sections[1].by, "Proxmox", "a section whose title another source's shares");
  assert.equal(out.sections[2].by, "Beszel");
  assert.equal(out.sections[3].by, undefined, "a section no other source titles so");
  assert.equal(out.charts[0].by, "Primary DNS");
  assert.equal(out.figures[0].by, "Primary DNS");
  assert.equal(out.chores[0].by, "Primary DNS");
  assert.equal(out.chores[0].from, "2026-11-01", "a chore keeps the day it's due from");
  const [alone] = named({ ...snap, chores: [{ name: "Dust", source: "Proxmox" }] }).chores;
  assert.equal(alone.by, undefined, "a chore no other source's shares the list with");
  const mixed = named({
    ...snap,
    chores: [
      { name: "Dust", source: "Chores" },
      { name: "Nosferatu", source: "Seerr" },
    ],
  });
  assert.deepEqual(
    mixed.chores.map((c) => c.by),
    ["Chores", "Seerr"],
    "chores from two sources each say where they wait",
  );
  assert.equal(out.events[0].by, undefined);
  assert.equal(out.events[1].by, undefined, "sources that don't say their adapter aren't of one kind");
  assert.equal(snap.sections[0].by, undefined, "the snapshot's own parts are left as they were");
});

test("two hosts' guests are one tile: their units, details and meters added, split by source", () => {
  const guests = (source, up, of, cpu, tone) => ({
    area: "servers",
    label: { key: "proxmox.guests" },
    adds: true,
    source,
    by: source,
    value: up,
    unit: { key: "proxmox.of_up", args: { n: of } },
    detail: { key: "proxmox.hosts_online", args: { online: 1, n: 1 } },
    meters: [{ label: { key: "proxmox.cpu_meter" }, used: cpu, total: 8 }],
    tone,
  });
  const [tile] = together([guests("pve1", 7, 8, 2, "orange"), guests("pve2", 5, 5, 4)]);
  assert.equal(tile.value, 12);
  assert.deepEqual(tile.unit.args, { n: 13 });
  assert.deepEqual(tile.detail.args, { online: 2, n: 2 });
  assert.deepEqual(tile.meters, [{ label: { key: "proxmox.cpu_meter" }, used: 6, total: 16 }]);
  assert.equal(tile.tone, "orange", "a guest down on either host shows");
  assert.deepEqual(tile.shares, [
    { label: { name: "pve1" }, value: 7 },
    { label: { name: "pve2" }, value: 5 },
  ]);
  assert.equal(tile.by, undefined, "the tile is both sources', so it names neither");
  const [one] = together([guests("pve1", 7, 8, 2)]);
  assert.equal(one.shares, undefined, "one source alone isn't split");
  const [lost] = together([guests("pve1", 7, 8, 2), { ...guests("pve2", 5, 5, 4), meters: undefined }]);
  assert.equal(lost.meters, undefined, "meters only one host has would mislead");
});

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
  const by = choresBy(chores, new Date(2026, 9, 28, 23, 59));
  assert.deepEqual(
    by.due.map((c) => c.name),
    ["overdue", "today", "whenever"],
  );
  assert.deepEqual(
    by.week.map((c) => c.name),
    ["tomorrow", "in six days"],
  );
  assert.equal(by.next.name, "in a week");
  assert.equal(+by.next.from, +new Date(2026, 10, 4), "a day is midnight where the page is viewed");
  assert.equal(choresBy([], new Date()).next, null);
});

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
  const [both, apart] = joinCharts(charts);
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
  const [alone] = joinCharts([chart("Primary", 168, 10)]);
  assert.equal(alone.series[0].hours.length, 168);
  assert.deepEqual(
    alone.sources.map((s) => s.name),
    ["Primary"],
  );
});

test("past eight shares, the smallest fold into one; toned ones never fold", () => {
  const person = (name, value) => ({ label: { name }, value });
  const nine = ["A", "B", "C", "D", "E", "F", "G", "H", "I"].map((n, i) => person(n, i === 2 ? 1 : 10 + i));
  const out = fold(nine);
  assert.equal(out.length, 8);
  assert.deepEqual(
    out.slice(0, 7).map((s) => s.label.name),
    ["B", "D", "E", "F", "G", "H", "I"],
    "the seven largest keep their order",
  );
  const others = out[7];
  assert.equal(others.others, true);
  assert.deepEqual(others.label, { key: "shares.others", args: { n: 2 } });
  assert.equal(others.value, 10 + 1);
  assert.deepEqual(
    others.rows.map((r) => r.title.name),
    ["A", "C"],
  );
  assert.equal(fold(nine.slice(0, 8)).length, 8, "eight fit");
  const guests = Array.from({ length: 47 }, (_, i) => ({ ...person(`g${i}`, 1), tone: "blue" }));
  assert.equal(fold(guests).length, 47, "each guest backed up or not means something");
});

test("past most lines, the highest keep their places and the rest give way", () => {
  const line = (n) => ({ label: { name: `h${n}` }, hours: [n, n] });
  assert.deepEqual(busiest([line(1), line(2)], 8), [0, 1]);
  const ten = [3, 9, 1, 7, 5, 8, 2, 6, 4, 0].map(line);
  assert.deepEqual(busiest(ten, 8), [0, 1, 3, 4, 5, 7, 8], "the seven highest, in their order");
});

test("levels from several sources are one chart, their lines side by side over the hours they all have", () => {
  const cpu = (source, names, n) => ({
    area: "servers",
    title: { key: "chart.processor" },
    measures: true,
    source,
    by: source,
    series: names.map((name, k) => ({ label: { name }, hours: Array.from({ length: n }, () => 10 * (k + 1)) })),
  });
  const [one] = joinCharts([cpu("Cluster", ["alder", "birch"], 5), cpu("Vault", ["vault"], 3)]);
  assert.deepEqual(
    one.series.map((s) => [s.label.name, s.hours.length]),
    [
      ["alder", 3],
      ["birch", 3],
      ["vault", 3],
    ],
  );
  assert.equal(one.series[1].hours[0], 20, "never added up");
  assert.deepEqual(
    one.sources.map((s) => s.name),
    ["Cluster", "Vault"],
  );
  assert.equal(one.by, undefined);
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
  const [tile, other] = joinFigures([films, photos, shows]);
  assert.equal(tile.label, library);
  assert.equal(tile.value, 412, "the films' number leads");
  assert.equal(tile.detail, undefined, "each one's line moves to its part");
  assert.deepEqual(tile.also, [shows]);
  assert.deepEqual(
    tile.shares.map((s) => s.value),
    [2600, 1300],
  );
  assert.equal(other, photos);
  assert.deepEqual(joinFigures([films, photos]), [films, photos], "films alone keep their own tile");
  const [twin] = joinFigures([
    { ...films, by: "Radarr" },
    { ...films, by: "Radarr 4K" },
  ]);
  assert.deepEqual(
    twin.shares.map((s) => s.by),
    ["Radarr", "Radarr 4K"],
    "two Radarrs' parts say whose they are",
  );
});
