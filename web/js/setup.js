// The setup page (ADR 0014): first the setup code from Overlook's log (ADR
// 0018), then every adapter, grouped as the README lists them, each with the
// fields its sources need; each source tried once through Overlook, and
// those that answered put into the two files to save. The code and the keys
// stay in this tab's memory and nowhere else: a reload starts again.

import { anchor, dotenv, readme, yaml } from "./config.js";
import { badge, stateIcon, svg } from "./draw.js";
import { AREAS } from "./page.js";
import { english, esc, list, loadWords, say, t, translate } from "./text.js";

const GROUPS = ["checkengines", ...AREAS, "chores"];
const group = (a) => (a.kind === "checkengine" ? "checkengines" : a.area || "chores");
const groupName = (g) => (AREAS.includes(g) ? t(`area.${g}`) : t(`setup.group.${g}`));
const $ = (id) => document.getElementById(id);

// The sources that answered their try, in the order they were added: each
// its adapter's key, its name, and its fields as typed.
const added = [];
let adapters = [];
// The README of the Overlook that runs, which each adapter links.
let base = "";
// The setup code, once Overlook has taken it, sent with every try.
let code = "";

const post = (body) =>
  fetch("setup", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });

// Shows the code's form, saying why, in place of the adapters; or, with no
// reason, the adapters, once Overlook has taken the code.
function ask(why) {
  const form = $("code-form");
  const answer = form.querySelector(".answer");
  answer.className = "answer bad";
  answer.innerHTML = why ?? "";
  $("code").hidden = why === undefined;
  $("groups").hidden = why !== undefined;
  if (why === undefined) $("groups").querySelector("summary").focus();
  else form.elements.code.focus();
}

// Asks Overlook whether the code typed is the one in its log: the code
// alone, with no source, which it answers with nothing when it is.
async function takeCode(form) {
  const button = form.querySelector("button");
  const typed = form.elements.code.value.trim();
  button.disabled = true;
  let r;
  try {
    r = await post({ code: typed });
  } catch {
    r = undefined;
  } finally {
    button.disabled = false;
  }
  if (r?.ok) {
    code = typed;
    form.reset();
    return ask();
  }
  ask(r?.status === 401 ? t("setup.code_wrong") : r ? esc((await r.text()).trim()) : t("overlook.no_answer"));
}

// One field of a source to type.
function field(ask) {
  const label = t(`setup.field.${ask.field}`);
  return `<label class="ask"><span>${label}${ask.optional ? `<i>${t("setup.optional")}</i>` : ""}</span>
    <input name="${ask.field}" type="${ask.key ? "password" : "text"}" placeholder="${esc(ask.example ?? "")}"
      autocomplete="off" spellcheck="false" autocapitalize="off"${ask.optional ? "" : " required"}></label>`;
}

function adapter(a) {
  const asks = [...a.asks.filter((k) => !k.optional), ...a.asks.filter((k) => k.optional)];
  const link = `${base}#${anchor(english(`setup.adapter.${a.key}`))}`;
  return `<details class="card adapter" id="adapter-${a.key}">
    <summary>${svg("caret", "ic caret")}${badge({ icon: a.icon }, "xs")}<span class="adapter-name">${t(`setup.adapter.${a.key}`)}</span><span class="n"></span></summary>
    <ul class="added"></ul>
    <form class="asks" data-key="${a.key}">
      ${asks.map(field).join("")}
      <div class="asks-foot">
        <button class="action strong" type="submit">${t("setup.try")}</button>
        <a class="readme" href="${link}" target="_blank" rel="noreferrer">${t("setup.readme")}${svg("out")}</a>
        <p class="answer" role="status"></p>
      </div>
    </form>
  </details>`;
}

// What a try found, in the page's words, or that it found nothing.
function found(got) {
  const parts = [
    got.checks && t("setup.found.checks", { n: got.checks }),
    got.problems && t("setup.found.problems", { n: got.problems }),
    got.sections && t("setup.found.sections", { n: got.sections }),
    got.charts && t("setup.found.charts", { n: got.charts }),
    got.figures && t("setup.found.figures", { n: got.figures }),
    got.events && t("setup.found.events", { n: got.events }),
    got.chores && t("setup.found.chores", { n: got.chores }),
  ].filter(Boolean);
  return parts.length ? `${t("setup.answered")} · ${list(parts)}` : t("setup.answered_empty");
}

// Why a try failed, as what to do about it, which the server words, with
// the error as Overlook got it beneath; or the error alone, when it's the
// config's own, such as "key is missing", or one of no kind the server
// tells apart.
const why = (got) => (got.why ? `${say(got.why)}<span class="detail">${esc(got.error)}</span>` : esc(got.error));

