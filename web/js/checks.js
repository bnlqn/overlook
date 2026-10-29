// What the page asks of the snapshot: each area's state, the problems, how
// long the homelab has been quiet, and today's figures.

export const AREAS = ["servers", "network", "storage", "apps"];

/** The snapshot's checks, with their times as dates. */
export function readChecks(snap) {
  return snap.checks.map((c) => {
    const recent = c.results.map((r) => ({ ...r, at: new Date(r.at) }));
    return {
      ...c,
      recent,
      failing: c.run > 0,
      ms: recent.length ? recent[recent.length - 1].ms : null,
      since: c.since ? new Date(c.since) : null,
      watched: c.watched ? new Date(c.watched) : null,
      outages: (c.outages || []).map((o) => ({ check: c, from: new Date(o.from), to: o.to ? new Date(o.to) : null })),
    };
  });
}

/** The areas with anything in them, which are the ones shown. */
export const shown = (snap) =>
  AREAS.filter((a) =>
    [snap.checks, snap.sections, snap.charts, snap.figures, snap.events].some((l) => l.some((x) => x.area === a)),
  );

/**
 * The snapshot with each part named by its source, as by, where another
 * source of the same adapter is read, so that two of one kind can be told
 * apart. One that didn't answer still counts, so the names don't come and go.
 * A check is named where another check engine has a check of its name.
 */
export function named(snap) {
  const twins = new Set(
    snap.sources
      .filter((s) => s.adapter && snap.sources.some((o) => o !== s && o.adapter === s.adapter))
      .map((s) => s.name),
  );
  const mark = (parts) => parts.map((p) => (twins.has(p.source) ? { ...p, by: p.source } : p));
  // A check is named only where another engine has one of its name.
  const shared = (c) => snap.checks.some((o) => o.name === c.name && o.source !== c.source);
  return {
    ...snap,
    checks: snap.checks.map((c) => (shared(c) ? { ...c, by: c.source } : c)),
    sections: mark(snap.sections),
    charts: mark(snap.charts),
    figures: mark(snap.figures),
    events: mark(snap.events),
    chores: mark(snap.chores),
  };
}

/**
 * The figures, those that add up and share a label key made one, such as two
 * resolvers' lookups: their values, their hours and the parts within them
 * hour by hour, their meters, and the numbers in their units and details.
 * Hours that only some of them have are left out. The one they make is split
 * by source, and is no longer any one source's.
 */
