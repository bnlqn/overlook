import assert from "node:assert/strict";
import { test } from "node:test";
import { ago, bytes, ms, say, setWords, span, t, when } from "../js/text.js";

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

test("spans, sizes and response times", () => {
  setWords(en, {}, "en");
  assert.equal(span(37 * 60e3), "37 min");
  assert.equal(span(36 * 3600e3), "36 h");
  assert.equal(span(3 * 864e5), "3 days");
  assert.equal(bytes(512), "512 B");
  assert.equal(bytes(785e6), "785 MB");
  assert.equal(bytes(8.4e12), "8.4 TB");
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
