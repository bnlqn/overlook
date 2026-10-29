// The pieces every page draws, as HTML strings: icons, state glyphs, logos,
// tooltips, the strip of results, rows, sections, sparklines, charts by the
// hour as bars or areas, and shares. Every value from the snapshot is escaped
// here or by t().

import { busiest, checkState, fold } from "./checks.js";
import { clock, compact, day, esc, ms, number, say, t, weekday } from "./text.js";

// 16 px strokes for the pages, figures and events; apps and tools show their
// own logos.
const ICON = {
  pulse: '<path d="M1.75 8.5h2.5l1.75-4.5 3.5 8.5 1.75-4h3"/>',
  server:
    '<rect x="2.5" y="2.5" width="11" height="4.5" rx="1.25"/><rect x="2.5" y="9" width="11" height="4.5" rx="1.25"/><path d="M5 4.75h.01M5 11.25h.01"/>',
  network:
    '<circle cx="8" cy="8" r="5.75"/><path d="M2.25 8h11.5M8 2.25c1.8 1.9 1.8 9.6 0 11.5M8 2.25c-1.8 1.9-1.8 9.6 0 11.5"/>',
  storage:
    '<ellipse cx="8" cy="4" rx="5.25" ry="2"/><path d="M2.75 4v8c0 1.1 2.35 2 5.25 2s5.25-.9 5.25-2V4M2.75 8c0 1.1 2.35 2 5.25 2s5.25-.9 5.25-2"/>',
  apps: '<rect x="2.5" y="2.5" width="4.5" height="4.5" rx="1.2"/><rect x="9" y="2.5" width="4.5" height="4.5" rx="1.2"/><rect x="2.5" y="9" width="4.5" height="4.5" rx="1.2"/><rect x="9" y="9" width="4.5" height="4.5" rx="1.2"/>',
  shield: '<path d="M8 1.75 3 3.5v3.75c0 3 2 5.4 5 6.5 3-1.1 5-3.5 5-6.5V3.5Z"/>',
  photos:
    '<rect x="2" y="3" width="12" height="10" rx="2"/><circle cx="5.75" cy="6.5" r="1.1"/><path d="m14 10.5-3.25-3.25L4 13"/>',
  film: '<rect x="2.5" y="2.25" width="11" height="11.5" rx="1.75"/><path d="M5.25 2.25v11.5M10.75 2.25v11.5M2.5 6h2.75M2.5 10h2.75M10.75 6h2.75M10.75 10h2.75"/>',
  search: '<circle cx="7" cy="7" r="4.25"/><path d="m10.25 10.25 3.5 3.5"/>',
  archive:
    '<rect x="2" y="2.75" width="12" height="3.5" rx="1"/><path d="M3 6.25v6a1.5 1.5 0 0 0 1.5 1.5h7a1.5 1.5 0 0 0 1.5-1.5v-6M6.5 9h3"/>',
  plug: '<path d="M5.5 1.75v3.5M10.5 1.75v3.5M3.75 5.25h8.5v2a4.25 4.25 0 0 1-8.5 0ZM8 11.5v2.75"/>',
  stack: '<path d="m8 1.75 6 3-6 3-6-3ZM2 8l6 3 6-3M2 11.25l6 3 6-3"/>',
  box: '<path d="m8 1.75 5.75 3v6.5L8 14.25l-5.75-3v-6.5ZM2.25 4.75 8 7.75l5.75-3M8 7.75v6.5"/>',
  download: '<path d="M8 2.25v8M4.75 7 8 10.25 11.25 7M2.75 13.25h10.5"/>',
  play: '<path d="M4.75 2.75v10.5L13 8Z" stroke-linejoin="round"/>',
  cloud:
    '<path d="M4.75 12.75h6.75a3 3 0 0 0 .35-5.98 4.25 4.25 0 0 0-8.1 1.03 2.5 2.5 0 0 0 1 4.95Z" stroke-linejoin="round"/>',
  document: '<path d="M4 1.75h5.25l3 3v9.5H4Z" stroke-linejoin="round"/><path d="M9 1.75V5h3.25M6 8.25h4M6 10.75h4"/>',
  refresh: '<path d="M13.25 8a5.25 5.25 0 1 1-1.54-3.71M13.25 2.5v3h-3"/>',
  menu: '<path d="M2.5 4.5h11M2.5 8h11M2.5 11.5h11"/>',
  out: '<path d="M6 3.25h6.75V10M12.5 3.5 3.5 12.5"/>',
  pr: '<circle cx="4.5" cy="3.75" r="1.75"/><circle cx="4.5" cy="12.25" r="1.75"/><circle cx="11.5" cy="12.25" r="1.75"/><path d="M4.5 5.5v5M11.5 10.5V6.25a2 2 0 0 0-2-2H7.5M9 2.75 7.5 4.25 9 5.75"/>',
  chevron: '<path d="m6 3.5 4.5 4.5L6 12.5"/>',
  caret: '<path d="m4.5 6.5 3.5 3.5 3.5-3.5"/>',
  mark: '<path d="M2.75 7.4 8 2.75l5.25 4.65V13a.75.75 0 0 1-.75.75h-9a.75.75 0 0 1-.75-.75Z"/><path d="M5.75 13.75v-3h4.5v3M5.75 8h4.5"/>',
};

