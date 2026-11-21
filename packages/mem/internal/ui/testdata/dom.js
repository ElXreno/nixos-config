"use strict";
const vm = require("vm");
const fs = require("fs");
const [jsPath, dataPath] = process.argv.slice(2);
const bundle = JSON.parse(fs.readFileSync(dataPath, "utf8"));
const writes = [];
function stub(name) {
  const own = {};
  const p = new Proxy(function () {}, {
    get(_, k) {
      if (k === Symbol.toPrimitive) return () => 0;
      if (k === Symbol.iterator) return function* () {};
      if (k === "then") return undefined;
      if (k === "dataset") return own.dataset || (own.dataset = {});
      if (k === "classList")
        return {
          toggle() {},
          add() {},
          remove() {},
          contains() {
            return false;
          },
        };
      if (k === "style")
        return (
          own.style ||
          (own.style = {
            setProperty() {},
            getPropertyValue() {
              return "";
            },
            removeProperty() {},
          })
        );
      if (k in own) return own[k];
      return stub(name + "." + String(k));
    },
    set(_, k, v) {
      if (k === "innerHTML" || k === "outerHTML" || k === "insertAdjacentHTML")
        writes.push(String(v));
      own[k] = v;
      return true;
    },
    apply() {
      return stub(name + "()");
    },
  });
  return p;
}
const reply = (d) =>
  Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(d) });
