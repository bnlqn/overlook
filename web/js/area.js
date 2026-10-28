// An area's page: its state and checks, its charts by the hour, those of
// several sources added up, then each section its sources add.

import { areaState, joinCharts, shown, silent } from "./checks.js";
import { areas, checkRows, hourStart, legend, section, seriesColours, stateIcon, TONE, word } from "./draw.js";
import { HERE, start } from "./shell.js";
import { clock, day, esc, list, number, say, t } from "./text.js";

// The hours the charts show: the last day, or all their sources keep, until
// the owner picks.
let range = Number.POSITIVE_INFINITY;
let last = null;

// A chart in a card: its title, the range's sums and busiest hour, the switch
// between the last day and every hour when there are more, the chart, and
// its legend.
function chart(c) {
  const all = c.series[0].hours.length;
  const n = Math.min(range, all);
  const cut = (x) => ({ ...x, hours: x.hours.slice(all - n) });
  const series = c.series.map(cut);
  const colours = seriesColours(series.length);
  const sum = (hours) => hours.reduce((a, v) => a + v, 0);
  const whole = sum(series[0].hours);
  const peak = series[0].hours.indexOf(Math.max(...series[0].hours));
  const at = hourStart(peak, n);
  const sums = [
    `${say(series[0].label)} <b>${number(whole)}</b>`,
    ...series
      .slice(1)
      .map(
        (s) =>
          `${say(s.label)} <b>${t("percent", { n: whole ? Math.round((1000 * sum(s.hours)) / whole) / 10 : 0 })}</b>`,
      ),
    `${t("chart.busiest")} <b>${n > 24 ? `${day(at)}, ` : ""}${clock(at)}</b>`,
  ];
  const pills =
    all > 24
      ? `<div class="pills">${[24, all]
          .map(
            (h) =>
              `<button type="button" data-hours="${h === all ? "all" : h}" aria-pressed="${h === n}">${h === all ? t("chart.all", { ms: all * 3600e3 }) : t("chart.day")}</button>`,
          )
          .join("")}</div>`
      : "";
  return `<section class="card chart">
    <div class="chart-head"><div><h2 class="card-title">${say(c.title)}</h2><div class="detail">${sums.join(" · ")}</div></div>${pills}</div>
    ${areas(series, c.sources.map(cut))}
    ${legend(series, colours)}</section>`;
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
  document.getElementById("charts").innerHTML = joinCharts(snap.charts.filter((c) => c.area === area))
    .map(chart)
    .join("");
  // The sections, in two columns, each in the order its sources gave them.
  // Each knows its place in the snapshot, which one column goes back to.
  const mine = snap.sections.filter((s) => s.area === area).map((s, i) => section(s, i));
  document.getElementById("left").innerHTML = mine.filter((_, i) => i % 2 === 0).join("");
  document.getElementById("right").innerHTML = mine.filter((_, i) => i % 2 === 1).join("");
}

document.addEventListener("click", (e) => {
  const b = e.target.closest(".pills button");
  if (!b || !last) return;
  range = b.dataset.hours === "all" ? Number.POSITIVE_INFINITY : +b.dataset.hours;
  draw(last.snap, last.checks);
});

start(draw);