/** One of the page's icons by name; an unknown name draws the pulse. */
export const svg = (name, cls = "ic") =>
  `<svg class="${cls}" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICON[name] || ICON.pulse}</svg>`;

// A state's glyph: a filled circle with a tick, a half-filled ring, a filled
// circle with a bang, or a dashed ring.
const STATE_ICON = {
  up: '<circle cx="7" cy="7" r="6" fill="currentColor"/><path d="m4.4 7.2 1.8 1.8 3.5-3.7" style="stroke:var(--m-panel)" stroke-width="1.6" fill="none" stroke-linecap="round" stroke-linejoin="round"/>',
  degraded:
    '<circle cx="7" cy="7" r="5.25" stroke="currentColor" stroke-width="1.5" fill="none"/><path d="M7 3.5a3.5 3.5 0 0 1 0 7Z" fill="currentColor"/>',
  down: '<circle cx="7" cy="7" r="6" fill="currentColor"/><path d="M7 3.9v3.6M7 10h.01" style="stroke:var(--m-panel)" stroke-width="1.7" stroke-linecap="round"/>',
  unknown:
    '<circle cx="7" cy="7" r="5.25" stroke="currentColor" stroke-width="1.5" fill="none" stroke-dasharray="2 2.1"/>',
};

/** Each state's tone. */
export const TONE = { up: "green", degraded: "orange", down: "red", unknown: "gray" };

/** A state's glyph, in its tone. */
export const stateIcon = (state) =>
  `<svg class="si tone-${TONE[state]}" viewBox="0 0 14 14" aria-hidden="true">${STATE_ICON[state]}</svg>`;

/** A state's word. */
export const word = (state) => t(`state.${state}`);

/** The colours of a result that passed and one that failed. */
export const GREEN = "#4CB782";
export const RED = "#EB5757";

/** Each area's colour, for its tile, its figures, its charts and its row in the week. */
export const TINT = { servers: "#7170FF", network: "#4EA7FC", storage: "#F2994A", apps: "#F06595" };

/**
 * The colour of what a figure or a chart counts by the hour: the page's
 * accent, which custom.css may change. The part within it, such as the
 * blocked lookups, is orange.
 */
export const HOURS = "var(--m-accent-2)";
const PART = "var(--m-orange)";

/** The eight colours that tell apart the parts of one thing, in order. */
export const SERIES = Array.from({ length: 8 }, (_, i) => `var(--m-series-${i + 1})`);

/**
 * A share's colour: its tone, which means something, or else its slot's,
 * from where its figure starts in the colours. The parts folded into
 * "others" are gray.
 */