const fetched = [];
const navLog = [];
const rafQ = [];
const drive = (n) => {
  for (let i = 0; i < n; i++) for (const f of rafQ.splice(0)) f(1000 + i * 16);
};
let graphOverride = null;
let diffOverride = null;
const els = new Map();
const el = (sel) => {
  if (!els.has(sel)) {
    const e = stub(sel);
    if (sel === "#cv" || sel === "#ov") {
      e.getContext = (kind) =>
        kind === "webgl2" ? null : stub(sel + "." + kind);
      e.getBoundingClientRect = () => ({
        left: 0,
        top: 0,
        width: 1200,
        height: 800,
      });
    }
    els.set(sel, e);
  }
  return els.get(sel);
};
const sandbox = {
  document: {
    querySelector: el,
    querySelectorAll: () => [],
    getElementById: el,
    createElement: () => stub("new"),
    addEventListener() {},
    activeElement: { tagName: "BODY", blur() {} },
    body: stub("body"),
    documentElement: stub("html"),
    hidden: false,
  },
  console,
  Math,
  Date,
  JSON,
  Promise,
  Set,
  Map,
  Float32Array,
  Float64Array,
  Int32Array,
  Uint8Array,
  Array,
  Object,
  String,
  Number,
  RegExp,
  parseInt,
  parseFloat,
  isNaN,
  isFinite,
  URLSearchParams,
  encodeURIComponent,
  matchMedia: () => ({ matches: false, addEventListener() {} }),
  localStorage: { getItem: () => null, setItem() {}, removeItem() {} },
  requestAnimationFrame: (f) => {
    rafQ.push(f);
    return rafQ.length;
  },
  cancelAnimationFrame() {},
  setTimeout: () => 0,
  clearTimeout() {},
  setInterval: () => 0,
  clearInterval() {},
  performance: { now: () => 0 },
  ResizeObserver: class {
    observe() {}
  },
  addEventListener() {},
  location: { hash: "", pathname: "/", search: "" },
  innerWidth: 1600,
  innerHeight: 900,
  history: {
    state: null,
    pushState(s, t, u) {
      sandbox.location.hash = String(u).startsWith("#") ? String(u) : "";
      navLog.push(["push", String(u)]);
    },
    replaceState(s, t, u) {
      sandbox.location.hash = String(u).startsWith("#") ? String(u) : "";
      navLog.push(["replace", String(u)]);
    },
  },
  fetch: (u) => {
    u = String(u);
    fetched.push(u);
    if (u.startsWith("/api/turns")) return reply(bundle.turns);
    if (u.startsWith("/api/sessions")) return reply(bundle.sessions);
    if (u.startsWith("/api/detail")) {
      const key = decodeURIComponent(u.split("key=")[1] || "");
      return bundle.details[key]
        ? reply(bundle.details[key])
        : Promise.resolve({ ok: false, status: 404 });
    }
    if (u.startsWith("/api/search")) return reply(bundle.search);
    if (u.startsWith("/api/health")) return reply(bundle.health);
    if (u.startsWith("/api/hygiene")) return reply(bundle.hygiene);
    if (u.startsWith("/api/core")) return reply(bundle.core);
    if (u.startsWith("/api/fingerprint")) return reply({ fingerprint: "x" });
    if (u.startsWith("/api/graph/diff"))
      return diffOverride
        ? reply(diffOverride)
        : Promise.resolve({ ok: false, status: 410 });
    if (u.startsWith("/api/graph")) return reply(graphOverride || bundle.graph);
    return new Promise(() => {});
  },
  devicePixelRatio: 1,
  getComputedStyle: () => ({ getPropertyValue: () => "" }),
};
sandbox.window = sandbox;
sandbox.__MEMDATA = bundle.graph;
sandbox.__MEMHYG = bundle.hygiene;
vm.createContext(sandbox);
vm.runInContext(
  fs.readFileSync(jsPath, "utf8") +
    "\n;globalThis.__x={NODES,TOPICS,TURNS,S,FILTER,SQ,DET,select,setTab,renderResults,renderHyg,renderSide,renderTopics,renderFeed,renderStrip,renderDetail,renderChip,renderSessions,renderHealth,renderCore,renderSctl,renderExamples,focusTopic,focusTurn,hilite,tok,stem,showTip,fetchSearch,loadDetail,refilter,resetFilters,applyFilters,ACTIVE:()=>ACTIVE,reloadGraph,syncGraph,DATA:()=>DATA,checkFingerprint,GFX,NAV,BYKEY,cam,EGO:()=>EGO,egoLevels,focusInfo,clearFocus,q1,lastSearch:()=>lastSearch};boot();",
  sandbox,
);
const x = sandbox.__x;
const tick = async (n) => {
  for (let i = 0; i < (n || 12); i++) await new Promise((r) => setImmediate(r));
};
const driveNav = async () => {
  const N = x.NAV,
    nv = sandbox.__nav,
    ov = sandbox.document.querySelector("#ov"),
    sr = () => String(sandbox.document.querySelector("#sr").textContent);
  const pushes = () => navLog.filter((e) => e[0] === "push").length;
  const key = (k, extra) => {
    let prevented = 0;
    nv.key(
      Object.assign(
        {
          key: k,
          preventDefault() {
            prevented++;
          },
        },
        extra || {},
      ),
    );
    return prevented;
  };
  const r = {};
  x.clearFocus();
  navLog.length = 0;
  const a = x.NODES.find(
    (n) =>
      n.nbr.size === 2 &&
      [...n.nbr].some((o) => o !== n && Math.hypot(o.x - n.x, o.y - n.y) > 1),
  );
  const b = [...a.nbr].find((o) => o !== a);
  x.select(a);
  r.pushOnSelect = pushes();
  r.pushUrl = navLog[navLog.length - 1][1];
  r.announced = sr().startsWith(a.key);
  x.select(a);
  r.noDuplicatePush = pushes() === 1;
  r.upToTopic =
    N.up() === true && x.S.selected === null && x.S.topicFocus === a.topic;
  r.topicUrl = navLog[navLog.length - 1][1];
  r.upToMap =
    N.up() === true && x.S.topicFocus === null && x.S.selected === null;
  r.upAtMapIsNoop = N.up() === false;
  r.pushesAfterUp = pushes();
  const before = pushes();
  nv.apply({ n: b.key }, false);
  r.applySelects = x.S.selected === b && pushes() === before;
  nv.apply({ t: x.TOPICS[a.topic].id }, false);
  r.applyTopic =
    x.S.selected === null && x.S.topicFocus === a.topic && pushes() === before;
  nv.apply({}, false);
  r.applyClears =
    x.S.selected === null && x.S.topicFocus === null && x.S.turnFocus === null;
  const evil = nv.parse(
    "#n=__proto__&t=constructor&turn=%E0%A4%A&cam=NaN,1,2&l=zz&__proto__=1&constructor=2&toString=3",
  );
  r.parseKeys = Object.keys(evil).sort().join(",");
  r.protoClean =
    !Object.prototype.hasOwnProperty.call(evil, "__proto__") &&
    evil.toString !== "3" &&
    evil.constructor !== "2";
  nv.apply(evil, false);
  r.hostileHashSelectsNothing =
    x.S.selected === null && x.S.topicFocus === null;
  sandbox.document.activeElement = ov;
  x.clearFocus();
  x.select(a);
  const dx = b.x - a.x,
    dy = b.y - a.y;
  const toward =
    Math.abs(dx) >= Math.abs(dy)
      ? dx > 0
        ? "ArrowRight"
        : "ArrowLeft"
      : dy > 0
        ? "ArrowDown"
        : "ArrowUp";
  const away = {
    ArrowRight: "ArrowLeft",
    ArrowLeft: "ArrowRight",
    ArrowDown: "ArrowUp",
    ArrowUp: "ArrowDown",
  };
  r.arrowPrevents = key(toward) === 1;
  r.arrowFollowsLink = x.S.selected === b;
  r.arrowBack = (key(away[toward]), x.S.selected === a);
  r.arrowNoNeighbourStays = (key(away[toward]), x.S.selected === a);
  r.arrowNoNeighbourSays = sr().includes("связей нет");
  x.clearFocus();
  key("ArrowRight");
  r.arrowFromNothingSelects = x.S.selected !== null;
  x.clearFocus();
  const list = x.TOPICS.filter((t) => t.members.some((m) => !m.hidden));
  const steps = [];
  for (let i = 0; i < list.length; i++)
    steps.push(key("Tab") === 1 && x.S.topicFocus === list[i].i);
  r.tabCycles = steps.every(Boolean) && steps.length === list.length;
  r.tabLeavesAtEnd = key("Tab") === 0 && x.S.topicFocus === null;
  r.shiftTabLeavesAtStart =
    key("Tab", { shiftKey: true }) === 0 && x.S.topicFocus === null;
  x.clearFocus();
  const c2 = { id: 9001, hidden: false, nbr: new Set() },
    c3 = { id: 9002, hidden: false, nbr: new Set() },
    hid = { id: 9003, hidden: true, nbr: new Set() };
  b.nbr.add(c2);
  c2.nbr.add(b);
  c2.nbr.add(c3);
  c3.nbr.add(c2);
  b.nbr.add(hid);
  const lv2 = x.egoLevels(a, 2),
    lv3 = x.egoLevels(a, 3);
  r.egoLevels =
    lv2.get(a) === 1 &&
    lv2.get(b) === 1 &&
    lv2.get(c2) === 0.3 &&
    !lv2.has(c3) &&
    lv3.get(c3) === 0.15 &&
    !lv3.has(hid);
  b.nbr.delete(c2);
  b.nbr.delete(hid);
  x.select(a);
  r.egoStart = x.EGO() === 1 && x.focusInfo().lv === undefined;
  key("]");
  const f2 = x.focusInfo();
  r.egoTwo =
    x.EGO() === 2 &&
    f2.lv instanceof Map &&
    f2.lv.get(a) === 1 &&
    f2.lv.get(b) === 1;
  r.egoAnnounced = sr() === "Глубина 2";
  key("]");
  key("]");
  r.egoClamps = x.EGO() === 3;
  key("[");
  key("[");
  key("[");
  r.egoFloor = x.EGO() === 1 && x.focusInfo().lv === undefined;
  key("]");
  x.select(b);
  r.egoResetsOnNewSelection = x.EGO() === 1;
  r.altLeftSafe = (nv.back(), true);
  const lh = nv.layoutHash();
  r.layoutHashShape = /^[0-9a-f]{8}$/.test(lh);
  nv.apply({ cam: "10.5,20.5,2", l: lh }, true);
  const cc = x.cam.center();
  r.camRestored =
    Math.abs(cc.x - 10.5) < 1e-6 &&
    Math.abs(cc.y - 20.5) < 1e-6 &&
    Math.abs(x.cam.k - 2) < 1e-9;
  nv.apply({ cam: "99,99,3", l: "ffffffff" }, true);
  r.camIgnoredOnOtherLayout =
    Math.abs(x.cam.k - 3) > 1e-6 && Math.abs(x.cam.center().x - 99) > 1;
  nv.apply({ cam: "junk", l: lh }, true);
  r.camIgnoredWhenMalformed =
    Number.isFinite(x.cam.k) && Number.isFinite(x.cam.x);
  const moved = x.TOPICS[0].sR;
  x.TOPICS[0].sR = moved + 50;
  r.layoutHashSeesTerritories = nv.layoutHash() !== lh;
  x.TOPICS[0].sR = moved;
  r.layoutHashStable = nv.layoutHash() === lh;
  const T0 = x.TURNS.find((t) => t.id);
  x.clearFocus();
  nv.apply({ turn: T0.id }, false);
  r.turnRestored =
    !!x.S.turnFocus && x.S.turnFocus.n === T0.n && nv.state().turn === T0.id;
  nv.apply({ turn: "no-such-turn" }, false);
  r.turnUnknownIsQuiet = x.S.turnFocus === null && (x.NAV.turns(), true);
  x.clearFocus();
  x.q1.value = "img";
  N.palette.open();
  await x.fetchSearch("img");
  r.paletteRows = writes.some((h) => h.includes('class="gsr'));
  r.paletteActive = N.palette.active();
  const picked = N.palette.pick(x.lastSearch().list);
  r.palettePick = !!picked && !!picked.n;
  r.paletteKey =
    N.palette.key({ key: "ArrowDown", preventDefault() {} }) === true;
  N.palette.close();
  r.paletteClosed =
    !N.palette.isOpen() && N.palette.pick(x.lastSearch().list) === null;
  sandbox.document.activeElement = { tagName: "BODY", blur() {} };
  x.clearFocus();
  const rev0 = x.DATA().rev,
    n0 = x.NODES.length;
  r.graphHasRev = typeof rev0 === "string" && rev0.length === 16;
  let mark = fetched.length;
  await x.syncGraph();
  await tick();
  const seq = fetched.slice(mark).filter((u) => u.startsWith("/api/graph"));
  r.goneFallsBackToFullReload =
    seq.length === 2 &&
    seq[0] === "/api/graph/diff?from=" + rev0 &&
    seq[1] === "/api/graph" &&
    x.NODES.length === n0;
  const g = x.DATA();
  diffOverride = {
    from: rev0,
    rev: "r2r2r2r2r2r2r2r2",
    built: g.built,
    links: g.links,
    rows: g.rows,
    layout: g.layout,
    topics: g.topics.map((t) => ({ id: t.id, same: true })),
    add: [],
    set: [],
    del: [g.records[g.records.length - 1].k],
  };
  mark = fetched.length;
  await x.syncGraph();
  await tick();
  const seq2 = fetched.slice(mark).filter((u) => u.startsWith("/api/graph"));
  r.diffAppliesWithoutFullFetch =
    seq2.length === 1 &&
    seq2[0].startsWith("/api/graph/diff") &&
    x.NODES.length === n0 - 1 &&
    x.DATA().rev === "r2r2r2r2r2r2r2r2";
  diffOverride = {
    from: "r2r2r2r2r2r2r2r2",
    rev: "r3",
    topics: [{ id: "unknown_topic", same: true }],
    add: [],
    set: [],
    del: [],
  };
  await x.syncGraph();
  await tick();
  r.brokenDiffFallsBackToFullReload =
    x.NODES.length === n0 && x.DATA().rev === rev0;
  diffOverride = null;
  return r;
};
(async () => {
  await tick();
  const turns = x.TURNS.length;
  const items = x.TURNS.reduce((s, T) => s + T.items.length, 0);
  const drawn = x.TURNS.reduce(
    (s, T) => s + T.items.filter((i) => i.node).length,
    0,
  );
  writes.length = 0;
  for (const n of x.NODES) {
    x.select(n);
    x.showTip(n, { x: 1, y: 1 });
    await tick(4);
    x.renderDetail();
    drive(2);
  }
  for (const t of x.TOPICS) x.focusTopic(t.i);
  x.S.topicFocus = null;
  x.renderSide();
  x.renderHyg();
  x.renderTopics();
  x.renderFeed();
  x.renderStrip();
  x.renderSctl();
  x.renderExamples();
  x.renderHealth(bundle.health);
  x.renderCore(bundle.core);
  for (const q of ["amp", "quot", "lt gt", "img onerror", "script", "svg"]) {
    await x.fetchSearch(q);
  }
  x.renderResults("amp", [...new Set(x.tok("amp").map(x.stem))], {
    list: x.NODES.map((n) => ({
      n,
      h: {
        key: n.key,
        hit: n.key,
        score: 0.01,
        words_rank: 1,
        cos: 0.7,
        rerank: 0.95,
        snippet: n.summary,
      },
    })),
    hiddenN: 0,
    mode: "fused",
    asked: "fused",
    degraded: {},
  });
  x.S.selected = null;
  x.S.turnFocus = null;
  x.setTab("feed");
  x.renderFeed();
  x.renderStrip();
  x.renderDetail();
  x.renderSessions();
  for (const T of x.TURNS) {
    x.focusTurn(T.n);
    x.renderChip();
  }
  x.FILTER.hideType.add("rule");
  x.FILTER.projects.add("legacy");
  x.refilter();
  const filtered = x.ACTIVE().length;
  x.resetFilters();
  const lastId = x.TOPICS[x.TOPICS.length - 1].id,
    focusId = x.TOPICS[0].id;
  x.FILTER.topic = x.TOPICS.length - 1;
  x.S.topicFocus = 0;
  const g2 = JSON.parse(JSON.stringify(bundle.graph));
  g2.topics.unshift({ id: "zz_new", label: "zz", n: 0 });
  for (const r of g2.records) r.g += 1;
  graphOverride = g2;
  await x.reloadGraph();
  await tick();
  const topicStable =
    x.FILTER.topic != null &&
    x.TOPICS[x.FILTER.topic].id === lastId &&
    x.S.topicFocus != null &&
    x.TOPICS[x.S.topicFocus].id === focusId;
  graphOverride = null;
  x.S.topicFocus = null;
  x.resetFilters();
  const navReport = await driveNav();
  const tags = new Set();
  const attrs = new Set();
  for (const html of writes) {
    for (const m of html.matchAll(/<([a-zA-Z][a-zA-Z0-9]*)((?:\s[^<>]*)?)>/g)) {
      tags.add(m[1].toLowerCase());
      for (const a of m[2].matchAll(/\s([a-zA-Z][a-zA-Z0-9-]*)(?==")/g))
        attrs.add(a[1].toLowerCase());
    }
  }
  const seen = (s) => writes.some((h) => h.includes(s));
  const escaped = {};
  for (const [name, needle] of Object.entries(bundle.needles))
    escaped[name] = seen(needle);
  const hl = [
    ["a & b", ["amp"]],
    ["x <b> y", ["lt", "gt"]],
    ['q "z"', ["quot"]],
    ["it's", ["39"]],
    ["Amp & amp", ["amp"]],
    ["naïve test", ["test"]],
    ["prefill", ["pre"]],
  ].map(([t, st]) => x.hilite(t, st));
  drive(40);
  const backend = x.GFX.backend,
    frames = x.GFX.stats().frames;
  const live = {};
  x.setTab("graph");
  x.resetFilters();
  const la = x.NODES.find((n) => n.deg > 0),
    lb = [...la.nbr].find((o) => o !== la);
  x.GFX.grab(la, la.hx + 90, la.hy + 10);
  drive(30);
  live.pinned =
    x.GFX.live.pinned() === la &&
    Math.abs(la.x - (la.hx + 90)) < 1e-6 &&
    Math.abs(la.y - (la.hy + 10)) < 1e-6;
  live.neighbourFollows = Math.hypot(lb.x - lb.hx, lb.y - lb.hy) > 1;
  live.hitFollows = x.GFX.posVer() > 0;
  x.GFX.drop();
  drive(300);
  live.restored = x.NODES.every((n) => n.x === n.hx && n.y === n.hy);
  live.asleep = !x.GFX.awake() && rafQ.length === 0;
  live.staticWithoutWebGL = (() => {
    const v = x.GFX.posVer();
    drive(20);
    return x.GFX.posVer() === v && rafQ.length === 0;
  })();
  process.stdout.write(
    JSON.stringify({
      nav: navReport,
      navPushes: navLog.length,
      backend,
      frames,
      writes: writes.length,
      nodes: x.NODES.length,
      tags: [...tags].sort(),
      attrs: [...attrs].sort(),
      hilite: hl,
      turns,
      items,
      drawn,
      escaped,
      fetched,
      filtered,
      topicStable,
      undefinedWrites: writes.filter((h) => /undefined|NaN|\[object/.test(h))
        .length,
      live,
      raw: bundle.dump ? writes : undefined,
    }),
  );
})().catch((e) => {
  process.stderr.write(String((e && e.stack) || e));
  process.exit(1);
});
