// The pieces every page draws, as HTML strings: icons, state glyphs, logos,
// tooltips, the strip of results, rows, sections and sparklines. Every value
// from the snapshot is escaped here or by t().

import { checkState } from "./checks.js";
import { clock, esc, ms, say, t } from "./text.js";

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

/** How many results a strip shows. */
export const RECENT = 30;

/** An icon's address: a URL as given, or a file the server has in icons/. */
export const iconSrc = (icon) => (/^(https?:)?\/\//.test(icon) ? icon : `icons/${icon}`);

/** An app's or a tool's logo on its tile; size is xs, sm, md or lg. */
export const badge = (s, size = "md") =>
  `<span class="app-icon ${size}" style="--tint:${esc(s.color || "#7170FF")}">${
    s.icon ? `<img src="${esc(iconSrc(s.icon))}" alt="" decoding="async">` : svg("apps")
  }</span>`;

/**
 * A thin bar for how full something is, in its tone, or with none blue,
 * turning orange past 80% and red past 90%, as a disk's does.
 */
export function meter(used, total, tone) {
  const tn = tone || (used / total > 0.9 ? "red" : used / total > 0.8 ? "orange" : "blue");
  return `<span class="meter tone-${tn}"><span style="width:${Math.min(100, (100 * used) / total).toFixed(1)}%"></span></span>`;
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
    return `<i class="${r.ok ? "ok" : "bad"}" data-tip="${tp}"></i>`;
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
          ? t("check.failed_of", { run: c.run, threshold: c.threshold, why: c.why || "" })
          : "";
      return `<a class="check" href="${esc(c.url)}" target="_blank" rel="noopener">
      ${stateIcon(checkState(c))}
      <span class="copy"><span class="title">${esc(c.name)}</span>${detail ? `<span class="detail">${detail}</span>` : ""}</span>
      ${strip(c.recent)}<span class="value">${c.failing ? "" : ms(c.ms)}</span></a>`;
    })
    .join("");
}

/**
 * A row of a list: a title (and a line under it), and its value. With a
 * meter, the meter runs the row's whole width under both and the line goes
 * under the meter, so every row's bar has the same edges.
 */
export function fact(title, value, detail = "", bar = "") {
  if (bar) {
    return `<div class="row"><span class="copy"><span class="title">${title}</span></span><span class="value">${value}</span>${bar}${detail ? `<span class="detail under">${detail}</span>` : ""}</div>`;
  }
  return `<div class="row"><span class="copy"><span class="title">${title}</span>${detail ? `<span class="detail">${detail}</span>` : ""}</span><span class="value">${value}</span></div>`;
}

/** A figure's meter: its label, its bar, and how full it is. */
export const figureMeter = (m) =>
  `<span class="figure-meter"><span>${m.label ? say(m.label) : ""}</span>${meter(m.used, m.total, m.tone)}<span>${t("percent", { n: Math.round((100 * m.used) / m.total) })}</span></span>`;

/**
 * A section of the snapshot: its title, then its rows. It carries its place
 * among the page's sections, i, so a page in one column can keep their order.
 */
export function section(s, i) {
  const rows = s.rows.map((r) => {
    const value = r.value ? (r.tone ? `<span class="state tone-${r.tone}">${say(r.value)}</span>` : say(r.value)) : "";
    return fact(
      say(r.title),
      value,
      r.detail ? say(r.detail) : "",
      r.meter ? meter(r.meter.used, r.meter.total, r.meter.tone) : "",
    );
  });
  return `<section style="--i:${i}"><h2 class="section-title">${say(s.title)}</h2><div class="list card">${rows.join("")}</div></section>`;
}

let uid = 0;
/**
 * A line over its area, fading down: values left to right, from zero. Each
 * point's column brings its tooltip, from tips.
 */
export function sparkline(values, color, tips = []) {
  const v = values.map((x) => (x === null || !Number.isFinite(x) ? 0 : x));
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
