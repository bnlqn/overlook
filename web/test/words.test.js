// Every message the page names is in the English table, so no viewer ever
// sees a key where a word should be.

import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { test } from "node:test";
import { AREAS, page } from "../js/page.js";
import { tables, translatedFrom } from "./translated.mjs";

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
    read("setup.html"),
  ].join("\n");
  const named = [
    ...keysGiven(sources),
    // Texts written as data; a page's shortcut, also a key, is one letter.
    ...[...sources.matchAll(/\bkey: "([\w.]{2,})"/g)].map((m) => m[1]),
    ...[...sources.matchAll(/data-t="([\w.]+)"/g)].map((m) => m[1]),
    ...[...sources.matchAll(/(?:group|pageName)\([^)]*"([\w.]+)"\)/g)].map((m) => m[1]),
    // The messages the page names by a variable's value.
    ...AREAS.map((a) => `area.${a}`),
    ...["up", "degraded", "failing", "down", "unknown"].map((s) => `state.${s}`),
    ...["down", "degraded"].flatMap((s) => [`status.areas_${s}`, `status.every_${s}`]),
    // The setup page's groups past the areas; its adapters', fields' and
    // failures' words are checked by the server's tests, which know them.
    "setup.group.checkengines",
    "setup.group.chores",
    // The setup code's field, drawn as the adapters' are.
    "setup.field.code",
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

// Two sections of an area that read the same must be told apart by their
// sources. The page names a section by its source only where another source
// gives one of the same key, so adapters that read the same thing share the
// key of its title, as Hosts and Certificates are shared.
test("no area of the demo's homelabs shows two sections a reader can't tell apart", () => {
  for (const name of ["snapshot", "small", "large"]) {
    const snap = JSON.parse(readFileSync(new URL(`../demo/${name}.json`, web), "utf8"));
    for (const a of page(snap, Date.parse(snap.at)).areas) {
      const shown = a.sections.map(
        (s) => `${s.title.key ? english[s.title.key] : s.title.name}${s.by ? ` · ${s.by}` : ""}`,
      );
      assert.deepEqual(
        shown.filter((r, i) => shown.indexOf(r) !== i),
        [],
        `${name}'s ${a.area}`,
      );
    }
  }
});

// A translation (ADR 0006) may lack keys, which show in English, but what it
// has must say what English says: the same values, the same markup, and the
// plural forms of its own language, "other" among them. Its file is named
// as the server serves it, by its language's code.
test("every translation says what English says, in its own language's plural forms", () => {
  const files = readdirSync(new URL("i18n/", web)).filter((f) => f.endsWith(".json"));
  const placeholders = (s) => [...s.matchAll(/\{\w+(?::\w+)?\}/g)].map((m) => m[0]);
  const markup = (s) =>
    [...s.matchAll(/<\/?\w+/g)]
      .map((m) => m[0])
      .sort()
      .join(" ");
  const forms = (v) => (typeof v === "string" ? { other: v } : v);
  for (const file of files) {
    assert.match(file, /^[a-z]{2,3}\.json$/, `${file} isn't named by a language's code`);
    const language = file.replace(".json", "");
    const plurals = new Intl.PluralRules(language).resolvedOptions().pluralCategories;
    for (const [key, value] of Object.entries(JSON.parse(read(`i18n/${file}`)))) {
      assert.ok(key in english, `${file}: ${key} isn't an English key`);
      const en = forms(english[key]);
      const may = new Set(Object.values(en).flatMap(placeholders));
      // What every English form says, each form must; a plural form may give
      // the number in words.
      const must = Object.values(en)
        .map(placeholders)
        .reduce((a, b) => a.filter((p) => b.includes(p)));
      const plural = typeof value !== "string";
      if (plural) assert.ok("other" in value, `${file}: ${key} lacks "other"`);
      for (const [form, s] of Object.entries(forms(value))) {
        const where = `${file}: ${key}${plural ? `.${form}` : ""}`;
        if (plural) assert.ok(plurals.includes(form), `${where} isn't one of ${language}'s plural forms`);
        for (const p of placeholders(s)) assert.ok(may.has(p), `${where} has ${p}, which English doesn't`);
        for (const p of must) assert.ok(placeholders(s).includes(p) || (plural && p === "{n}"), `${where} lacks ${p}`);
        assert.equal(markup(s), markup(en.other), `${where} changes the markup`);
      }
    }
  }
});

// Each translation was made from the English it says now (ADR 0020): a
// message whose English has changed since is updated, or deleted so that it
// shows in English, and `make translated` records it. How much of English
// each has is said, so that a lag shows without failing.
test("every translation was made from today's English, as make translated recorded", (t) => {
  const { english, translations } = tables();
  const recorded = JSON.parse(read("test/translated.json"));
  const now = translatedFrom({ english, translations });
  const problems = [];
  for (const [language, keys] of Object.entries(now)) {
    t.diagnostic(`${language}: ${Object.keys(keys).length} of ${Object.keys(english).length} messages`);
    const was = recorded[language] ?? {};
    for (const [key, hash] of Object.entries(keys)) {
      if (!(key in was)) problems.push(`${language}: ${key} isn't recorded: run make translated`);
      else if (was[key] !== hash)
        problems.push(
          `${language}: ${key} was translated from older English: update it in web/i18n/${language}.json, or delete it so that it shows in English, then run make translated`,
        );
    }
    for (const key of Object.keys(was)) {
      if (!(key in keys))
        problems.push(`${language}: ${key} is recorded but no longer translated: run make translated`);
    }
  }
  for (const language of Object.keys(recorded)) {
    if (!(language in now)) problems.push(`${language} is recorded but has no translation: run make translated`);
  }
  assert.deepEqual(problems, []);
});
