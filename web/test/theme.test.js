// theme.js picks the look before the page is drawn: the viewer's choice,
// or the system's without one, and again when either changes. It runs here
// as the browser runs it, in a context of its own, with a document, a
// browser's keeping and a system's look made up for each case.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import vm from "node:vm";

const source = readFileSync(new URL("../js/theme.js", import.meta.url), "utf8");

/** Runs theme.js with a system's look, a choice kept, or a browser that keeps nothing. */
function run({ systemLight = false, chosen = null, keepsNothing = false } = {}) {
  const listeners = {};
  const system = { matches: systemLight, addEventListener: (_, f) => (listeners.system = f) };
  const metas = [
    { dataset: { theme: "light" }, media: "(prefers-color-scheme: light)" },
    { dataset: { theme: "dark" }, media: "" },
  ];
  const kept = { "overlook.theme": chosen };
  const context = {
    document: { documentElement: { dataset: {} }, querySelectorAll: () => metas },
    localStorage: {
      getItem: (key) => {
        if (keepsNothing) throw new Error("this browser keeps nothing");
        return kept[key] ?? null;
      },
    },
    matchMedia: () => system,
    addEventListener: (_, f) => (listeners.storage = f),
  };
  vm.runInNewContext(source, context);
  return { root: context.document.documentElement, metas, system, kept, listeners };
}

test("with no choice, the page takes the system's look", () => {
  assert.equal(run({ systemLight: true }).root.dataset.theme, "light");
  assert.equal(run({ systemLight: false }).root.dataset.theme, "dark");
});

test("a choice wins over the system's look, and the phone's bar takes its colour", () => {
  const { root, metas } = run({ systemLight: true, chosen: "dark" });
  assert.equal(root.dataset.theme, "dark");
  assert.deepEqual(
    metas.map((m) => m.media),
    ["not all", "all"],
  );
  assert.equal(run({ systemLight: false, chosen: "light" }).root.dataset.theme, "light");
});

test("a choice it doesn't know, or a browser that keeps nothing, follows the system", () => {
  assert.equal(run({ systemLight: true, chosen: "sepia" }).root.dataset.theme, "light");
  assert.equal(run({ systemLight: true, keepsNothing: true }).root.dataset.theme, "light");
});

test("it follows the system as it changes, and a choice made later or in another tab", () => {
  const { root, system, kept, listeners } = run({ systemLight: false });
  system.matches = true;
  listeners.system();
  assert.equal(root.dataset.theme, "light", "the system turned light");
  kept["overlook.theme"] = "dark";
  listeners.storage({ key: "something.else" });
  assert.equal(root.dataset.theme, "light", "another key changes nothing");
  listeners.storage({ key: "overlook.theme" });
  assert.equal(root.dataset.theme, "dark", "the choice, told as another tab's is");
});
