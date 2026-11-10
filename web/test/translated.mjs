// The English each translation was made from (ADR 0020): for each language,
// and each key its file has, the first eight hex digits of the SHA-256 of
// that key's English. `make translated` writes it to translated.json beside
// this file, from the files as they are, and the page's tests compare that
// record with them, so that a translation never says what English no longer
// does.

import { createHash } from "node:crypto";
import { readdirSync, readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const web = new URL("../", import.meta.url);
const read = (path) => JSON.parse(readFileSync(new URL(path, web), "utf8"));

/** English's table, every adapter's words in it, and each translation by its language. */
export function tables() {
  const english = Object.assign({}, ...readdirSync(new URL("i18n/en/", web)).map((f) => read(`i18n/en/${f}`)));
  const translations = Object.fromEntries(
    readdirSync(new URL("i18n/", web))
      .filter((f) => f.endsWith(".json"))
      .map((f) => [f.replace(".json", ""), read(`i18n/${f}`)]),
  );
  return { english, translations };
}

/** The record as it would be if every translation were made from today's English. */
export function translatedFrom({ english, translations } = tables()) {
  const hash = (entry) => createHash("sha256").update(JSON.stringify(entry)).digest("hex").slice(0, 8);
  return Object.fromEntries(
    Object.keys(translations)
      .sort()
      .map((language) => [
        language,
        Object.fromEntries(
          Object.keys(translations[language])
            .filter((key) => key in english)
            .sort()
            .map((key) => [key, hash(english[key])]),
        ),
      ]),
  );
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  writeFileSync(new URL("test/translated.json", web), `${JSON.stringify(translatedFrom(), null, 2)}\n`);
}
