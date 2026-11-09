// What the page says: the snapshot read at a moment, made into what the
// overview, each area's page, the sidebar and ⌘K show. Every decision the page
// makes is here, where Node tests it (ADR 0007), but the states, which Go
// decides and the snapshot carries, and the page only words (ADR 0017);
// overview.js, area.js and shell.js only write what page() returns.

import { anchor, readme } from "./config.js";
import { english } from "./text.js";

export const AREAS = ["servers", "network", "storage", "apps"];

/**
 * The check engines Overlook reads, by their adapters' keys, in the order
 * the setup page offers them, which the page names to a homelab with none.
 * The server's tests keep it in step with the adapters.
 */
export const CHECK_ENGINES = ["gatus", "uptimekuma", "grafana", "prometheus", "checkmate"];

/** How many results a strip shows: as many as a check engine gives (snapshot.Recent). */
export const RECENT = 30;

const DAY = 864e5;

/**
 * What the page says of the snapshot read, at now:
 *
 * - homelab: its state, its headline and its summary, as texts;
 * - problems: the checks that are problems;
 * - checks: every check, each with its state, what its state doesn't say,
 *   and what its strip says in words;
 * - areas: the four, in the page's order, each with whether it's shown and
 *   whether its checks judge it, its state, its checks and problems, how many
 *   are up and how many are in each state, its summary, its middle response
 *   time, its strip and what it says in words, its charts, its sections and
 *   its silent sources;
 * - today: its tiles and how many columns they take;
 * - events: this week's, newest first, and what to say with none;
 * - week: its seven days and each area's outages and events along them;
 * - apps: each link with its check, the check's state and the foot of its
 *   tile, whether the tiles go small, and how many columns they take; tools:
 *   each link with its check and the check's state;
 * - chores: those due, this week's and the next, and the updates;
 * - checkEngines: with no check engine in the homelab, those Overlook
 *   reads, each its name and its section of the README;
 * - version: the Overlook that read it, if it says.
 *
 * Texts are the snapshot's own: a message's key and its values, or a name.
 * Moments are dates; where the page draws along a span, as along the week,
 * a place on it is a fraction of it.
 */
export function page(read, now) {
  const snap = named(read);
  const checks = readChecks(snap);
  const silence = silent(snap);
  const engines = checkEngines(snap);
  const areas = AREAS.map((area) => areaOf(area, snap, checks, engines));
  return {
    now,
    homelab: homelab(snap, checks, areas, silence, engines, now),
    problems: checks.filter((c) => c.problem),
    checks,
    areas,
    today: today(snap),
    events: events(snap, checks, silence, now),
    week: week(snap, checks, areas, now),
    apps: launch(snap.apps, checks),
    tools: snap.tools.map((link) => linked(link, checks)),
    chores: { ...choresBy(snap.chores, now), updates: snap.updates },
    checkEngines: engines,
    version: snap.version,
  };
}

/**
 * With no check engine to read, nothing says what's wrong, so the page
 * names the check engines Overlook reads, each linking its section of the
 * README of the Overlook that runs, as the setup page does.
 */
function checkEngines(snap) {
  if (snap.sources.some((s) => s.kind === "checkengine")) return [];
  return CHECK_ENGINES.map((adapter) => {
    const key = `setup.adapter.${adapter}`;
    return { key, url: `${readme(snap.version)}#${anchor(english(key))}` };
  });
}

// The snapshot's checks, with their times as dates.
function readChecks(snap) {
  return snap.checks.map((c) => {
    const recent = c.results.map((r) => ({ ...r, at: new Date(r.at) }));
    const check = {
      ...c,
      recent,
      failing: c.run > 0,
      ms: recent.length ? recent[recent.length - 1].ms : null,
      since: c.since ? new Date(c.since) : null,
      watched: c.watched ? new Date(c.watched) : null,
      outages: (c.outages || []).map((o) => ({ check: c, from: new Date(o.from), to: o.to ? new Date(o.to) : null })),
    };
    check.health = health(check);
    check.lately = lately(recent);
    return check;
  });
}

