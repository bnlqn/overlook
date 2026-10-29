// What the page says: the snapshot read at a moment, made into what the
// overview, each area's page, the sidebar and ⌘K show. Every decision the page
// makes is here, where Node tests it (ADR 0007); overview.js, area.js and
// shell.js only write what page() returns.

export const AREAS = ["servers", "network", "storage", "apps"];

/** How many results a strip shows: as many as a check engine gives (snapshot.Recent). */
export const RECENT = 30;

const DAY = 864e5;

/**
 * What the page says of the snapshot read, at now:
 *
 * - homelab: its state, its headline and its summary, as texts;
 * - problems: the checks that are problems;
 * - checks: every check, each with its state;
 * - areas: the four, in the page's order, each with whether it's shown, its
 *   state, its checks and problems, its middle response time, its strip, its
 *   charts, its sections and its silent sources;
 * - today: its tiles and how many columns they take;
 * - events: this week's, newest first, and what to say with none;
 * - week: its seven days and each area's outages and events along them;
 * - apps: each link with its check and the check's state, whether the tiles
 *   go small, and how many columns they take; tools: each link with its
 *   check and the check's state;
 * - chores: those due, this week's and the next, and the updates.
 *
 * Texts are the snapshot's own: a message's key and its values, or a name.
 * Moments are dates; where the page draws along a span, as along the week,
 * a place on it is a fraction of it.
 */
export function page(read, now) {
  const snap = named(read);
  const checks = readChecks(snap);
  const silence = silent(snap);
  const areas = AREAS.map((area) => areaOf(area, snap, checks));
  return {
    now,
    homelab: homelab(snap, checks, areas, silence, now),
    problems: checks.filter((c) => c.problem),
    checks,
    areas,
    today: today(snap),
    events: events(snap, checks, silence, now),
    week: week(snap, checks, areas, now),
    apps: launch(snap.apps, checks),
    tools: snap.tools.map((link) => linked(link, checks)),
    chores: { ...choresBy(snap.chores, now), updates: snap.updates },
  };
}

// The snapshot's checks, with their times as dates and their states.
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
    check.state = checkState(check);
    return check;
  });
}

/**
 * The snapshot with each part named by its source, as by, where another
 * source of the same adapter is read, so that two of one kind can be told
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
 * The homelab's state, headline and summary. The summary is parts, each
 * texts said one after the other.
 */
