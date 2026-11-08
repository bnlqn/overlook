// An area's page: its state and checks, its charts by the hour, those of
// several sources added up, then each section its sources add. It writes what
// page() says of the area (page.js) and decides nothing.

import { chart, checkRows, section, stateIcon, summary, TONE, word } from "./draw.js";
import { HERE, start } from "./shell.js";
import { t } from "./text.js";

// The hours the charts show: the last day, or all their sources keep, until
// the owner picks.
let range = Number.POSITIVE_INFINITY;
let last = null;

function draw(m) {
  last = m;
  const a = m.areas.find((x) => x.area === HERE.area);
  const title = t(`area.${a.area}`);
  document.title = title;
  document.getElementById("title").innerHTML = title;
  document.getElementById("checks").hidden = !a.checks.length;
  document.querySelector("#checks .checks").innerHTML = checkRows(a.checks);
  // With no checks, its state says nothing, and the summary says why.
  const state = a.checks.length
    ? `<span class="state tone-${TONE[a.state]}">${stateIcon(a.state)}${word(a.state)}</span>`
    : "";
  document.getElementById("summary").innerHTML = [state, summary(a.summary)].filter(Boolean).join(" · ");
  document.getElementById("charts").innerHTML = a.charts.map((c) => chart(c, range, m.now)).join("");
  // The sections in the order their sources gave them, each source's
  // together, flowing down two columns of about the same height.
  document.getElementById("sections").innerHTML = a.sections.map((s, i) => section(s, i)).join("");
}

document.addEventListener("click", (e) => {
  const b = e.target.closest(".pills button");
  if (!b || !last) return;
  range = b.dataset.hours === "all" ? Number.POSITIVE_INFINITY : +b.dataset.hours;
  draw(last);
});

start(draw);