/**
 * What a check's state doesn't say by itself: while it's failing, how many
 * times in a row, against how many make a problem where its check engine
 * says; with no result yet, that it has none. Up or down, its state's word
 * says it all, and there's nothing more.
 */
function health(c) {
  if (c.state === "failing") {
    return c.threshold
      ? { key: "check.failed_n", args: { run: c.run, threshold: c.threshold } }
      : { key: "check.failed_run", args: { n: c.run } };
  }
  return c.state === "unknown" ? { key: "check.no_results" } : null;
}

/**
 * What a strip says in words, for a screen reader and the keyboard, since
 * its marks say it by shape and colour: how many of a check's results
 * passed, or of an area's ticks all its checks did, and, when one failed,
 * when the last did, the newest or one before it, and in an area's, which
 * checks. With no result, nothing: its state says so.
 */
function lately(marks, area = false) {
  const had = marks.filter(Boolean).slice(-RECENT);
  if (!had.length) return null;
  const n = had.length;
  const last = had.findLast((m) => !m.ok);
  if (!last) return area ? { key: "results.area_all", args: { n } } : { key: "results.all", args: { n } };
  const args = { n, passed: had.filter((m) => m.ok).length, at: last.at };
  const newest = last === had[n - 1];
  if (!area) return newest ? { key: "results.newest_failed", args } : { key: "results.last_failed", args };
  args.names = last.failed.map((f) => f.check.name);
  return newest ? { key: "results.area_newest_failed", args } : { key: "results.area_last_failed", args };
}

/**
 * The snapshot with each part named by its source, as by, where another
 * source of the same adapter is read, so that two of one adapter can be told
 * apart. One that didn't answer still counts, so the names don't come and go.
 * A check is named where another check engine has a check of its name, a
 * section where another source gives one of its title, such as the hosts of
 * Proxmox and of Beszel, and a chore where another source gives chores, such
 * as Seerr's requests beside a to-do list's, so that each says where it waits.
 */
function named(snap) {
  const twins = new Set(
    snap.sources
      .filter((s) => s.adapter && snap.sources.some((o) => o !== s && o.adapter === s.adapter))
      .map((s) => s.name),
  );
  const mark = (parts) => parts.map((p) => (twins.has(p.source) ? { ...p, by: p.source } : p));
  // A check is named only where another engine has one of its name.
  const shared = (c) => snap.checks.some((o) => o.name === c.name && o.source !== c.source);
  const titled = (s) => snap.sections.some((o) => o.title.key === s.title.key && o.source !== s.source);
  const listed = (c) => snap.chores.some((o) => o.source !== c.source);
  return {
    ...snap,
    checks: snap.checks.map((c) => (shared(c) ? { ...c, by: c.source } : c)),
    sections: mark(snap.sections).map((s) => (titled(s) ? { ...s, by: s.source } : s)),
    charts: mark(snap.charts),
    figures: mark(snap.figures),
    events: mark(snap.events),
    chores: mark(snap.chores).map((c) => (listed(c) ? { ...c, by: c.source } : c)),
  };
}

// ── The homelab and its areas ────────────────────────────────────────────

/**
 * The homelab's state, as the snapshot carries it, and its headline and
 * summary, which say what the state means. The summary is parts, each texts
 * said one after the other. With no check engine, it first names those
 * Overlook reads, as readable.
 */