function homelab(snap, checks, areas, silence, now) {
  const problems = checks.filter((c) => c.problem);
  const judged = areas.filter((a) => a.shown && a.checks.length);
  // ponytail: blind is no check at all with any source silent, so a homelab
  // with no check engine and a silent resolver is down. Once a source says
  // its kind (candidate 3), blind is no check engine answering.
  const blind = !checks.length && silence.length > 0;
  const state = blind ? "down" : !problems.length ? "up" : judged.some((a) => a.state === "down") ? "down" : "degraded";
  const silentText = { key: "source.no_answer", args: { names: silence } };
  if (blind) {
    return {
      state,
      headline: silentText,
      summary: [[{ key: "source.no_answer_why", args: { error: snap.sources.find((s) => !s.answered).error } }]],
    };
  }
  // Areas in one state are named together, and all of them as every area.
  const states = byState(judged).map((g) =>
    g.areas.length === judged.length
      ? { key: `status.every_${g.state}` }
      : { key: `status.areas_${g.state}`, args: { areas: g.areas } },
  );
  const quiet = quietSince(checks);
  const parts = problems.length
    ? states.map((s) => [s])
    : [
        quiet.since && [
          quiet.problemFree
            ? { key: "status.quiet_since", args: { at: quiet.since } }
            : { key: "status.quiet_for", args: { ms: now - quiet.since } },
        ],
        [
          { key: "status.checks", args: { n: checks.length } },
          { key: "status.areas", args: { n: judged.length } },
        ],
      ];
  if (silence.length) parts.push([silentText]);
  return {
    state,
    headline: problems.length ? { key: "status.problems", args: { n: problems.length } } : { key: "status.up" },
    summary: parts.filter(Boolean),
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
 * An area: whether it's shown, which it is with anything in it; its state,
 * down when every check in it is a problem, degraded with any, up with none,
 * and unknown with no checks, since nothing says whether it works;
 * its checks and problems; the middle response time of those answering; its
 * strip; its charts, those of several sources added up; its sections; and
 * the sources that didn't answer, its own and those of every area.
 */
function areaOf(area, snap, checks) {
  const mine = checks.filter((c) => c.area === area);
  const problems = mine.filter((c) => c.problem);
  const times = mine
    .filter((c) => !c.failing && c.ms !== null)
    .map((c) => c.ms)
    .sort((x, y) => x - y);
  return {
    area,
    shown: [snap.checks, snap.sections, snap.charts, snap.figures, snap.events].some((l) =>
      l.some((x) => x.area === area),
    ),
    state: !mine.length ? "unknown" : !problems.length ? "up" : problems.length === mine.length ? "down" : "degraded",
    checks: mine,
    problems,
    ms: times.length ? times[times.length >> 1] : null,
    strip: worst(mine),
    charts: joinCharts(snap.charts.filter((c) => c.area === area)).map((c) => ({
      ...c,
      lacking: missing(
        snap,
        c.sources.map((s) => s.name),
      ),
    })),
    sections: snap.sections.filter((s) => s.area === area),
    silent: silent(snap, area),
  };
}

/**
 * Each moment's worst result across an area's checks, as RECENT ticks:
 * whether all passed, the share that failed, when it was, how many results
 * there were, and those that failed with their checks.
 */
function worst(checks) {
  return Array.from({ length: RECENT }, (_, k) => {
    const at = checks.map((c) => [c, c.recent[c.recent.length - RECENT + k]]).filter(([, r]) => r);
    if (!at.length) return null;
    const failed = at.filter(([, r]) => !r.ok).map(([check, r]) => ({ check, why: r.why }));
    return { ok: !failed.length, bad: failed.length / at.length, at: at[0][1].at, n: at.length, failed };
  });
}

/**
 * A check's state: down when a problem, degraded while failing, up when its
 * last result passed, and unknown with no result yet.
 */
const checkState = (c) => (c.problem ? "down" : c.failing ? "degraded" : c.recent.length ? "up" : "unknown");

/**
 * The names of the sources that didn't answer: on an area's page, those of
 * its area and those of every area, such as a check engine.
 */
const silent = (snap, area) =>
  snap.sources.filter((s) => !s.answered && (!area || !s.area || s.area === area)).map((s) => s.name);

/**
 * The sources of the same adapter as those a part adds up that answered but
 * gave none of it, such as a resolver that keeps its counts by the day.
 */
function missing(snap, names) {
  // ponytail: asks only the first source's adapter, so a Pi-hole that gave
  // none beside AdGuard Homes is never named. Once a source says its kind
  // (candidate 3), it asks for those of the same kind.
  const adapter = snap.sources.find((s) => s.name === names[0])?.adapter;
  return snap.sources.filter((s) => s.answered && s.adapter === adapter && !names.includes(s.name)).map((s) => s.name);
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
 * hour by hour, their meters, and the numbers in their units and details.
 * Hours that only some of them have are left out. The one they make is split
 * by source, and is no longer any one source's.
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
    const hours = f.hours || [];
    into.value += f.value;
    into.hours = into.hours.length === hours.length ? into.hours.map((v, i) => v + hours[i]) : [];
    // The part within the hours adds up only where both have it, hour for hour.
    const part = f.within?.hours || [];
    into.within =
      into.within && into.hours.length && into.within.hours.length === part.length
        ? { ...into.within, hours: into.within.hours.map((v, i) => v + part[i]) }
        : undefined;
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
    into.series = into.series.map((s, k) => {
      const more = last(c.series[k]?.hours || []);
      return { ...s, hours: last(s.hours).map((v, i) => v + (more[i] || 0)) };
    });
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

/** A link with the check that says whether it answers, and that check's state. */
function linked(link, checks) {
  const check = link.check ? linkCheck(link, checks) : undefined;
  return { link, check, state: check ? check.state : "unknown" };
}

/**
 * The check that says whether a link answers: the one of its name in the
 * check engine the config names, or, with none named, the worst of those of
 * its name, so a check in two engines never hides a problem.
 */
function linkCheck(link, checks) {
  const rank = { down: 3, degraded: 2, unknown: 1, up: 0 };
  return checks
    .filter((c) => c.name === link.check && (!link.engine || c.source === link.engine))
    .sort((a, b) => rank[b.state] - rank[a.state])[0];
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