async function tryOne(form) {
  const a = adapters.find((x) => x.key === form.dataset.key);
  const answer = form.querySelector(".answer");
  const button = form.querySelector("button");
  const fields = {};
  for (const ask of a.asks) {
    const v = form.elements[ask.field].value.trim();
    if (v) fields[ask.field] = ask.list ? v.split(/[\s,]+/).filter(Boolean) : v;
  }
  const tell = (tone, html) => {
    answer.className = `answer ${tone}`;
    answer.innerHTML = html;
  };
  button.disabled = true;
  tell("", t("setup.trying"));
  let got;
  try {
    const r = await post({ code, adapter: a.key, fields });
    // Overlook has started again, with a new code, since this tab took one.
    got = r.status === 401 ? { code: false } : r.ok ? await r.json() : { error: (await r.text()).trim() };
  } catch {
    got = {};
  } finally {
    button.disabled = false;
  }
  if (got.code === false) {
    tell("", "");
    return ask(t("setup.code_again"));
  }
  if (!got.name && !got.error) return tell("bad", t("overlook.no_answer"));
  if (got.error) return tell("bad", why(got));
  if (added.some((s) => s.name === got.name)) return tell("bad", t("setup.same_name", { name: got.name }));
  added.push({ adapter: a.key, name: got.name, fields });
  form.reset();
  tell("good", found(got));
  draw();
}

// The sources added, under each adapter, and the two files.
function draw() {
  for (const a of adapters) {
    const mine = added.filter((s) => s.adapter === a.key);
    const card = $(`adapter-${a.key}`);
    card.querySelector(".n").textContent = mine.length || "";
    card.querySelector(".added").innerHTML = mine
      .map(
        (s) =>
          `<li>${stateIcon("up")}<b>${esc(s.name)}</b><span class="where">${esc(s.fields.url ?? s.fields.host ?? [s.fields.repos].flat().join(" "))}</span>
          <button class="icon-btn" type="button" data-drop="${added.indexOf(s)}" aria-label="${t("setup.drop")}">×</button></li>`,
      )
      .join("");
  }
  $("files").hidden = !added.length;
  $("yaml").textContent = yaml(adapters, added, base);
  $("env").textContent = dotenv(adapters, added);
}

// Copies a file's text, or, where the browser won't let a page copy, as on
// an address without HTTPS, selects it for the owner to copy.
async function copy(button) {
  const pre = $(button.dataset.copy);
  const label = button.querySelector("span");
  try {
    await navigator.clipboard.writeText(pre.textContent);
    label.innerHTML = t("setup.copied");
  } catch {
    getSelection().selectAllChildren(pre);
    label.innerHTML = t("setup.selected");
  }
  setTimeout(() => {
    label.innerHTML = t("setup.copy");
  }, 2000);
}

function download(button) {
  const a = document.createElement("a");
  a.href = URL.createObjectURL(new Blob([$(button.dataset.save).textContent], { type: "text/plain" }));
  a.download = button.dataset.name;
  a.click();
  URL.revokeObjectURL(a.href);
}

async function start() {
  await loadWords();
  translate(document);
  document.title = t("setup.title");
  $("status-icon").innerHTML = svg("mark", "ic si");
  $("code-form").insertAdjacentHTML("afterbegin", field({ field: "code" }));
  const setup = await fetch("setup").then((r) => r.json());
  adapters = setup.adapters;
  base = readme(setup.version);
  if (setup.unraid) {
    $("code-ask").innerHTML = t("setup.code_ask_unraid");
    $("restart").innerHTML = t("setup.restart_unraid");
  }
  $("groups").innerHTML = GROUPS.map(
    (g) => `<section class="block">
      <h2 class="block-title">${groupName(g)}</h2>${g === "checkengines" ? `<p class="block-ask">${t("setup.group.checkengines_ask")}</p>` : ""}
      <div class="adapters">${adapters
        .filter((a) => group(a) === g)
        .map(adapter)
        .join("")}</div>
    </section>`,
  ).join("");
  document.addEventListener("submit", (e) => {
    e.preventDefault();
    if (e.target.id === "code-form") takeCode(e.target);
    else tryOne(e.target);
  });
  document.addEventListener("click", (e) => {
    const b = e.target.closest("button");
    if (b?.dataset.drop) {
      added.splice(+b.dataset.drop, 1);
      draw();
    } else if (b?.dataset.copy) copy(b);
    else if (b?.dataset.save) download(b);
  });
  draw();
  // Once Overlook is back with its config, it answers /setup no more, and
  // this tab becomes the page; while it's away, restarting, the page says so.
  setInterval(async () => {
    let away = false;
    try {
      if ((await fetch("setup", { cache: "no-store" })).status === 404) location.reload();
    } catch {
      away = true;
    }
    $("away").hidden = !away;
  }, 2000);
}

start();
