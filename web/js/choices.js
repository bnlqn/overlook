// The viewer's own choices, in the footer of the page and of the setup page,
// and in ⌘K: the look, and the language. This browser keeps them for the
// next visit; Overlook keeps nothing (ADR 0004). A look changes at once,
// through theme.js, and a language reads the page again in it.

import { svg } from "./draw.js";
import { autonym, chosenLanguage, esc, LANGUAGE, LANGUAGES, t } from "./text.js";

// Where this browser keeps the look the viewer chose, which theme.js reads
// too, as text.js's LANGUAGE keeps the language.
const THEME = "overlook.theme";

/** The looks, each with its word and its icon: the system's, light and dark. */
const LOOKS = [
  ["", "theme.auto", "monitor"],
  ["light", "theme.light", "sun"],
  ["dark", "theme.dark", "moon"],
];

/** The languages the page speaks, each by its own name, in their order. */
const languages = () => LANGUAGES.map((code) => [code, autonym(code)]).sort((a, b) => a[1].localeCompare(b[1]));

/**
 * The footer's choices: the looks as pills, each an icon, its word a
 * tooltip and what a screen reader says; and the language the page speaks,
 * as a pill over the browser's own list, which a phone shows as its own
 * picker.
 */
export function choices() {
  const look = kept(THEME);
  const shown = LANGUAGES.includes(document.documentElement.lang) ? document.documentElement.lang : "en";
  const chosen = chosenLanguage();
  const pills = LOOKS.map(
    ([value, key, icon]) =>
      `<button type="button" data-look="${value}" aria-pressed="${value === look}" aria-label="${t(key)}" data-tip="${t(key)}">${svg(icon)}</button>`,
  );
  const options = languages().map(
    ([code, name]) =>
      `<option value="${code}" lang="${code}"${code === chosen ? " selected" : ""}>${esc(name)}</option>`,
  );
  return `<div class="choices">
      <div class="pills looks" role="group" aria-label="${t("theme")}">${pills.join("")}</div>
      <label class="language">${svg("language")}<span lang="${shown}">${esc(autonym(shown))}</span>${svg("caret", "ic caret")}
        <select aria-label="${t("language")}"><option value="">${t("language.auto")}</option>${options.join("")}</select>
      </label>
    </div>`;
}

/** Listens for a choice in the footer, on whichever page draws it. */
export function listen() {
  document.addEventListener("click", (e) => {
    const pill = e.target.closest("[data-look]");
    if (pill) choose({ look: pill.dataset.look });
  });
  document.addEventListener("change", (e) => {
    if (e.target.closest(".language")) choose({ language: e.target.value });
  });
}

/** ⌘K's entries for the choices, found by their words and their group's. */
export function choiceItems() {
  const plain = (key) => t(key).replace(/<[^>]*>/g, "");
  return [
    ...LOOKS.map(([value, key, icon]) => ({
      group: t("theme"),
      name: t(key),
      words: [plain(key), plain("theme")],
      glyph: svg(icon),
      act: () => choose({ look: value }),
    })),
    ...[["", t("language.auto")], ...languages()].map(([code, name]) => ({
      group: t("language"),
      name: code ? esc(name) : name,
      words: [code ? name : plain("language.auto"), code, plain("language")],
      glyph: svg("language"),
      act: () => choose({ language: code }),
    })),
  ];
}

/**
 * Applies a choice: a look at once, through theme.js, told as it is of a
 * choice made in another tab, and the pills with it; a language by reading
 * the page again in it.
 */
function choose({ look, language }) {
  if (look !== undefined) {
    keep(THEME, look);
    dispatchEvent(new StorageEvent("storage", { key: THEME }));
    for (const pill of document.querySelectorAll("[data-look]")) {
      pill.setAttribute("aria-pressed", String(pill.dataset.look === look));
    }
  }
  if (language !== undefined) {
    keep(LANGUAGE, language);
    location.reload();
  }
}

/** What this browser keeps for the viewer under key, or nothing, as in a browser that keeps nothing. */
function kept(key) {
  try {
    return localStorage.getItem(key) ?? "";
  } catch {
    return "";
  }
}

/** Keeps value under key in this browser, or forgets the key for none. */
function keep(key, value) {
  try {
    if (value) localStorage.setItem(key, value);
    else localStorage.removeItem(key);
  } catch {
    // A browser that keeps nothing follows the system and the browser.
  }
}
