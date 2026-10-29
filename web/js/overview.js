// The overview: how everything is, what is wrong, today's figures, each area
// and its checks, today by the hour, what happened, the week, the apps, and
// the chores. It writes what page() says (page.js) and decides nothing.

import {
  also,
  badge,
  bars,
  by,
  figureMeter,
  GREEN,
  HOURS,
  hourStart,
  hourTip,
  legend,
  more,
  RED,
  seriesColours,
  shares,
  sparkline,
  stateIcon,
  strip,
  svg,
  TINT,
  TONE,
  tip,
  word,
} from "./draw.js";
import { PAGES, pageName, start } from "./shell.js";
import { ago, clock, date, day, esc, list, ms, number, say, span, t, weekday, when } from "./text.js";

const page = (area) => PAGES.find((p) => p.area === area);
const where = (area) => (area ? t(`area.${area}`) : "");
const $ = (id) => document.getElementById(id);

// Figures count up to their value, from what they showed before.
const before = new Map();
const num = (key, n) =>
  `<span class="count-up" data-key="${esc(key)}" data-n="${n}">${number(before.get(key) ?? 0)}</span>`;
function countUp(root) {
  const still = matchMedia("(prefers-reduced-motion: reduce)").matches;
  for (const el of root.querySelectorAll("[data-n]")) {
    const to = +el.dataset.n;
    const from = before.get(el.dataset.key) ?? 0;
    before.set(el.dataset.key, to);
    if (still || from === to) {
      el.textContent = number(to);
      continue;
    }
    const t0 = performance.now();
    const step = (now) => {
      const k = Math.min(1, Math.max(0, now - t0) / 900);
      el.textContent = number(Math.round(from + (to - from) * (1 - (1 - k) ** 3)));
      if (k < 1) requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
  }
}

// ── How everything is, and the quick ways out ────────────────────────────

function renderStatus(homelab) {
  $("status").className = `status tone-${TONE[homelab.state]}`;
  $("status-icon").innerHTML = stateIcon(homelab.state);
  $("headline").innerHTML = say(homelab.headline);
  $("summary").innerHTML = homelab.summary.map((part) => part.map(say).join(" ")).join(" · ");
}

function renderActions(updates, tools) {
  const target =
    updates.length === 1 ? `href="${esc(updates[0].url)}" target="_blank" rel="noopener"` : 'href="#chores-block"';
  const merge = updates.length
    ? `<a class="action strong" ${target}><span class="tone-green">${svg("pr")}</span>${t("actions.merge", { n: updates.length })}</a>`
    : "";
  const links = tools
    .slice(0, 5)
    .map(
      ({ link: s }) =>
        `<a class="action" href="${esc(s.url)}" target="_blank" rel="noopener">${badge(s, "sm")}${esc(s.name)}</a>`,
    );
  $("actions").innerHTML =
    merge +
    links.join("") +
    `<button class="action" type="button" data-palette>${svg("search")}${t("actions.everything")}<kbd>⌘K</kbd></button>`;
}

function renderProblems(problems, now) {
  $("problems").hidden = !problems.length;
  $("problem-n").textContent = number(problems.length);
  $("problem-list").innerHTML = problems
    .map((c) => {
      const since = c.since && t("problem.since", { at: c.since, ms: now - c.since });
      return `
    <a class="inbox-row" href="${esc(c.url)}" target="_blank" rel="noopener">
      ${stateIcon("down")}
      <span class="copy"><span class="title strong">${esc(c.name)}</span>
        <span class="detail">${[where(c.area), since].filter(Boolean).join(" · ")}</span></span>
      <code class="why">${esc(c.why || "")}</code>
      <span class="go">${t("problem.open", { source: c.source })} ${svg("out")}</span></a>`;
    })
    .join("");
}

// ── Today: each figure, opening its area's page ──────────────────────────

// A figure's hours as series: its own, then the part within them, if any.
const seriesOf = (f) => [{ label: f.label, hours: f.hours }, ...(f.within ? [f.within] : [])];

function renderToday({ tiles, columns }, now) {
  $("today-block").hidden = !tiles.length;
  $("today").style.setProperty("--cols", columns);
  $("today").innerHTML = tiles
    .map((f, i) => {
      const hours = f.hours || [];
      const series = seriesOf(f);
      const colours = seriesColours(series.length);
      const tips = hours.map((_, k) =>
        hourTip(series, colours, k, hourStart(k, hours.length, now), { last: k === hours.length - 1 }),
      );
      // A tile's label has no room for its source, so the line under the
      // number starts with it. A tile several figures share is none of their
      // sources': each of the others is named after its number, and every one
      // in the bar. One added up from some of its sources says so.
      const some =
        f.lacking.length > 0 &&
        `<span data-tip="${tip(esc(list(f.lacking)), [], t("figure.gave_none"))}">${t("figure.of_sources", { n: f.sources.length, total: f.sources.length + f.lacking.length })}</span>`;
      const sub = f.wide
        ? ""
        : f.also
          ? list(f.also.map(also))
          : [f.by && esc(f.by), f.detail && say(f.detail), some].filter(Boolean).join(" · ");
      return `<a class="figure-tile${f.wide ? " wide" : ""}${f.tone ? ` tone-${f.tone}` : ""}" href="${f.area}" style="--i:${i}">
      <span class="label">${svg(f.icon)}${say(f.label)}</span>
      <span class="figure">${num(`${f.area}.${i}`, f.value)}${f.unit ? `<small> ${say(f.unit)}</small>` : ""}</span>
      <span class="sub">${sub}</span>
      <span class="viz">${sparkline(hours, HOURS, tips)}${f.shares?.length || f.wide ? shares(f, f.start) : ""}${(f.meters || []).map(figureMeter).join("")}</span></a>`;
    })
    .join("");
}

// ── Today by the hour: each figure with hours, as bars, beside the events.
// With none, the week takes their place there ───────────────────────────

function renderHours(tiles, now) {
  const mine = tiles.filter((f) => f.hours?.length);
  $("hours-block").hidden = !mine.length;
  if (mine.length) $("day").after($("week-block"));
  else $("events-block").before($("week-block"));
  $("hours").innerHTML = mine
    .map((f, i) => {
      const series = seriesOf(f);
      const colours = seriesColours(series.length);
      return `<div class="card chart" style="--i:${i}">
      <div class="chart-head"><span class="label">${svg(f.icon)}${say(f.label)}${by(f)}</span>${legend(series, colours)}</div>
      <div class="chart-top"><span class="figure">${num(`hours.${f.area}.${i}`, f.value)}</span><span class="detail">${f.detail ? say(f.detail) : ""}</span></div>
      ${bars(series, colours, now)}</div>`;
    })
    .join("");
}

// ── The areas, as a table that unfolds into their checks ─────────────────

const unfolded = new Set();

// An area's moment: red when any check failed then, each named in its tooltip.
const moment = (r) =>
  r && {
    ...r,
    tip: tip(
      clock(r.at),
      r.failed.length
        ? r.failed.map(({ check }) => [RED, esc(check.name) + by(check), t("areas.failed")])
        : [[GREEN, t("areas.all_passed", { n: r.n }), ""]],
      r.failed.length === 1 ? esc(r.failed[0].why || "") : "",
    ),
  };

// A check's health in the table, by its state.
const health = (c) =>
  ({
    down: word("down"),
    degraded: t("check.failed_n", { run: c.run, threshold: c.threshold }),
    up: word("up"),
    unknown: t("check.no_results"),
  })[c.state];

function renderAreas(areas) {
  $("areas-block").hidden = !areas.length;
  $("areas").innerHTML = areas
    .map((a, i) => {
      const p = page(a.area);
      const fine = a.checks.length - a.problems.length;
      const ring = `<svg class="pie" viewBox="0 0 14 14" aria-hidden="true"><circle cx="7" cy="7" r="5.5"/><circle cx="7" cy="7" r="5.5" pathLength="1" style="stroke-dasharray:${(fine / a.checks.length).toFixed(3)} 1"/></svg>`;
      const count = tip(
        t("areas.checks_of", { area: a.area }),
        [
          [GREEN, word("up"), number(fine)],
          ...(a.problems.length ? [[RED, t("problems.title"), number(a.problems.length)]] : []),
        ],
        a.problems.length ? esc(list(a.problems.map((c) => c.name))) : "",
      );
      const kids = a.checks.map(
        (c) => `<a class="trow kid" href="${esc(c.url)}" target="_blank" rel="noopener">
        <span class="tname">${stateIcon(c.state)}<span>${esc(c.name)}${by(c)}</span></span>
        <span class="health tone-${TONE[c.state]}">${health(c)}</span>
        <span class="tcount detail">${c.failing ? esc(c.why || "") : ""}</span>
        <span class="tms">${c.failing ? "" : ms(c.ms)}</span>${strip(c.recent)}</a>`,
      );
      return `<details class="area" data-area="${a.area}" style="--i:${i}"${unfolded.has(a.area) ? " open" : ""}>
      <summary class="trow">
        <span class="tname">${svg("chevron", "ic twist")}<span class="badge" style="--tint:${TINT[a.area]}">${svg(p.icon)}</span><a href="${p.href}">${pageName(p)}</a></span>
        <span class="health tone-${TONE[a.state]}">${stateIcon(a.state)}${word(a.state)}</span>
        <span class="tcount" data-tip="${count}">${ring}${number(fine)} / ${number(a.checks.length)}</span>
        <span class="tms">${ms(a.ms)}</span>
        ${strip(a.strip.map(moment))}
      </summary>
      <div class="tkids">${kids.join("")}</div>
    </details>`;
    })
    .join("");
}

// ── This week's events, newest first ─────────────────────────────────────

function renderEvents({ items, empty }, now) {
  const mark = (m) => (m.state ? stateIcon(m.state) : `<span class="tone-${m.tone}">${svg(m.icon)}</span>`);
  $("events").innerHTML =
    items
      .map(
        (it, i) => `<li style="--i:${i}">
    <span class="mark">${mark(it.mark)}</span><span class="feed-text">${say(it.text)} <span class="meta">${[...it.about.map(say), ago(it.at, now)].join(" · ")}</span></span></li>`,
      )
      .join("") || `<li class="empty">${say(empty)}</li>`;
}

// ── This week: each area along the last 7 days, its problems in red, its
// events as diamonds ──────────────────────────────────────────────────────

const at = (left) => `${(100 * left).toFixed(2)}%`;

function renderWeek({ days, rows }, now) {
  $("week-block").hidden = !rows.length;
  const head = days.map(
    (d, i) =>
      `<span style="left:${at(d.left)}"${i === 6 ? ' class="today"' : ""}><span class="wd">${weekday(d.at)} </span><b>${date(d.at)}</b></span>`,
  );
  const lines = rows.map((row, i) => {
    const summary = tip(
      t("week.area", { area: row.area }),
      row.outages.length
        ? [[RED, t("week.problems", { n: row.outages.length }), t("week.in_all", { ms: row.down })]]
        : [[GREEN, t("week.no_problems"), ""]],
    );
    const outages = row.outages.map((o) => {
      const tips = [
        [RED, o.to ? t("week.was_down") : t("week.down"), o.to ? span(o.ms) : t("week.for", { ms: o.ms })],
        ["", t("week.from"), when(o.from, now)],
        ...(o.to ? [["", t("week.back"), when(o.to, now)]] : []),
      ];
      return `<span class="tl-out${o.to ? "" : " now"}" style="left:${at(o.left)};width:${at(o.width)}" data-tip="${tip(esc(o.check.name), tips)}"></span>`;
    });
    const diamonds = row.events.map(({ events, left, bad }) => {
      const colour = (e) => (e.tone === "red" ? RED : "var(--m-blue)");
      const tp =
        events.length === 1
          ? tip(say(events[0].text) + by(events[0]), [[colour(events[0]), when(events[0].at, now), ""]])
          : tip(
              t("week.events", { n: events.length }),
              events.map((e) => [colour(e), say(e.text) + by(e), clock(new Date(e.at))]),
            );
      return `<span class="tl-ms${bad ? " bad" : ""}${events.length > 1 ? " many" : ""}" style="left:${at(left)}" data-tip="${tp}"></span>`;
    });
    return `<div class="tl-row" style="--i:${i};--tint:${TINT[row.area]}">
      <span class="tl-label">${svg(page(row.area).icon)}${t(`area.${row.area}`)}</span>
      <span class="tl-track"><span class="tl-bar" data-tip="${summary}"></span>${outages.join("")}${diamonds.join("")}</span></div>`;
  });
  $("week").innerHTML = `<div class="tl-row tl-head"><span></span><span class="tl-track">${head.join("")}</span></div>
    <div class="tl-grid">${days.map((d) => `<i style="left:${at(d.left)}"></i>`).join("")}</div>${lines.join("")}`;
}

// ── The apps: each one's door, whether it answers, and how fast lately ───

function renderApps({ links, compact, columns }) {
  $("apps-block").hidden = !links.length;
  // Small tiles show the logo, the name, the dot and the time, the
  // description in the tooltip.
  $("apps").classList.toggle("compact", compact);
  $("apps").style.setProperty("--cols", columns);
  $("apps").innerHTML = links
    .map(({ link: s, check: c, state }, i) => {
      const tint = esc(s.color || "#7170FF");
      const foot = !s.check ? t("apps.no_check") : !c || c.failing ? word(state) : ms(c.ms);
      const tips = c
        ? c.recent.map((r) =>
            tip(
              clock(r.at),
              [[r.ok ? tint : RED, r.ok ? t("apps.answered") : t("apps.failed_after"), ms(r.ms)]],
              esc(r.why || ""),
            ),
          )
        : [];
      const about = compact && s.description ? ` data-tip="${tip(esc(s.name), [], esc(s.description))}"` : "";
      return `<a class="app" href="${esc(s.url)}" target="_blank" rel="noopener" style="--tint:${tint};--i:${i}"${about}>
      <span class="app-top">${badge(s, "lg")}<span class="dot tone-${TONE[state]}"></span></span>
      <span class="title">${esc(s.name)}</span><span class="detail">${esc(s.description || "")}</span>
      <span class="app-foot"><span>${foot}</span>${
        c
          ? sparkline(
              c.recent.map((r) => r.ms),
              tint,
              tips,
            )
          : ""
      }</span></a>`;
    })
    .join("");
}

// ── The chores: the updates waiting for their merge, then the chores on the
// to-do lists that are due, then those this week. A chore is done when it's
// marked done on its list (ADR 0010) ───────────────────────────────────────

function renderChores({ updates, due, week, next }, now) {
  $("chores-block").hidden = !updates.length && !due.length && !week.length && !next;
  const merge = `<span class="tone-green">${svg("pr")}</span>`;
  const row = (mark, title, detail, meta, href) => {
    const copy = `${mark}<span class="copy"><span class="title">${title}</span>${detail ? `<span class="detail">${detail}</span>` : ""}</span><span class="meta">${meta}</span>`;
    return href
      ? `<a href="${esc(href)}" target="_blank" rel="noopener" class="chore">${copy}</a>`
      : `<div class="chore">${copy}</div>`;
  };
  const groups = [
    [
      merge,
      t("chores.merge"),
      updates.map((u) => row(merge, esc(u.name), t("chores.update"), ago(new Date(u.at), now), u.url)),
    ],
    [
      stateIcon("degraded"),
      t("chores.due"),
      due.map((c) => row(stateIcon("degraded"), esc(c.name) + by(c), esc(c.note || ""), t("chores.due"), c.url)),
    ],
    [
      stateIcon("unknown"),
      t("chores.week"),
      week.map((c) => row(stateIcon("unknown"), esc(c.name) + by(c), esc(c.note || ""), day(c.from), c.url)),
    ],
  ].filter(([, , rows]) => rows.length);
  $("chores").innerHTML =
    groups
      .map(
        ([mark, name, rows]) => `<div class="group">
    <div class="group-head">${mark}<span>${name}</span><span class="n">${number(rows.length)}</span></div>${more(rows, 5)}</div>`,
      )
      .join("") ||
    // With nothing waiting, due or this week, the block shows only for a chore
    // after it, and with none, it's hidden.
    (next ? `<div class="empty card">${t("chores.next", { name: next.name, at: next.from })}</div>` : "");
}

$("areas").addEventListener(
  "toggle",
  (e) => {
    if (e.target.open) unfolded.add(e.target.dataset.area);
    else unfolded.delete(e.target.dataset.area);
  },
  true,
);

start((m) => {
  renderStatus(m.homelab);
  renderActions(m.chores.updates, m.tools);
  renderProblems(m.problems, m.now);
  renderToday(m.today, m.now);
  renderHours(m.today.tiles, m.now);
  renderAreas(m.areas.filter((a) => a.shown && a.checks.length));
  renderEvents(m.events, m.now);
  renderWeek(m.week, m.now);
  renderApps(m.apps);
  renderChores(m.chores, m.now);
  countUp(document);
});