export const shareColour = (s, i, start = 0) =>
  s.tone ? `var(--m-${s.tone})` : s.others ? "var(--m-gray)" : SERIES[((s.slot ?? i) + start) % SERIES.length];

/** How many results a strip shows: as many as a check engine gives (snapshot.Recent). */
export const RECENT = 30;

/** An icon's address: a URL as given, or a file the server has in icons/. */
const iconSrc = (icon) => (/^(https?:)?\/\//.test(icon) ? icon : `icons/${icon}`);

/** An app's or a tool's logo on its tile; size is xs, sm, md or lg. */
export const badge = (s, size = "md") =>
  `<span class="app-icon ${size}" style="--tint:${esc(s.color || "#7170FF")}">${
    s.icon ? `<img src="${esc(iconSrc(s.icon))}" alt="" decoding="async">` : svg("apps")
  }</span>`;

/**
 * A meter's tone: its own, or with none blue, turning orange past 80% and red
 * past 90%, as a disk's does.
 */
const meterTone = (m) => m.tone || (m.used / m.total > 0.9 ? "red" : m.used / m.total > 0.8 ? "orange" : "blue");

/** A thin bar for how full something is, in its meter's tone. */
export function meter(used, total, tone) {
  return `<span class="meter tone-${meterTone({ used, total, tone })}"><span style="width:${Math.min(100, (100 * used) / total).toFixed(1)}%"></span></span>`;
}

/**
 * A tooltip, for a data-tip attribute, from HTML: a title, rows of a colour,
 * a label and a value, then a line.
 */
export function tip(title, rows = [], note = "") {
  const lines = rows
    .map(
      ([c, label, value]) =>
        `<span class="tr"><i style="--c:${c || "transparent"}"></i><span>${label}</span><span>${value ?? ""}</span></span>`,
    )
    .join("");
  return esc(`<b>${title}</b>${lines}${note ? `<p>${note}</p>` : ""}`);
}

/**
 * The last RECENT results as ticks, oldest first: green, red, or empty where
 * there's no result yet. A result may bring its own tooltip.
 */
export function strip(recent) {
  const pad = Array(Math.max(0, RECENT - recent.length)).fill(null);
  const ticks = [...pad, ...recent.slice(-RECENT)].map((r) => {
    if (!r) return "<i></i>";
    const label = t(r.ok ? "result.passed" : "result.failed");
    const tp = r.tip || tip(clock(r.at), [[r.ok ? GREEN : RED, label, ms(r.ms)]], esc(r.why || ""));
    // An area's moment shows the share of its checks that failed then.
    const part = !r.ok && r.bad < 1 ? ` part" style="--bad:${r.bad.toFixed(3)}` : "";
    return `<i class="${r.ok ? "ok" : "bad"}${part}" data-tip="${tp}"></i>`;
  });
  return `<span class="strip">${ticks.join("")}</span>`;
}

/** The rows of an area's checks, each linking to its page in its engine. */
export function checkRows(checks) {
  return checks
    .map((c) => {
      const detail = c.problem
        ? esc(c.why || "")
        : c.failing
          ? c.threshold
            ? t("check.failed_of", { run: c.run, threshold: c.threshold, why: c.why || "" })
            : t("check.failed_run_why", { n: c.run, why: c.why || "" })
          : c.recent.length
            ? ""
            : t("check.no_results");
      return `<a class="check" href="${esc(c.url)}" target="_blank" rel="noopener">
      ${stateIcon(checkState(c))}
      <span class="copy"><span class="title">${esc(c.name)}${by(c)}</span>${detail ? `<span class="detail">${detail}</span>` : ""}</span>
      ${strip(c.recent)}<span class="value">${c.failing ? "" : ms(c.ms)}</span></a>`;
    })
    .join("");
}

/** A figure's meter: its label, its bar, and how full it is. */
export const figureMeter = (m) =>
  `<span class="figure-meter"><span>${m.label ? say(m.label) : ""}</span>${meter(m.used, m.total, m.tone)}<span>${t("percent", { n: Math.round((100 * m.used) / m.total) })}</span></span>`;

/**
 * A section of the snapshot: its title, then its rows, each a title (and a
 * line under it) and its value. With a meter, the meter runs the row's whole
 * width under both and the line goes under the meter, so every row's bar has
 * the same edges. It carries its place among the page's sections, i, so a
 * page in one column can keep their order.
 */
export function section(s, i) {
  // Past twelve rows, a section shows those with a tone that says something
  // is wrong, a nearly full disk's meter too, then its first rows up to
  // twelve; the rest wait behind "Show N more", in their order.
  const wrong = (r) =>
    ["red", "orange", "gray"].includes(r.tone) || (r.meter && ["red", "orange"].includes(meterTone(r.meter)));
  const kept = new Set(s.rows.length > LONG ? s.rows.filter(wrong) : s.rows);
  for (const r of s.rows) if (kept.size < LONG) kept.add(r);
  const order = [...s.rows.filter((r) => kept.has(r)), ...s.rows.filter((r) => !kept.has(r))];
  const rows = order.map((r) => {
    const title = `<span class="title">${say(r.title)}</span>`;
    const value = r.value ? (r.tone ? `<span class="state tone-${r.tone}">${say(r.value)}</span>` : say(r.value)) : "";
    const detail = r.detail ? say(r.detail) : "";
    if (r.meter) {
      return `<div class="row"><span class="copy">${title}</span><span class="value">${value}</span>${meter(r.meter.used, r.meter.total, r.meter.tone)}${detail ? `<span class="detail under">${detail}</span>` : ""}</div>`;
    }
    // Several meters, such as a host's processor and memory, sit side by side
    // under the row, each with its label.
    const meters = r.meters?.length ? `<span class="meters under">${r.meters.map(figureMeter).join("")}</span>` : "";
    return `<div class="row"><span class="copy">${title}${detail ? `<span class="detail">${detail}</span>` : ""}</span><span class="value">${value}</span>${meters}</div>`;
  });
  return `<section style="--i:${i}"><h2 class="section-title">${say(s.title)}${by(s)}</h2><div class="list card">${more(rows, kept.size)}</div></section>`;
}

/** How many rows a section shows before the rest wait behind "Show N more". */
const LONG = 12;

/**
 * Rows, the first keep of them shown and the rest behind "Show N more",
 * which shows them in place.
 */
export const more = (rows, keep) =>
  rows.length <= keep
    ? rows.join("")
    : `${rows.slice(0, keep).join("")}<details class="more"><summary><span>${t("more.show", { n: rows.length - keep })}</span><span>${t("more.fewer")}</span></summary>${rows.slice(keep).join("")}</details>`;

/** The source a part came from, after its title, where the page names it. */
export const by = (p) => (p.by ? `<span class="by"> · ${esc(p.by)}</span>` : "");

let uid = 0;
/**
 * A line over its area, fading down: values left to right, from zero. Each
 * point's column brings its tooltip, from tips.
 */
export function sparkline(values, color, tips = []) {
  const v = values.map((x) => (Number.isFinite(x) ? x : 0));
  const n = v.length;
  if (n < 2) return "";
  const max = Math.max(...v) || 1;
  const id = `sp${++uid}`;
  const step = 100 / (n - 1);
  const pts = v.map((y, i) => `${(step * i).toFixed(1)} ${(27 - (24 * y) / max).toFixed(1)}`);
  const hits = tips
    .map((tp, i) => {
      const x0 = Math.max(0, (i - 0.5) * step);
      const x1 = Math.min(100, (i + 0.5) * step);
      return `<rect class="hit" x="${x0.toFixed(2)}" y="0" width="${(x1 - x0).toFixed(2)}" height="28" data-tip="${tp}"/>`;
    })
    .join("");
  return `<svg class="spark" viewBox="0 0 100 28" preserveAspectRatio="none" aria-hidden="true">
    <defs><linearGradient id="${id}" x1="0" y1="0" x2="0" y2="1"><stop offset="0" style="stop-color:${color};stop-opacity:.32"/><stop offset="1" style="stop-color:${color};stop-opacity:0"/></linearGradient></defs>
    <path d="M${pts.join(" L")} L100 28 L0 28Z" fill="url(#${id})"/>
    <path class="line" d="M${pts.join(" L")}" pathLength="1" style="stroke:${color}"/>${hits}</svg>`;
}

/** The start of each of the last n hours, the last one this hour. */
export function hourStart(i, n, now = new Date()) {
  const d = new Date(now);
  d.setMinutes(0, 0, 0);
  return new Date(+d - (n - 1 - i) * 3600e3);
}

// The top of a chart's scale: the least of 1, 2, 2.5 or 5 times a power of
// ten that holds v.
function niceMax(v) {
  if (!(v > 0)) return 1;
  const step = 10 ** Math.floor(Math.log10(v));
  return [1, 2, 2.5, 5, 10].map((f) => f * step).find((x) => x >= v);
}

// A number on a chart's scale, short once it's long: "5,000", "250K".
const scaleNumber = (v) => (v >= 1e5 ? compact(v) : number(v));

/** The colours of a chart's series: the whole, then each part within it. */
export const seriesColours = (n) => [HOURS, PART, GREEN, RED].slice(0, n);

/**
 * An hour's tooltip: when it was, each series' count, a part's with its
 * share of the whole, then each source's own whole where several add up.
 */
export function hourTip(series, colours, i, from, { long = false, last = false, sources = [] } = {}) {
  const whole = series[0].hours[i];
  const rows = series.map((s, k) => {
    const v = s.hours[i];
    const share = k ? ` · ${t("percent", { n: whole ? Math.round((100 * v) / whole) : 0 })}` : "";
    return [colours[k], say(s.label), `${number(v)}${share}`];
  });
  for (const s of sources) rows.push(["", esc(s.name), number(s.hours[i])]);
  return tip(hourTitle(from, long), rows, last ? t("chart.so_far") : "");
}

// An hour's tooltip title: "14:00 – 15:00", after its day over a long chart.
const hourTitle = (from, long) => `${long ? `${day(from)}, ` : ""}${clock(from)} – ${clock(+from + 3600e3)}`;

/** A chart's series by colour, in a row. */
export const legend = (series, colours) =>
  `<span class="legend">${series.map((s, k) => `<span><i style="--c:${colours[k]}"></i>${say(s.label)}</span>`).join("")}</span>`;

// Where a chart's scale lines go: the bottom, its quarters or halves, the top.
const gridOf = (max, steps) => Array.from({ length: steps + 1 }, (_, k) => (max * k) / steps);

/**
 * The last day by the hour as bars, oldest first, ending with this hour: the
 * whole of each, and each part within it from its foot. Each bar brings its
 * tooltip. Every sixth hour under them, and the scale beside.
 */
export function bars(series, now = new Date()) {
  const n = series[0].hours.length;
  if (n < 2) return "";
  const colours = seriesColours(series.length);
  const max = niceMax(Math.max(...series[0].hours));
  const pct = (v) => `${((100 * v) / max).toFixed(2)}%`;
  const cols = series[0].hours.map((v, i) => {
    const tp = hourTip(series, colours, i, hourStart(i, n, now), { last: i === n - 1 });
    const parts = series
      .slice(1)
      .map((s, k) => `<i style="height:${pct(s.hours[i])};--c:${colours[k + 1]}"></i>`)
      .join("");
    return `<span class="hour" style="--i:${i}" data-tip="${tp}"><i class="whole" style="height:${pct(v)};--c:${HOURS}"></i>${parts}</span>`;
  });
  const { marks } = hourAxis(n, now);
  const grid = gridOf(max, 2).map(
    (v) =>
      `<i class="line" style="bottom:${pct(v)}"></i><span class="scale" style="bottom:${pct(v)}">${scaleNumber(v)}</span>`,
  );
  return `<div class="hour-chart"><div class="plot">${grid.join("")}<div class="cols">${cols.join("")}</div></div><div class="axis">${marks.join("")}</div></div>`;
}

let charts = 0;
/**
 * Many hours as areas, oldest first, ending with this hour: the whole, and
 * each part within it from the foot, each with its line. Each hour is a
 * column that lights up under the pointer and brings its tooltip, with each
 * source's own whole when several add up. Over more than two days, the days
 * alternate in shade and are named under their middle; over fewer, every
 * sixth hour is. The scale is beside.
 */
export function areas(series, sources = [], now = new Date()) {
  const n = series[0].hours.length;
  if (n < 2) return "";
  const id = `ch${++charts}`;
  const colours = seriesColours(series.length);
  const { long, at, bands, marks } = hourAxis(n, now);
  const max = niceMax(Math.max(...series[0].hours));
  const y = (v) => (100 - (100 * v) / max).toFixed(2);
  const line = (h) => h.map((v, i) => `${i ? "L" : "M"}${i + 0.5},${y(v)}`).join("");
  const defs = colours
    .map(
      (c, k) =>
        `<linearGradient id="${id}-${k}" x1="0" y1="0" x2="0" y2="1"><stop offset="0" style="stop-color:${c};stop-opacity:.38"/><stop offset="1" style="stop-color:${c};stop-opacity:.03"/></linearGradient>`,
    )
    .join("");
  const fills = series
    .map((s, k) => `<path d="${line(s.hours)}L${n - 0.5},100L0.5,100Z" fill="url(#${id}-${k})"/>`)
    .join("");
  const lines = series
    .map((s, k) => `<path class="stroke" d="${line(s.hours)}" style="stroke:${colours[k]}"/>`)
    .join("");
  const cols = series[0].hours
    .map((_, i) => {
      const tp = hourTip(series, colours, i, at(i), {
        long,
        last: i === n - 1,
        sources: sources.length > 1 ? sources : [],
      });
      return `<g class="col"><rect class="hit" x="${i}" y="0" width="1" height="100" data-tip="${tp}"/><line class="cur" x1="${i + 0.5}" x2="${i + 0.5}" y1="0" y2="100"/></g>`;
    })
    .join("");
  const grid = gridOf(max, 4).map(
    (v) =>
      `<i class="line" style="bottom:${((100 * v) / max).toFixed(2)}%"></i><span class="scale" style="bottom:${((100 * v) / max).toFixed(2)}%">${scaleNumber(v)}</span>`,
  );
  return `<div class="hour-chart areas${long ? " long" : ""}"><div class="plot">${grid.join("")}
    <svg viewBox="0 0 ${n} 100" preserveAspectRatio="none" aria-hidden="true"><defs>${defs}</defs>${bands}<g class="series">${fills}${lines}</g>${cols}</svg>
  </div><div class="axis">${marks.join("")}</div></div>`;
}

/**
 * The axis under n hours ending with this one: over more than two days, the
 * days alternate in shade and are named under their middle; over fewer,
 * every sixth hour is.
 */
function hourAxis(n, now) {
  const long = n > 48;
  const at = (i) => hourStart(i, n, now);
  const left = (x) => `${((100 * x) / n).toFixed(2)}%`;
  let bands = "";
  const marks = [];
  if (long) {
    const starts = [0, ...Array.from({ length: n }, (_, i) => i).filter((i) => i && at(i).getHours() === 0)];
    starts.forEach((from, k) => {
      const to = starts[k + 1] ?? n;
      // Every other calendar day is shaded, so a day keeps its shade as the hours go by.
      if (Math.round(new Date(at(from)).setHours(0, 0, 0, 0) / 864e5) % 2) {
        bands += `<rect class="band" x="${from}" y="0" width="${to - from}" height="100"/>`;
      }
      if ((to - from) / n >= 0.08)
        marks.push(`<span style="left:${left((from + to) / 2)}">${weekday(at(from))}</span>`);
    });
  } else {
    for (let i = 0; i < n; i++) {
      if (at(i).getHours() % 6 === 0) marks.push(`<span style="left:${left(i + 0.5)}">${clock(at(i))}</span>`);
    }
  }
  return { long, at, bands, marks };
}

/** A line among many that isn't one of the highest: thin and grey. */
const QUIET = "var(--m-label-3)";

/**
 * Each level's colour: its slot's, or past eight, the seven highest over the
 * hours theirs and the rest grey, each keeping its place among them.
 */
export function lineColours(series) {
  const kept = new Set(busiest(series, SERIES.length));
  let slot = 0;
  return series.map((_, k) => (kept.has(k) ? SERIES[slot++] : QUIET));
}

/**
 * Levels by the hour as lines from 0 to 100%, oldest first, ending with this
 * hour, such as each host's processor. Each hour is a column that lights up
 * under the pointer, its tooltip every line at that hour, highest first.
 */
export function lines(series, now = new Date()) {
  const n = series[0].hours.length;
  if (n < 2) return "";
  const colours = lineColours(series);
  const { long, at, bands, marks } = hourAxis(n, now);
  const path = (h) => h.map((v, i) => `${i ? "L" : "M"}${i + 0.5},${(100 - v).toFixed(2)}`).join("");
  // The grey ones first, so the coloured ones are drawn over them.
  const order = series
    .map((_, k) => k)
    .sort((a, b) => (colours[a] === QUIET) - (colours[b] === QUIET))
    .reverse();
  const strokes = order
    .map(
      (k) =>
        `<path class="stroke${colours[k] === QUIET ? " quiet" : ""}" d="${path(series[k].hours)}" style="stroke:${colours[k]}"/>`,
    )
    .join("");
  const cols = Array.from({ length: n }, (_, i) => {
    const rows = series
      .map((s, k) => ({ v: s.hours[i], row: [colours[k], say(s.label), t("percent", { n: Math.round(s.hours[i]) })] }))
      .sort((a, b) => b.v - a.v)
      .map((r) => r.row);
    return `<g class="col"><rect class="hit" x="${i}" y="0" width="1" height="100" data-tip="${tip(hourTitle(at(i), long), rows, i === n - 1 ? t("chart.so_far") : "")}"/><line class="cur" x1="${i + 0.5}" x2="${i + 0.5}" y1="0" y2="100"/></g>`;
  }).join("");
  const grid = [0, 50, 100].map(
    (v) =>
      `<i class="line" style="bottom:${v}%"></i><span class="scale" style="bottom:${v}%">${t("percent", { n: v })}</span>`,
  );
  return `<div class="hour-chart areas lines${long ? " long" : ""}"><div class="plot">${grid.join("")}
    <svg viewBox="0 0 ${n} 100" preserveAspectRatio="none" aria-hidden="true">${bands}<g class="series">${strokes}</g>${cols}</svg>
  </div><div class="axis">${marks.join("")}</div></div>`;
}

/**
 * A figure's shares as one bar, each as wide as its share of the whole, in
 * its colour, past eight the smallest folded into one. Its tooltip is its
 * label over its rows, or over its value and share.
 */
export function shares(f, start = 0) {
  const all = fold(f.shares);
  const whole = all.reduce((sum, s) => sum + s.value, 0);
  const parts = all.map((s, i) => {
    const c = shareColour(s, i, start);
    const rows = s.rows?.length
      ? s.rows.map((r, k) => [k ? "" : c, say(r.title), r.value ? say(r.value) : ""])
      : [[c, number(s.value), t("percent", { n: Math.round((100 * s.value) / whole) })]];
    return `<i class="${s.tone ? `tone-${s.tone}` : ""}" style="flex:${s.value};--c:${c}" data-tip="${tip(say(s.label) + by(s), rows)}"></i>`;
  });
  return `<span class="shares${all.length > 12 ? " many" : ""}">${parts.join("")}</span>`;
}
