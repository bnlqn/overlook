import assert from "node:assert/strict";
import { test } from "node:test";
import { badge, checkRows, RECENT, section, strip, tip } from "../js/draw.js";
import { setWords } from "../js/text.js";

setWords({ "result.passed": "Passed", "result.failed": "Failed", ms: "{n} ms", up: "Up" }, {}, "en-GB");

test("a tooltip is safe inside an attribute", () => {
  const attr = tip("<b>x</b>", [["#fff", "Label", "1"]], "a note");
  assert.doesNotMatch(attr, /[<>"]/);
});

test("a strip always shows RECENT ticks, empty where there's no result yet", () => {
  const html = strip([{ ok: false, at: new Date(2026, 9, 28, 10, 0), ms: null, why: "timeout" }]);
  assert.equal(html.match(/<i[ >]/g).length, RECENT);
  assert.equal(html.match(/class="bad"/g).length, 1);
});

test("a logo is a file in icons/ or a URL, its colour and address escaped", () => {
  assert.match(badge({ icon: "gatus.svg" }), /src="icons\/gatus\.svg"/);
  assert.match(badge({ icon: "https://cdn.example.com/x.svg" }), /src="https:\/\/cdn\.example\.com\/x\.svg"/);
  assert.doesNotMatch(badge({ icon: 'x" onerror="go()', color: '"><script>' }), /onerror="|<script>/);
});

test("a check's name and failure are escaped", () => {
  const html = checkRows([
    {
      name: "<script>go()</script>",
      url: 'https://status.example.com/" onclick="go()',
      problem: true,
      failing: true,
      why: "<img>",
      recent: [],
      ms: null,
    },
  ]);
  assert.doesNotMatch(html, /<script>|<img>|" onclick=/);
});

test("a section draws each row, its value in its tone and its meter", () => {
  const html = section({
    area: "storage",
    title: { name: "Space" },
    rows: [{ title: { name: "NAS" }, value: { name: "Full" }, tone: "red", meter: { used: 95, total: 100 } }],
  });
  assert.match(html, /<h2 class="section-title">Space<\/h2>/);
  assert.match(html, /class="state tone-red">Full/);
  assert.match(html, /class="meter tone-red"/);
});
