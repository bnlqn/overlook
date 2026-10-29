import assert from "node:assert/strict";
import { test } from "node:test";
import { areas, badge, bars, checkRows, figureMeter, RECENT, section, shares, strip, tip } from "../js/draw.js";
import { setWords } from "../js/text.js";

setWords(
  { "result.passed": "Passed", "result.failed": "Failed", ms: "{n} ms", up: "Up", percent: "{n}%" },
  {},
  "en-GB",
);

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

test("a section draws its place, each row, its value in its tone and its meter", () => {
  const html = section(
    {
      area: "storage",
      title: { name: "Space" },
      rows: [{ title: { name: "NAS" }, value: { name: "Full" }, tone: "red", meter: { used: 95, total: 100 } }],
    },
    3,
  );
  assert.match(html, /<section style="--i:3"><h2 class="section-title">Space<\/h2>/);
  assert.match(html, /class="state tone-red">Full/);
  assert.match(html, /class="meter tone-red"/);
});

test("a meter in its own tone stays in it, however full", () => {
  const html = section({
    area: "servers",
    title: { name: "UPS" },
    rows: [{ title: { name: "Battery" }, meter: { used: 100, total: 100, tone: "green" } }],
  });
  assert.match(html, /class="meter tone-green"/);
});

test("a figure's meter names itself and says how full it is", () => {
  const html = figureMeter({ label: { name: "<b>Battery</b>" }, used: 96.6, total: 100, tone: "red" });
  assert.match(
    html,
    /^<span class="figure-meter"><span>&lt;b&gt;Battery&lt;\/b&gt;<\/span><span class="meter tone-red">/,
  );
  assert.match(html, /<span>97%<\/span><\/span>$/);
});

const lookups = (n, v = 5) => ({ label: { name: "Lookups" }, hours: Array(n).fill(v) });
const blocked = (n) => ({ label: { name: "<b>Blocked</b>" }, hours: Array(n).fill(1) });
const now = new Date(2026, 9, 28, 14, 30);
const axis = (html) => html.match(/class="axis">(.*?)<\/div>/s)[1].match(/<span/g)?.length ?? 0;

test("a day as bars: each hour's whole with its part inside, every sixth hour named", () => {
  const html = bars([lookups(24, 24), blocked(24)], now);
  assert.equal(html.match(/class="hour"/g).length, 24);
  assert.equal(
    html.match(/<i style="height:[\d.]+%;--c:var\(--m-orange\)">/g).length,
    24,
    "each hour's part, inside it",
  );
  assert.equal(axis(html), 4, "00:00, 06:00, 12:00 and 18:00");
  assert.match(html, /class="scale" style="bottom:100.00%">25</, "the scale rounds up the busiest hour");
  assert.doesNotMatch(html, /<b>Blocked/, "a label is escaped in its tooltip");
  assert.match(html, /class="whole" style="height:[\d.]+%;--c:var\(--m-accent-2\)"/, "the whole in the accent");
  assert.equal(bars([lookups(1)], now), "");
});

test("a week as areas: the days named and shaded, each source in the tooltip when several add up", () => {
  const sources = [
    { name: "Primary", hours: Array(168).fill(3) },
    { name: "<i>Standby</i>", hours: Array(168).fill(2) },
  ];
  const html = areas([lookups(168), blocked(168)], sources, now);
  assert.match(html, /class="hour-chart areas long"/);
  assert.equal(axis(html), 7, "a day each, its first part too short to name");
  assert.ok(html.match(/class="band"/g).length >= 3, "every other day shaded");
  assert.equal(html.match(/class="stroke"/g).length, 2, "a line for the whole and one for its part");
  assert.match(html, /Primary/);
  assert.doesNotMatch(html, /<i>Standby/, "a source's name is escaped");
  const alone = areas([lookups(24)], [sources[0]], now);
  assert.doesNotMatch(alone, /Primary/, "one source needs no naming");
  assert.equal(axis(alone), 4, "a day names its hours instead");
});

test("shares are as wide as their values, in their tone or the next colour, their names escaped", () => {
  const html = shares({
    label: { name: "Photos" },
    shares: [
      { label: { name: "<i>Kim</i>" }, value: 1 },
      { label: { name: "Noor" }, value: 3 },
      {
        label: { name: "Old" },
        value: 1,
        tone: "gray",
        rows: [{ title: { name: "Last" }, value: { name: "a while ago" } }],
      },
    ],
  });
  assert.match(html, /^<span class="shares"><i class="" style="flex:1;--c:var\(--m-series-1\)"/);
  assert.match(html, /<i class="" style="flex:3;--c:var\(--m-series-2\)"/);
  assert.match(html, /<i class="tone-gray" style="flex:1;--c:var\(--m-gray\)"/);
  assert.match(html, /60%/, "Noor's share of all five, with no rows of her own");
  assert.match(html, /a while ago/, "rows stand for the value and share");
  assert.doesNotMatch(html, /<i>Kim/);
  const many = {
    label: { name: "Backups" },
    shares: Array.from({ length: 13 }, (_, i) => ({ label: { name: `${i}` }, value: 1, tone: "blue" })),
  };
  assert.match(shares(many), /class="shares many"/, "many guests, each backed up or not, sit closer");
});

test("a tile's shares may start further on in the colours, and go round", () => {
  const two = {
    label: { name: "Library" },
    shares: [
      { label: { name: "Films" }, value: 1 },
      { label: { name: "Shows" }, value: 1 },
    ],
  };
  assert.match(shares(two, 2), /--c:var\(--m-series-3\).*--c:var\(--m-series-4\)/);
  assert.match(shares(two, 7), /--c:var\(--m-series-8\).*--c:var\(--m-series-1\)/, "after the eighth, the first");
});
