// An area's page: its state and checks, then each section its sources add,
// with its chart by the hour above its rows where it has one.

import { areaState, shown, silent } from "./checks.js";
import { checkRows, hourChart, section, stateIcon, TINT, TONE, word } from "./draw.js";
import { HERE, start } from "./shell.js";
import { esc, list, t } from "./text.js";

// The hours a chart shows, the last day or all its source keeps; all until
// the owner picks.
let range = Number.POSITIVE_INFINITY;
let last = null;

// A section's chart: the hours in range, and the switch between the last day
// and all of them when there are more.
function chart(s) {
  const all = s.hours.hours.length;
  const n = Math.min(range, all);
  const pick = (x) => x && { ...x, hours: x.hours.slice(all - n) };
  const pills =
    all > 24
      ? `<div class="range">${[24, all]
          .map(
            (h) =>
              `<button type="button" data-hours="${h === all ? "all" : h}" aria-pressed="${h === n}">${h === all ? t("chart.all", { ms: all * 3600e3 }) : t("chart.day")}</button>`,
          )
          .join("")}</div>`
      : "";
  return `<div class="card chart">${pills}${hourChart(pick(s.hours), pick(s.within), TINT[s.area])}</div>`;
}

function draw(snap, checks) {
  last = { snap, checks };
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
  const mine = snap.sections.filter((s) => s.area === area).map((s, i) => section(s, i, s.hours ? chart(s) : ""));
  document.getElementById("left").innerHTML = mine.filter((_, i) => i % 2 === 0).join("");
  document.getElementById("right").innerHTML = mine.filter((_, i) => i % 2 === 1).join("");
}

document.addEventListener("click", (e) => {
  const b = e.target.closest(".range button");
  if (!b || !last) return;
  range = b.dataset.hours === "all" ? Number.POSITIVE_INFINITY : +b.dataset.hours;
  draw(last.snap, last.checks);
});

start(draw);
