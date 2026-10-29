// Every message the page asks for is in the English table, so no viewer
// ever sees a key where a word should be.

import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { test } from "node:test";
import { AREAS } from "../js/checks.js";

const web = new URL("../", import.meta.url);
const read = (path) => readFileSync(new URL(path, web), "utf8");
const english = Object.assign(
  {},
  ...readdirSync(new URL("i18n/en/", web)).map((file) => JSON.parse(read(`i18n/en/${file}`))),
);

test("every message the page names is in the English table", () => {
  const sources = [
    ...readdirSync(new URL("js/", web)).map((file) => read(`js/${file}`)),
    read("index.html"),
    read("area.html"),
  ].join("\n");
  const named = [
    ...[...sources.matchAll(/\bt\("([\w.]+)"/g)].map((m) => m[1]),
    ...[...sources.matchAll(/data-t="([\w.]+)"/g)].map((m) => m[1]),
    ...[...sources.matchAll(/(?:group|pageName)\([^)]*"([\w.]+)"\)/g)].map((m) => m[1]),
    // The messages the page names by a variable's value.
    ...AREAS.map((a) => `area.${a}`),
    ...["up", "degraded", "down", "unknown"].map((s) => `state.${s}`),
    ...["down", "degraded"].flatMap((s) => [`status.areas_${s}`, `status.every_${s}`]),
  ];
  assert.ok(named.length > 50, `only ${named.length} messages found: is the pattern still right?`);
  const missing = [...new Set(named)].filter((key) => !(key in english));
  assert.deepEqual(missing, []);
});