function homelab(snap, checks, areas, silence, readable, now) {
  const { state } = snap.homelab;
  const problems = checks.filter((c) => c.problem);
  const judged = areas.filter((a) => a.judged);
  const silentText = { key: "source.no_answer", args: { names: silence } };
  const silentPart = silence.length ? [[silentText]] : [];
  // Down with no checks can only mean that no check engine answered, since
  // only a check engine gives checks: nothing says what works, and the first
  // one's error says why.
  if (state === "down" && !checks.length) {
    const { error } = snap.sources.find((s) => s.kind === "checkengine");
    return { state, headline: silentText, summary: [[{ key: "source.no_answer_why", args: { error } }]] };
  }
  if (state === "unknown") {
    const how = readable.length ? [[{ key: "status.no_engine", args: { engines: readable } }]] : [];
    return { state, headline: { key: "status.no_checks" }, summary: [...how, ...silentPart] };
  }
  if (state === "up") {
    const quiet = quietSince(checks);
    return {
      state,
      headline: { key: "status.up" },
      summary: [
        ...(quiet.since
          ? [
              [
                quiet.problemFree
                  ? { key: "status.quiet_since", args: { at: quiet.since } }
                  : { key: "status.quiet_for", args: { ms: now - quiet.since } },
              ],
            ]
          : []),
        [
          { key: "status.checks", args: { n: checks.length } },
          { key: "status.areas", args: { n: judged.length } },
        ],
        ...silentPart,
      ],
    };
  }
  // Areas in one state are named together, all of them as every area, and
  // the checks in no area by their names.
  const states = byState(judged).map((g) =>
    g.areas.length === judged.length
      ? { key: `status.every_${g.state}` }
      : { key: `status.areas_${g.state}`, args: { areas: g.areas } },
  );
  const elsewhere = problems.filter((c) => !c.area).map((c) => c.name);
  if (elsewhere.length) states.push({ key: "status.checks_down", args: { names: elsewhere } });
  return {
    state,
    headline: { key: "status.problems", args: { n: problems.length } },
    summary: [...states.map((s) => [s]), ...silentPart],
  };
}

/**
 * The areas not up, grouped by their state, the worst first, each group's
 * areas in the page's order.
 */
function byState(areas) {
  return ["down", "degraded"]
    .map((state) => ({ state, areas: areas.filter((a) => a.state === state).map((a) => a.area) }))
    .filter((g) => g.areas.length);
}

/**
 * When the last problem ended, or, with none remembered, since when the
 * check engine has watched.
 */
function quietSince(checks) {
  let end = null;
  let start = null;
  for (const c of checks) {
    for (const o of c.outages) if (o.to && (!end || o.to > end)) end = o.to;
    if (c.watched && (!start || c.watched < start)) start = c.watched;
  }
  return { since: end || start, problemFree: !end };
}

/**
 * An area: whether it's shown, which it is with anything in it, and whether
 * its checks judge it, which they do when it's shown with any; its state, as
 * the snapshot carries it; its checks and problems; how many of its checks
 * are up, and how many are in each state, those up first, so that a failing
 * check is never counted as up; its summary; the middle response time of
 * those answering; its strip, and what it says in words; its charts, those
 * of several sources added up;
 * its sections; and the sources the snapshot says it names that didn't
 * answer.
 */
function areaOf(area, snap, checks, engines) {
  const decided = snap.areas.find((a) => a.area === area);
  const mine = checks.filter((c) => c.area === area);
  const problems = mine.filter((c) => c.problem);
  const times = mine
    .filter((c) => !c.failing && c.ms !== null)
    .map((c) => c.ms)
    .sort((x, y) => x - y);
  const shown = [snap.checks, snap.sections, snap.charts, snap.figures, snap.events].some((l) =>
    l.some((x) => x.area === area),
  );
  const counts = CHECK_STATES.map((state) => ({ state, n: mine.filter((c) => c.state === state).length })).filter(
    (c) => c.n,
  );
  const up = mine.filter((c) => c.state === "up").length;
  const silence = silent(snap, decided.sources);
  const strip = worst(mine);
  return {
    area,
    shown,
    judged: shown && mine.length > 0,
    state: decided.state,
    checks: mine,
    problems,
    up,
    counts,
    summary: areaSummary(shown, mine, problems, up, engines, silence),
    ms: times.length ? times[times.length >> 1] : null,
    strip,
    lately: lately(strip, true),
    charts: joinCharts(snap.charts.filter((c) => c.area === area)).map((c) => ({
      ...c,
      lacking: missing(
        snap,
        c.sources.map((s) => s.name),
      ),
    })),
    sections: snap.sections.filter((s) => s.area === area),
    silent: silence,
  };
}

/**
 * What an area's own page says of it after its state, as parts, each texts
 * said one after the other, as the homelab's summary is: with no checks, that
 * it has none, or nothing at all; with problems, their checks down; else how
 * many of its checks are up, of how many where some aren't. Then, with no
 * check engine, those Overlook reads, and the sources that didn't answer.
 */
