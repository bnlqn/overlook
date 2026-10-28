// What the page asks of the snapshot's checks: each area's state, the
// problems, and how long the homelab has been quiet.

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
