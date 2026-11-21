"use strict";
const $ = (s, r = document) => r.querySelector(s);
const esc = (s) =>
  String(s).replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
const clamp = (v, a, b) => (v < a ? a : v > b ? b : v);
const TH = window.MemGfx.theme;
const mixHex = TH.mix;
const plural = (n, f) => {
  const a = Math.abs(n) % 100,
    b = a % 10;
  if (a > 10 && a < 20) return f[2];
  if (b > 1 && b < 5) return f[1];
  if (b === 1) return f[0];
  return f[2];
};
const fmtN = (n) => n.toLocaleString("ru-RU");
let REDUCED = matchMedia("(prefers-reduced-motion: reduce)").matches;
matchMedia("(prefers-reduced-motion: reduce)").addEventListener(
  "change",
  (e) => {
    REDUCED = e.matches;
    if (window.__gfx) window.__gfx.invalidate();
  },
);

const C = TH.c;
const TYPES = {
  rule: { c: TH.kind.rule, ru: "правила", one: "правило" },
  fact: { c: TH.kind.fact, ru: "факты", one: "факт" },
  state: { c: TH.kind.state, ru: "статусы проектов", one: "статус проекта" },
  incident: { c: TH.kind.incident, ru: "инциденты", one: "инцидент" },
  howto: { c: TH.kind.howto, ru: "инструкции", one: "инструкция" },
  log: { c: TH.kind.log, ru: "логи", one: "лог" },
};
const TKEYS = Object.keys(TYPES);
const STATUSES = {
  current: "актуальные",
  expired: "срок истёк",
  superseded: "заменены",
  retired: "сняты",
};
const SKEYS = Object.keys(STATUSES);
const DAY = 864e5;
let DATA = null,
  NOW = Date.now();
const ageStr = (d) =>
  d < 1
    ? "сегодня"
    : d < 14
      ? d + " дн."
      : d < 60
        ? Math.round(d / 7) + " нед."
        : d < 365
          ? Math.round(d / 30) + " мес."
          : (d / 365).toFixed(1).replace(".", ",") + " г.";
const TOPICS = [],
  NODES = [],
  BYKEY = new Map();
let GVER = 0,
  LINKS = [],
  ACTIVE = [],
  ALINKS = [],
  byDeg = [],
  nodeSeq = 0,
  LAYOUT = {};
const FILTER = {
  hideType: new Set(),
  hideStatus: new Set(),
  projects: new Set(),
  hosts: new Set(),
  topic: null,
};
const projectsOf = (sc) =>
  sc.filter((s) => s.startsWith("project:")).map((s) => s.slice(8));
const hostsOf = (sc) =>
  sc.filter((s) => s.startsWith("host:")).map((s) => s.slice(5));
const HUB_DEG = 20;