function areaSummary(shown, mine, problems, up, engines, silence) {
  const said = !mine.length
    ? { key: shown ? "area.no_checks" : "area.empty" }
    : problems.length
      ? { key: "status.checks_down", args: { names: problems.map((c) => c.name) } }
      : up === mine.length
        ? { key: "area.checks_up", args: { n: up } }
        : { key: "area.checks_up_of", args: { up, n: mine.length } };
  return [
    [said],
    ...(engines.length ? [[{ key: "status.no_engine", args: { engines } }]] : []),
    ...(silence.length ? [[{ key: "source.no_answer", args: { names: silence } }]] : []),
  ];
}

/**
 * An area's recent past as RECENT ticks, each as long as its most frequent
 * check takes between results and the last ending with its newest result,
 * so that checks run at different paces line up by time. In each tick, a
 * check is the worst result it had in it or, with none, the one in force
 * from before, as its state is its last result: a check failing at a slower
 * pace fails all along, not every so often. A tick says whether all passed,
 * the share of the checks that failed, when it ends, how many checks there
 * were, and those that failed. A tick before any result is empty.
 */
function worst(checks) {
  const ticks = Array.from({ length: RECENT }, () => new Map());
  const all = checks.flatMap((c) => c.recent);
  if (!all.length) return ticks.map(() => null);
  const end = Math.max(...all.map((r) => +r.at));
  // With no check's pace known, every result is in the last tick.
  const step = Math.min(...checks.map((c) => pace(c.recent)));
  const tickOf = (r) => RECENT - 1 - Math.floor((end - r.at) / step);
  for (const c of checks) {
    let held = null;
    let i = 0;
    for (let k = 0; k < RECENT; k++) {
      let worst = null;
      for (; i < c.recent.length && tickOf(c.recent[i]) <= k; i++) {
        const r = c.recent[i];
        if (tickOf(r) === k && (!worst || (worst.ok && !r.ok))) worst = r;
        held = r;
      }
      if (worst || held) ticks[k].set(c, worst || held);
    }
  }
  return ticks.map((tick, k) => {
    if (!tick.size) return null;
    const at = [...tick];
    const failed = at.filter(([, r]) => !r.ok).map(([check, r]) => ({ check, why: r.why }));
    const ends = new Date(k === RECENT - 1 ? end : end - (RECENT - 1 - k) * step);
    return { ok: !failed.length, bad: failed.length / at.length, at: ends, n: at.length, failed };
  });
}

// How long a check takes between results: the middle of its gaps, so that a
// gap while its engine was away doesn't stretch it. With fewer than two
// results, it isn't known.
function pace(recent) {
  const gaps = recent
    .slice(1)
    .map((r, i) => r.at - recent[i].at)
    .filter((gap) => gap > 0)
    .sort((a, b) => a - b);
  return gaps.length ? gaps[gaps.length >> 1] : Number.POSITIVE_INFINITY;
}

/** A check's states (CONTEXT.md, Check), in the order an area counts them. */
const CHECK_STATES = ["up", "failing", "down", "unknown"];

/**
 * The names of the sources that didn't answer: of those named, as an area
 * names its own, or else of them all.
 */
const silent = (snap, names) =>
  snap.sources.filter((s) => !s.answered && (!names || names.includes(s.name))).map((s) => s.name);

/**
 * The sources of the same kind as those a part adds up that answered but gave
 * none of it, such as a Pi-hole that keeps its counts by the day beside
 * AdGuard Homes (ADR 0012). A part of sources with no kind, such as hosts'
 * guests, lacks none: each of its adapter's sources gives it.
 */
function missing(snap, names) {
  const kind = snap.sources.find((s) => s.name === names[0])?.kind;
  return snap.sources
    .filter((s) => kind && s.answered && s.kind === kind && !names.includes(s.name))
    .map((s) => s.name);
}

// ── Today: the figures, added up and joined into tiles ───────────────────

/**
 * Today's tiles and how many columns they take. Each tile is a figure, added
 * up and joined, and says whether it's wide, which colour its parts start
 * from, and the sources of its kind that gave none of it.
 */
