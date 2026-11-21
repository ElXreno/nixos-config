"use strict";
const str = (v) => (typeof v === "string" ? v : "");
const arr = (v) => (Array.isArray(v) ? v : []);
const num = (v) => (typeof v === "number" && isFinite(v) ? v : null);
const sidOf = (s) => (s ? s.slice(0, 8) : "-");
const hms = (iso) => {
  const d = new Date(iso);
  return isNaN(d) ? "" : d.toLocaleTimeString("ru-RU");
};
const dt = (iso) => {
  const s = str(iso);
  return s && !s.startsWith("0001") ? s.slice(0, 16).replace("T", " ") : "";
};
const own = (o, k) =>
  Object.prototype.hasOwnProperty.call(o, k) ? o[k] : undefined;
const f3 = (v) => (v == null ? "" : (+v).toFixed(3));
const jget = (u) =>
  fetch(u, { cache: "no-store" }).then((r) =>
    r.ok ? r.json() : Promise.reject(new Error("HTTP " + r.status)),
  );
const baseKey = (k) => {
  const i = k.lastIndexOf("#");
  if (i <= 0 || i === k.length - 1) return k;
  const s = k.slice(i + 1);
  return /^\d+$/.test(s) && +s >= 1 ? k.slice(0, i) : k;
};
const nodeOf = (k) => BYKEY.get(k) || BYKEY.get(baseKey(k)) || null;
const JOURNAL_RE =
  /(^|\n)([ \t]*(?:[-*][ \t]+)?(?:\*\*)?(?:Narrowed |Corrected |Updated )?20\d\d-\d\d-\d\d[^\n]*)/g;
const GATES = { cos: 0.5, rerank_text: 0.99, rerank_pointer: 0.9 };

let HYG = window.__MEMHYG || null;
const HYG_TEXT = {
  broken: {
    t: "Битые ссылки",
    d: "Ссылки на ключи, которых нет. Сначала ищется точный ключ, затем записи с derived_from на этот слаг и его куски, затем записи с таким префиксом; здесь остались те, что не нашлись. Токены вроде MEMORY и SendMessage не ключи и отброшены.",
    h: "mem put KEY --update, либо убрать ссылку из текста",
  },
  orphans: {
    t: "Сироты",
    d: "Актуальные записи без темы, без свежего вектора или, при журнале ходов за 30 суток и больше, ни разу не поданные агенту за 30 суток. Ссылки между записями здесь ни при чём: агент находит записи поиском, векторами и картой тем.",
    h: "mem topics assign, mem reindex",
  },
  expired: {
    t: "Срок статуса истёк",
    d: "Статусы проектов со статусом expired: ttl прошёл, mem expire их пометил. Агенту они не отдаются, пока не подтверждены.",
    h: "mem verify KEY или mem retire KEY",
  },
  stale: {
    t: "Статусы старше ttl",
    d: "Текущие статусы, у которых ttl уже вышел, но mem expire ещё не помечал их.",
    h: "mem expire",
  },
  dangling: {
    t: "Обрывы замены",
    d: "superseded_by указывает на несуществующую запись, либо запись заменена, а преемника нет.",
    h: "mem put NEW --supersedes OLD",
  },
  triggers: {
    t: "Сломанные триггеры",
    d: "Регулярка или маска инструмента не компилируется, триггер не срабатывает никогда.",
    h: "mem put KEY --update --trigger TOOL:REGEX",
  },
  verify: {
    t: "Очередь проверки",
    d: "Записи с verify_cmd, которую ещё ни разу не запускали. Не считается в счётчике: это очередь работы, а не поломка.",
    h: "mem verify KEY",
  },
  scopes: {
    t: "Проекты не в виде владелец/репозиторий",
    d: "Область project:имя без слэша осталась от импорта. Не считается в счётчике.",
    h: "mem rescope",
  },
  duplicates: {
    t: "Пары, признанные различными",
    d: "Связи relates с пометкой distinct: пишет гейт записи, когда нашёл близкую запись и решил, что это не дубль. Не считается в счётчике.",
    h: "mem put NEW --distinct OLD",
  },
  journals: {
    t: "История правок в тексте",
    d: "Три и больше строк, начинающихся с даты: агент читает историю как текущий факт. Не считается в счётчике; уходит при дистилляции.",
    h: "mem put KEY --update",
  },
  retired: {
    t: "Сняты",
    d: "Записи со статусом retired: сняты вручную, агенту не отдаются. Не считаются в счётчике.",
    h: "mem get KEY --history",
  },
  superseded: {
    t: "Заменены",
    d: "История, а не проблема: запись заменена новой. Показаны для просмотра, в счётчике не участвуют.",
    h: "mem get KEY --history",
  },
};
const REASON = {
  "no-topic": "нет темы",
  "no-vector": "нет вектора",
  "stale-vector": "вектор устарел",
  unsurfaced: "не подавалась 30 сут.",
};
const TIER = {
  strong: "",
  lexical: "слова",
  pointer: "указатель",
  ghost: "тень",
  tool: "триггер",
  session: "core",
  candidate: "кандидат",
};

function itemsOf(id) {
  const s = HYG && HYG.sections.find((x) => x.id === id);
  return s ? s.items : [];
}

const facetPass = (n, skip) => nodeVisible(n, skip);
function renderSide() {
  const cnt = (skip, get) => {
    const c = new Map();
    for (const n of NODES)
      if (facetPass(n, skip)) {
        const v = get(n);
        for (const x of Array.isArray(v) ? v : [v])
          c.set(x, (c.get(x) || 0) + 1);
      }
    return c;
  };
  const ct = cnt("type", (n) => n.type),
    cs = cnt("status", (n) => n.status);
  $("#types").innerHTML = TKEYS.filter((t) => NODES.some((n) => n.type === t))
    .map(
      (t) =>
        `<button class="trow" data-type="${t}" aria-pressed="${!FILTER.hideType.has(t)}" style="--c:${TYPES[t].c}"><span class="tdot"></span><span class="tn">${TYPES[t].ru}</span><span class="tc">${ct.get(t) || 0}</span></button>`,
    )
    .join("");
  $("#statuses").innerHTML = SKEYS.filter((s) =>
    NODES.some((n) => n.status === s),
  )
    .map(
      (s) =>
        `<button class="trow" data-status="${s}" aria-pressed="${!FILTER.hideStatus.has(s)}" style="--c:${s === "current" ? "#cccac3" : "#7b7975"}"><span class="tdot"></span><span class="tn">${STATUSES[s]}</span><span class="tc">${cs.get(s) || 0}</span></button>`,
    )
    .join("");
  const cp = cnt("project", (n) => (n.projects.length ? n.projects : [""])),
    ch = cnt("host", (n) => (n.hosts.length ? n.hosts : [""]));
  const chips = (m, sel, attr, none) => {
    const all = new Set([...m.keys(), ...sel]);
    const ks = [...all].sort(
      (a, b) => (m.get(b) || 0) - (m.get(a) || 0) || a.localeCompare(b),
    );
    return ks
      .map(
        (k) =>
          `<button class="chip" data-${attr}="${esc(k)}" aria-pressed="${sel.has(k)}" title="${esc(k || none)}"><span class="cl">${esc(k || none)}</span><span class="cc">${m.get(k) || 0}</span></button>`,
      )
      .join("");
  };
  $("#projects").innerHTML = chips(
    cp,
    FILTER.projects,
    "proj",
    "(без проекта)",
  );
  $("#projN").textContent = [...cp.keys()].filter(Boolean).length;
  $("#hosts").innerHTML = chips(ch, FILTER.hosts, "host", "(любой хост)");
  $("#hostsN").textContent = [...ch.keys()].filter(Boolean).length;
  renderTopics();
}
function renderTopics() {
  const tops = TOPICS.slice().sort((a, b) => b.size - a.size);
  $("#topics").innerHTML = tops
    .map((t) => {
      const c = {};
      let size = 0;
      for (const m of t.members)
        if (facetPass(m, "topic")) {
          c[m.type] = (c[m.type] || 0) + 1;
          size++;
        }
      const mix = TKEYS.filter((k) => c[k])
        .map((k) => `<i style="flex:${c[k]};background:${TYPES[k].c}"></i>`)
        .join("");
      const on = S.topicFocus === t.i || FILTER.topic === t.i;
      return `<button class="toprow${on ? " on" : ""}${t.id === "" ? " none" : ""}" data-topic="${t.i}" title="${esc(t.kw.join(", "))}"><span class="l1"><span class="tl">${esc(t.label)}</span><span class="tc">${size}</span></span><span class="mix">${mix}</span></button>`;
    })
    .join("");
  $("#topN").textContent = TOPICS.filter((t) => t.id !== "").length;
}
function filtersActive() {
  return (
    FILTER.hideType.size ||
    FILTER.hideStatus.size ||
    FILTER.projects.size ||
    FILTER.hosts.size ||
    FILTER.topic != null
  );
}
function refilter() {
  applyFilters();
  renderSide();
  updateCounts();
  renderChip();
  renderDetail();
  redoSearch();
  renderHyg();
  renderFeed();
  renderStrip();
}
function resetFilters() {
  FILTER.hideType.clear();
  FILTER.hideStatus.clear();
  FILTER.projects.clear();
  FILTER.hosts.clear();
  FILTER.topic = null;
  refilter();
}
function toggleIn(set, v) {
  set.has(v) ? set.delete(v) : set.add(v);
}
$("#side").addEventListener("click", (e) => {
  const b = e.target.closest(
    "[data-type],[data-status],[data-proj],[data-host]",
  );
  if (!b) return;
  if (b.dataset.type) toggleIn(FILTER.hideType, b.dataset.type);
  else if (b.dataset.status) toggleIn(FILTER.hideStatus, b.dataset.status);
  else if (b.dataset.proj != null) toggleIn(FILTER.projects, b.dataset.proj);
  else if (b.dataset.host != null) toggleIn(FILTER.hosts, b.dataset.host);
  refilter();
});
$("#filtersReset").onclick = resetFilters;
function updateCounts() {
  $("#cRec").textContent = fmtN(ACTIVE.length);
  $("#cRecW").textContent = plural(ACTIVE.length, [
    "запись",
    "записи",
    "записей",
  ]);
  $("#cRecS").title =
    `Видно ${ACTIVE.length} из ${NODES.length} узлов. Логи сгруппированы по файлам; строк в базе ${DATA.rows}.`;
  $("#cTop").textContent = TOPICS.filter((t) => t.id !== "").length;
  $("#cTopS").title =
    `Тем в таблице topics: ${TOPICS.filter((t) => t.id !== "").length}; записей без темы: ${(TOPICS.find((t) => t.id === "") || { size: 0 }).size}`;
  $("#cLnk").textContent = fmtN(ALINKS.length);
  $("#cLnkS").title =
    `Связей между видимыми узлами: ${ALINKS.length} из ${LINKS.length}; пары без направления, типы связей в панели записи`;
  $("#hygN").textContent = HYG ? HYG.count : 0;
  $("#coreN").textContent = NODES.filter(
    (n) => n.core && n.status === "current",
  ).length;
}

