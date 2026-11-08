// The config the setup page hands back (ADR 0014): overlook.yaml, each key
// in it a ${VAR}, and the .env that holds the keys, put together here so
// that the server never sends a key back.

import { english, t } from "./text.js";

/** Where Overlook comes from: its repository, whose README says the rest. */
export const REPO = "https://github.com/bnlqn/overlook";

/**
 * The README of the Overlook that runs: a release's, at its tag, or the
 * repository's own for any other build, such as one from a commit.
 */
export const readme = (version) =>
  /^\d+\.\d+\.\d+(-rc\.\d+)?$/.test(version ?? "") ? `${REPO}/blob/v${version}/README.md` : REPO;

/** A README heading's anchor, as GitHub makes it: "Plex, through Tautulli" is plex-through-tautulli. */
export const anchor = (heading) =>
  heading
    .toLowerCase()
    .replace(/[^\w\- ]/g, "")
    .replaceAll(" ", "-");

/**
 * A value as YAML: as typed, where nothing in it means something else to
 * YAML, and quoted otherwise, such as a number or a word YAML reads as true.
 */
export const scalar = (v) =>
  /^[\w$][\w$./:@!?=&%+~{}-]*$/.test(v) && !v.endsWith(":") && !/^(true|false|yes|no|on|off|null|[\d.+-]+)$/i.test(v)
    ? v
    : JSON.stringify(v);

/**
 * Each source's keys, by field, as the names .env gives them: its
 * adapter's and the field's, numbered from an adapter's second source on,
 * as PROXMOX_SECRET and PROXMOX_2_SECRET.
 */
function names(adapters, sources) {
  const seen = {};
  return sources.map((s) => {
    seen[s.adapter] = (seen[s.adapter] ?? 0) + 1;
    const n = seen[s.adapter];
    const asks = adapters.find((a) => a.key === s.adapter).asks;
    return Object.fromEntries(
      asks
        .filter((a) => a.key && s.fields[a.field] !== undefined)
        .map((a) => [a.field, [s.adapter, n > 1 ? n : "", a.field].filter(Boolean).join("_").toUpperCase()]),
    );
  });
}

/**
 * overlook.yaml for sources, each an adapter's key and its fields as typed,
 * in the order of adapters, each adapter with its section of the README at
 * base.
 */
export function yaml(adapters, sources, base) {
  const vars = names(adapters, sources);
  const lines = [`# ${t("setup.yaml.head")}`, "", "sources:"];
  for (const a of adapters) {
    const mine = sources.flatMap((s, i) => (s.adapter === a.key ? [[s, vars[i]]] : []));
    if (!mine.length) continue;
    lines.push(
      `  ${a.key}:`,
      `    # ${t(`setup.adapter.${a.key}`)}: ${base}#${anchor(english(`setup.adapter.${a.key}`))}`,
    );
    for (const [s, keys] of mine) {
      let first = true;
      for (const ask of a.asks) {
        const v = s.fields[ask.field];
        if (v === undefined) continue;
        const at = first ? "    - " : "      ";
        first = false;
        if (keys[ask.field]) lines.push(`${at}${ask.field}: \${${keys[ask.field]}}`);
        else if (ask.list) lines.push(`${at}${ask.field}:`, ...v.map((item) => `        - ${scalar(item)}`));
        else lines.push(`${at}${ask.field}: ${scalar(v)}`);
      }
    }
  }
  lines.push("", ...t("setup.yaml.links").split("\n"));
  return `${lines.join("\n")}\n`;
}

/** The .env for sources: each key as NAME=value, the value as typed. */
export function dotenv(adapters, sources) {
  const lines = [`# ${t("setup.env.head")}`];
  names(adapters, sources).forEach((keys, i) => {
    for (const [field, name] of Object.entries(keys)) lines.push(`${name}=${sources[i].fields[field]}`);
  });
  return `${lines.join("\n")}\n`;
}
