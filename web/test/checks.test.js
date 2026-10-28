import assert from "node:assert/strict";
import { test } from "node:test";
import { areaState, checkState, quietSince, readChecks, shown, silent } from "../js/checks.js";

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
