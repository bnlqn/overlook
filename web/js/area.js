// An area's page: its state and checks, its charts by the hour, those of
// several sources added up, then each section its sources add. It writes what
// page() says of the area (page.js) and decides nothing.

import {
  areas,
  by,
  checkRows,
  hourStart,
  legend,
  lineColours,
  lines,
  section,
  seriesColours,
  stateIcon,
  TONE,
  tip,
  word,
} from "./draw.js";
import { HERE, start } from "./shell.js";
import { clock, day, esc, list, number, say, t } from "./text.js";

// The hours the charts show: the last day, or all their sources keep, until
// the owner picks.
let range = Number.POSITIVE_INFINITY;
let last = null;

// A chart in a card: its title, the range's sums and busiest hour, the switch
// between the last day and every hour when there are more, the chart, and
// its legend.
function chart(c, now) {
  const all = c.series[0].hours.length;
  const n = Math.min(range, all);
  const cut = (x) => ({ ...x, hours: x.hours.slice(all - n) });
  const series = c.series.map(cut);
  const colours = c.measures ? lineColours(series) : seriesColours(series.length);
  const when = (i) => {
    const at = hourStart(i, n, now);
    return `<b>${n > 24 ? `${day(at)}, ` : ""}${clock(at)}</b>`;
  };
  const sum = (hours) => hours.reduce((a, v) => a + v, 0);
  const whole = sum(series[0].hours);
  // Levels say which was highest and when; counts, their sums and the
  // busiest hour.
  const [top, topAt, topOf] = series
    .flatMap((s) => s.hours.map((v, i) => [v, i, s]))
    .reduce((a, b) => (b[0] > a[0] ? b : a));
  const sums = c.measures
    ? [`${t("chart.highest")} <b>${say(topOf.label)} ${t("percent", { n: Math.round(top) })}</b>`, when(topAt)]
    : [
        `${say(series[0].label)} <b>${number(whole)}</b>`,
        ...series
          .slice(1)
          .map(
            (s) =>
              `${say(s.label)} <b>${t("percent", { n: whole ? Math.round((1000 * sum(s.hours)) / whole) / 10 : 0 })}</b>`,
          ),
        `${t("chart.busiest")} ${when(series[0].hours.indexOf(Math.max(...series[0].hours)))}`,
      ];
  // Added up from some of an adapter's sources, it says so when another
  // answered but gave none.
  if (c.lacking.length) {
    const of = t("figure.of_sources", { n: c.sources.length, total: c.sources.length + c.lacking.length });
    sums.push(`<span data-tip="${tip(esc(list(c.lacking)), [], t("figure.gave_none"))}">${of}</span>`);
  }
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
    <div class="chart-head"><div><h2 class="card-title">${say(c.title)}${by(c)}</h2><div class="detail">${sums.join(" · ")}</div></div>${pills}</div>
    ${c.measures ? lines(series, now) : areas(series, c.sources.map(cut), now)}
    ${legend(series, colours)}</section>`;
}

function draw(m) {
  last = m;
  const a = m.areas.find((x) => x.area === HERE.area);
  const title = t(`area.${a.area}`);
  document.title = title;
  document.getElementById("title").innerHTML = title;
  document.getElementById("checks").hidden = !a.checks.length;
  document.querySelector("#checks .checks").innerHTML = checkRows(a.checks);
  const state = !a.checks.length
    ? a.shown
      ? t("area.no_checks")
      : t("area.empty")
    : `<span class="state tone-${TONE[a.state]}">${stateIcon(a.state)}${word(a.state)}</span>`;
  const problems = a.problems.length
    ? esc(list(a.problems.map((c) => c.name)))
    : a.checks.length
      ? t("area.checks_up", { n: a.checks.length })
      : "";
  const sources = a.silent.length ? t("source.no_answer", { names: a.silent }) : "";
  document.getElementById("summary").innerHTML = [state, problems, sources].filter(Boolean).join(" · ");
  document.getElementById("charts").innerHTML = a.charts.map((c) => chart(c, m.now)).join("");
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
