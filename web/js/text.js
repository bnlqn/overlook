// The page's words (ADR 0006), and how it writes numbers, times, sizes and
// spans: English, under the viewer's language wherever a translation has it.
// And whether the letters typed into ⌘K match a name.

let words = { en: {}, mine: {} };
let locale = "en";
let numbers = new Intl.NumberFormat(locale);
// The moment the page is drawn for, which a message's {at:when} counts from.
let moment = Number.NaN;

/** Uses the English table, the viewer's own, and the viewer's locale. */
export function setWords(en, mine = {}, viewer = "en") {
  words = { en, mine };
  locale = viewer;
  numbers = new Intl.NumberFormat(locale);
}

/**
 * Sets the moment the page is drawn for, as start() reads it, which a
 * message's {at:when} counts from: a message can't be handed it, as ago()
 * and when() are.
 */
export function drawnAt(now) {
  moment = now;
}

/** Reads the English table and the one for the viewer's language. */
export async function loadWords(viewer = navigator.language || "en") {
  const get = (language) =>
    fetch(`i18n/${language}.json`).then(
      (r) => (r.ok ? r.json() : {}),
      () => ({}),
    );
  const base = viewer.split("-")[0].toLowerCase();
  const [en, mine] = await Promise.all([get("en"), base === "en" ? {} : get(base)]);
  setWords(en, mine, viewer);
  // The page says which language its words are in, so that a screen reader
  // reads them in it and Chinese and Japanese take their own glyphs.
  // ponytail: one file a language, so zh-TW reads zh.json's Simplified
  // Chinese and pt-PT pt.json's Brazilian Portuguese; a file per region,
  // tried before its language's, if a translation for one comes.
  document.documentElement.lang = Object.keys(mine).length ? base : "en";
}

/** Escapes text for HTML, attributes included. */
export const esc = (s) =>
  String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);

/**
 * A message as HTML: its values escaped, numbers and {name:format}s written
 * the viewer's way, and the plural form that `n` calls for in the language
 * the message came from.
 */
export function t(key, args = {}) {
  const own = Object.hasOwn(words.mine, key);
  const entry = own ? words.mine[key] : words.en[key];
  if (entry === undefined) return esc(key);
  const form =
    typeof entry === "string"
      ? entry
      : (entry[new Intl.PluralRules(own ? locale : "en").select(args.n)] ?? entry.other);
  return form.replace(/\{(\w+)(?::(\w+))?\}/g, (_, name, as) => {
    const v = args[name];
    if (as) return FORMAT[as](v);
    return typeof v === "number" ? numbers.format(v) : esc(v ?? "");
  });
}

/** A message as the English table has it, such as a README heading. */
export const english = (key) => words.en[key] ?? key;

/** A snapshot's text: a message, or a name as written. */
export const say = (x) => (x.key ? t(x.key, x.args) : esc(x.name));

/**
 * Whether the letters typed are in the words, in order but not always side
 * by side, as ⌘K finds "Proxmox VE" for "pxv". Case and accents don't count,
 * nothing typed matches every word, and a word missing is left out. The
 * words are as written, never escaped, so the markup a page draws them in
 * can't match.
 */
export function matches(typed, ...words) {
  const plain = (s) => s.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase();
  const want = [...plain(typed.trim())];
  let i = 0;
  for (const ch of plain(words.filter(Boolean).join(" "))) if (ch === want[i]) i++;
  return i === want.length;
}

/** Fills each element with a data-t attribute with that message. */
export function translate(root) {
  for (const el of root.querySelectorAll("[data-t]")) el.innerHTML = t(el.dataset.t);
}

/** A number, the viewer's way. */
export const number = (n) => numbers.format(n);

/** A number in few characters, the viewer's way: "250K". */
export const compact = (n) => new Intl.NumberFormat(locale, { notation: "compact" }).format(n);

/** Items as one phrase, the viewer's way: "a, b and c". */
export const list = (items) => new Intl.ListFormat(locale).format(items);

/** A time of day: "14:05", or "2:05 PM". */
export const clock = (d) => new Date(d).toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit" });

/** A day of the week, short: "Tue". */
export const weekday = (d) => new Date(d).toLocaleDateString(locale, { weekday: "short" });

/** A day of the month: "28". */
export const date = (d) => new Date(d).toLocaleDateString(locale, { day: "numeric" });

/** A day: "Tue 28 Oct", or "Tue, Oct 28". */
export const day = (d) => new Date(d).toLocaleDateString(locale, { weekday: "short", day: "numeric", month: "short" });

/** A span of milliseconds: "37 min", "5 h", "3 days". */
export function span(ms) {
  const m = Math.max(0, Math.round(ms / 60e3));
  if (m < 60) return t("span.minutes", { n: m });
  const h = Math.round(m / 60);
  if (h < 48) return t("span.hours", { n: h });
  return t("span.days", { n: Math.round(h / 24) });
}

/** How long ago a moment was, from now: "just now", "5 h ago". */
export const ago = (d, now) => (now - d < 60e3 ? t("ago.now") : t("ago", { ms: now - d }));

/** A moment, from now: today's or yesterday's time, or the day when longer ago. */
export function when(d, now) {
  const at = new Date(d);
  const today = new Date(now);
  today.setHours(0, 0, 0, 0);
  if (at >= today) return t("when.today", { at });
  if (at >= today - 864e5) return t("when.yesterday", { at });
  return day(at);
}

/**
 * Bytes in the units disks are sold in, as the viewer's language writes
 * them: "785 MB", "8.4 TB", or "8,4 To" in French. Under a kilobyte it says
 * "B" in every language, where the browser's English would say "512 byte".
 */
export function bytes(n) {
  const units = ["byte", "kilobyte", "megabyte", "gigabyte", "terabyte", "petabyte"];
  let i = 0;
  let v = n;
  while (v >= 1000 && i < units.length - 1) {
    v /= 1000;
    i++;
  }
  if (i === 0) return `${numbers.format(Math.round(v))} B`;
  return new Intl.NumberFormat(locale, {
    style: "unit",
    unit: units[i],
    maximumFractionDigits: v >= 100 ? 0 : 1,
  }).format(v);
}

/** A response time: "42 ms", "<1 ms", or nothing without one. */
export const ms = (v) => (v === null || v === undefined ? "" : v < 1 ? t("ms.under") : t("ms", { n: Math.round(v) }));

// The formats a message's {name:format} can ask for.
// A list is of names as written, areas of the page's areas, a choice of links;
// an adapter is its key, named as the setup page names it.
const FORMAT = {
  span,
  when: (d) => when(d, moment),
  day,
  clock,
  bytes,
  ms,
  area: (a) => t(`area.${a}`),
  adapter: (key) => t(`setup.adapter.${key}`),
  areas: (areas) => list(areas.map((a) => t(`area.${a}`))),
  list: (names) => list(names.map(esc)),
  // A choice of links, each a text and its address: "a, b or c".
  choice: (links) =>
    new Intl.ListFormat(locale, { type: "disjunction" }).format(
      links.map((l) => `<a href="${esc(l.url)}" target="_blank" rel="noopener">${say(l)}</a>`),
    ),
};
