import assert from "node:assert/strict";
import { test } from "node:test";
import { ago, autonym, bytes, drawnAt, matches, ms, say, setWords, span, t, viewerLanguage, when } from "../js/text.js";

const en = {
  "span.minutes": "{n} min",
  "span.hours": "{n} h",
  "span.days": { one: "{n} day", other: "{n} days" },
  ago: "{ms:span} ago",
  "ago.now": "just now",
  "when.today": "today, {at:clock}",
  "when.yesterday": "yesterday, {at:clock}",
  ms: "{n} ms",
  "ms.under": "&lt;1 ms",
  back: "<b>{name}</b> came back after {ms:span}",
  count: "{n}",
  problems: { one: "{n} problem", other: "{n} problems" },
};

test("a message escapes its values and writes numbers the viewer's way", () => {
  setWords(en, {}, "en-GB");
  assert.equal(
    t("back", { name: '<img src=x onerror="go()">', ms: 2 * 3600e3 }),
    "<b>&lt;img src=x onerror=&quot;go()&quot;&gt;</b> came back after 2 h",
  );
  setWords(en, {}, "de-DE");
  assert.equal(t("count", { n: 1234.5 }), "1.234,5");
});

test("a choice of links is each a link, its address escaped, the last after or", () => {
  setWords({ ...en, pick: "add one, such as {to:choice}", a: "A" }, {}, "en-GB");
  const to = [
    { key: "a", url: 'https://a.example.com/?q="x"' },
    { name: "B & C", url: "https://b.example.com" },
  ];
  assert.equal(
    t("pick", { to }),
    'add one, such as <a href="https://a.example.com/?q=&quot;x&quot;" target="_blank" rel="noopener">A</a> or ' +
      '<a href="https://b.example.com" target="_blank" rel="noopener">B &amp; C</a>',
  );
});

test("a message names an adapter by its key, as the setup page names it", () => {
  setWords({ ...en, "setup.adapter.nut": "NUT", gone: "{name:adapter} refused the key" }, {}, "en");
  assert.equal(t("gone", { name: "nut" }), "NUT refused the key");
});

test("a plural follows the rules of the language its message came from", () => {
  const fr = { problems: { one: "{n} problème", other: "{n} problèmes" } };
  setWords(en, fr, "fr-FR");
  assert.equal(t("problems", { n: 0 }), "0 problème");
  assert.equal(t("problems", { n: 2 }), "2 problèmes");
  setWords(en, {}, "fr-FR");
  assert.equal(t("problems", { n: 0 }), "0 problems", "English's own rules for an English message");
});

test("a message the viewer's language lacks is English, and one no table has is its key", () => {
  setWords(en, { "ms.under": "&lt;1 ms" }, "fr-FR");
  assert.equal(t("span.days", { n: 1 }), "1 day");
  assert.equal(t("no.such<key>"), "no.such&lt;key&gt;");
});

test("a snapshot's text is a message, or a name as written", () => {
  setWords(en, {}, "en");
  assert.equal(say({ key: "problems", args: { n: 1 } }), "1 problem");
  assert.equal(say({ name: "web & <db>" }), "web &amp; &lt;db&gt;");
});

test("⌘K matches the letters typed, in order, whatever their case and accents", () => {
  assert.ok(matches("pxv", "Proxmox VE"));
  assert.ok(!matches("xp", "Proxmox VE"));
  assert.ok(matches("PROX", "proxmox"));
  assert.ok(matches("medias", "Médias"));
  assert.ok(matches("dnsnet", "DNS", undefined, "Network"), "across the words, a missing one left out");
  assert.ok(matches("  ", "DNS"), "nothing typed matches every word");
  // The words are as written: the markup that names a check's source, a
  // <span class="by">, is the page's and never matches.
  assert.ok(!matches("span", "DNS", "gatus"));
});

test("spans, sizes and response times", () => {
  setWords(en, {}, "en");
  assert.equal(span(37 * 60e3), "37 min");
  assert.equal(span(36 * 3600e3), "36 h");
  assert.equal(span(3 * 864e5), "3 days");
  assert.equal(bytes(512), "512 B");
  assert.equal(bytes(785e6), "785 MB");
  assert.equal(bytes(8.4e12), "8.4 TB");
  setWords(en, {}, "fr");
  assert.match(bytes(8.4e12), /^8,4\sTo$/, "in the units French writes");
  assert.equal(bytes(512), "512 B");
  setWords(en, {}, "en");
  assert.equal(ms(0.4), "&lt;1 ms");
  assert.equal(ms(42.4), "42 ms");
  assert.equal(ms(null), "");
});

test("moments, from now", () => {
  setWords(en, {}, "en-GB");
  const now = new Date(2026, 9, 28, 15, 0);
  assert.equal(ago(new Date(2026, 9, 28, 14, 59, 30), +now), "just now");
  assert.equal(ago(new Date(2026, 9, 28, 10, 0), +now), "5 h ago");
  assert.equal(when(new Date(2026, 9, 28, 14, 5), now), "today, 14:05");
  assert.equal(when(new Date(2026, 9, 27, 9, 30), now), "yesterday, 09:30");
  assert.match(when(new Date(2026, 9, 20, 9, 30), now), /20 Oct/);
});

test("a message's moment counts from the moment the page is drawn for", () => {
  setWords({ ...en, since: "since {at:when}" }, {}, "en-GB");
  drawnAt(+new Date(2026, 9, 28, 15, 0));
  assert.equal(t("since", { at: new Date(2026, 9, 27, 9, 30) }), "since yesterday, 09:30");
  drawnAt(+new Date(2026, 9, 29, 15, 0));
  assert.match(t("since", { at: new Date(2026, 9, 27, 9, 30) }), /27 Oct/);
});

test("the footer names each language as it names itself, and keeps a browser's region", () => {
  assert.equal(autonym("de"), "Deutsch");
  assert.equal(autonym("fr"), "Français", "with a capital, as a list of languages writes it");
  assert.equal(autonym("pt"), "Português (Brasil)", "Portuguese as its file writes it");
  assert.equal(autonym("zh"), "简体中文", "Chinese as its file writes it");
  assert.equal(viewerLanguage("", "en-GB"), "en-GB", "with no choice, the browser's");
  assert.equal(viewerLanguage("de", "en-GB"), "de");
  assert.equal(viewerLanguage("en", "en-GB"), "en-GB", "the browser's own language keeps its region");
  assert.equal(viewerLanguage("fr", "fr-CA"), "fr-CA");
  assert.equal(viewerLanguage("pt", "en-US"), "pt-BR", "its numbers and dates as its words");
  assert.equal(viewerLanguage("zh", "fr"), "zh-Hans");
  assert.equal(viewerLanguage("xx", "en-GB"), "en-GB", "one the page doesn't speak is no choice");
});
