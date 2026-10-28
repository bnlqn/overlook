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
 * The figures, those that add up and share a label key made one, such as two
 * resolvers' lookups: their values, their hours and the parts within them
 * hour by hour, and the numbers in their details. Hours that only some of
 * them have are left out.
 */
export function together(figures) {
  const byKey = new Map();
  const out = [];
  for (const f of figures) {
    const into = f.adds && byKey.get(f.label.key);
    if (!into) {
      const copy = { ...f, hours: [...(f.hours || [])] };
      if (f.within) copy.within = { ...f.within, hours: [...f.within.hours] };
      if (f.detail) copy.detail = { ...f.detail, args: { ...f.detail.args } };
      if (f.adds) byKey.set(f.label.key, copy);
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
    for (const [name, v] of Object.entries(f.detail?.args || {})) {
      if (typeof v === "number" && typeof into.detail?.args[name] === "number") into.detail.args[name] += v;
    }
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
    return [{ ...first, label: f.join, also: group.slice(1), shares: group.map((g) => g.part).filter(Boolean) }];
  });
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
    const into = c.adds && byKey.get(c.title.key);
    if (!into) {
      const copy = { ...c, series: c.series.map((s) => ({ ...s, hours: [...s.hours] })) };
      copy.sources = [{ name: c.source, hours: copy.series[0].hours }];
      if (c.adds) byKey.set(c.title.key, copy);
      out.push(copy);
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
 * from today or before; those from the next six days; and the first after
 * them, the next.
 */
export function choresBy(chores, now = new Date()) {
  const today = new Date(now);
  today.setHours(0, 0, 0, 0);
  const week = new Date(today);
  week.setDate(week.getDate() + 7);
  // A day with no time is midnight where the page is viewed.
  const dated = chores.map((c) => ({ ...c, from: new Date(`${c.from}T00:00`) })).sort((a, b) => a.from - b.from);
  return {
    due: dated.filter((c) => c.from <= today),
    week: dated.filter((c) => c.from > today && c.from < week),
    next: dated.find((c) => c.from >= week) || null,
  };
}

/** The names of the sources that didn't answer. */
export const silent = (snap) => snap.sources.filter((s) => !s.answered).map((s) => s.name);

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

/** A check's state: down when a problem, degraded while failing, else up. */
export const checkState = (c) => (c.problem ? "down" : c.failing ? "degraded" : "up");
