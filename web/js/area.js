// An area's page: its state and checks, then each section its sources add.

import { areaState, shown, silent } from "./checks.js";
import { checkRows, section, stateIcon, TONE, word } from "./draw.js";
import { HERE, start } from "./shell.js";
import { esc, list, t } from "./text.js";

start((snap, checks) => {
  const area = HERE.area;
  const a = areaState(area, checks);
  const silence = silent(snap);
  const title = t(`area.${area}`);
  document.title = title;
  document.getElementById("title").innerHTML = title;
  document.getElementById("checks").hidden = !a.checks.length;
  document.querySelector("#checks .checks").innerHTML = checkRows(a.checks);
  const state = !a.checks.length
    ? t(shown(snap).includes(area) ? "area.no_checks" : "area.empty")
    : `<span class="state tone-${TONE[a.state]}">${stateIcon(a.state)}${word(a.state)}</span>`;
  const problems = a.problems.length
    ? esc(list(a.problems.map((c) => c.name)))
    : a.checks.length
      ? t("area.checks_up", { n: a.checks.length })
      : "";
  const sources = silence.length ? t("source.no_answer", { name: list(silence) }) : "";
  document.getElementById("summary").innerHTML = [state, problems, sources].filter(Boolean).join(" · ");
  // The sections, in two columns, each in the order its sources gave them.
  // Each knows its place in the snapshot, which one column goes back to.
  const mine = snap.sections.filter((s) => s.area === area).map((s, i) => section(s, i));
  document.getElementById("left").innerHTML = mine.filter((_, i) => i % 2 === 0).join("");
  document.getElementById("right").innerHTML = mine.filter((_, i) => i % 2 === 1).join("");
});