function today(snap) {
  // Each tile whose parts take the colours starts two on from the one
  // before, so tiles side by side differ, and the page stays colourful.
  let coloured = 0;
  const tiles = joinFigures(together(snap.figures)).map((f) => ({
    ...f,
    // A tile three or more figures share, such as two Radarrs' films and
    // two Sonarrs' shows, is two columns wide.
    wide: f.also?.length > 1,
    start: f.shares?.some((s) => !s.tone) ? 2 * coloured++ : 0,
    lacking: f.sources ? missing(snap, f.sources) : [],
  }));
  // As many columns as make the rows even, a wide tile counting two.
  const slots = tiles.reduce((n, f) => n + (f.wide ? 2 : 1), 0);
  return { tiles, columns: evenColumns(slots, 6) };
}

/**
 * The figures, those that add up and share a label key made one, such as two
 * resolvers' lookups: their values, their hours and the parts within them
 * hour by hour, over the hours they all keep, their meters, and the numbers
 * in their units and details. The one they make is split by source, and is
 * no longer any one source's.
 */
function together(figures) {
  const byKey = new Map();
  const bySource = new Map();
  const out = [];
  for (const f of figures) {
    const into = f.adds && byKey.get(f.label.key);
    if (!into) {
      const copy = { ...f, hours: [...(f.hours || [])] };
      if (f.within) copy.within = { ...f.within, hours: [...f.within.hours] };
      if (f.unit) copy.unit = { ...f.unit, args: { ...f.unit.args } };
      if (f.detail) copy.detail = { ...f.detail, args: { ...f.detail.args } };
      if (f.meters) copy.meters = f.meters.map((m) => ({ ...m }));
      if (f.adds) {
        byKey.set(f.label.key, copy);
        bySource.set(copy, [{ label: { name: f.source }, value: f.value }]);
        copy.sources = [f.source];
      }
      out.push(copy);
      continue;
    }
    into.value += f.value;
    into.hours = addHours(into.hours, f.hours || []);
    // The part within the hours adds up only where both have it, over the
    // same hours.
    const within = into.within && f.within && addHours(into.within.hours, f.within.hours);
    into.within = within?.length && within.length === into.hours.length ? { ...into.within, hours: within } : undefined;
    for (const field of ["unit", "detail"]) {
      for (const [name, v] of Object.entries(f[field]?.args || {})) {
        if (typeof v === "number" && typeof into[field]?.args[name] === "number") into[field].args[name] += v;
      }
    }
    // Meters add up only where both have the same ones, such as two hosts'
    // processors and memory.
    into.meters =
      into.meters && into.meters.length === f.meters?.length
        ? into.meters.map((m, i) => ({ ...m, used: m.used + f.meters[i].used, total: m.total + f.meters[i].total }))
        : undefined;
    into.tone ||= f.tone;
    into.sources.push(f.source);
    into.shares = bySource.get(into);
    into.shares.push({ label: { name: f.source }, value: f.value });
    into.by = undefined;
  }
  return out;
}

/**
 * The figures, those that share a join key made one tile when there's more
 * than one, such as films and shows in one library: under the join's words,
 * the first one's number and unit, the others in also, and each one's part
 * as a share of the bar. One alone stays as it is.
 */
function joinFigures(figures) {
  const groups = new Map();
  for (const f of figures) {
    if (f.join) groups.set(f.join.key, [...(groups.get(f.join.key) || []), f]);
  }
  return figures.flatMap((f) => {
    const group = f.join ? groups.get(f.join.key) : [f];
    if (group.length < 2) return [f];
    if (group[0] !== f) return [];
    const { detail, ...first } = f;
    // Each part keeps its figure's place in the tile, and so its colour.
    const shares = group.flatMap((g, slot) => (g.part ? [{ ...g.part, by: g.by, slot }] : []));
    return [{ ...first, label: f.join, also: group.slice(1), shares }];
  });
}

/**
 * Two sources' hours added up hour by hour over the hours both keep, the
 * latest last: a week and a day make a day (CONTEXT.md, Chart and Figure).
 */
function addHours(a, b) {
  const n = Math.min(a.length, b.length);
  return a.slice(a.length - n).map((v, i) => v + b[b.length - n + i]);
}

