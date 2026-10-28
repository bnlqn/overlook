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
  AREAS.filter((a) => [snap.checks, snap.sections, snap.figures, snap.events].some((l) => l.some((x) => x.area === a)));

/**
 * The figures, those that share a label key added up into one, such as two
 * resolvers' lookups: their values, their hours hour by hour, and the numbers
 * in their details. Hours that only some of them have are left out.
 */
export function together(figures) {
  const byKey = new Map();
  const out = [];
  for (const f of figures) {
    const into = f.label.key && byKey.get(f.label.key);
    if (!into) {
      const copy = { ...f, hours: [...(f.hours || [])] };
      if (f.detail) copy.detail = { ...f.detail, args: { ...f.detail.args } };
      if (f.label.key) byKey.set(f.label.key, copy);
      out.push(copy);
      continue;
    }
    const hours = f.hours || [];
    into.value += f.value;
    into.hours = into.hours.length === hours.length ? into.hours.map((v, i) => v + hours[i]) : [];
    for (const [name, v] of Object.entries(f.detail?.args || {})) {
      if (typeof v === "number" && typeof into.detail?.args[name] === "number") into.detail.args[name] += v;
    }
  }
  return out;
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
