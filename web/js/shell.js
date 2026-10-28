// The shell every page sits in: the sidebar, the panel's bar, ⌘K and the
// tooltips, and start(), which reads the snapshot and draws the page from it.

import { areaState, asIfNow, checkState, readChecks, shown } from "./checks.js";
import { badge, stateIcon, svg, TONE } from "./draw.js";
import { esc, loadWords, number, t, translate } from "./text.js";

/** The pages, in the sidebar's order; key goes after G to jump there. */
export const PAGES = [
  { href: "./", icon: "pulse", key: "o" },
  { href: "servers", area: "servers", icon: "server", key: "s" },
  { href: "network", area: "network", icon: "network", key: "n" },
  { href: "storage", area: "storage", icon: "storage", key: "t" },
  { href: "apps", area: "apps", icon: "apps", key: "a" },
];

/** A page's name. */
export const pageName = (p) => t(p.area ? `area.${p.area}` : "page.overview");

/**
 * The demo, which has no server, marks its pages (demo/main.go): it reads an
 * invented snapshot.json, as if just read, in place of the server's snapshot.
 */
const DEMO = document.documentElement.hasAttribute("data-demo");

/** The page showing. */
export const HERE = PAGES.find((p) => p.area && p.area === location.pathname.split("/").pop()) || PAGES[0];

// The last snapshot and its checks, for ⌘K.
let last = { snap: null, checks: [] };

function shell() {
  const group = (id, name) =>
    `<details class="sec" id="${id}" open hidden><summary>${t(name)}${svg("caret", "ic caret")}</summary><div></div></details>`;
  const areas = PAGES.filter((p) => p.area).map(
    (p) =>
      `<a class="nav${p === HERE ? " here" : ""}" href="${p.href}" data-area="${p.area}" hidden>${svg(p.icon)}<span>${pageName(p)}</span><i class="dot"></i></a>`,
  );
  document.documentElement.classList.add("boot");
  document.body.insertAdjacentHTML(
    "afterbegin",
    `<aside class="side" id="side">
      <div class="ws">
        <a class="ws-name" href="./"><span class="logo">${svg("mark")}</span>${t("title")}</a>
        <button class="icon-btn" type="button" data-palette aria-label="${t("search.label")}">${svg("search")}</button>
      </div>
      <nav>
        <a class="nav${HERE === PAGES[0] ? " here" : ""}" href="./">${svg("pulse")}<span>${pageName(PAGES[0])}</span><span class="count" id="problem-count" hidden></span></a>
        <details class="sec" open><summary>${t("side.areas")}${svg("caret", "ic caret")}</summary>${areas.join("")}</details>
        ${group("side-apps", "side.apps")}
        ${group("side-tools", "side.tools")}
      </nav>
      <div class="side-foot">
        <svg class="ring" viewBox="0 0 16 16" aria-hidden="true"><circle cx="8" cy="8" r="6"/><circle cx="8" cy="8" r="6"/></svg>
        <span id="refreshed">${t("loading")}</span><kbd>⌘K</kbd>
      </div>
    </aside>
    <div class="scrim" data-menu></div>`,
  );
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
  document.addEventListener("click", (e) => {
    if (e.target.closest("[data-menu]")) document.body.classList.toggle("nav-open");
  });
  palette();
  tooltips();
}

