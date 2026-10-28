// What every page shares: the words (ADR 0006), the snapshot the server reads
// (ADR 0002), the helpers that draw its shapes, and the shell: the sidebar,
// the panel's bar and ⌘K.

const AREAS = ["servers", "network", "storage", "apps"];
const esc = s => String(s).replace(/[&<>"]/g, c => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
// Numbers and dates in the viewer's own way of writing them.
const fmt = new Intl.NumberFormat();

// ── Words: English, under the viewer's language where a translation has it ─

const LANG = navigator.language || "en";
let WORDS = { en: {}, mine: {} };
async function loadWords() {
  const get = lang => fetch(`i18n/${lang}.json`).then(r => (r.ok ? r.json() : {}), () => ({}));
  const base = LANG.split("-")[0];
  const [en, mine] = await Promise.all([get("en"), base === "en" ? {} : get(base)]);
  WORDS = { en, mine };
}

// A message as HTML: its values escaped, numbers and {name:format}s written
// the viewer's way, and the plural that n calls for in the message's language.
function t(key, args = {}) {
  const mine = key in WORDS.mine, entry = mine ? WORDS.mine[key] : WORDS.en[key];
  if (entry === undefined) return esc(key);
  const form = typeof entry === "string" ? entry : entry[new Intl.PluralRules(mine ? LANG : "en").select(args.n)] ?? entry.other;
  return form.replace(/\{(\w+)(?::(\w+))?\}/g, (_, name, as) => {
    const v = args[name];
    return as ? FORMAT[as](v) : typeof v === "number" ? fmt.format(v) : esc(v ?? "");
  });
}
// A snapshot's text: a message, or a name as written.
const say = x => (x.key ? t(x.key, x.args) : esc(x.name));
// The words of the page's own HTML, from each element's data-t.
const translate = root => root.querySelectorAll("[data-t]").forEach(el => { el.innerHTML = t(el.dataset.t); });

// ── Icons: 16 px strokes for the pages and figures; apps and tools show
// their own logos ─────────────────────────────────────────────────────────

const ICON = {
  pulse: '<path d="M1.75 8.5h2.5l1.75-4.5 3.5 8.5 1.75-4h3"/>',
  server: '<rect x="2.5" y="2.5" width="11" height="4.5" rx="1.25"/><rect x="2.5" y="9" width="11" height="4.5" rx="1.25"/><path d="M5 4.75h.01M5 11.25h.01"/>',
  network: '<circle cx="8" cy="8" r="5.75"/><path d="M2.25 8h11.5M8 2.25c1.8 1.9 1.8 9.6 0 11.5M8 2.25c-1.8 1.9-1.8 9.6 0 11.5"/>',
  storage: '<ellipse cx="8" cy="4" rx="5.25" ry="2"/><path d="M2.75 4v8c0 1.1 2.35 2 5.25 2s5.25-.9 5.25-2V4M2.75 8c0 1.1 2.35 2 5.25 2s5.25-.9 5.25-2"/>',
  apps: '<rect x="2.5" y="2.5" width="4.5" height="4.5" rx="1.2"/><rect x="9" y="2.5" width="4.5" height="4.5" rx="1.2"/><rect x="2.5" y="9" width="4.5" height="4.5" rx="1.2"/><rect x="9" y="9" width="4.5" height="4.5" rx="1.2"/>',
  shield: '<path d="M8 1.75 3 3.5v3.75c0 3 2 5.4 5 6.5 3-1.1 5-3.5 5-6.5V3.5Z"/>',
  photos: '<rect x="2" y="3" width="12" height="10" rx="2"/><circle cx="5.75" cy="6.5" r="1.1"/><path d="m14 10.5-3.25-3.25L4 13"/>',
  film: '<rect x="2.5" y="2.25" width="11" height="11.5" rx="1.75"/><path d="M5.25 2.25v11.5M10.75 2.25v11.5M2.5 6h2.75M2.5 10h2.75M10.75 6h2.75M10.75 10h2.75"/>',
  search: '<circle cx="7" cy="7" r="4.25"/><path d="m10.25 10.25 3.5 3.5"/>',
  archive: '<rect x="2" y="2.75" width="12" height="3.5" rx="1"/><path d="M3 6.25v6a1.5 1.5 0 0 0 1.5 1.5h7a1.5 1.5 0 0 0 1.5-1.5v-6M6.5 9h3"/>',
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
const svg = (name, cls = "ic") => `<svg class="${cls}" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${ICON[name] || ICON.pulse}</svg>`;

// A check's or an area's state: a filled circle with a tick, a half-filled
// ring, a filled circle with a bang, or a dashed ring.
const STATE_ICON = {
  up: '<circle cx="7" cy="7" r="6" fill="currentColor"/><path d="m4.4 7.2 1.8 1.8 3.5-3.7" style="stroke:var(--m-panel)" stroke-width="1.6" fill="none" stroke-linecap="round" stroke-linejoin="round"/>',
  degraded: '<circle cx="7" cy="7" r="5.25" stroke="currentColor" stroke-width="1.5" fill="none"/><path d="M7 3.5a3.5 3.5 0 0 1 0 7Z" fill="currentColor"/>',
  down: '<circle cx="7" cy="7" r="6" fill="currentColor"/><path d="M7 3.9v3.6M7 10h.01" style="stroke:var(--m-panel)" stroke-width="1.7" stroke-linecap="round"/>',
  unknown: '<circle cx="7" cy="7" r="5.25" stroke="currentColor" stroke-width="1.5" fill="none" stroke-dasharray="2 2.1"/>',
};
const stateIcon = state => `<svg class="si tone-${TONE[state]}" viewBox="0 0 14 14" aria-hidden="true">${STATE_ICON[state]}</svg>`;
const TONE = { up: "green", degraded: "orange", down: "red", unknown: "gray" };
const word = state => t(`state.${state}`);

// An app's or a tool's logo on its tile, `size` xs, sm, md or lg: a file
// the server has in icons/, or a URL.
const iconSrc = icon => (/^(https?:)?\/\//.test(icon) ? icon : `icons/${icon}`);
const badge = (s, size = "md") => `<span class="app-icon ${size}" style="--tint:${esc(s.color || "#7170FF")}">${s.icon
  ? `<img src="${esc(iconSrc(s.icon))}" alt="" decoding="async">` : svg("apps")}</span>`;

// The pages, in the sidebar's order; `key` goes after G to jump there.
const PAGES = [
  { href: "./", icon: "pulse", key: "o" },
  { href: "servers", area: "servers", icon: "server", key: "s" },
  { href: "network", area: "network", icon: "network", key: "n" },
  { href: "storage", area: "storage", icon: "storage", key: "t" },
  { href: "apps", area: "apps", icon: "apps", key: "a" },
];
const pageName = p => t(p.area ? `area.${p.area}` : "page.overview");
const HERE = PAGES.find(p => p.area && p.area === location.pathname.split("/").pop()) || PAGES[0];

// ── The snapshot ──────────────────────────────────────────────────────────

async function getJSON(url) {
  const response = await fetch(url, { cache: "no-store" });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  return response.json();
}

// The checks, with their times as dates, and what the page asks of them.
let lastChecks = [];
function readChecks(snap) {
  lastChecks = snap.checks.map(c => {
    const recent = c.results.map(r => ({ ...r, at: new Date(r.at) }));
    return {
      ...c, recent, failing: c.run > 0, ms: recent.length ? recent[recent.length - 1].ms : null,
      since: c.since ? new Date(c.since) : null, watched: c.watched ? new Date(c.watched) : null,
      outages: (c.outages || []).map(o => ({ check: c, from: new Date(o.from), to: o.to ? new Date(o.to) : null })),
    };
  });
  return lastChecks;
}

// An area is shown when anything is in it.
const shown = snap => AREAS.filter(a => [snap.checks, snap.sections, snap.figures, snap.events].some(list => list.some(x => x.area === a)));
// The sources that didn't answer, by name.
const silent = snap => snap.sources.filter(s => !s.answered).map(s => s.name);
const list = items => new Intl.ListFormat(LANG).format(items);

// When the last problem ended, or, with none remembered, since when the
// check engine has watched.
function quietSince(checks) {
  let end = null, start = null;
  for (const c of checks) {
    for (const o of c.outages) if (o.to && (!end || o.to > end)) end = o.to;
    if (c.watched && (!start || c.watched < start)) start = c.watched;
  }
  return { since: end || start, problemFree: !end };
}

// An area is down when every check in it is a problem; one problem
// otherwise makes it degraded.
function areaState(area, checks) {
  const mine = checks.filter(c => c.area === area);
  const problems = mine.filter(c => c.problem);
  const state = !problems.length ? "up" : problems.length === mine.length ? "down" : "degraded";
  return { area, checks: mine, problems, state };
}
const checkState = c => (c.problem ? "down" : c.failing ? "degraded" : "up");

// ── Writing times, sizes and durations ────────────────────────────────────

const clock = d => new Date(d).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
const day = d => new Date(d).toLocaleDateString(undefined, { weekday: "short", day: "numeric", month: "short" });
// "37 min", "5 h", "3 days".
function span(ms) {
  const m = Math.max(0, Math.round(ms / 60e3));
  if (m < 60) return t("span.minutes", { n: m });
  const h = Math.round(m / 60);
  if (h < 48) return t("span.hours", { n: h });
  return t("span.days", { n: Math.round(h / 24) });
}
const ago = d => (Date.now() - d < 60e3 ? t("ago.now") : t("ago", { ms: Date.now() - d }));
// Since a moment, or the day it was when longer ago than yesterday.
function when(d) {
  d = new Date(d);
  const today = new Date(); today.setHours(0, 0, 0, 0);
  if (d >= today) return t("when.today", { at: d });
  if (d >= today - 864e5) return t("when.yesterday", { at: d });
  return day(d);
}
// Bytes in the units disks are sold in: "785 MB", "8.4 TB".
function bytes(n) {
  const units = ["B", "kB", "MB", "GB", "TB", "PB"];
  let i = 0;
  while (n >= 1000 && i < units.length - 1) { n /= 1000; i++; }
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: n >= 100 || i === 0 ? 0 : 1 }).format(n) + ` ${units[i]}`;
}
const msText = ms => (ms === null || ms === undefined ? "" : ms < 1 ? t("ms.under") : t("ms", { n: Math.round(ms) }));
// The formats a message's {name:format} can ask for.
const FORMAT = { span, when, day, clock, bytes, ms: msText, area: a => t(`area.${a}`) };

// A thin bar under a row's title: how full, in the tone it calls for.
function meter(used, total, tone = used / total > 0.9 ? "red" : used / total > 0.8 ? "orange" : "blue") {
  return `<span class="meter tone-${tone}"><span style="width:${Math.min(100, 100 * used / total).toFixed(1)}%"></span></span>`;
}

// The last RECENT results as ticks, oldest first: green, red, or empty
// where there's no result yet. A result may bring its own tooltip.
const RECENT = 30;
const GREEN = "#4CB782", RED = "#EB5757";
function strip(recent) {
  const pad = Array(Math.max(0, RECENT - recent.length)).fill(null);
  return `<span class="strip">${[...pad, ...recent.slice(-RECENT)].map(r => r
    ? `<i class="${r.ok ? "ok" : "bad"}" data-tip="${r.tip || tip(clock(r.at), [[r.ok ? GREEN : RED, t(r.ok ? "result.passed" : "result.failed"), msText(r.ms)]], esc(r.why || ""))}"></i>`
    : "<i></i>").join("")}</span>`;
}

// A tooltip, for a data-tip attribute, from HTML: a title, rows of a colour,
// a label and a value, then a line. Anything with data-tip shows it under the
// pointer, or under a finger while it presses.
function tip(title, rows = [], note = "") {
  return esc(`<b>${title}</b>${rows.map(([c, label, value]) =>
    `<span class="tr"><i style="--c:${c || "transparent"}"></i><span>${label}</span><span>${value ?? ""}</span></span>`).join("")}${note ? `<p>${note}</p>` : ""}`);
}
function tooltips() {
  const pop = document.createElement("div");
  pop.className = "tip-pop";
  pop.setAttribute("role", "tooltip");
  pop.hidden = true;
  document.body.append(pop);
  let on = null;
  const show = e => {
    const el = e.target.closest && e.target.closest("[data-tip]");
    if (el !== on) { on = el; pop.hidden = !el; if (el) pop.innerHTML = el.dataset.tip; }
    if (!el) return;
    const w = pop.offsetWidth, h = pop.offsetHeight;
    const x = Math.min(Math.max(8, e.clientX - w / 2), innerWidth - w - 8);
    pop.style.transform = `translate(${x}px, ${e.clientY - h - 14 < 8 ? e.clientY + 20 : e.clientY - h - 14}px)`;
  };
  document.addEventListener("pointermove", show);
  document.addEventListener("pointerdown", show);
  document.addEventListener("pointerout", e => { if (!e.relatedTarget) { on = null; pop.hidden = true; } });
  addEventListener("scroll", () => { on = null; pop.hidden = true; }, { passive: true });
}

// The rows of an area's checks.
function checkRows(checks) {
  return checks.map(c => {
    const detail = c.problem ? esc(c.why || "") : c.failing ? t("check.failed_of", { run: c.run, threshold: c.threshold, why: c.why || "" }) : "";
    return `<a class="check" href="${esc(c.url)}" target="_blank" rel="noopener">
      ${stateIcon(checkState(c))}
      <span class="copy"><span class="title">${esc(c.name)}</span>${detail ? `<span class="detail">${detail}</span>` : ""}</span>
      ${strip(c.recent)}<span class="value">${c.failing ? "" : msText(c.ms)}</span></a>`;
  }).join("");
}

// A row of the grouped list: a title (and a line under it), and its value.
// With a meter, the meter runs the row's whole width under both, and the
// line goes under the meter, so every row's bar has the same edges.
function fact(title, value, detail = "", bar = "") {
  if (bar) return `<div class="row"><span class="copy"><span class="title">${title}</span></span><span class="value">${value}</span>${bar}${detail ? `<span class="detail under">${detail}</span>` : ""}</div>`;
  return `<div class="row"><span class="copy"><span class="title">${title}</span>${detail ? `<span class="detail">${detail}</span>` : ""}</span><span class="value">${value}</span></div>`;
}
// A section of the snapshot: its title, then its rows.
const section = s => `<section><h2 class="section-title">${say(s.title)}</h2><div class="list card">${s.rows.map(r => fact(say(r.title),
  r.value ? (r.tone ? `<span class="state tone-${r.tone}">${say(r.value)}</span>` : say(r.value)) : "",
  r.detail ? say(r.detail) : "", r.meter ? meter(r.meter.used, r.meter.total) : "")).join("")}</div></section>`;

// ── The shell: the sidebar, and the panel the page's <main> sits in ───────

function shell() {
  const group = (id, name) => `<details class="sec" id="${id}" open hidden><summary>${t(name)}${svg("caret", "ic caret")}</summary><div></div></details>`;
  document.documentElement.classList.add("boot");
  document.body.insertAdjacentHTML("afterbegin", `
    <aside class="side" id="side">
      <div class="ws">
        <a class="ws-name" href="./"><span class="logo">${svg("mark")}</span>${t("title")}</a>
        <button class="icon-btn" type="button" data-palette aria-label="${t("search.label")}">${svg("search")}</button>
      </div>
      <nav>
        <a class="nav${HERE === PAGES[0] ? " here" : ""}" href="./">${svg("pulse")}<span>${pageName(PAGES[0])}</span><span class="count" id="problem-count" hidden></span></a>
        <details class="sec" open><summary>${t("side.areas")}${svg("caret", "ic caret")}</summary>
          ${PAGES.slice(1).map(p => `<a class="nav${p === HERE ? " here" : ""}" href="${p.href}" data-area="${p.area}" hidden>${svg(p.icon)}<span>${pageName(p)}</span><i class="dot"></i></a>`).join("")}</details>
        ${group("side-apps", "side.apps")}
        ${group("side-tools", "side.tools")}
      </nav>
      <div class="side-foot">
        <svg class="ring" viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="6"/><circle cx="8" cy="8" r="6"/></svg>
        <span id="refreshed">${t("loading")}</span><kbd>⌘K</kbd>
      </div>
    </aside>
    <div class="scrim" data-menu></div>`);
  const panel = document.createElement("div");
  panel.className = "panel";
  panel.innerHTML = `<header class="bar">
      <button class="icon-btn menu" type="button" data-menu aria-label="${t("menu")}">${svg("menu")}</button>
      <span class="crumb">${svg(HERE.icon)}<span>${pageName(HERE)}</span></span>
      <span class="bar-end">
        <button class="icon-btn spin" type="button" data-refresh aria-label="${t("refresh")}">${svg("refresh")}</button>
        <button class="kbd-btn" type="button" data-palette>${svg("search")}<span>${t("search")}</span><kbd>⌘K</kbd></button>
      </span></header>`;
  const main = document.querySelector("main");
  main.before(panel);
  panel.append(main);
  document.addEventListener("click", e => {
    if (e.target.closest("[data-menu]")) document.body.classList.toggle("nav-open");
  });
  palette();
  tooltips();
}

// The sidebar from the snapshot just read: the areas with something in
// them, their dots, the count of problems, and the apps and tools.
let SNAP = null;
function paintShell(snap, checks) {
  const areas = shown(snap);
  document.querySelectorAll(".side [data-area]").forEach(a => {
    a.hidden = !areas.includes(a.dataset.area);
    a.querySelector(".dot").className = `dot tone-${TONE[areaState(a.dataset.area, checks).state]}`;
  });
  const byName = Object.fromEntries(checks.map(c => [c.name, c]));
  const link = s => `<a class="nav" href="${esc(s.url)}" target="_blank" rel="noopener">
    ${badge(s, "xs")}<span>${esc(s.name)}</span><i class="dot tone-${s.check ? (byName[s.check] ? TONE[checkState(byName[s.check])] : "gray") : ""}"></i></a>`;
  for (const [id, links] of [["side-apps", snap.apps], ["side-tools", snap.tools]]) {
    const sec = document.getElementById(id);
    sec.hidden = !links.length;
    sec.querySelector("div").innerHTML = links.map(link).join("");
  }
  const count = document.getElementById("problem-count");
  const n = checks.filter(c => c.problem).length;
  count.hidden = !n;
  count.textContent = fmt.format(n);
}

// ⌘K (or /): go to a page, open an app or a tool, or a check in its check
// engine, by typing a few letters of it. G then a page's key jumps there.
function palette() {
  const box = document.createElement("dialog");
  box.className = "palette";
  box.innerHTML = `<div class="pal-in">${svg("search")}<input id="palette-q" type="text" placeholder="${t("palette.placeholder")}" autocomplete="off" spellcheck="false"></div>
    <div class="pal-list" role="listbox"></div>
    <footer><span><kbd>↑</kbd><kbd>↓</kbd> ${t("palette.move")}</span><span><kbd>↵</kbd> ${t("palette.open")}</span><span><kbd>esc</kbd> ${t("palette.close")}</span></footer>`;
  document.body.append(box);
  const input = box.querySelector("input"), list = box.querySelector(".pal-list");
  const fuzzy = (q, s) => { let i = 0; for (const ch of s) if (ch === q[i]) i++; return i === q.length; };
  const pages = () => PAGES.filter(p => !p.area || (SNAP && shown(SNAP).includes(p.area)));
  let items = [], at = 0;
  const all = () => [
    ...pages().map(p => ({ group: t("palette.go"), name: pageName(p), glyph: svg(p.icon), href: p.href, hint: `<kbd>G</kbd><kbd>${p.key.toUpperCase()}</kbd>` })),
    ...(SNAP ? SNAP.apps : []).map(s => ({ group: t("side.apps"), name: esc(s.name), what: esc(s.description || ""), glyph: badge(s, "sm"), href: s.url, out: true })),
    ...(SNAP ? SNAP.tools : []).map(s => ({ group: t("side.tools"), name: esc(s.name), what: esc(s.description || ""), glyph: badge(s, "sm"), href: s.url, out: true })),
    ...lastChecks.map(c => ({ group: t("palette.checks"), name: esc(c.name), what: c.area ? t(`area.${c.area}`) : "", glyph: stateIcon(checkState(c)), href: c.url, out: true })),
  ];
  const draw = () => {
    const q = input.value.trim().toLowerCase();
    items = all().filter(i => fuzzy(q, `${i.name} ${i.what || ""}`.toLowerCase()));
    at = Math.max(0, Math.min(at, items.length - 1));
    let group = "";
    list.innerHTML = items.map((i, n) => (i.group !== group ? `<p class="pal-group">${(group = i.group)}</p>` : "") +
      `<a class="pal-item${n === at ? " on" : ""}" href="${esc(i.href)}" data-n="${n}" role="option" ${i.out ? 'target="_blank" rel="noopener"' : ""}>
        ${i.glyph}<span class="title">${i.name}</span>${i.what ? `<span class="detail">${i.what}</span>` : ""}
        <span class="pal-hint">${i.hint || (i.out ? svg("out") : "")}</span></a>`).join("") || `<p class="pal-empty">${t("palette.nothing")}</p>`;
    list.querySelector(".on")?.scrollIntoView({ block: "nearest" });
  };
  const open = () => { input.value = ""; at = 0; draw(); box.showModal(); input.focus(); };
  const go = i => { box.close(); if (i.out) window.open(i.href, "_blank", "noopener"); else location.href = i.href; };
  input.addEventListener("input", () => { at = 0; draw(); });
  input.addEventListener("keydown", e => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") { e.preventDefault(); at = (at + (e.key === "ArrowDown" ? 1 : -1) + items.length) % items.length; draw(); }
    if (e.key === "Enter" && items[at]) { e.preventDefault(); go(items[at]); }
  });
  list.addEventListener("mousemove", e => { const a = e.target.closest(".pal-item"); if (a && +a.dataset.n !== at) { at = +a.dataset.n; draw(); } });
  list.addEventListener("click", e => { const a = e.target.closest(".pal-item"); if (a) { e.preventDefault(); go(items[+a.dataset.n]); } });
  box.addEventListener("click", e => { if (e.target === box) box.close(); });
  document.addEventListener("click", e => { if (e.target.closest("[data-palette]")) open(); });
  let g = 0;
  document.addEventListener("keydown", e => {
    const typing = e.target.closest("input, textarea, [contenteditable]");
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") { e.preventDefault(); box.open ? box.close() : open(); return; }
    if (typing || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === "/") { e.preventDefault(); open(); return; }
    const p = Date.now() - g < 800 && pages().find(p => p.key === e.key.toLowerCase());
    if (p) location.href = p.href;
    g = e.key.toLowerCase() === "g" ? Date.now() : 0;
  });
}

// Draws the page from a new snapshot every minute while it shows, on coming
// back to it, and on ↻. The ring in the sidebar runs down to the next read.
async function start(draw) {
  await loadWords();
  translate(document);
  shell();
  const run = async () => {
    document.body.classList.add("loading");
    try {
      const snap = await getJSON("snapshot").catch(() => null);
      document.getElementById("refreshed").innerHTML = snap ? t("updated", { at: new Date() }) : t("overlook.no_answer");
      if (!snap) return;
      SNAP = snap;
      const checks = readChecks(snap);
      paintShell(snap, checks);
      draw(snap, checks);
    } finally {
      document.body.classList.remove("loading");
      const ring = document.querySelector(".ring circle:last-child");
      ring.style.animation = "none"; void ring.getBoundingClientRect(); ring.style.animation = "";
      setTimeout(() => document.documentElement.classList.remove("boot"), 1200);
    }
  };
  run();
  setInterval(() => { if (!document.hidden) run(); }, 60e3);
  document.addEventListener("visibilitychange", () => { if (!document.hidden) run(); });
  document.addEventListener("click", e => { if (e.target.closest("[data-refresh]")) run(); });
}
