// The overview: how everything is, what is wrong, today's figures, each area
// and its checks, the week, what happened, the apps, and the chores.

import { areaState, checkState, choresBy, quietSince, shown, silent, together } from "./checks.js";
import {
  badge,
  figureMeter,
  GREEN,
  hourChart,
  RECENT,
  RED,
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
import { ago, clock, day, esc, list, ms, number, say, span, t, when } from "./text.js";

const DAY = 864e5;
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

function renderStatus(snap, checks) {
  const problems = checks.filter((c) => c.problem);
  const areas = shown(snap)
    .filter((a) => checks.some((c) => c.area === a))
    .map((a) => areaState(a, checks));
  const silence = silent(snap);
  const blind = !checks.length && silence.length > 0;
  const state = blind ? "down" : !problems.length ? "up" : areas.some((a) => a.state === "down") ? "down" : "degraded";
  const quiet = quietSince(checks);
  $("status").className = `status tone-${TONE[state]}`;
  $("status-icon").innerHTML = stateIcon(state);
  $("headline").innerHTML = blind
    ? t("source.no_answer", { name: list(silence) })
    : problems.length
      ? t("status.problems", { n: problems.length })
      : t("status.up");
  if (blind) {
    $("summary").innerHTML = t("source.no_answer_why", { error: snap.sources.find((s) => !s.answered).error });
    return;
  }
  const parts = problems.length
    ? [list(areas.filter((a) => a.state !== "up").map((a) => t(`status.area_${a.state}`, { area: a.area })))]
    : [
        quiet.since &&
          (quiet.problemFree
            ? t("status.quiet_since", { at: quiet.since })
            : t("status.quiet_for", { ms: Date.now() - quiet.since })),
        `${t("status.checks", { n: checks.length })} ${t("status.areas", { n: areas.length })}`,
      ];
  if (silence.length) parts.push(t("source.no_answer", { name: list(silence) }));
  $("summary").innerHTML = parts.filter(Boolean).join(" · ");
}

function renderActions(snap) {
  const u = snap.updates;
  const target = u.length === 1 ? `href="${esc(u[0].url)}" target="_blank" rel="noopener"` : 'href="#chores-block"';
  const merge = u.length
    ? `<a class="action strong" ${target}><span class="tone-green">${svg("pr")}</span>${t("actions.merge", { n: u.length })}</a>`
    : "";
  const tools = snap.tools
    .slice(0, 5)
    .map(
      (s) =>
        `<a class="action" href="${esc(s.url)}" target="_blank" rel="noopener">${badge(s, "sm")}${esc(s.name)}</a>`,
    );
  $("actions").innerHTML =
    merge +
    tools.join("") +
    `<button class="action" type="button" data-palette>${svg("search")}${t("actions.everything")}<kbd>⌘K</kbd></button>`;
}

function renderProblems(checks) {
  const problems = checks.filter((c) => c.problem);
  $("problems").hidden = !problems.length;
  $("problem-n").textContent = number(problems.length);
  $("problem-list").innerHTML = problems
    .map((c) => {
      const since = c.since && t("problem.since", { at: c.since, ms: Date.now() - c.since });
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

// ── Today: each figure, opening its area's page. One with hours takes a row
// of its own for them, as a chart ────────────────────────────────────────

function renderToday(figures) {
  $("today-block").hidden = !figures.length;
  $("today").innerHTML = figures
    .map((f, i) => {
      const tint = TINT[f.area];
      const hours = f.hours?.length ? hourChart({ label: f.label, hours: f.hours }, f.within, tint) : "";
      // Shares without a tone of their own take the area's colour, then the others'.
      const tints = [tint, ...Object.values(TINT).filter((c) => c !== tint)];
      return `<a class="figure-tile${hours ? " wide" : ""}${f.tone ? ` tone-${f.tone}` : ""}" href="${f.area}" style="--i:${i}">
      <span class="label">${svg(f.icon)}${say(f.label)}</span>
      <span class="figure">${num(`${f.area}.${i}`, f.value)}${f.unit ? `<small> ${say(f.unit)}</small>` : ""}</span>
      <span class="sub">${f.detail ? say(f.detail) : ""}</span>
      <span class="viz">${hours}${f.shares?.length ? shares(f, tints) : ""}${(f.meters || []).map(figureMeter).join("")}</span></a>`;
    })
    .join("");
}

// ── The areas, as a table that unfolds into their checks ─────────────────

const unfolded = new Set();

// Each moment's worst result across an area's checks: red when any failed.
function worst(a) {
  return Array.from({ length: RECENT }, (_, k) => {
    const at = a.checks.map((c) => [c, c.recent[c.recent.length - RECENT + k]]).filter(([, r]) => r);
    if (!at.length) return null;
    const failed = at.filter(([, r]) => !r.ok);
    const rows = failed.length
      ? failed.map(([c]) => [RED, esc(c.name), t("areas.failed")])
      : [[GREEN, t("areas.all_passed", { n: at.length }), ""]];
    return {
      ok: !failed.length,
      at: at[0][1].at,
      tip: tip(clock(at[0][1].at), rows, failed.length === 1 ? esc(failed[0][1].why || "") : ""),
    };
  });
}

function renderAreas(checks, areas) {
  $("areas-block").hidden = !areas.length;
  $("areas").innerHTML = areas
    .map((name, i) => {
      const a = areaState(name, checks);
      const p = page(name);
      const fine = a.checks.filter((c) => !c.problem).length;
      const times = a.checks
        .filter((c) => !c.failing && c.ms !== null)
        .map((c) => c.ms)
        .sort((x, y) => x - y);
      const ring = `<svg class="pie" viewBox="0 0 14 14" aria-hidden="true"><circle cx="7" cy="7" r="5.5"/><circle cx="7" cy="7" r="5.5" pathLength="1" style="stroke-dasharray:${(fine / a.checks.length).toFixed(3)} 1"/></svg>`;
      const count = tip(
        t("areas.checks_of", { area: name }),
        [
          [GREEN, word("up"), number(fine)],
          ...(a.problems.length ? [[RED, t("problems.title"), number(a.problems.length)]] : []),
        ],
        a.problems.length ? esc(list(a.problems.map((c) => c.name))) : "",
      );
      const kids = a.checks.map((c) => {
        const health = c.problem
          ? word("down")
          : c.failing
            ? t("check.failed_n", { run: c.run, threshold: c.threshold })
            : word("up");
        return `<a class="trow kid" href="${esc(c.url)}" target="_blank" rel="noopener">
        <span class="tname">${stateIcon(checkState(c))}<span>${esc(c.name)}</span></span>
        <span class="health tone-${TONE[checkState(c)]}">${health}</span>
        <span class="tcount detail">${c.failing ? esc(c.why || "") : ""}</span>
        <span class="tms">${c.failing ? "" : ms(c.ms)}</span>${strip(c.recent)}</a>`;
      });
      return `<details class="area" data-area="${name}" style="--i:${i}"${unfolded.has(name) ? " open" : ""}>
      <summary class="trow">
        <span class="tname">${svg("chevron", "ic twist")}<span class="badge" style="--tint:${TINT[name]}">${svg(p.icon)}</span><a href="${p.href}">${pageName(p)}</a></span>
        <span class="health tone-${TONE[a.state]}">${stateIcon(a.state)}${word(a.state)}</span>
        <span class="tcount" data-tip="${count}">${ring}${number(fine)} / ${number(a.checks.length)}</span>
        <span class="tms">${times.length ? ms(times[times.length >> 1]) : ""}</span>
        ${strip(worst(a))}
      </summary>
      <div class="tkids">${kids.join("")}</div>
    </details>`;
    })
    .join("");
}

// ── This week's events, newest first ─────────────────────────────────────

function renderEvents(checks, events, updates, silence) {
  const weekAgo = Date.now() - 7 * DAY;
  const items = [];
  for (const c of checks) {
    for (const o of c.outages) {
      if (o.from >= weekAgo) {
        items.push({
          at: o.from,
          mark: stateIcon("down"),
          text: t("events.down", { name: c.name }),
          meta: where(c.area),
        });
      }
      if (o.to && o.to >= weekAgo) {
        const text = t("events.back", { name: c.name, ms: o.to - o.from });
        items.push({ at: o.to, mark: stateIcon("up"), text, meta: where(c.area) });
      }
    }
  }
  for (const e of events) {
    const at = new Date(e.at);
    if (at >= weekAgo) {
      const mark = `<span class="tone-${e.tone || "blue"}">${svg(e.icon)}</span>`;
      items.push({ at, mark, text: say(e.text), meta: where(e.area) });
    }
  }
  for (const u of updates) {
    const mark = `<span class="tone-green">${svg("pr")}</span>`;
    items.push({
      at: new Date(u.at),
      mark,
      text: t("events.update", { name: u.name }),
      meta: t("events.update_meta"),
    });
  }
  items.sort((a, b) => b.at - a.at);
  // With nothing to show and a source silent, the week may not have been quiet.
  const empty = silence.length ? t("source.no_answer", { name: list(silence) }) : t("events.quiet");
  $("events").innerHTML =
    items
      .slice(0, 8)
      .map(
        (it, i) => `<li style="--i:${i}">
    <span class="mark">${it.mark}</span><span class="feed-text">${it.text} <span class="meta">${[it.meta, ago(it.at)].filter(Boolean).join(" · ")}</span></span></li>`,
      )
      .join("") || `<li class="empty">${empty}</li>`;
}

// ── This week: each area along the last 7 days, its problems in red, its
// events as diamonds ──────────────────────────────────────────────────────

function renderWeek(checks, events, areas) {
  $("week-block").hidden = !areas.length;
  // Midnights by the calendar, not 24 h apart: a day the clocks change lasts 23 or 25.
  const days = Array.from({ length: 7 }, (_, i) => {
    const d = new Date();
    d.setHours(0, 0, 0, 0);
    d.setDate(d.getDate() - 6 + i);
    return d;
  });
  const start = +days[0];
  const end = Date.now();
  const x = (at) => `${((100 * (Math.max(at, start) - start)) / (end - start)).toFixed(2)}%`;
  const head = days.map(
    (d, i) =>
      `<span style="left:${x(+d)}"${i === 6 ? ' class="today"' : ""}><span class="wd">${d.toLocaleDateString(undefined, { weekday: "short" })} </span><b>${d.toLocaleDateString(undefined, { day: "numeric" })}</b></span>`,
  );
  const rows = areas.map((name, i) => {
    const mine = checks
      .filter((c) => c.area === name)
      .flatMap((c) => c.outages)
      .filter((o) => !o.to || o.to >= start);
    const happened = events.filter((e) => e.area === name && new Date(e.at) >= start);
    const down = mine.reduce((sum, o) => sum + ((o.to || end) - Math.max(+o.from, start)), 0);
    const summary = tip(
      t("week.area", { area: name }),
      mine.length
        ? [[RED, t("week.problems", { n: mine.length }), t("week.in_all", { ms: down })]]
        : [[GREEN, t("week.no_problems"), ""]],
    );
    const outages = mine.map((o) => {
      const rows = [
        [
          RED,
          t(o.to ? "week.was_down" : "week.down"),
          o.to ? span(o.to - o.from) : t("week.for", { ms: end - o.from }),
        ],
        ["", t("week.from"), when(o.from)],
        ...(o.to ? [["", t("week.back"), when(o.to)]] : []),
      ];
      return `<span class="tl-out${o.to ? "" : " now"}" style="left:${x(+o.from)};width:calc(${x(+(o.to || end))} - ${x(+o.from)})" data-tip="${tip(esc(o.check.name), rows)}"></span>`;
    });
    const diamonds = happened.map((e) => {
      const bad = e.tone === "red";
      return `<span class="tl-ms${bad ? " bad" : ""}" style="left:${x(+new Date(e.at))}" data-tip="${tip(say(e.text), [[bad ? RED : "#4EA7FC", when(e.at), ""]])}"></span>`;
    });
    return `<div class="tl-row" style="--i:${i};--tint:${TINT[name]}">
      <span class="tl-label">${svg(page(name).icon)}${t(`area.${name}`)}</span>
      <span class="tl-track"><span class="tl-bar" data-tip="${summary}"></span>${outages.join("")}${diamonds.join("")}</span></div>`;
  });
  $("week").innerHTML = `<div class="tl-row tl-head"><span></span><span class="tl-track">${head.join("")}</span></div>
    <div class="tl-grid">${days.map((d) => `<i style="left:${x(+d)}"></i>`).join("")}</div>${rows.join("")}`;
}

// ── The apps: each one's door, whether it answers, and how fast lately ───

function renderApps(apps, checks) {
  const byName = Object.fromEntries(checks.map((c) => [c.name, c]));
  $("apps-block").hidden = !apps.length;
  $("apps").innerHTML = apps
    .map((s, i) => {
      const c = s.check && byName[s.check];
      const state = c ? checkState(c) : "unknown";
      const tint = esc(s.color || "#7170FF");
      const foot = !s.check ? t("apps.no_check") : !c || c.failing ? word(state) : ms(c.ms);
      const tips = c
        ? c.recent.map((r) =>
            tip(
              clock(r.at),
              [[r.ok ? tint : RED, t(r.ok ? "apps.answered" : "apps.failed_after"), ms(r.ms)]],
              esc(r.why || ""),
            ),
          )
        : [];
      return `<a class="app" href="${esc(s.url)}" target="_blank" rel="noopener" style="--tint:${tint};--i:${i}">
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

// ── The chores: the updates waiting for their merge, then the chores the
// config writes down that are due, then those this week. A chore is done when
// its lines leave the config ───────────────────────────────────────────────

function renderChores(updates, chores) {
  $("chores-block").hidden = !updates.length && !chores.length;
  const { due, week, next } = choresBy(chores);
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
      updates.map((u) => row(merge, esc(u.name), t("chores.update"), ago(new Date(u.at)), u.url)),
    ],
    [
      stateIcon("degraded"),
      t("chores.due"),
      due.map((c) => row(stateIcon("degraded"), esc(c.name), esc(c.note || ""), t("chores.due"))),
    ],
    [
      stateIcon("unknown"),
      t("chores.week"),
      week.map((c) => row(stateIcon("unknown"), esc(c.name), esc(c.note || ""), day(c.from))),
    ],
  ].filter(([, , rows]) => rows.length);
  $("chores").innerHTML =
    groups
      .map(
        ([mark, name, rows]) => `<div class="group">
    <div class="group-head">${mark}<span>${name}</span><span class="n">${number(rows.length)}</span></div>${rows.join("")}</div>`,
      )
      .join("") ||
    // With nothing waiting, due or this week, the block shows only for a chore after it.
    `<div class="empty card">${t("chores.next", { name: next.name, at: next.from })}</div>`;
}

$("areas").addEventListener(
  "toggle",
  (e) => {
    if (e.target.open) unfolded.add(e.target.dataset.area);
    else unfolded.delete(e.target.dataset.area);
  },
  true,
);

start((snap, checks) => {
  const areas = shown(snap);
  renderStatus(snap, checks);
  renderActions(snap);
  renderProblems(checks);
  renderToday(together(snap.figures));
  renderAreas(
    checks,
    areas.filter((a) => checks.some((c) => c.area === a)),
  );
  renderEvents(checks, snap.events, snap.updates, silent(snap));
  renderWeek(checks, snap.events, areas);
  renderApps(snap.apps, checks);
  renderChores(snap.updates, snap.chores);
  countUp(document);
});