export function together(figures) {
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
export function joinFigures(figures) {
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
 * The places of the series with the highest mean, at most most of them, in
 * their order: past most, the rest give way.
 */
export function busiest(series, most) {
  if (series.length <= most) return series.map((_, k) => k);
  const mean = (s) => s.hours.reduce((a, v) => a + v, 0) / (s.hours.length || 1);
  return series
    .map((s, k) => [mean(s), k])
    .sort((a, b) => b[0] - a[0])
    .slice(0, most - 1)
    .map(([, k]) => k)
    .sort((a, b) => a - b);
}

/**
 * Shares past most folded into one, the smallest first, so that each one
 * kept has a colour of its own. Shares with a tone mean something one by
 * one, such as each guest backed up or not, and are never folded.
 */
export function fold(shares, most = 8) {
  if (shares.length <= most || shares.some((s) => s.tone)) return shares;
  const kept = new Set([...shares].sort((a, b) => b.value - a.value).slice(0, most - 1));
  const rest = shares.filter((s) => !kept.has(s));
  const whole = shares.reduce((sum, s) => sum + s.value, 0);
  const others = {
    label: { key: "shares.others", args: { n: rest.length } },
    value: rest.reduce((sum, s) => sum + s.value, 0),
    others: true,
    rows: rest.map((s) => ({
      title: s.label,
      value: { key: "percent", args: { n: Math.round((100 * s.value) / whole) } },
    })),
  };
  return [...shares.filter((s) => kept.has(s)), others];
}

/**
 * The charts, those that add up and share a title key made one, such as two
 * resolvers' lookups: each series hour by hour, over the hours they all
 * have, the latest last. Each keeps its sources, each with its own whole, for
 * the tooltip.
 */
export function joinCharts(charts) {
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

// A time as the snapshot writes it, in RFC 3339, and a day, as a chore's.
const TIME = /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(\.\d+)?(Z|[+-]\d\d:\d\d)$/;
const DAY = /^\d{4}-\d\d-\d\d$/;

/**
 * A snapshot as if it had just been read: every time in it, those in its
 * texts' values too, moved on by as long as has passed since it was read,
 * and every day by as many whole days. The demo's invented snapshot is drawn
 * this way, so it never looks old.
 */
export function asIfNow(snap, now = Date.now()) {
  const by = now - Date.parse(snap.at);
  const days = Math.round(by / 864e5);
  return JSON.parse(JSON.stringify(snap), (_, v) => {
    if (typeof v !== "string") return v;
    if (TIME.test(v)) return new Date(Date.parse(v) + by).toISOString();
    if (DAY.test(v)) return new Date(Date.parse(v) + days * 864e5).toISOString().slice(0, 10);
    return v;
  });
}

/**
 * The chores by the viewer's today, each with its day as a date: those due,
 * from today or before, or with no day; those from the next six days; and
 * the first after them, the next.
 */
export function choresBy(chores, now = new Date()) {
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

/**
 * The names of the sources that didn't answer: on an area's page, those of
 * its area and those of every area, such as a check engine.
 */
export const silent = (snap, area) =>
  snap.sources.filter((s) => !s.answered && (!area || !s.area || s.area === area)).map((s) => s.name);

/**
 * The sources of the same adapter as those a part adds up that answered but
 * gave none of it, such as a resolver that keeps its counts by the day.
 */
export function missing(snap, names) {
  const adapter = snap.sources.find((s) => s.name === names[0])?.adapter;
  return snap.sources.filter((s) => s.answered && s.adapter === adapter && !names.includes(s.name)).map((s) => s.name);
}

/**
 * The areas not up, grouped by their state, the worst first, each group's
 * areas in the page's order.
 */
export function byState(areas) {
  return ["down", "degraded"]
    .map((state) => ({ state, areas: areas.filter((a) => a.state === state).map((a) => a.area) }))
    .filter((g) => g.areas.length);
}

/**
 * How many columns n tiles take so their rows come out even, at most most
 * to a row: 8 tiles in rows of 4, not 6 and 2.
 */
export function evenColumns(n, most) {
  return n ? Math.ceil(n / Math.ceil(n / most)) : 1;
}

/**
 * When the last problem ended, or, with none remembered, since when the
 * check engine has watched.
 */
export function quietSince(checks) {
  let end = null;
  let start = null;
  for (const c of checks) {
    for (const o of c.outages) if (o.to && (!end || o.to > end)) end = o.to;
    if (c.watched && (!start || c.watched < start)) start = c.watched;
  }
  return { since: end || start, problemFree: !end };
}

/**
 * An area's checks, its problems, and its state: down when every check in it
 * is a problem, degraded with any, up with none.
 */
export function areaState(area, checks) {
  const mine = checks.filter((c) => c.area === area);
  const problems = mine.filter((c) => c.problem);
  const state = !problems.length ? "up" : problems.length === mine.length ? "down" : "degraded";
  return { area, checks: mine, problems, state };
}

/**
 * A check's state: down when a problem, degraded while failing, up when its
 * last result passed, and unknown with no result yet.
 */
export const checkState = (c) => (c.problem ? "down" : c.failing ? "degraded" : c.recent.length ? "up" : "unknown");

/**
 * The check that says whether a link answers: the one of its name in the
 * check engine the config names, or, with none named, the worst of those of
 * its name, so a check in two engines never hides a problem.
 */
export function linkCheck(link, checks) {
  const rank = { down: 3, degraded: 2, unknown: 1, up: 0 };
  return checks
    .filter((c) => c.name === link.check && (!link.engine || c.source === link.engine))
    .sort((a, b) => rank[checkState(b)] - rank[checkState(a)])[0];
}