function nodeFields(n, r) {
  const age = Math.max(0, Math.floor((NOW - Date.parse(r.m)) / DAY));
  Object.assign(n, {
    key: r.k,
    type: r.t,
    topic: r.g,
    age,
    date: r.m.slice(0, 10),
    summary: r.d || r.k,
    size: r.s || 0,
    journal: r.j || 0,
    core: !!r.c,
    dangerous: !!r.z,
    open: !!r.op,
    status: r.st || "current",
    succKey: r.su || "",
    scopes: r.sc || [],
    ver: r.ver || 1,
    ttl: r.tt || 0,
    chunks: r.ch || 0,
    source: r.src || "",
    vec: r.vs || "",
    trig: r.tr | 0,
    verify: !!r.vf,
    verified: r.vf && r.vd ? r.vd.slice(0, 10) : "",
    outKeys: r.o || [],
    outRel: r.rl || "",
    brokenKeys: r.x || [],
  });
  n.projects = projectsOf(n.scopes);
  n.hosts = hostsOf(n.scopes);
}
function ingestGraph(data, first) {
  DATA = data;
  NOW = Date.parse(data.built);
  LAYOUT = data.layout || {};
  const keepFilter =
      FILTER.topic != null && TOPICS[FILTER.topic]
        ? TOPICS[FILTER.topic].id
        : null,
    keepFocus =
      S.topicFocus != null && TOPICS[S.topicFocus]
        ? TOPICS[S.topicFocus].id
        : null;
  TOPICS.length = 0;
  data.topics.forEach((t, i) => {
    TOPICS.push({
      i,
      id: t.id,
      label: t.label,
      kw: t.kw || [],
      size: 0,
      members: [],
      ax: t.x || 0,
      ay: t.y || 0,
      R: t.R || 10,
      sx: t.x || 0,
      sy: t.y || 0,
      sR: t.R || 10,
      hull: t.hull || [],
      lx: t.lx == null ? t.x || 0 : t.lx,
      ly: t.ly == null ? t.y || 0 : t.ly,
      cap: t.cap || 0,
    });
  });
  const seen = new Set();
  for (const r of data.records) {
    seen.add(r.k);
    let n = BYKEY.get(r.k);
    const isNew = !n;
    if (isNew) {
      n = {
        id: nodeSeq++,
        out: [],
        in: [],
        broken: [],
        recalls: 0,
        lastTurn: -1,
        flashAt: -1e9,
        hl: 1,
        sc: first ? 1 : 0,
        x: 0,
        y: 0,
      };
      BYKEY.set(r.k, n);
      NODES.push(n);
    }
    nodeFields(n, r);
    n.out = [];
    n.in = [];
    n.broken = [];
    TOPICS[n.topic].members.push(n);
    TOPICS[n.topic].size++;
  }
  for (let i = NODES.length - 1; i >= 0; i--) {
    const n = NODES[i];
    if (!seen.has(n.key)) {
      NODES.splice(i, 1);
      BYKEY.delete(n.key);
      if (S.selected === n) S.selected = null;
    }
  }
  const LINKSET = new Set();
  LINKS = [];
  for (const n of NODES) {
    n.outKeys.forEach((k, j) => {
      const b = BYKEY.get(k);
      if (!b || b === n) return;
      n.out.push(b);
      b.in.push(n);
      const p = n.id < b.id ? n.id + ">" + b.id : b.id + ">" + n.id;
      if (!LINKSET.has(p)) {
        LINKSET.add(p);
        LINKS.push({ s: n, t: b, rel: n.outRel[j] || "r" });
      }
    });
    for (const k of n.brokenKeys) n.broken.push(k);
  }
  for (const n of NODES) {
    n.deg = n.in.length + n.out.length;
  }
  byDeg = NODES.slice().sort((a, b) => b.deg - a.deg);
  byDeg.forEach((n, i) => (n.rank = 1 - i / NODES.length));
  for (const n of NODES) {
    n.hub = !n.core && n.deg >= HUB_DEG;
    n.shape = n.core ? 1 : n.hub ? 2 : 0;
    n.r = clamp(2.6 + Math.sqrt(n.deg) * 1.35 + (n.core ? 1.2 : 0), 2.6, 12);
    const base = TYPES[n.type].c;
    n.color =
      n.type === "state"
        ? mixHex(
            base,
            C.aged,
            clamp(n.age / Math.max(1, n.ttl || 14), 0, 1) * 0.9,
          )
        : base;
    n.nbr = new Set([n, ...n.in, ...n.out]);
  }
  for (const l of LINKS) {
    l.phase = ((l.s.id * 7919 + l.t.id * 104729) % 997) / 997;
    if (l.rel === "s") {
      l.a = l.t;
      l.b = l.s;
    } else {
      l.a = l.s;
      l.b = l.t;
    }
  }
  const reindex = (id) => {
    if (id == null) return null;
    const t = TOPICS.find((x) => x.id === id);
    return t ? t.i : null;
  };
  FILTER.topic = reindex(keepFilter);
  S.topicFocus = reindex(keepFocus);
  for (const r of data.records) {
    const n = BYKEY.get(r.k),
      t = TOPICS[n.topic],
      hx = r.px != null && r.py != null ? r.px : t.sx,
      hy = r.px != null && r.py != null ? r.py : t.sy;
    n.hx = hx;
    n.hy = hy;
    if (!n.placed) {
      n.x = hx;
      n.y = hy;
      n.placed = true;
    }
  }
  GVER++;
  applyFilters();
}
const nodeVisible = (n, skip) =>
  (skip === "type" || !FILTER.hideType.has(n.type)) &&
  (skip === "status" || !FILTER.hideStatus.has(n.status)) &&
  (skip === "project" ||
    !FILTER.projects.size ||
    n.projects.some((p) => FILTER.projects.has(p)) ||
    (FILTER.projects.has("") && !n.projects.length)) &&
  (skip === "host" ||
    !FILTER.hosts.size ||
    n.hosts.some((h) => FILTER.hosts.has(h)) ||
    (FILTER.hosts.has("") && !n.hosts.length)) &&
  (skip === "topic" || FILTER.topic == null || n.topic === FILTER.topic);
function applyFilters() {
  let changed = false;
  for (const n of NODES) {
    const hide = !nodeVisible(n);
    if (hide !== !!n.hidden) {
      n.hidden = hide;
      changed = true;
    }
  }
  ACTIVE = NODES.filter((n) => !n.hidden);
  ALINKS = LINKS.filter((l) => !l.s.hidden && !l.t.hidden);
  if (S.selected && S.selected.hidden) S.selected = null;
  if (changed) GVER++;
  return changed;
}

const S = {
  hover: null,
  selected: null,
  topicFocus: null,
  turnFocus: null,
  tab: "graph",
  live: true,
  drag: null,
  session: "",
};
const TURNS = [];
let turnNo = 0;
const arcs = [];
let EGO = 1,
  egoMemo = { key: "", lv: null };
const EGO_LEVEL = [1, 1, 0.3, 0.15];
function egoLevels(n, depth) {
  const key = n.id + "|" + depth + "|" + GVER;
  if (egoMemo.key === key) return egoMemo.lv;
  const lv = new Map([[n, 1]]);
  let ring = [n];
  for (let d = 1; d <= depth && ring.length; d++) {
    const next = [];
    for (const m of ring)
      for (const o of m.nbr)
        if (!lv.has(o) && !o.hidden) {
          lv.set(o, EGO_LEVEL[d]);
          next.push(o);
        }
    ring = next;
  }
  egoMemo = { key, lv };
  return lv;
}
function focusInfo() {
  const D = S.drag;
  const f = (D && D.n) || (D && D.moved ? null : S.hover) || S.selected;
  if (f)
    return EGO > 1 && f === S.selected
      ? { node: f, set: f.nbr, lv: egoLevels(f, EGO) }
      : { node: f, set: f.nbr };
  if (S.turnFocus) return { set: S.turnFocus.set };
  if (S.topicFocus != null)
    return { set: new Set(TOPICS[S.topicFocus].members), topic: S.topicFocus };
  return null;
}