const D = $("#detail");
const DET = new Map();
S.histOpen = new Set();
S.resolved = null;
function loadDetail(n, force) {
  const c = DET.get(n.key);
  if (c && c.loading) return;
  if (c && c.data && !force && Date.now() - c.at < 15000) return;
  DET.set(n.key, { data: c && c.data, at: Date.now(), loading: true });
  jget("/api/detail?key=" + encodeURIComponent(n.key))
    .then((d) => {
      DET.set(n.key, { data: d, at: Date.now() });
      if (S.selected === n) renderDetail();
    })
    .catch((e) => {
      DET.set(n.key, { data: c && c.data, at: 0, err: e.message });
      if (S.selected === n) renderDetail();
    });
}
function recRow(n, extra = "") {
  return `<button class="rrow" data-id="${n.id}"><span class="dot" style="--c:${n.color}"></span><span class="k">${esc(n.key)}</span><span class="s">${esc(n.summary.replace(/`/g, ""))}</span>${extra}</button>`;
}
function nogRow(key, note, extra = "") {
  return `<div class="rrow nog"><span class="dot" style="--c:var(--mute1)"></span><span class="k">${esc(key)}</span><span class="s">${esc(note)}</span>${extra}</div>`;
}
const empty = (t) => `<div class="empty" style="padding:4px 0">${t}</div>`;
function wlink(k, label, refs) {
  const o = BYKEY.get(k) || BYKEY.get(baseKey(k));
  if (o) return `<button class="wl" data-id="${o.id}">${label}</button>`;
  const r = refs && refs[k];
  if (r && r.keys.length)
    return `<button class="wl res" data-ref="${esc(k)}" title="Ссылка по ${esc(r.how)}: ${r.keys.length} ${plural(r.keys.length, ["запись", "записи", "записей"])}">${label}</button>`;
  return `<span class="brk" title="Записи нет">${label}</span>`;
}
function inlineMd(s, bol, refs) {
  let h = esc(s);
  h = h.replace(JOURNAL_RE, (m, p, line, off) =>
    off === 0 && !p && !bol ? m : `${p}<span class="upd">${line}</span>`,
  );
  h = h.replace(/\*\*([^*\n]+)\*\*/g, "<b>$1</b>");
  h = h.replace(/\[\[([a-z0-9_.#-]+)\]\]/g, (m, k) => wlink(k, k, refs));
  h = h.replace(/\[([^\]\n]+)\]\(([a-z0-9_]+)\.md\)/g, (m, t, k) =>
    wlink(k, t, refs),
  );
  return h;
}
const RT_RE = /```[^\n]*\n([\s\S]*?)```|`([^`\n]+)`/g;
function richText(n, text, refs) {
  const s = text;
  let out = "",
    last = 0,
    m;
  RT_RE.lastIndex = 0;
  while ((m = RT_RE.exec(s))) {
    out += inlineMd(
      s.slice(last, m.index),
      last === 0 || s[last - 1] === "\n",
      refs,
    );
    const tok = m[2] != null ? m[2] : "";
    const ko = tok && (BYKEY.get(tok) || (refs && refs[tok]));
    out +=
      m[1] != null
        ? `<code class="blk">${esc(m[1].replace(/\n$/, ""))}</code>`
        : ko && BYKEY.get(tok) !== n
          ? wlink(tok, esc(tok), refs)
          : `<code>${esc(m[2])}</code>`;
    last = RT_RE.lastIndex;
  }
  return (
    out + inlineMd(s.slice(last), last === 0 || s[last - 1] === "\n", refs)
  );
}
function scopeChip(s) {
  if (s.startsWith("project:"))
    return `<button class="sc2 p" data-fproj="${esc(s.slice(8))}" title="Показать только этот проект">${esc(s)}</button>`;
  if (s.startsWith("host:"))
    return `<button class="sc2 h" data-fhost="${esc(s.slice(5))}" title="Показать только этот хост">${esc(s)}</button>`;
  if (s.startsWith("topic:")) {
    const t = TOPICS.find((x) => x.id === s.slice(6));
    return t
      ? `<button class="sc2 t" data-topic="${t.i}">${esc(s)}</button>`
      : `<span class="sc2">${esc(s)}</span>`;
  }
  return `<span class="sc2">${esc(s)}</span>`;
}
const kv = (k, v) =>
  v === "" || v == null ? "" : `<span>${k}</span><b>${v}</b>`;
const REL = {
  supersedes: "заменяет",
  derived_from: "выведена из",
  relates: "связана",
  unresolved: "ссылка в тексте",
};
function linksHtml(n, d) {
  if (!d) return empty("Загрузка...");
  const by = {};
  for (const l of d.links) (by[l.rel] = by[l.rel] || []).push(l);
  const order = ["supersedes", "derived_from", "relates", "unresolved"].filter(
    (r) => by[r],
  );
  if (!order.length) return empty("Связей нет");
  return order
    .map(
      (rel) =>
        `<div class="lgrp"><h4>${own(REL, rel) || esc(rel)} <span>${by[rel].length}</span></h4>${by[
          rel
        ]
          .map((l) => {
            const arrow = l.dir === "out" ? "&rarr;" : "&larr;";
            const note = l.note
              ? `<div class="lnote">${esc(l.note)}</div>`
              : "";
            let main;
            const tag = `<span class="sc">${arrow}</span>`;
            if (l.in_graph && BYKEY.get(l.other))
              main = recRow(BYKEY.get(l.other), tag);
            else if (l.rel === "unresolved" && l.non_key)
              main = nogRow(l.other, "не ключ записи", tag);
            else if (l.rel === "unresolved" && arr(l.resolved).length)
              main =
                `<div class="rrow nog"><span class="dot" style="--c:var(--mute1)"></span><span class="k">${esc(l.other)}</span><span class="s">найдено по ${esc(l.how)}</span></div>` +
                l.resolved
                  .map((k) =>
                    BYKEY.get(k)
                      ? recRow(
                          BYKEY.get(k),
                          `<span class="sc">${esc(l.how)}</span>`,
                        )
                      : nogRow(k, "нет в графе"),
                  )
                  .join("");
            else
              main = nogRow(
                l.other,
                l.rel === "unresolved" ? "записи нет" : "нет в графе",
                tag,
              );
            return main + note;
          })
          .join("")}</div>`,
    )
    .join("");
}
function histHtml(n, d) {
  if (!d) return empty("Загрузка...");
  const cur = `<li class="cur"><span class="v">v${n.ver}</span><span class="dt">${dt(d.record.updated_at)}</span><span>текущая версия</span></li>`;
  const old = d.history
    .map((v) => {
      const key = n.key + ":" + v.version;
      const open = S.histOpen.has(key);
      const diff = open
        ? `<div class="vbody">${arr(v.meta)
            .map((m) => `<div class="vmeta">${esc(m)}</div>`)
            .join(
              "",
            )}<pre class="diff">${v.diff.map((l) => (l.op === "skip" ? `<span class="dl skip">... ${l.n} ${plural(l.n, ["строка без изменений", "строки без изменений", "строк без изменений"])}</span>` : l.op === "~" ? `<span class="dl skip">${esc(str(l.text))}</span>` : `<span class="dl ${l.op === "+" ? "add" : l.op === "-" ? "del" : "same"}">${esc(l.op)} ${esc(str(l.text))}</span>`)).join("")}</pre><details class="oldbody"><summary>Тело этой версии</summary><div class="d-text">${esc(v.body)}</div></details></div>`
        : "";
      return `<li><span class="v">v${v.version}</span><span class="dt">${dt(v.updated_at || v.changed_at)}</span><span><button class="linkbtn" data-hist="${esc(key)}" aria-expanded="${open}">${esc(v.reason || "правка")}</button></span>${diff}</li>`;
    })
    .join("");
  return (
    `<ol class="ver">${cur}${old}</ol>` +
    (n.journal >= 3
      ? `<p class="vnote">${n.journal} ${plural(n.journal, ["строка", "строки", "строк"])} с датой в начале текста: история лежит внутри записи, а не в версиях</p>`
      : "")
  );
}
function injectedHtml(d) {
  if (!d) return empty("Загрузка...");
  const i = d.injected;
  if (!i.total && !i.last.length)
    return empty("Ни разу не подавалась в журнале ходов");
  return (
    `<p class="vnote" style="margin:0 0 6px">Подана ${i.total} ${plural(i.total, ["раз", "раза", "раз"])} за весь журнал${i.candidates ? `, кандидатом реранкера ${i.candidates}` : ""}</p>` +
    i.last
      .map(
        (e) =>
          `<div class="inj"><span class="tier ${esc(e.tier)}">${esc(own(TIER, e.tier) || e.tier || "текст")}</span><span class="dt">${esc(hms(e.time))} ${esc(str(e.time).slice(5, 10))}</span><span class="isid">${esc(sidOf(e.session))}</span>${e.cos != null ? `<span class="sc">cos ${f3(e.cos)}</span>` : ""}${e.rerank != null ? `<span class="sc">r ${f3(e.rerank)}</span>` : ""}${e.tool ? `<span class="sc">${esc(e.tool)}</span>` : ""}${e.input ? `<span class="iin" title="${esc(e.input)}">${esc(e.input)}</span>` : ""}${e.regex ? `<code>${esc(e.regex)}</code>` : ""}</div>`,
      )
      .join("")
  );
}
function fieldsHtml(n, d) {
  const r = d && d.record;
  const v = d && d.vector;
  const vecState = v
    ? `<span class="vec ${esc(v.state)}">${{ fresh: "свежий", stale: "устарел", missing: "нет", unknown: "неизвестно" }[v.state] || esc(v.state)}</span>${v.dim ? ` ${esc(v.dim)} изм.` : ""}${v.updated_at ? ` ${esc(dt(v.updated_at))}` : ""}${v.model ? ` <span class="mono">${esc(v.model.split("/").pop())}</span>` : ""}`
    : "";
  const ver =
    r && r.verify_cmd
      ? `<code>${esc(r.verify_cmd)}</code> ${r.verified_at && !r.verified_at.startsWith("0001") ? "проверена " + esc(dt(r.verified_at)) : '<span class="vec missing">ни разу не запускалась</span>'}`
      : "";
  return `<div class="kv">${kv("вид", TYPES[n.type].one)}${kv("статус", STATUSES[n.status] || esc(n.status))}${kv("core", n.core ? "да" : "нет")}${kv("версия", "v" + n.ver)}
   ${r ? kv("создана", esc(dt(r.created_at))) + kv("обновлена", esc(dt(r.updated_at))) + kv("действует с", esc(dt(r.valid_from))) + kv("действует до", esc(dt(r.valid_to))) : kv("обновлена", ageStr(n.age) + (n.age >= 1 ? " назад" : ""))}
   ${kv("ttl", n.type === "state" && n.ttl ? n.ttl + " дн." + (d && d.expires ? " до " + esc(dt(d.expires)) : "") : "")}
   ${r ? kv("источник", r.source ? `<span class="mono">${esc(r.source)}</span>` : "") : kv("источник", n.source ? `<span class="mono">${esc(n.source)}</span>` : "")}
   ${r && r.superseded_by ? kv("заменена записью", BYKEY.get(r.superseded_by) ? `<button class="wl" data-id="${BYKEY.get(r.superseded_by).id}">${esc(r.superseded_by)}</button>` : esc(r.superseded_by)) : ""}
   ${kv("проверка", ver)}${kv("вектор", vecState)}${kv("куски", n.chunks > 1 ? String(n.chunks) : "")}</div>
   <div class="scopes">${n.scopes.map(scopeChip).join("")}</div>`;
}
function renderResolved() {
  const R = S.resolved;
  if (!R) return "";
  return `<section class="resolved"><h3>Ссылка ${esc(R.token)} <span>${R.keys.length}</span></h3>${R.keys.map((k) => (BYKEY.get(k) ? recRow(BYKEY.get(k)) : nogRow(k, "нет в графе"))).join("")}<button class="btn mini" data-clearref>Закрыть</button></section>`;
}
function emptyDetail() {
  const last = TURNS[TURNS.length - 1];
  const core = NODES.filter((x) => x.dangerous).length,
    open = NODES.filter((x) => x.open).length;
  return `<div class="d-empty"><h2>Запись не выбрана</h2><p>Нажмите на узел графа, строку в поиске или ленте. Двойной клик приближает узел, Esc снимает выбор. Узел можно потянуть: соседи идут за ним, отпущенный возвращается на место.</p>
    ${last ? `<section><h3>Последний ход <span>${esc(last.time)} ${esc(last.sid)}</span></h3><p style="margin:0 0 8px;color:var(--text)">${turnWhat(last)}</p>${last.items.length ? `<ul class="items">${last.items.map(itemRow).join("")}</ul>` : '<div class="empty" style="padding:4px 0">Хук ничего не подтянул</div>'}</section>` : ""}
    <section><h3>Состояние памяти</h3><div class="health">
      <span>Опасных записей (core-правила)</span><b style="color:var(--danger)">${core}</b>
      <span>Открытых статусов (ttl или verify)</span><b>${open}</b>
      <span>Проблем гигиены</span><b>${HYG ? HYG.count : 0}</b></div>
      <button class="btn" data-tabgo="hyg">Открыть гигиену</button> <button class="btn" data-tabgo="health">Здоровье</button></section></div>`;
}
function renderDetail() {
  const n = S.selected;
  D.classList.toggle("has", !!n);
  if (!n) {
    D.innerHTML = emptyDetail();
    return;
  }
  const c = DET.get(n.key),
    d = c && c.data,
    refs = d ? d.refs : null;
  const t = TOPICS[n.topic];
  const flags = [];
  if (n.lastTurn === turnNo && turnNo > 0)
    flags.push(`<span class="flag now">используется в этом ходе</span>`);
  if (n.dangerous)
    flags.push(`<span class="flag danger">опасная: core-правило</span>`);
  else if (n.core) flags.push(`<span class="flag core">core</span>`);
  if (n.status === "superseded")
    flags.push(`<span class="flag sup">заменена</span>`);
  if (n.status === "expired")
    flags.push(`<span class="flag open">срок истёк</span>`);
  if (n.status === "retired") flags.push(`<span class="flag sup">снята</span>`);
  if (n.open)
    flags.push(`<span class="flag open">открыто: ttl или verify</span>`);
  if (n.trig) flags.push(`<span class="flag">триггеров: ${n.trig}</span>`);
  const flagged = itemsOf("orphans").find((i) => i.k === n.key);
  if (flagged)
    flags.push(
      `<span class="flag open" title="Раздел гигиены">сирота: ${flagged.w.map((w) => esc(own(REASON, w) || w)).join(", ")}</span>`,
    );
  if (n.broken.length)
    flags.push(
      `<span class="flag danger">битых ссылок: ${n.broken.length}</span>`,
    );
  const succ = n.succKey && BYKEY.get(n.succKey);
  const trig =
    d && d.triggers.length
      ? `<section><h3>Триггеры инструментов <span>${d.triggers.length}</span></h3>${d.triggers.map((x) => `<div class="trg"><span class="tier tool">${esc(x.tool)}</span><code>${esc(x.regex)}</code>${x.ok ? "" : `<span class="warn-t d">${esc(x.error)}</span>`}</div>`).join("")}</section>`
      : "";
  D.innerHTML = `<div class="d-key">${esc(n.key)}</div><p class="d-sum">${esc(n.summary.replace(/`/g, ""))}</p>
  <div class="d-meta"><span class="ty"><span class="dot" style="--c:${TYPES[n.type].c}"></span>${TYPES[n.type].one}</span><button class="linkbtn" data-topic="${t.i}">${esc(t.label)}</button><span>обновлена ${ageStr(n.age)}${n.age >= 1 ? " назад" : ""}</span><span class="mono">v${n.ver}</span></div>
  ${flags.length ? `<div class="flags">${flags.join("")}</div>` : ""}
  ${c && c.err ? `<p class="vnote">Не удалось загрузить подробности: ${esc(c.err)}</p>` : ""}
  ${renderResolved()}
  ${succ ? `<section><h3>Заменена записью</h3>${recRow(succ)}</section>` : n.status === "superseded" ? `<section><h3>Заменена</h3>${empty("Преемник не указан")}</section>` : ""}
  <section><h3>Поля</h3>${fieldsHtml(n, d)}</section>
  <section><h3>Текст</h3><div class="d-text">${d && d.record.body != null ? richText(n, d.record.body, refs) : empty("Загрузка...")}</div></section>
  ${trig}
  <section><h3>Связи <span>${d ? d.links.length : ""}</span></h3>${linksHtml(n, d)}</section>
  <section><h3>История версий</h3>${histHtml(n, d)}</section>
  <section><h3>Подавалась агенту</h3>${injectedHtml(d)}</section>
  <div class="d-actions"><button class="btn" data-reveal="${n.id}">${S.tab !== "graph" ? "Показать на графе" : "Приблизить"}</button><button class="btn" data-clear>Снять выбор</button></div>`;
}

function select(n, opts = {}) {
  if (n.hidden) resetFilters();
  if (S.selected !== n) EGO = 1;
  S.selected = n;
  S.turnFocus = null;
  S.resolved = null;
  loadDetail(n);
  renderDetail();
  renderChip();
  if (opts.reveal) {
    setTab("graph");
    const k = Math.max(cam.k, 1.5);
    camTo(n.x, n.y, k, undefined, n);
  }
  NAV.step("node");
}
function clearFocus() {
  EGO = 1;
  S.selected = null;
  S.topicFocus = null;
  S.turnFocus = null;
  S.resolved = null;
  renderDetail();
  renderTopics();
  renderChip();
  renderStrip();
  NAV.step("map");
}
function focusTopic(i, opts = {}) {
  EGO = 1;
  if (S.topicFocus === i && !opts.force) {
    S.topicFocus = null;
  } else {
    S.topicFocus = i;
    S.selected = null;
    S.turnFocus = null;
    setTab("graph");
    if (!opts.noFit) fitNodes(TOPICS[i].members, 70);
  }
  renderTopics();
  renderChip();
  renderDetail();
  NAV.step(S.topicFocus == null ? "map" : "topic");
}
function focusTurn(tn) {
  const T = TURNS.find((t) => t.n === tn);
  if (!T) return;
  EGO = 1;
  if (S.turnFocus && S.turnFocus.n === tn) {
    S.turnFocus = null;
  } else {
    const ns = T.items.filter((x) => x.node).map((x) => x.node);
    S.turnFocus = { n: tn, set: new Set(ns) };
    S.selected = null;
    S.topicFocus = null;
    setTab("graph");
    fitNodes(ns, 120);
  }
  renderChip();
  renderStrip();
  renderDetail();
  renderTopics();
  NAV.step(S.turnFocus ? "turn" : "map");
}
function crumbHtml() {
  const parts = [];
  const map = '<button class="crumb" data-crumb="map">Карта</button>';
  const tcrumb = (T) =>
    `<button class="crumb" data-crumb="${T.i}" title="Вся тема">${esc(T.label)}</button>`;
  if (S.turnFocus) {
    const T = TURNS.find((x) => x.n === S.turnFocus.n);
    return T
      ? `${map}<i class="sep">›</i><span class="cur">${esc(T.time)} ${esc(T.sid)}: ${turnWhat(T)}</span>`
      : "";
  }
  if (S.selected) {
    const n = S.selected;
    parts.push(
      map,
      tcrumb(TOPICS[n.topic]),
      `<span class="cur mono">${esc(n.key)}</span>${EGO > 1 ? `<em class="cnt">глубина ${EGO}</em>` : ""}`,
    );
    return parts.join('<i class="sep">›</i>');
  }
  if (S.topicFocus != null) {
    const T = TOPICS[S.topicFocus];
    return `${map}<i class="sep">›</i><span class="cur">${esc(T.label)}</span><em class="cnt" title="${T.size} ${plural(T.size, ["запись", "записи", "записей"])}">${T.size}</em>`;
  }
  if (FILTER.topic != null) {
    const T = TOPICS[FILTER.topic];
    return `${map}<i class="sep">›</i><span class="cur">Только тема «${esc(T.label)}»</span>`;
  }
  return "";
}
function renderChip() {
  const c = $("#gchip");
  const t = crumbHtml();
  const only = $("#gchipOnly");
  c.classList.toggle("on", !!t);
  $("#gchipT").innerHTML = t;
  GFX.invalidate();
  only.hidden = S.topicFocus == null;
  only.textContent =
    FILTER.topic === S.topicFocus ? "показать все темы" : "только эта тема";
}
$("#gchipOnly").onclick = () => {
  if (S.topicFocus == null) return;
  FILTER.topic = FILTER.topic === S.topicFocus ? null : S.topicFocus;
  S.topicFocus = null;
  refilter();
  fitView();
  NAV.step("map");
};
$("#gchipX").onclick = () => {
  S.selected = null;
  S.topicFocus = null;
  S.turnFocus = null;
  FILTER.topic = null;
  EGO = 1;
  refilter();
  NAV.step("map");
};

function setTab(t) {
  S.tab = t;
  document
    .querySelectorAll(".tab")
    .forEach((b) => b.setAttribute("aria-selected", b.dataset.tab === t));
  document
    .querySelectorAll(".view")
    .forEach((v) => v.classList.toggle("on", v.id === "v-" + t));
  if (t !== "graph") NAV.palette.close();
  if (t === "graph") {
    resize();
    GFX.invalidate();
  }
  if (t === "feed") renderFeed();
  if (t === "hyg") refreshHyg();
  if (t === "health") refreshHealth();
  if (t === "core") refreshCore();
  hideTip();
  if (S.selected) renderDetail();
}
document.querySelector(".tabs").addEventListener("click", (e) => {
  const b = e.target.closest(".tab");
  if (b) setTab(b.dataset.tab);
});

const STOP = new Set(
  "на по не ни из за от до для при без про как что это или ли же бы то так уже ещё еще опять снова почему где когда там тут его её их мне все всё после перед через под над чем кто можно нужно надо and the for with from into that this not are was what why how".split(
    " ",
  ),
);
const tok = (s) =>
  s
    .toLowerCase()
    .split(/[^a-zа-яё0-9_]+/i)
    .filter((w) => w.length >= 2 && !STOP.has(w));
const stem = (w) => {
  if (w.length < 4) return w;
  let s = w;
  for (let i = 0; i < 2 && s.length > 3 && /[аяыиеоуюйьё]$/.test(s); i++)
    s = s.slice(0, -1);
  return s;
};
const escRe = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
function hilite(text, st) {
  if (!st.length) return esc(text);
  const re = new RegExp("(" + st.map(escRe).join("|") + ")[a-zа-яё0-9]*", "gi");
  let out = "",
    last = 0,
    m;
  while ((m = re.exec(text)) && m[0]) {
    out += esc(text.slice(last, m.index)) + "<mark>" + esc(m[0]) + "</mark>";
    last = m.index + m[0].length;
  }
  return out + esc(text.slice(last));
}
const SQ = {
  mode: "fused",
  rerank: false,
  status: "current",
  logs: false,
  kinds: new Set(),
  project: "",
};
const SMODES = {
  words: "слова",
  trigram: "триграммы",
  fused: "слова+триграммы",
  vector: "вектор",
  hybrid: "гибрид",
};
let HEALTH = null,
  searchSeq = 0,
  searchTimer = null,
  lastSearch = null;
const svcOk = (k) =>
  HEALTH && HEALTH[k] && HEALTH[k].configured && HEALTH[k].ok;
function renderSctl() {
  const projects = [...new Set(NODES.flatMap((n) => n.projects))].sort();
  const mode = Object.keys(SMODES)
    .map((m) => {
      const need = m === "vector" || m === "hybrid";
      const dis = need && HEALTH && !svcOk("embedder");
      return `<button data-smode="${m}" aria-pressed="${SQ.mode === m}"${dis ? " disabled" : ""} title="${dis ? "Эмбеддер не отвечает или не настроен" : ""}">${SMODES[m]}</button>`;
    })
    .join("");
  const rdis = HEALTH && !svcOk("reranker");
  $("#sctl").innerHTML =
    `<div class="seg" role="group" aria-label="Режим поиска">${mode}</div>
   <button class="tgl" data-srerank aria-pressed="${SQ.rerank}"${rdis ? " disabled" : ""} title="${rdis ? "Реранкер не отвечает или не настроен" : "Переоценить первые 12 результатов кросс-энкодером"}">реранкер</button>
   <div class="seg" role="group" aria-label="Статус"><button data-sstatus="current" aria-pressed="${SQ.status === "current"}">актуальные</button><button data-sstatus="all" aria-pressed="${SQ.status === "all"}">любые статусы</button></div>
   <button class="tgl" data-slogs aria-pressed="${SQ.logs}">логи</button>
   <div class="seg" role="group" aria-label="Вид">${TKEYS.filter(
     (k) => k !== "log",
   )
     .map(
       (k) =>
         `<button data-skind="${k}" aria-pressed="${SQ.kinds.has(k)}" style="--c:${TYPES[k].c}">${TYPES[k].one}</button>`,
     )
     .join("")}</div>
   <label class="sel">проект <select id="sproj"><option value="">все</option>${projects.map((p) => `<option value="${esc(p)}"${SQ.project === p ? " selected" : ""}>${esc(p)}</option>`).join("")}</select></label>`;
  const sel = $("#sproj");
  if (sel)
    sel.onchange = () => {
      SQ.project = sel.value;
      redoSearch();
    };
}
function renderExamples() {
  const topics = TOPICS.filter((t) => t.id !== "")
    .sort((a, b) => b.size - a.size)
    .slice(0, 8)
    .map((t) => t.label);
  const seen = new Set(),
    recent = [];
  for (let i = TURNS.length - 1; i >= 0 && recent.length < 5; i--) {
    const T = TURNS[i];
    if (T.kind !== "prompt" || T.status === "skipped" || !T.prompt) continue;
    const p = T.prompt.slice(0, 48);
    if (!seen.has(p)) {
      seen.add(p);
      recent.push(p);
    }
  }
  $("#examples").innerHTML =
    (recent.length
      ? `<span class="exh">недавние запросы</span>` +
        recent
          .map((x) => `<button data-ex="${esc(x)}">${esc(x)}</button>`)
          .join("")
      : "") +
    (topics.length
      ? `<span class="exh">темы</span>` +
        topics
          .map((x) => `<button data-ex="${esc(x)}">${esc(x)}</button>`)
          .join("")
      : "");
}
const bar = (label, v, text, title, ticks) =>
  `<span title="${esc(title || "")}">${label}</span><span class="bar" style="${(ticks || []).map((t, i) => `--t${i + 1}:${t * 100}%`).join(";")}"><i style="width:${clamp(v, 0, 1) * 100}%;background:${C.bright}"></i>${(ticks || []).map((t, i) => `<u class="tick" style="left:${t * 100}%"></u>`).join("")}</span><b>${esc(text)}</b>`;
const rrfBar = (r) => 61 / (60 + r);
function hitBars(h) {
  const rows = [];
  if (h.words_rank)
    rows.push(
      bar(
        "слова",
        rrfBar(h.words_rank),
        "#" + h.words_rank,
        `место ${h.words_rank} в списке слов, bm25 ${f3(h.words_bm25)}`,
      ),
    );
  if (h.tri_rank)
    rows.push(
      bar(
        "триграммы",
        rrfBar(h.tri_rank),
        "#" + h.tri_rank,
        `место ${h.tri_rank} в списке триграмм, bm25 ${f3(h.tri_bm25)}`,
      ),
    );
  if (h.cos != null)
    rows.push(
      bar(
        "косинус",
        h.cos,
        f3(h.cos),
        "косинус запроса и документа" +
          (h.vec_rank ? `; место ${h.vec_rank} среди векторов` : ""),
        [GATES.cos],
      ),
    );
  if (h.rerank != null)
    rows.push(
      bar(
        "реранк",
        h.rerank,
        f3(h.rerank),
        "оценка кросс-энкодера; пороги: указатель " +
          GATES.rerank_pointer +
          ", текст " +
          GATES.rerank_text,
        [GATES.rerank_pointer, GATES.rerank_text],
      ),
    );
  if (!rows.length)
    rows.push(
      bar(
        "счёт",
        (h.score * 61) / 1.3,
        (+h.score).toFixed(4),
        "RRF по спискам слов и триграмм",
      ),
    );
  else if (
    h.score &&
    h.vec_rank &&
    h.cos != null &&
    (h.words_rank || h.tri_rank)
  )
    rows.push(
      bar(
        "сплав",
        (h.score * 61) / 1.3,
        (+h.score).toFixed(4),
        "RRF по словам, триграммам и векторам",
      ),
    );
  return rows.join("");
}
const DEGR = {
  "not-configured": "не настроен",
  down: "недоступен",
  timeout: "таймаут",
  http: "ошибка HTTP",
  response: "неверный ответ",
  dimension: "не совпала размерность",
  "no-vectors": "нет векторов",
  store: "ошибка базы",
};
function renderResults(q, st, res) {
  const box = $("#results");
  const list = res.list;
  if (!list.length && !res.hiddenN) {
    box.innerHTML = `<div class="empty">По запросу «${esc(q)}» ничего не нашлось. Попробуйте короче, другими словами или смените режим.</div>`;
    return;
  }
  const dg = Object.entries(res.degraded || {}).map(
    ([k, v]) =>
      `${k === "embed" ? "эмбеддер" : "реранкер"}: ${esc(own(DEGR, v) || v)}`,
  );
  const head = `${list.length} ${plural(list.length, ["запись", "записи", "записей"])}, режим ${esc(SMODES[res.mode] || res.mode)}${res.asked !== res.mode ? ` (запрошен ${esc(SMODES[res.asked] || res.asked)})` : ""}${res.reranked ? ", порядок по реранкеру" : ""}${res.embedMs ? `, embed ${res.embedMs} мс` : ""}${res.rerankMs ? `, реранк ${res.rerankMs} мс` : ""}`;
  box.innerHTML =
    `<div class="empty" style="padding:0 0 8px">${head}${dg.length ? ` <span class="warn-t o">${dg.join("; ")}</span>` : ""}${res.hiddenN ? ` <button class="linkbtn" data-reset-filters>скрыто фильтрами: ${res.hiddenN}</button>` : ""}</div>` +
    list
      .map(({ n, h }) =>
        n
          ? `<button class="res" data-id="${n.id}">
    <span><span class="k"><span class="dot" style="--c:${n.color}"></span><span>${hilite(n.key, st)}</span>${n.core ? '<span class="tier core">core</span>' : ""}${n.dangerous ? '<span class="warn-t d">опасная</span>' : ""}${n.status !== "current" ? `<span class="warn-t o">${esc(STATUSES[n.status] || n.status)}</span>` : ""}</span><span class="s" style="display:block">${hilite(n.summary.replace(/`/g, ""), st)}</span>
    ${h.snippet && h.hit !== n.key ? `<span class="t" style="display:block">${hilite(h.snippet, st)}</span>` : ""}
    <span class="t" style="display:block">${TYPES[n.type].one}, ${esc(TOPICS[n.topic].label)}, ${ageStr(n.age)}</span></span>
    <span class="bars">${hitBars(h)}</span></button>`
          : `<div class="res nog"><span><span class="k"><span class="dot" style="--c:var(--mute1)"></span>${esc(h.key)}</span><span class="s" style="display:block">нет в графе: граф обновится сам</span></span><span class="bars">${hitBars(h)}</span></div>`,
      )
      .join("");
}
function searchUrl(q) {
  const p = new URLSearchParams({
    q,
    limit: "40",
    mode: SQ.mode,
    status: SQ.status,
    logs: SQ.logs ? "1" : "0",
  });
  if (SQ.rerank) p.set("rerank", "1");
  for (const k of SQ.kinds) p.append("kind", k);
  if (SQ.project) p.set("project", SQ.project);
  return "/api/search?" + p;
}
function fetchSearch(q) {
  const seq = ++searchSeq;
  return jget(searchUrl(q)).then((j) => {
    if (seq !== searchSeq) return null;
    const st = [...new Set(tok(q).map(stem))];
    let hiddenN = 0,
      missing = false;
    const list = [];
    for (const h of arr(j.hits)) {
      const n = BYKEY.get(h.key);
      if (!n) missing = true;
      if (n && n.hidden && filtersActive()) {
        hiddenN++;
        continue;
      }
      list.push({ n, h });
    }
    const res = {
      q,
      list,
      hiddenN,
      mode: str(j.mode),
      asked: str(j.asked),
      degraded: j.degraded || {},
      reranked: !!j.reranked,
      embedMs: num(j.embed_ms),
      rerankMs: num(j.rerank_ms),
    };
    lastSearch = res;
    renderResults(q, st, res);
    NAV.palette.results(res);
    if (missing) checkFingerprint();
    return list;
  });
}
function runSearch(q) {
  const box = $("#results");
  clearTimeout(searchTimer);
  if (!q.trim()) {
    searchSeq++;
    lastSearch = null;
    box.innerHTML =
      '<div class="empty">Введите слова или вопрос. Режимы вектор и гибрид идут через эмбеддер, реранкер переоценивает первые результаты; если сервис недоступен, поиск откатится на слова с пометкой.</div>';
    return;
  }
  searchTimer = setTimeout(
    () => {
      fetchSearch(q).catch((e) => {
        box.innerHTML =
          '<div class="empty">Поиск недоступен: ' + esc(e.message) + "</div>";
      });
    },
    SQ.mode === "fused" || SQ.mode === "words" || SQ.mode === "trigram"
      ? 120
      : 350,
  );
}
function redoSearch() {
  if (q1.value.trim()) runSearch(q1.value);
}
const q1 = $("#q"),
  q2 = $("#q2");
q1.addEventListener("input", () => {
  q2.value = q1.value;
  if (S.tab === "graph") NAV.palette.open();
  else if (S.tab !== "search") setTab("search");
  runSearch(q1.value);
});
q2.addEventListener("input", () => {
  q1.value = q2.value;
  runSearch(q2.value);
});
q1.addEventListener("keydown", (e) => {
  if (NAV.palette.key(e)) return;
  if (e.key !== "Enter") return;
  e.preventDefault();
  const q = q1.value;
  if (!q.trim()) return;
  clearTimeout(searchTimer);
  const go = (list) => {
    const f = NAV.palette.pick(list) || (list && list.find((x) => x.n));
    if (f) {
      NAV.palette.close();
      select(f.n, { reveal: true });
      q1.blur();
      ov.focus({ preventScroll: true });
    }
  };
  if (lastSearch && lastSearch.q === q) go(lastSearch.list);
  else
    fetchSearch(q)
      .then(go)
      .catch(() => {});
});
$("#sctl").addEventListener("click", (e) => {
  const b = e.target.closest("button");
  if (!b || b.disabled) return;
  if (b.dataset.smode) SQ.mode = b.dataset.smode;
  else if (b.hasAttribute("data-srerank")) SQ.rerank = !SQ.rerank;
  else if (b.dataset.sstatus) SQ.status = b.dataset.sstatus;
  else if (b.hasAttribute("data-slogs")) SQ.logs = !SQ.logs;
  else if (b.dataset.skind) toggleIn(SQ.kinds, b.dataset.skind);
  else return;
  renderSctl();
  redoSearch();
});

let TURN_CAP = 80;
const LIVE_MS = 2000,
  ANIM_LAST = 3;
const injectedOf = (T) => T.items.filter((x) => x.tier !== "ghost");
function turnWhat(T) {
  if (T.kind === "tool")
    return `<span class="tk">${esc(T.toolName)}</span>${T.input ? ` <span class="tin">${esc(T.input)}</span>` : ""}`;
  if (T.kind === "session")
    return `старт сессии${T.marker ? ` (${esc(T.marker)})` : ""}`;
  if (T.status === "skipped")
    return `пропущено: ${esc(T.marker || "не ввод пользователя")}`;
  return `«${esc(T.prompt)}${T.cut ? "..." : ""}»`;
}
function embedStr(T) {
  const p = [];
  if (T.kind === "session")
    return [
      T.topics ? `карта тем ${T.topics}` : "",
      T.chars ? `${T.chars} симв.` : "",
      T.totalMs != null ? `всего ${T.totalMs} мс` : "",
    ]
      .filter(Boolean)
      .join(", ");
  if (T.kind === "tool")
    return [
      T.chars ? `${T.chars} симв.` : "",
      T.totalMs != null ? `всего ${T.totalMs} мс` : "",
    ]
      .filter(Boolean)
      .join(", ");
  if (T.status === "skipped") return "";
  if (T.status === "error")
    return T.totalMs != null ? `всего ${T.totalMs} мс` : "";
  p.push(
    T.mode === "vector"
      ? `${T.pool || "vector"}, embed ${T.embed || "-"}${T.embedMs != null && T.embed ? " " + T.embedMs + " мс" : ""}`
      : T.mode || "lexical",
  );
  if (T.rerank)
    p.push(`реранк ${T.rerank}${T.rerankMs ? " " + T.rerankMs + " мс" : ""}`);
  if (T.chars) p.push(`${T.chars} симв.`);
  if (T.totalMs != null) p.push(`всего ${T.totalMs} мс`);
  if (T.shadow)
    p.push(
      `тень ${T.shadow.pool || "-"} ${T.shadow.embed || "-"}${T.shadow.embedMs != null ? " " + T.shadow.embedMs + " мс" : ""}`,
    );
  return p.join(", ");
}
function warnFor(n) {
  const w = [];
  if (n.status === "superseded")
    w.push('<span class="warn-t d">заменена</span>');
  if (n.status === "expired")
    w.push('<span class="warn-t o">срок истёк</span>');
  if (n.dangerous) w.push('<span class="warn-t d">опасная</span>');
  return w.join("");
}
function itemRow(x) {
  const sc = `${own(TIER, x.tier) ? `<span class="tier ${x.tier}">${own(TIER, x.tier)}</span>` : ""}${x.cos != null ? `<span class="sc">${x.cos.toFixed(3)}</span>` : ""}${x.rr != null ? `<span class="sc rr">r ${x.rr.toFixed(3)}</span>` : ""}`;
  if (x.node)
    return `<li class="${x.tier}">${recRow(x.node, `${x.tier === "ghost" ? "" : warnFor(x.node)}${sc}`)}</li>`;
  return `<li class="${x.tier}"><div class="rrow nog"><span class="dot" style="--c:var(--mute1)"></span><span class="k">${esc(x.key)}</span><span class="s">нет в графе</span>${sc}</div></li>`;
}
const STATUS_RU = {
  none: "ничего не подано",
  skipped: "пропущен",
  error: "ошибка хука",
  "no-db": "нет базы",
  deny: "запрет",
};
function turnHead(T, i) {
  const kind =
    T.kind === "tool" ? "tool" : T.kind === "session" ? "session" : "turn";
  const where = [T.project, T.host, T.harness]
    .filter(Boolean)
    .map(esc)
    .join(" / ");
  const agent = T.agent
    ? ` ${esc(T.agentType || "агент")} ${esc(T.agent.slice(0, 6))}`
    : "";
  return `<div class="turn-h"><button class="n" data-turn="${T.n}">${kind} ${T.n}</button><span class="tm">${esc(T.time)}</span><span class="sid" title="${esc(T.session + (T.agent ? " / " + T.agent : "") + (T.cwd ? " / " + T.cwd : ""))}">${esc(T.sid)}${agent}</span>${where ? `<span class="whr" title="${esc(T.cwd)}">${where}</span>` : ""}${T.status && own(STATUS_RU, T.status) ? `<span class="warn-t ${T.status === "error" ? "d" : "o"}">${own(STATUS_RU, T.status)}</span>` : ""}${T.origin === "hook.log" ? '<span class="tier ghost" title="Старый формат hook.log: нет текста запроса, теней и реранка">hook.log</span>' : ""}<span class="p">${turnWhat(T)}</span><span class="em">${esc(embedStr(T))}</span></div>`;
}
function firedHtml(T) {
  return T.fired.length
    ? `<div class="fired">${T.fired.map((f) => `<span>сработал <code>${esc(f.regex)}</code> на ${esc(f.tool)} для ${esc(f.key)}</span>`).join("")}</div>`
    : "";
}
function candHtml(T) {
  if (!T.reranked.length) return "";
  return `<details class="cands"><summary>Кандидаты реранкера (${T.reranked.length})</summary>${T.reranked.map((x) => `<div class="cand"><span class="k">${esc(x.key)}</span><span class="sc">cos ${x.cos == null ? "-" : x.cos.toFixed(3)}</span><span class="sc rr">r ${x.rr == null ? "-" : x.rr.toFixed(3)}</span></div>`).join("")}</details>`;
}
let turnSkipped = 0;
function renderFeed() {
  const lead = $("#feedLead");
  const by = { turns: 0, log: 0, skipped: 0, error: 0 };
  for (const T of TURNS) {
    if (T.origin === "hook.log") by.log++;
    else by.turns++;
    if (T.status === "skipped") by.skipped++;
    if (T.status === "error") by.error++;
  }
  lead.textContent = `${TURNS.length} ${plural(TURNS.length, ["ход", "хода", "ходов"])} в окне: ${by.turns} из turns.jsonl, ${by.log} из hook.log старого формата без текста и теней; пропущено ${by.skipped}, ошибок ${by.error}. Сплошная дуга: запись подана целиком, пунктир: указателем, точки: нашёл только теневой пул. Квадратная метка: триггер перед вызовом инструмента.`;
  $("#feedN").textContent = TURNS.length;
  if (S.tab !== "feed") return;
  renderSessions();
  const more =
    turnSkipped > 0
      ? `<button class="btn" data-morefeed>Показать ещё ${Math.min(160, turnSkipped)} ранее (скрыто ${turnSkipped})</button>`
      : "";
  $("#feed").innerHTML = TURNS.length
    ? TURNS.slice()
        .reverse()
        .map(
          (
            T,
            i,
          ) => `<div class="turn${i === 0 ? " latest" : ""}${T.kind === "tool" ? " tool" : ""}${T.kind === "session" ? " sess" : ""}${T.status === "error" ? " err" : ""}${T.status === "skipped" ? " skp" : ""}">${turnHead(T, i)}${firedHtml(T)}
   <ul>${T.items.map(itemRow).join("")}</ul>${candHtml(T)}</div>`,
        )
        .join("") + more
    : `<div class="empty">${turnErr ? "Лента недоступна: " + esc(turnErr) : "Ходов пока нет. Хук пишет их в turns.jsonl, лента опрашивает его каждые две секунды, пока включён live."}</div>`;
}
function renderStrip() {
  const last = TURNS.slice(-14).reverse();
  $("#turns").innerHTML = last
    .map((T, i) => {
      const inj = injectedOf(T);
      return `<button class="tchip${T.kind === "tool" ? " tool" : ""}${i === 0 ? " latest" : ""}${S.turnFocus && S.turnFocus.n === T.n ? " on" : ""}${T.fresh ? " enter" : ""}${T.status === "error" ? " err" : ""}${T.status === "skipped" ? " skp" : ""}" data-turn="${T.n}" title="${esc(T.time + " " + T.session + ": " + (T.kind === "tool" ? T.toolName + " " + T.input : T.kind === "session" ? "старт сессии" : T.prompt))}"><span class="n">${T.kind === "tool" ? '<i class="tm"></i>' + esc(T.toolName.slice(0, 16)) : T.kind === "session" ? "старт " + esc(T.sid) : esc(T.sid)}</span><span class="ds">${T.items
        .filter((x) => x.node)
        .map(
          (x) =>
            `<i class="${x.tier}" style="--c:${TYPES[x.node.type].c}"></i>`,
        )
        .join(
          "",
        )}</span><span class="c">${inj.length} ${plural(inj.length, ["запись", "записи", "записей"])}</span></button>`;
    })
    .join("");
  $("#stripH").textContent = S.session
    ? "Последние ходы, сессия " + S.session.slice(0, 8)
    : "Последние ходы";
  for (const T of TURNS) T.fresh = false;
}
let SESSIONS = [];
function renderSessions() {
  const box = $("#sessions");
  const cnt = (x) => (x.prompts | 0) + (x.tools | 0) + (x.starts | 0);
  const total = SESSIONS.reduce((s, x) => s + cnt(x), 0);
  const list = SESSIONS.slice(0, 24);
  if (S.session && !list.some((x) => x.session === S.session))
    list.push({ session: S.session, prompts: 0, tools: 0, last: "" });
  box.innerHTML =
    `<button data-session="" aria-pressed="${!S.session}">все сессии <b>${total}</b></button>` +
    list
      .map((x) => {
        const s = str(x.session);
        const where = [str(x.project), str(x.host), str(x.harness)]
          .filter(Boolean)
          .join(" / ");
        return `<button class="${x.test ? "test" : ""}" data-session="${esc(s)}" aria-pressed="${S.session === s}" title="${esc(s + (where ? ", " + where : "") + (x.last ? ", последний ход " + hms(str(x.last)) : "") + (x.test ? ", похоже на тестовую сессию" : ""))}">${esc(sidOf(s))}${where ? ` <i>${esc(str(x.project) || str(x.host))}</i>` : ""} <b>${cnt(x)}</b></button>`;
      })
      .join("");
}
function loadSessions() {
  jget("/api/sessions")
    .then((j) => {
      SESSIONS = arr(j.sessions);
      renderSessions();
    })
    .catch(() => {});
}

function turnItems(t) {
  const out = [],
    seen = new Set();
  const add = (key, cos, tier, rr) => {
    key = str(key);
    if (!key || seen.has(key)) return;
    seen.add(key);
    out.push({ key, cos: num(cos), rr: num(rr), tier, node: nodeOf(key) });
  };
  const hit = (h, tier) => {
    if (h && typeof h === "object") add(h.key, h.cos, tier, h.rerank);
  };
  if (t.event === "tool") {
    for (const k of arr(t.keys)) add(k, null, "tool");
    return out;
  }
  if (t.event === "session") {
    for (const k of arr(t.keys)) add(k, null, "session");
    return out;
  }
  for (const h of arr(t.strong)) hit(h, "strong");
  for (const k of arr(t.lexical)) add(k, null, "lexical");
  for (const h of arr(t.pointers)) hit(h, "pointer");
  if (t.shadow && typeof t.shadow === "object")
    for (const h of arr(t.shadow.only)) hit(h, "ghost");
  return out;
}
let turnCursor = "",
  turnTimer = null,
  turnSeq = 0,
  turnErr = "";
function makeTurn(t, animate, delay) {
  const items = turnItems(t);
  turnNo++;
  const prompt = str(t.prompt);
  const kind =
    t.event === "tool" ? "tool" : t.event === "session" ? "session" : "prompt";
  const sh =
    t.shadow && typeof t.shadow === "object"
      ? {
          pool: str(t.shadow.pool),
          embed: str(t.shadow.embed),
          embedMs: num(t.shadow.embed_ms),
        }
      : null;
  const T = {
    n: turnNo,
    id: str(t.id),
    kind,
    toolName: str(t.tool),
    session: str(t.session),
    sid: sidOf(str(t.session)),
    agent: str(t.agent),
    agentType: str(t.agent_type),
    harness: str(t.harness),
    host: str(t.host),
    project: str(t.project),
    cwd: str(t.cwd),
    input: str(t.input),
    fired: arr(t.fired)
      .filter((f) => f && typeof f === "object")
      .map((f) => ({
        key: str(f.key),
        tool: str(f.tool),
        regex: str(f.regex),
      })),
    prompt,
    cut: t.prompt_cut === true,
    time: hms(str(t.time)),
    status: str(t.status),
    marker: str(t.source),
    origin: str(t.origin),
    mode: str(t.mode),
    pool: str(t.pool),
    topics: num(t.topics),
    chars: num(t.chars),
    embed: str(t.embed),
    embedMs: num(t.embed_ms),
    rerank: str(t.rerank),
    rerankMs: num(t.rerank_ms),
    totalMs: num(t.total_ms),
    shadow: sh,
    items,
    fresh: animate,
    reranked: arr(t.reranked)
      .filter((h) => h && typeof h === "object")
      .map((h) => ({ key: str(h.key), cos: num(h.cos), rr: num(h.rerank) })),
  };
  TURNS.push(T);
  if (TURNS.length > TURN_CAP) TURNS.shift();
  const pnow = performance.now() + (delay || 0);
  injectedOf(T)
    .filter((x) => x.node)
    .forEach((x, i) => {
      const n = x.node;
      n.recalls++;
      n.lastTurn = turnNo;
      if (animate) {
        n.flashAt = pnow + (REDUCED ? 0 : 250 + i * 150 + 780);
        GFX.noteFlash(n.flashAt + 9000);
      }
    });
  const drawn = items.filter((x) => x.node && !x.node.hidden);
  if (animate && drawn.length) {
    const label =
      (kind === "tool" ? T.toolName.slice(0, 24) : T.sid) + " " + T.time;
    arcs.push({
      n: turnNo,
      t0: pnow,
      slot: 0.16 + ((turnNo * 3) % 6) * 0.13,
      items: drawn,
      tool: kind === "tool",
      label,
    });
    GFX.fireRecall(
      turnNo,
      injectedOf(T)
        .filter((x) => x.node)
        .map((x) => ({ node: x.node, t: x.node.flashAt })),
      label,
    );
  }
  GFX.invalidate();
  return T;
}
function ingest(list, animate) {
  const fresh = list.filter(
    (t) =>
      t && typeof t === "object" && !(t.id && TURNS.some((x) => x.id === t.id)),
  );
  if (!fresh.length) return;
  const from = fresh.length - (animate ? Math.min(ANIM_LAST, fresh.length) : 0);
  fresh.forEach((t, i) => {
    const live = i >= from;
    makeTurn(t, live, live ? (i - from) * 900 : 0);
  });
  if (S.turnFocus && !TURNS.some((T) => T.n === S.turnFocus.n))
    S.turnFocus = null;
  if (TURNS.some((T) => T.items.some((x) => !x.node))) checkFingerprint();
  renderStrip();
  renderFeed();
  renderExamples();
  if (!S.selected || S.selected.lastTurn > turnNo - fresh.length)
    renderDetail();
  NAV.turns();
}
function turnQuery() {
  const p = new URLSearchParams();
  if (turnCursor) p.set("since", turnCursor);
  else p.set("limit", String(TURN_CAP));
  if (S.session) p.set("session", S.session);
  return p.toString();
}
function pollTurns(animate) {
  clearTimeout(turnTimer);
  turnTimer = null;
  const seq = ++turnSeq;
  jget("/api/turns?" + turnQuery())
    .then((j) => {
      if (seq !== turnSeq) return;
      const had = turnErr;
      turnErr = "";
      if (typeof j.cursor === "string" && j.cursor) turnCursor = j.cursor;
      if (!turnCursorWas) turnSkipped = j.skipped | 0;
      turnCursorWas = true;
      const list = arr(j.turns);
      if (list.length) {
        ingest(list, animate);
        loadSessions();
      } else if (had) renderFeed();
    })
    .catch((e) => {
      if (seq !== turnSeq) return;
      turnErr = e.message;
      renderFeed();
    })
    .then(() => {
      if (seq === turnSeq && S.live && !NET.live())
        turnTimer = setTimeout(() => pollTurns(true), LIVE_MS);
    });
}
let turnCursorWas = false;
function resetFeed() {
  turnCursor = "";
  turnCursorWas = false;
  turnSeq++;
  clearTimeout(turnTimer);
  turnTimer = null;
  TURNS.length = 0;
  arcs.length = 0;
  GFX.clearRecall();
  S.turnFocus = null;
  for (const n of NODES) {
    n.recalls = 0;
    n.lastTurn = -1;
    n.flashAt = -1e9;
  }
  GFX.state();
  renderChip();
  renderStrip();
  renderFeed();
  renderDetail();
  pollTurns(false);
}
function setLive(on) {
  S.live = on;
  $("#live").setAttribute("aria-checked", on);
  document.body.classList.toggle("is-live", on);
  clearTimeout(turnTimer);
  turnTimer = null;
  turnSeq++;
  if (on) pollTurns(true);
}
function setSession(s) {
  if (s === S.session) return;
  S.session = s;
  resetFeed();
}
$("#live").onclick = () => setLive(!S.live);

function hygRow(id, it) {
  const n = BYKEY.get(it.k);
  if (it.t === "scope")
    return `<div class="rrow nog"><span class="dot" style="--c:var(--mute1)"></span><span class="k">${esc(it.k)}</span><span class="s">${esc(it.d)} ${plural(+it.d, ["запись", "записи", "записей"])}</span></div>`;
  const extra =
    id === "broken"
      ? `<span class="brk" style="white-space:nowrap">${esc(it.d)}</span>`
      : id === "orphans"
        ? arr(it.w)
            .map(
              (w) =>
                `<span class="warn-t o">${esc(own(REASON, w) || w)}</span>`,
            )
            .join("")
        : it.d
          ? `<span class="sc">${esc(it.d)}</span>`
          : "";
  return n ? recRow(n, extra) : nogRow(it.k, "нет в графе", extra);
}
function renderHyg() {
  const lead = $("#hygLead");
  if (!HYG) {
    lead.textContent = "Гигиена недоступна";
    $("#hyg").innerHTML = "";
    return;
  }
  const sv = HYG.surfaced;
  lead.textContent = `В счётчик идут только записи, с которыми можно что-то сделать: ${HYG.count} ${plural(HYG.count, ["запись", "записи", "записей"])}. Заменённые, история правок, очередь проверки и пары «различны» показаны, но не считаются.${sv.applied ? "" : ` Признак «не подавалась 30 сут.» ещё не применяется: журнал ходов покрывает ${sv.covered_days} из ${sv.window_days} сут.`}`;
  const secs = HYG.sections
    .map((s) => {
      const meta = own(HYG_TEXT, s.id) || { t: esc(s.id), d: "", h: "" };
      const items = s.items.filter((it) => {
        const n = BYKEY.get(it.k);
        return it.t === "scope" || !n || !n.hidden || !filtersActive();
      });
      const hid = s.items.length - items.length;
      return `<div class="hsec"><h2>${meta.t}<b class="${s.items.length && s.counted ? "" : "ok"}">${s.items.length}</b>${s.counted ? "" : '<span class="nc">не считается</span>'}</h2><p>${meta.d}${meta.h ? ` <code>${esc(meta.h)}</code>` : ""}${hid ? ` Скрыто фильтрами: ${hid}.` : ""}</p><div class="list">${
        items
          .slice(0, 400)
          .map((it) => hygRow(s.id, it))
          .join("") || '<div class="empty">Чисто</div>'
      }${items.length > 400 ? `<div class="empty">и ещё ${items.length - 400}</div>` : ""}</div></div>`;
    })
    .join("");
  const imp = HYG.imports.length
    ? `<div class="hsec wide"><h2>Импорты по проектам</h2><p>Сколько записей проекта уже получили тему. Работа очереди дистилляции: записи без темы остаются сиротами.</p><div class="imp">${HYG.imports
        .map((r) => {
          const tot = r.records + r.logs;
          const done = tot ? (tot - r.untopiced) / tot : 1;
          return `<div class="irow"><span class="k">${esc(r.project)}</span><span class="bar"><i style="width:${done * 100}%;background:var(--fact)"></i></span><span class="sc">${r.records} + ${r.logs} логов, без темы ${r.untopiced}</span></div>`;
        })
        .join("")}</div></div>`
    : "";
  $("#hyg").innerHTML = secs + imp;
  $("#hygN").textContent = HYG.count;
}
function refreshHyg() {
  jget("/api/hygiene")
    .then((h) => {
      HYG = h;
      renderHyg();
      updateCounts();
      if (S.selected) renderDetail();
    })
    .catch(() => {});
}

const pct = (v) => (v * 100).toFixed(1) + "%";
function svcCard(title, s) {
  if (!s.configured)
    return `<div class="card"><h3>${title}</h3><p class="off">не настроен</p></div>`;
  return `<div class="card"><h3>${title} <i class="pipd ${s.ok ? "ok" : s.reachable ? "warn" : "bad"}"></i></h3><p class="mono">${esc(s.url || "")}</p><p>${s.ok ? "отвечает" : s.reachable ? "HTTP " + s.status : esc(s.error || "не отвечает")}${s.reachable ? `, ${s.ms} мс` : ""}</p><p class="mute">проверено ${esc(hms(s.checked_at))}</p></div>`;
}
function latRows(h) {
  return Object.entries(h.latency)
    .map(
      ([k, l]) =>
        `<tr><td>${esc(k.replace("_ms", ""))}</td><td>${l.n}</td><td>${l.p50}</td><td>${l.p90}</td><td>${l.p99}</td><td>${l.max}</td></tr>`,
    )
    .join("");
}
function histSvg(h, gates, label) {
  const max = Math.max(1, ...h.counts);
  const n = h.counts.length;
  const w = 100 / n;
  const bars = h.counts
    .map(
      (c, i) =>
        `<rect x="${i * w + 0.6}" y="${40 - (c / max) * 38}" width="${w - 1.2}" height="${(c / max) * 38}" fill="var(--fact)"><title>${esc(h.edges[i])} - ${esc(h.edges[i + 1] > 1 ? 1 : h.edges[i + 1])}: ${c}</title></rect>`,
    )
    .join("");
  const lines = gates
    .map((g) => {
      const i = h.edges.findIndex((e, j) => g >= e && g < h.edges[j + 1]);
      const x =
        i < 0
          ? 100
          : (i + (g - h.edges[i]) / (h.edges[i + 1] - h.edges[i])) * w;
      return `<line x1="${x}" x2="${x}" y1="0" y2="40" stroke="var(--live)" stroke-dasharray="2 2" stroke-width=".6"/>`;
    })
    .join("");
  return `<div class="hist"><h4>${label}</h4><svg viewBox="0 0 100 42" preserveAspectRatio="none">${bars}${lines}</svg><div class="hx"><span>${h.edges[0]}</span><span>${h.edges[Math.floor(n / 2)]}</span><span>1</span></div><div class="hn">${h.counts.reduce((a, b) => a + b, 0)} значений</div></div>`;
}
function sweepHtml(sw) {
  if (!arr(sw).length) return "";
  return `<h2 class="h2">Что дал бы другой порог косинуса</h2><p class="lead">Сколько косинусов из журнала не ниже порога: поданные записи, указатели и теневой пул. Сравните с действующим порогом ${GATES.cos}.</p><table class="tbl"><thead><tr><th>порог</th><th>поданные</th><th>указатели</th><th>теневой пул</th></tr></thead><tbody>${sw.map((r) => `<tr${Math.abs(r.t - GATES.cos) < 1e-9 ? ' class="cur"' : ""}><td>${esc(f3(r.t))}</td><td>${r.strong | 0}</td><td>${r.pointer | 0}</td><td>${r.shadow | 0}</td></tr>`).join("")}</tbody></table>`;
}
function renderHealth(h) {
  HEALTH = h;
  GATES.cos = h.gates.cos;
  GATES.rerank_text = h.gates.rerank_text;
  GATES.rerank_pointer = h.gates.rerank_pointer;
  renderSvc();
  renderSctl();
  const v = h.vectors,
    hk = h.hook,
    cal = h.calibration;
  const cov = v.current ? v.fresh / v.current : 0;
  $("#health").innerHTML =
    `<div class="cards">${svcCard("Эмбеддер", h.embedder)}${svcCard("Реранкер", h.reranker)}
   <div class="card"><h3>Векторы</h3><p class="mono">${esc(v.model || "модель не задана")}</p><div class="bar big"><i style="width:${cov * 100}%;background:var(--fact)"></i></div><p>свежих ${v.fresh} из ${v.current} (${pct(cov)}), устарело ${v.stale}, нет ${v.missing}${v.dim ? `, ${v.dim} изм.` : ""}</p>${
     Object.keys(v.missing_by_kind).length
       ? `<p class="mute">нет по видам: ${Object.entries(v.missing_by_kind)
           .map(([k, c]) => esc(k) + " " + c)
           .join(", ")}</p>`
       : ""
   }<p class="mute">Команда: mem reindex</p></div>
   <div class="card"><h3>Хук</h3><p>${hk.turns} ходов${hk.since ? " с " + esc(dt(hk.since)) : ""}</p><p>подача в ${pct(hk.injected_share)} запросов, в среднем ${Math.round(hk.avg_chars)} симв.</p><p>ошибок за сутки <b class="${hk.errors_24h ? "bad" : ""}">${hk.errors_24h}</b></p><p class="mute">${Object.entries(
     hk.by_status,
   )
     .map(([k, c]) => esc(k) + " " + c)
     .join(", ")}</p></div></div>
   <h2 class="h2">Латентность хука, мс</h2><table class="tbl"><thead><tr><th></th><th>n</th><th>p50</th><th>p90</th><th>p99</th><th>max</th></tr></thead><tbody>${latRows(hk)}</tbody></table>
   <h2 class="h2">Счётчики исходов эмбеддера</h2><div class="ctr">${
     Object.entries(h.counters)
       .map(([k, c]) => `<span><b>${c}</b> ${esc(k)}</span>`)
       .join("") || '<span class="mute">пока пусто</span>'
   }</div>
   <h2 class="h2">Калибровка порогов</h2><p class="lead">Распределения из turns.jsonl и shadow.jsonl. Пунктир: действующие пороги (косинус ${h.gates.cos}; реранк: указатель ${h.gates.rerank_pointer}, текст ${h.gates.rerank_text}). Теневых ходов ${cal.shadow_lines}, из них нашла бы лишнее ${cal.shadow_only} записей.</p>
   <div class="hists">${histSvg(cal.strong_cos, [h.gates.cos], "косинус поданных записей")}${histSvg(cal.pointer_cos, [h.gates.cos], "косинус указателей")}${histSvg(cal.shadow_cos, [h.gates.cos], "косинус теневого пула")}${histSvg(cal.rerank, [h.gates.rerank_pointer, h.gates.rerank_text], "оценки реранкера (кандидаты)")}</div>
   ${sweepHtml(cal.sweep)}
   ${hk.last_errors.length ? `<h2 class="h2">Последние ошибки хука</h2>${hk.last_errors.map((e) => `<div class="errl"><span class="tm">${esc(e.time)}</span> <b>${esc(e.event)}</b> ${esc(e.message)}</div>`).join("")}` : ""}`;
}
function renderSvc() {
  const h = HEALTH;
  if (!h) {
    $("#svc").innerHTML = "";
    return;
  }
  const dot = (n, s) =>
    `<span class="sv" title="${n}: ${!s.configured ? "не настроен" : s.ok ? "отвечает, " + s.ms + " мс" : esc(s.error || "HTTP " + s.status)}"><i class="pipd ${!s.configured ? "off" : s.ok ? "ok" : s.reachable ? "warn" : "bad"}"></i>${n}</span>`;
  $("#svc").innerHTML = dot("embed", h.embedder) + dot("rerank", h.reranker);
}
function refreshHealth() {
  return jget("/api/health")
    .then(renderHealth)
    .catch((e) => {
      $("#health").innerHTML =
        '<div class="empty">Здоровье недоступно: ' + esc(e.message) + "</div>";
    });
}

let CORE_Q = { project: "", host: "" };
function renderCore(c) {
  const over = c.size > c.budget;
  const projects = [...new Set(NODES.flatMap((n) => n.projects))].sort(),
    hosts = [...new Set(NODES.flatMap((n) => n.hosts))].sort();
  $("#core").innerHTML =
    `<div class="coreb"><div class="bar big"><i style="width:${clamp(c.size / c.budget, 0, 1) * 100}%;background:${over ? "var(--danger)" : "var(--fact)"}"></i></div><p>memory-core.md: ${c.size} из ${c.budget} симв${over ? ' <b class="bad">бюджет превышен</b>' : ""}. Записи не из global подаются хуком отдельно с бюджетом сессии.</p></div>
   <div class="sctl"><label class="sel">проект <select id="cproj"><option value="">все</option>${projects.map((p) => `<option value="${esc(p)}"${CORE_Q.project === p ? " selected" : ""}>${esc(p)}</option>`).join("")}</select></label>
   <label class="sel">хост <select id="chost"><option value="">все</option>${hosts.map((p) => `<option value="${esc(p)}"${CORE_Q.host === p ? " selected" : ""}>${esc(p)}</option>`).join("")}</select></label></div>
   <table class="tbl"><thead><tr><th>запись</th><th>вид</th><th>симв.</th><th>файл</th><th>области</th><th>обновлена</th></tr></thead><tbody>${c.records
     .map((r) => {
       const n = BYKEY.get(r.key);
       return `<tr><td>${n ? `<button class="linkbtn" data-id="${n.id}">${esc(r.key)}</button>` : esc(r.key)}<div class="mute">${esc(r.title)}</div></td><td>${esc(r.kind)}</td><td><span class="mini-bar"><i style="width:${clamp(r.chars / 400, 0, 1) * 100}%"></i></span> ${r.chars}</td><td>${r.in_file ? "да" : "нет"}</td><td>${r.scopes.map(scopeChip).join("")}</td><td>${esc(dt(r.as_of))}</td></tr>`;
     })
     .join("")}</tbody></table>`;
  const cp = $("#cproj"),
    ch = $("#chost");
  if (cp)
    cp.onchange = () => {
      CORE_Q.project = cp.value;
      refreshCore();
    };
  if (ch)
    ch.onchange = () => {
      CORE_Q.host = ch.value;
      refreshCore();
    };
}
function refreshCore() {
  const p = new URLSearchParams();
  if (CORE_Q.project) p.set("project", CORE_Q.project);
  if (CORE_Q.host) p.set("host", CORE_Q.host);
  return jget("/api/core?" + p)
    .then(renderCore)
    .catch((e) => {
      $("#core").innerHTML =
        '<div class="empty">Core недоступен: ' + esc(e.message) + "</div>";
    });
}

let fpLast = "",
  fpBusy = false;
function checkFingerprint() {
  if (fpBusy) return Promise.resolve();
  fpBusy = true;
  return jget("/api/fingerprint")
    .then((j) => {
      const fp = str(j.fingerprint);
      if (!fpLast) {
        fpLast = fp;
        return;
      }
      if (fp === fpLast) return;
      fpLast = fp;
      return syncGraph();
    })
    .catch(() => {})
    .then(() => {
      fpBusy = false;
    });
}
function renderStale() {
  const el = $("#stale");
  if (el) el.hidden = !(LAYOUT && LAYOUT.stale);
}
function swapGraph(g, h) {
  HYG = h;
  const sel = S.selected && S.selected.key;
  ingestGraph(g, false);
  for (const T of TURNS) for (const x of T.items) x.node = nodeOf(x.key);
  if (sel) S.selected = BYKEY.get(sel) || null;
  DET.clear();
  if (S.selected) loadDetail(S.selected, true);
  pushGraph();
  renderSide();
  updateCounts();
  renderChip();
  renderDetail();
  renderHyg();
  renderFeed();
  renderStrip();
  renderExamples();
  redoSearch();
  renderStale();
  if (S.tab === "core") refreshCore();
  $("#hostP").title = g.dir;
}
function reloadGraph() {
  return Promise.all([
    jget("/api/graph"),
    jget("/api/hygiene").catch(() => HYG),
  ]).then(([g, h]) => {
    if (g.layout && g.layout.state === "building") {
      fpLast = "\0";
      return;
    }
    swapGraph(g, h);
  });
}
let syncBusy = false,
  syncAgain = false;
function syncGraph() {
  if (syncBusy) {
    syncAgain = true;
    return Promise.resolve();
  }
  syncBusy = true;
  const from = DATA && DATA.rev;
  const p = from
    ? Promise.all([
        jget("/api/graph/diff?from=" + encodeURIComponent(from)),
        jget("/api/hygiene").catch(() => HYG),
      ])
        .then(([d, h]) => swapGraph(MemGfx.net.applyDiff(DATA, d), h))
        .catch(() => reloadGraph())
    : reloadGraph();
  return p
    .catch(() => {})
    .then(() => {
      syncBusy = false;
      if (syncAgain) {
        syncAgain = false;
        return syncGraph();
      }
    });
}
const NET = MemGfx.net.create({
  EventSource: window.EventSource,
  hidden: () => document.hidden,
  onGraph: (rev) => {
    if (!DATA || DATA.rev !== rev) syncGraph();
  },
  onTurn: (t, id) => {
    if (!S.live || (S.session && str(t.session) !== S.session)) return;
    if (id) turnCursor = id;
    ingest([t], true);
    loadSessions();
  },
  onOpen: () => {
    if (S.live) pollTurns(turnCursorWas);
  },
  onMode: (m) => {
    if (m !== "sse" && S.live && booted) pollTurns(turnCursorWas);
  },
});
document.addEventListener("visibilitychange", () => {
  if (document.hidden) NET.hide();
  else NET.show();
});

document.addEventListener("click", (e) => {
  const el = e.target.closest(
    "[data-crumb],[data-id],[data-topic],[data-turn],[data-ex],[data-reveal],[data-clear],[data-tabgo],[data-session],[data-ref],[data-clearref],[data-hist],[data-fproj],[data-fhost],[data-reset-filters],[data-morefeed]",
  );
  if (!el) return;
  if (el.dataset.crumb != null) {
    if (el.dataset.crumb === "map") {
      clearFocus();
      fitView();
    } else focusTopic(+el.dataset.crumb, { force: true });
    return;
  }
  if (el.hasAttribute("data-session")) {
    setSession(el.dataset.session || "");
    return;
  }
  if (el.hasAttribute("data-morefeed")) {
    TURN_CAP += 160;
    resetFeed();
    return;
  }
  if (el.hasAttribute("data-reset-filters")) {
    resetFilters();
    return;
  }
  if (el.hasAttribute("data-clearref")) {
    S.resolved = null;
    renderDetail();
    return;
  }
  if (el.dataset.ref) {
    const c = DET.get(S.selected && S.selected.key),
      r = c && c.data && c.data.refs[el.dataset.ref];
    if (!r) return;
    if (r.keys.length === 1 && BYKEY.get(r.keys[0])) {
      select(BYKEY.get(r.keys[0]), { reveal: S.tab === "graph" });
    } else {
      S.resolved = { token: el.dataset.ref, keys: r.keys };
      renderDetail();
    }
    return;
  }
  if (el.dataset.hist) {
    toggleIn(S.histOpen, el.dataset.hist);
    renderDetail();
    return;
  }
  if (el.dataset.fproj != null) {
    FILTER.projects.clear();
    FILTER.projects.add(el.dataset.fproj);
    refilter();
    return;
  }
  if (el.dataset.fhost != null) {
    FILTER.hosts.clear();
    FILTER.hosts.add(el.dataset.fhost);
    refilter();
    return;
  }
  if (el.dataset.reveal) {
    const n = NODES.find((x) => x.id === +el.dataset.reveal);
    if (n) select(n, { reveal: true });
    return;
  }
  if (el.hasAttribute("data-clear")) {
    clearFocus();
    return;
  }
  if (el.dataset.tabgo) {
    setTab(el.dataset.tabgo);
    return;
  }
  if (el.dataset.ex) {
    q1.value = q2.value = el.dataset.ex;
    runSearch(el.dataset.ex);
    return;
  }
  if (el.dataset.id != null) {
    const n = NODES.find((x) => x.id === +el.dataset.id);
    if (!n) return;
    select(n, { reveal: S.tab === "graph" });
    return;
  }
  if (el.dataset.topic != null) {
    focusTopic(+el.dataset.topic);
    return;
  }
  if (el.dataset.turn != null) {
    focusTurn(+el.dataset.turn);
    return;
  }
});
document.addEventListener("keydown", (e) => {
  const inField = /INPUT|TEXTAREA|SELECT/.test(document.activeElement.tagName);
  if (e.key === "/" && !inField) {
    e.preventDefault();
    q1.focus();
    q1.select();
    return;
  }
  if (e.key === "Escape") {
    if (inField) {
      NAV.palette.close();
      document.activeElement.blur();
      return;
    }
    if ($("#side").classList.contains("open")) {
      closeDrawer();
      return;
    }
    NAV.up();
  }
  if (inField) return;
  if (e.key === "f" && S.tab === "graph") fitView();
});
function closeDrawer() {
  $("#side").classList.remove("open");
  $("#backdrop").classList.remove("on");
  $("#filtersBtn").setAttribute("aria-expanded", "false");
}
$("#filtersBtn").onclick = () => {
  const open = !$("#side").classList.contains("open");
  $("#side").classList.toggle("open", open);
  $("#backdrop").classList.toggle("on", open);
  $("#filtersBtn").setAttribute("aria-expanded", String(open));
};
$("#backdrop").onclick = closeDrawer;

function boot() {
  $("#hostN").textContent = DATA.host;
  $("#hostP").textContent = DATA.dirShort;
  $("#hostP").title = DATA.dir;
  renderSide();
  updateCounts();
  renderHyg();
  renderSctl();
  renderExamples();
  runSearch("");
  $("#legend").innerHTML = MemGfx.theme.legendHtml();
  resize();
  pushGraph();
  fitView(0);
  renderStale();
  booted = true;
  $("#boot").remove();
  NAV.start();
  renderStrip();
  renderFeed();
  renderDetail();
  loadSessions();
  pollTurns(false);
  GFX.invalidate();
  refreshHealth();
  checkFingerprint();
  NET.start();
  setInterval(() => {
    if (!document.hidden && !NET.live()) checkFingerprint();
  }, 5000);
  let hc = 0;
  setInterval(() => {
    if (!document.hidden && (S.tab === "health" || ++hc % 3 === 0))
      refreshHealth();
  }, 10000);
  setInterval(() => {
    if (!document.hidden && S.tab === "hyg") refreshHyg();
  }, 15000);
}
ingestGraph(window.__MEMDATA, true);
(document.fonts && document.fonts.ready
  ? Promise.race([
      document.fonts.ready,
      new Promise((r) => setTimeout(r, 1500)),
    ])
  : Promise.resolve()
).then(() => {
  GFX.resetText();
  setTimeout(boot, 20);
});
(() => {
  const b = document.body,
    side = document.querySelector(".side"),
    det = document.querySelector(".detail");
  const lim = { lw: [180, 560], rw: [300, 1400] };
  const mk = (k) => {
    const s = document.createElement("div");
    s.className = "split";
    s.dataset.k = k;
    s.title = "Тяни, чтобы изменить ширину; двойной клик возвращает исходную";
    b.appendChild(s);
    return s;
  };
  const sl = mk("lw"),
    sr = mk("rw");
  const place = () => {
    sl.style.display = side && side.offsetParent ? "" : "none";
    sr.style.display = det && det.offsetParent ? "" : "none";
    if (side) sl.style.left = side.getBoundingClientRect().right + "px";
    if (det) sr.style.left = det.getBoundingClientRect().left + "px";
  };
  for (const k of ["lw", "rw"]) {
    const v = +localStorage.getItem("mem." + k);
    if (v)
      b.style.setProperty(
        "--" + k,
        Math.min(lim[k][1], Math.max(lim[k][0], v)) + "px",
      );
  }
  for (const s of [sl, sr]) {
    const k = s.dataset.k;
    s.addEventListener("pointerdown", (e) => {
      e.preventDefault();
      s.setPointerCapture(e.pointerId);
      s.classList.add("on");
      const mv = (ev) => {
        const w = k === "lw" ? ev.clientX : innerWidth - ev.clientX;
        b.style.setProperty(
          "--" + k,
          Math.round(Math.min(lim[k][1], Math.max(lim[k][0], w))) + "px",
        );
        place();
      };
      const up = () => {
        s.classList.remove("on");
        s.removeEventListener("pointermove", mv);
        s.removeEventListener("pointerup", up);
        const v = parseInt(b.style.getPropertyValue("--" + k));
        if (v) localStorage.setItem("mem." + k, v);
      };
      s.addEventListener("pointermove", mv);
      s.addEventListener("pointerup", up);
    });
    s.addEventListener("dblclick", () => {
      b.style.removeProperty("--" + k);
      localStorage.removeItem("mem." + k);
      place();
    });
  }
  addEventListener("resize", place);
  new ResizeObserver(place).observe(b);
  if (det) new ResizeObserver(place).observe(det);
  place();
})();
