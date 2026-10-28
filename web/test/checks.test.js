import assert from "node:assert/strict";
import { test } from "node:test";
import {
  areaState,
  asIfNow,
  checkState,
  choresBy,
  joinCharts,
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

test("chores group by the viewer's today: due, this week, and the next after it", () => {
  const chores = [
    { name: "later", from: "2026-11-20" },
    { name: "tomorrow", from: "2026-10-29" },
    { name: "today", from: "2026-10-28" },
    { name: "in six days", from: "2026-11-03" },
    { name: "in a week", from: "2026-11-04" },
    { name: "overdue", from: "2026-10-01" },
  ];
  const by = choresBy(chores, new Date(2026, 9, 28, 23, 59));
  assert.deepEqual(
    by.due.map((c) => c.name),
    ["overdue", "today"],
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