/**
 * How many columns n tiles take so their rows come out even, at most most
 * to a row: 8 tiles in rows of 4, not 6 and 2.
 */
function evenColumns(n, most) {
  return n ? Math.ceil(n / Math.ceil(n / most)) : 1;
}

/**
 * The charts, those that add up and share a title key made one, such as two
 * resolvers' lookups: each series hour by hour, over the hours they all
 * have, the latest last. Each keeps its sources, each with its own whole, for
 * the tooltip.
 */
function joinCharts(charts) {
  const byKey = new Map();
  const out = [];
  for (const c of charts) {
    const joins = c.adds || c.measures;
    const into = joins && byKey.get(c.title.key);
    if (!into) {
      const copy = { ...c, series: c.series.map((s) => ({ ...s, hours: [...s.hours] })) };
      copy.sources = [{ name: c.source, hours: copy.series[0].hours }];
      if (joins) byKey.set(c.title.key, copy);
      out.push(copy);
      continue;
    }
    // Levels, such as several Proxmox's hosts' processors, are lines side by
    // side over the hours they all have, never added up.
    if (c.measures) {
      const n = Math.min(into.series[0].hours.length, c.series[0].hours.length);
      into.series = [...into.series, ...c.series].map((s) => ({ ...s, hours: s.hours.slice(s.hours.length - n) }));
      into.sources.push({ name: c.source, hours: [] });
      into.by = undefined;
      continue;
    }
    const n = Math.min(into.series[0].hours.length, c.series[0].hours.length);
    const last = (hours) => hours.slice(hours.length - n);
    into.series = into.series.map((s, k) => ({
      ...s,
      hours: addHours(last(s.hours), c.series[k]?.hours ?? Array(n).fill(0)),
    }));
    into.sources = [...into.sources, { name: c.source, hours: c.series[0].hours }].map((s) => ({
      ...s,
      hours: last(s.hours),
    }));
    into.by = undefined;
  }
  return out;
}

// ── The week: what happened, newest first, and each area along it ────────

/**
 * This week's events, newest first, at most eight: the checks that went down
 * and came back, what the sources remember, and the updates waiting. Each
 * has its moment, its mark (a state, or an icon in a tone), its text, and
 * what it's about. With none, it says so, or which sources didn't answer,
 * since the week may not have been quiet.
 */
function events(snap, checks, silence, now) {
  const weekAgo = now - 7 * DAY;
  const where = (area) => (area ? [{ key: `area.${area}` }] : []);
  const items = [];
  for (const c of checks) {
    for (const o of c.outages) {
      if (o.from >= weekAgo) {
        items.push({
          at: o.from,
          mark: { state: "down" },
          text: { key: "events.down", args: { name: c.name } },
          about: where(c.area),
        });
      }
      if (o.to && o.to >= weekAgo) {
        items.push({
          at: o.to,
          mark: { state: "up" },
          text: { key: "events.back", args: { name: c.name, ms: o.to - o.from } },
          about: where(c.area),
        });
      }
    }
  }
  for (const e of snap.events) {
    const at = new Date(e.at);
    if (at >= weekAgo) {
      items.push({
        at,
        mark: { icon: e.icon, tone: e.tone || "blue" },
        text: e.text,
        about: [...where(e.area), ...(e.by ? [{ name: e.by }] : [])],
      });
    }
  }
  for (const u of snap.updates) {
    items.push({
      at: new Date(u.at),
      mark: { icon: "pr", tone: "green" },
      text: { key: "events.update", args: { name: u.name } },
      about: [{ key: "events.update_meta" }],
    });
  }
  items.sort((a, b) => b.at - a.at);
  return {
    items: items.slice(0, 8),
    empty: silence.length ? { key: "source.no_answer", args: { names: silence } } : { key: "events.quiet" },
  };
}

/**
 * The last seven days, from midnight by the calendar six days ago to now,
 * and each shown area along them: its outages, cut to the week, with how
 * long each lasted; how long it was down in all; and its events, those in
 * the same hour together, such as five hosts' backups the same night.
 */
