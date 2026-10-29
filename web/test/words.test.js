// Every message the page names is in the English table, so no viewer ever
// sees a key where a word should be.

import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { test } from "node:test";
import { AREAS, page } from "../js/page.js";

const web = new URL("../", import.meta.url);
const read = (path) => readFileSync(new URL(path, web), "utf8");
const english = Object.assign(
  {},
  ...readdirSync(new URL("i18n/en/", web)).map((file) => JSON.parse(read(`i18n/en/${file}`))),
);

// The first thing each t(…) is given, whole: a key, or a choice of keys.
function keysGiven(source) {
  const given = [];
  for (const m of source.matchAll(/\bt\(/g)) {
    let i = m.index + 2;
    let depth = 0;
    let quote = null;
    for (; i < source.length; i++) {
      const ch = source[i];
      if (quote) {
        if (ch === "\\") i++;
        else if (ch === quote) quote = null;
      } else if (`"'\``.includes(ch)) quote = ch;
      else if ("([{".includes(ch)) depth++;
      else if (")]}".includes(ch) && depth-- === 0) break;
      else if (ch === "," && !depth) break;
    }
    given.push(...[...source.slice(m.index + 2, i).matchAll(/"([\w.]+)"/g)].map((k) => k[1]));
  }
  return given;
}

// The texts page() returns, each a message's key and its values.
function textKeys(x, keys = []) {
  if (Array.isArray(x)) for (const v of x) textKeys(v, keys);
  else if (x && typeof x === "object" && !(x instanceof Date)) {
    if (typeof x.key === "string" && Object.keys(x).every((k) => k === "key" || k === "args")) keys.push(x.key);
    for (const v of Object.values(x)) textKeys(v, keys);
  }
  return keys;
}

test("every message the page names is in the English table", () => {
  const sources = [
    ...readdirSync(new URL("js/", web)).map((file) => read(`js/${file}`)),
    read("index.html"),
    read("area.html"),
  ].join("\n");
  const named = [
    ...keysGiven(sources),
    // Texts written as data; a page's shortcut, also a key, is one letter.
    ...[...sources.matchAll(/\bkey: "([\w.]{2,})"/g)].map((m) => m[1]),
    ...[...sources.matchAll(/data-t="([\w.]+)"/g)].map((m) => m[1]),
    ...[...sources.matchAll(/(?:group|pageName)\([^)]*"([\w.]+)"\)/g)].map((m) => m[1]),
    // The messages the page names by a variable's value.
    ...AREAS.map((a) => `area.${a}`),
    ...["up", "degraded", "down", "unknown"].map((s) => `state.${s}`),
    ...["down", "degraded"].flatMap((s) => [`status.areas_${s}`, `status.every_${s}`]),
  ];
  assert.ok(named.length > 50, `only ${named.length} messages found: is the pattern still right?`);
  for (const key of ["result.failed", "page.overview", "shares.others"]) {
    assert.ok(named.includes(key), `${key}, a choice's or a text's, is found`);
  }
  const missing = [...new Set(named)].filter((key) => !(key in english));
  assert.deepEqual(missing, []);
});

test("every message page() says of the demo's homelabs is in the English table", () => {
  const said = ["snapshot", "small", "large"].flatMap((name) => {
    const snap = JSON.parse(readFileSync(new URL(`../demo/${name}.json`, web), "utf8"));
    return textKeys(page(snap, Date.parse(snap.at)));
  });
  assert.ok(said.length > 100, `only ${said.length} texts found: is the walk still right?`);
  const missing = [...new Set(said)].filter((key) => !(key in english));
  assert.deepEqual(missing, []);
});