// The sidebar from the snapshot just read: the areas with anything in them,
// their dots, the count of problems, and the apps and tools.
function paintShell(snap, checks) {
  const areas = shown(snap);
  for (const a of document.querySelectorAll(".side [data-area]")) {
    a.hidden = !areas.includes(a.dataset.area);
    a.querySelector(".dot").className = `dot tone-${TONE[areaState(a.dataset.area, checks).state]}`;
  }
  const byName = Object.fromEntries(checks.map((c) => [c.name, c]));
  const dot = (s) => (!s.check ? "" : byName[s.check] ? TONE[checkState(byName[s.check])] : "gray");
  const link = (s) => `<a class="nav" href="${esc(s.url)}" target="_blank" rel="noopener">
    ${badge(s, "xs")}<span>${esc(s.name)}</span><i class="dot tone-${dot(s)}"></i></a>`;
  for (const [id, links] of [
    ["side-apps", snap.apps],
    ["side-tools", snap.tools],
  ]) {
    const sec = document.getElementById(id);
    sec.hidden = !links.length;
    sec.querySelector("div").innerHTML = links.map(link).join("");
  }
  const count = document.getElementById("problem-count");
  const n = checks.filter((c) => c.problem).length;
  count.hidden = !n;
  count.textContent = number(n);
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
  const input = box.querySelector("input");
  const results = box.querySelector(".pal-list");
  const fuzzy = (q, s) => {
    let i = 0;
    for (const ch of s) if (ch === q[i]) i++;
    return i === q.length;
  };
  const pages = () => PAGES.filter((p) => !p.area || (last.snap && shown(last.snap).includes(p.area)));
  const link = (group) => (s) => ({
    group,
    name: esc(s.name),
    what: esc(s.description || ""),
    glyph: badge(s, "sm"),
    href: s.url,
    out: true,
  });
  const everything = () => [
    ...pages().map((p) => ({
      group: t("palette.go"),
      name: pageName(p),
      glyph: svg(p.icon),
      href: p.href,
      hint: `<kbd>G</kbd><kbd>${p.key.toUpperCase()}</kbd>`,
    })),
    ...(last.snap ? last.snap.apps : []).map(link(t("side.apps"))),
    ...(last.snap ? last.snap.tools : []).map(link(t("side.tools"))),
    ...last.checks.map((c) => ({
      group: t("palette.checks"),
      name: esc(c.name),
      what: c.area ? t(`area.${c.area}`) : "",
      glyph: stateIcon(checkState(c)),
      href: c.url,
      out: true,
    })),
  ];
  let items = [];
  let at = 0;
  const draw = () => {
    const q = input.value.trim().toLowerCase();
    items = everything().filter((i) => fuzzy(q, `${i.name} ${i.what || ""}`.toLowerCase()));
    at = Math.max(0, Math.min(at, items.length - 1));
    const rows = items.map(
      (i, n) =>
        (i.group !== items[n - 1]?.group ? `<p class="pal-group">${i.group}</p>` : "") +
        `<a class="pal-item${n === at ? " on" : ""}" href="${esc(i.href)}" data-n="${n}" role="option" ${i.out ? 'target="_blank" rel="noopener"' : ""}>
        ${i.glyph}<span class="title">${i.name}</span>${i.what ? `<span class="detail">${i.what}</span>` : ""}
        <span class="pal-hint">${i.hint || (i.out ? svg("out") : "")}</span></a>`,
    );
    results.innerHTML = rows.join("") || `<p class="pal-empty">${t("palette.nothing")}</p>`;
    results.querySelector(".on")?.scrollIntoView({ block: "nearest" });
  };
  const open = () => {
    input.value = "";
    at = 0;
    draw();
    box.showModal();
    input.focus();
  };
  const go = (i) => {
    box.close();
    if (i.out) window.open(i.href, "_blank", "noopener");
    else location.href = i.href;
  };
  input.addEventListener("input", () => {
    at = 0;
    draw();
  });
  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      at = (at + (e.key === "ArrowDown" ? 1 : -1) + items.length) % items.length;
      draw();
    }
    if (e.key === "Enter" && items[at]) {
      e.preventDefault();
      go(items[at]);
    }
  });
  results.addEventListener("mousemove", (e) => {
    const a = e.target.closest(".pal-item");
    if (a && +a.dataset.n !== at) {
      at = +a.dataset.n;
      draw();
    }
  });
  results.addEventListener("click", (e) => {
    const a = e.target.closest(".pal-item");
    if (a) {
      e.preventDefault();
      go(items[+a.dataset.n]);
    }
  });
  box.addEventListener("click", (e) => {
    if (e.target === box) box.close();
  });
  document.addEventListener("click", (e) => {
    if (e.target.closest("[data-palette]")) open();
  });
  let g = 0;
  document.addEventListener("keydown", (e) => {
    const typing = e.target.closest("input, textarea, [contenteditable]");
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
      e.preventDefault();
      if (box.open) box.close();
      else open();
      return;
    }
    if (typing || e.metaKey || e.ctrlKey || e.altKey) return;
    if (e.key === "/") {
      e.preventDefault();
      open();
      return;
    }
    const p = Date.now() - g < 800 && pages().find((p) => p.key === e.key.toLowerCase());
    if (p) location.href = p.href;
    g = e.key.toLowerCase() === "g" ? Date.now() : 0;
  });
}

// Anything with data-tip shows it under the pointer, or under a finger while
// it presses.
function tooltips() {
  const pop = document.createElement("div");
  pop.className = "tip-pop";
  pop.setAttribute("role", "tooltip");
  pop.hidden = true;
  document.body.append(pop);
  let on = null;
  const show = (e) => {
    const el = e.target.closest?.("[data-tip]");
    if (el !== on) {
      on = el;
      pop.hidden = !el;
      if (el) pop.innerHTML = el.dataset.tip;
    }
    if (!el) return;
    const w = pop.offsetWidth;
    const h = pop.offsetHeight;
    const x = Math.min(Math.max(8, e.clientX - w / 2), innerWidth - w - 8);
    pop.style.transform = `translate(${x}px, ${e.clientY - h - 14 < 8 ? e.clientY + 20 : e.clientY - h - 14}px)`;
  };
  document.addEventListener("pointermove", show);
  document.addEventListener("pointerdown", show);
  document.addEventListener("pointerout", (e) => {
    if (!e.relatedTarget) {
      on = null;
      pop.hidden = true;
    }
  });
  addEventListener(
    "scroll",
    () => {
      on = null;
      pop.hidden = true;
    },
    { passive: true },
  );
}

/**
 * Draws the page from a new snapshot now, every minute while it shows, on
 * coming back to it, and on ↻. The ring in the sidebar runs down to the next
 * read. draw gets the snapshot and its checks.
 */
export async function start(draw) {
  await loadWords();
  translate(document);
  shell();
  const run = async () => {
    document.body.classList.add("loading");
    try {
      const read = await fetch(DEMO ? "snapshot.json" : "snapshot", { cache: "no-store" }).then(
        (r) => (r.ok ? r.json() : null),
        () => null,
      );
      const snap = read && DEMO ? asIfNow(read) : read;
      document.getElementById("refreshed").innerHTML = snap
        ? t("updated", { at: new Date() })
        : t("overlook.no_answer");
      if (!snap) return;
      const checks = readChecks(snap);
      last = { snap, checks };
      paintShell(snap, checks);
      draw(snap, checks);
    } finally {
      document.body.classList.remove("loading");
      const ring = document.querySelector(".ring circle:last-child");
      ring.style.animation = "none";
      ring.getBoundingClientRect();
      ring.style.animation = "";
      setTimeout(() => document.documentElement.classList.remove("boot"), 1200);
    }
  };
  run();
  setInterval(() => {
    if (!document.hidden) run();
  }, 60e3);
  document.addEventListener("visibilitychange", () => {
    if (!document.hidden) run();
  });
  document.addEventListener("click", (e) => {
    if (e.target.closest("[data-refresh]")) run();
  });
}