function week(snap, checks, areas, now) {
  // Midnights by the calendar, not 24 h apart: a day the clocks change lasts 23 or 25.
  const days = Array.from({ length: 7 }, (_, i) => {
    const d = new Date(now);
    d.setHours(0, 0, 0, 0);
    d.setDate(d.getDate() - 6 + i);
    return d;
  });
  const start = +days[0];
  const end = +now;
  const x = (at) => (Math.max(+at, start) - start) / (end - start);
  const rows = areas
    .filter((a) => a.shown)
    .map(({ area }) => {
      const mine = checks
        .filter((c) => c.area === area)
        .flatMap((c) => c.outages)
        .filter((o) => !o.to || o.to >= start);
      const happened = snap.events.filter((e) => e.area === area && new Date(e.at) >= start);
      const hours = Map.groupBy(happened, (e) => Math.floor(+new Date(e.at) / 3600e3));
      return {
        area,
        down: mine.reduce((sum, o) => sum + ((o.to || end) - Math.max(+o.from, start)), 0),
        outages: mine.map((o) => ({
          ...o,
          ms: (o.to || end) - o.from,
          left: x(o.from),
          width: x(o.to || end) - x(o.from),
        })),
        events: [...hours.values()].map((group) => ({
          events: group,
          at: new Date(group[0].at),
          left: x(new Date(group[0].at)),
          bad: group.some((e) => e.tone === "red"),
        })),
      };
    });
  return { days: days.map((at) => ({ at, left: x(at) })), rows };
}

// ── The apps, the tools and the chores ───────────────────────────────────

/**
 * The apps: each link with its check. Past twelve, the tiles go small, and
 * their rows come out even either way.
 */
function launch(apps, checks) {
  const compact = apps.length > 12;
  return {
    links: apps.map((link) => linked(link, checks)),
    compact,
    columns: evenColumns(apps.length, compact ? 4 : 3),
  };
}

/**
 * A link with the check that says whether it answers, in the check engine the
 * snapshot names for it, its state, as the snapshot carries it, and the foot
 * of its tile: with no check named, that it has none; while its check is
 * missing or failing, its state's word; and while it answers, nothing, as the
 * page then says how fast it answered.
 */
function linked(link, checks) {
  const check = link.check ? checks.find((c) => c.name === link.check && c.source === link.engine) : undefined;
  const { state } = link;
  const foot = !link.check ? { key: "apps.no_check" } : !check || check.failing ? { key: `state.${state}` } : null;
  return { link, check, state, foot };
}

/**
 * The chores by the viewer's today, each with its day as a date: those due,
 * from today or before, or with no day; those from the next six days; and
 * the first after them, the next.
 */
function choresBy(chores, now) {
  const today = new Date(now);
  today.setHours(0, 0, 0, 0);
  const week = new Date(today);
  week.setDate(week.getDate() + 7);
  // A day with no time is midnight where the page is viewed, and a chore
  // with no day is due today.
  const dated = chores
    .map((c) => ({ ...c, from: c.from ? new Date(`${c.from}T00:00`) : today }))
    .sort((a, b) => a.from - b.from);
  return {
    due: dated.filter((c) => c.from <= today),
    week: dated.filter((c) => c.from > today && c.from < week),
    next: dated.find((c) => c.from >= week) || null,
  };
}

// ── The demo ─────────────────────────────────────────────────────────────

// A time as the snapshot writes it, in RFC 3339, and a day, as a chore's.
const TIME = /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?(Z|[+-]\d\d:\d\d)$/;
const DATE = /^\d{4}-\d\d-\d\d$/;

/**
 * A snapshot as if it had just been read: every time in it, those in its
 * texts' values too, moved on by as long as has passed since it was read,
 * and every day by as many whole days. The demo's invented snapshot is drawn
 * this way, so it never looks old.
 */
export function asIfNow(snap, now) {
  const by = now - Date.parse(snap.at);
  const days = Math.round(by / DAY);
  return JSON.parse(JSON.stringify(snap), (_, v) => {
    if (typeof v !== "string") return v;
    if (TIME.test(v)) return new Date(Date.parse(v) + by).toISOString();
    if (DATE.test(v)) return new Date(Date.parse(v) + days * DAY).toISOString().slice(0, 10);
    return v;
  });
}
