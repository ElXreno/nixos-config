"use strict";
const vm = require("vm");
const fs = require("fs");
const [jsPath] = process.argv.slice(2);
const timers = [],
  cleared = [];
const sandbox = {
  Math,
  Map,
  Set,
  Array,
  Object,
  String,
  Number,
  Float32Array,
  Uint8Array,
  Int32Array,
  parseInt,
  parseFloat,
  performance: { now: () => 0 },
  console,
  setTimeout: (f, d) => {
    timers.push({ f, d });
    return timers.length;
  },
  clearTimeout: (id) => {
    cleared.push(id);
  },
};
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(fs.readFileSync(jsPath, "utf8"), sandbox);
const G = sandbox.MemGfx;
let bad = 0,
  calls = 0;
const ctx = new Proxy(
  {},
  {
    get(o, k) {
      if (k in o) return o[k];
      return (...a) => {
        calls++;
        for (const v of a) if (typeof v === "number" && !isFinite(v)) bad++;
      };
    },
    set(o, k, v) {
      o[k] = v;
      return true;
    },
  },
);
let reduced = false;
const host = { reduced: () => reduced };
const mkNode = (i, topic, x, y) => ({
  id: i,
  gi: i,
  key: "n" + i,
  topic,
  x,
  y,
  r: 4,
  deg: 0,
  hl: 1,
  sc: 1,
  hidden: false,
});
const nodes = [],
  links = [];
const node = (topic, x, y) => {
  const n = mkNode(nodes.length, topic, x, y);
  nodes.push(n);
  return n;
};
const link = (a, b) => {
  links.push({ a, b, s: a, t: b });
  a.deg++;
  b.deg++;
};
const S = node(0, 100, 100),
  S2 = node(1, 300, 100);
const A = [],
  B = [];
for (let i = 0; i < 5; i++) {
  const a = node(1, 120 + i * 10, 140);
  A.push(a);
  link(S, a);
}
for (const a of A)
  for (let j = 0; j < 3; j++) {
    const b = node(2, a.x + j * 5, 200);
    B.push(b);
    link(a, b);
  }
const C = B.map((b) => {
  const c = node(3, b.x, 260);
  link(b, c);
  return c;
});
const H = node(0, 90, 90);
link(S, H);
H.hidden = true;
const HUB = node(0, 500, 500),
  HUBN = [];
for (let i = 0; i < 100; i++) {
  const x = node(1, 500 + i, 520);
  HUBN.push(x);
  link(HUB, x);
}
const LONE = node(3, 900, 900);
const rc = G.recall.create(host);
rc.setGraph(nodes, links);
const out = {};
const hop = 140;
let w = rc.fire(1, [{ node: S, t: 1000 }], "lbl");
out.hop1 = w.hops.filter((h) => h.lv === 1).length;
out.hop2 = w.hops.filter((h) => h.lv === 2).length;
out.hop1Times = [
  ...new Set(w.hops.filter((h) => h.lv === 1).map((h) => h.t - 1000)),
];
out.hop2Times = [
  ...new Set(w.hops.filter((h) => h.lv === 2).map((h) => h.t - 1000)),
];
out.thirdHop = w.hops.some((h) => C.includes(h.node));
out.hiddenLit = w.hops.some((h) => h.node === H);
out.seedInHops = w.hops.some((h) => h.node === S);
out.edgesReal = w.edges.every((e) =>
  links.some((l) => l.a === e.a && l.b === e.b),
);
out.edgesFromParent = w.edges.every((e) => {
  const parent = e.fwd ? e.a : e.b,
    child = e.fwd ? e.b : e.a;
  const ph = w.hops.find((h) => h.node === parent),
    t0 = ph ? ph.t : 1000;
  const ch = w.hops.find((h) => h.node === child);
  return ch && Math.abs(e.t - t0) < 1e-9 && Math.abs(ch.t - e.t - hop) < 1e-9;
});
out.edgeCount = w.edges.length;
const cam = { x: 0, y: 0, k: 1 };
const W = 800,
  Hh = 600;
rc.under(ctx, 1070, cam, W, Hh);
out.cometsMid1 = rc.info(1070).comets;
rc.under(ctx, 1000 + hop + 70, cam, W, Hh);
out.cometsMid2 = rc.info(0).comets;
out.underAnimatingDuring = rc.under(ctx, 1500, cam, W, Hh);
out.underAnimatingAfter = rc.under(ctx, w.end + 10, cam, W, Hh);
const arr = new Float32Array(4);
const peaks = [];
for (let t = 1000; t <= 1000 + 3600 + 2 * hop; t += 20) {
  rc.breath(t, arr);
  peaks.push([...arr]);
}
out.breathMax = Math.max(...peaks.flat());
out.breathEnds = peaks[peaks.length - 1].every((v) => v === 0);
out.breathTouched = [0, 1, 2].every((ti) => peaks.some((p) => p[ti] > 0.3));
out.breathUntouched = peaks.every((p) => p[3] === 0);
const mono = (ti) => {
  let seenPeak = false,
    prev = 0,
    ok = true;
  for (const p of peaks) {
    const v = p[ti];
    if (v < prev - 1e-9) seenPeak = true;
    if (seenPeak && v > prev + 1e-9) ok = false;
    prev = v;
  }
  return ok;
};
out.breathOnce = [0, 1, 2].every(mono);
rc.breath(1000 + 140, arr);
out.breathPeak = arr[0];
rc.breath(1000 + 140 + 1800, arr);
out.breathLate = arr[0];
out.pillAtIgnite = rc.pill(1, 1000) > 0.9;
out.pillLater = rc.pill(1, 1600) === 0;
out.pillOther = rc.pill(7, 1000) === 0;
out.fireEmpty = rc.fire(9, [{ node: H, t: 5 }], "") === null;
rc.clear();
w = rc.fire(2, [{ node: HUB, t: 0 }], "");
out.hubHop1 = w.hops.filter((h) => h.lv === 1).length;
rc.clear();
w = rc.fire(3, [{ node: LONE, t: 0 }], "");
out.loneHops = w.hops.length;
rc.clear();
for (let i = 0; i < 5; i++) rc.fire(10 + i, [{ node: S, t: i * 10 }], "");
out.waveCap = rc.waves.length;
out.waveCapKeepsNewest = rc.waves.map((x) => x.n);
rc.clear();
reduced = true;
w = rc.fire(
  20,
  [
    { node: S, t: 0 },
    { node: S2, t: 150 },
  ],
  "r",
);
out.reducedHops = w.hops.length;
out.reducedEdges = w.edges.length;
out.reducedUnder = rc.under(ctx, 10, cam, W, Hh);
rc.breath(10, arr);
out.reducedBreath = arr.every((v) => v === 0);
rc.over(ctx, 10, { x: -5000, y: 0, k: 1 }, W, Hh, []);
out.reducedTicks = rc.info(10).ticks;
reduced = false;
rc.clear();
const far = node(0, -400, 300),
  far2 = node(0, -420, 320),
  top = node(0, 400, -500);
rc.setGraph(nodes, links);
rc.fire(
  30,
  [
    { node: far, t: 0 },
    { node: far2, t: 100 },
    { node: top, t: 200 },
    { node: S, t: 300 },
  ],
  "tick label",
);
rc.over(ctx, 500, cam, W, Hh, []);
const ti = rc.info(500);
out.ticks = ti.ticks;
out.tickCounts = ti.tickList.map((t) => t.n).sort();
const t0 = rc.tickAt(ti.tickList[0].x, ti.tickList[0].y);
out.tickHit = !!t0 && t0.label === "tick label" && t0.nodes.length >= 1;
out.tickMiss = rc.tickAt(400, 300) === null;
const merged = ti.tickList.find((t) => t.n === 2);
out.tickMergedNodes = merged ? rc.tickAt(merged.x, merged.y).nodes.length : 0;
out.tickInside = ti.tickList.every(
  (t) => t.x >= 0 && t.x <= W && t.y >= 0 && t.y <= Hh,
);
rc.over(ctx, 500 + 8000, cam, W, Hh, []);
out.tickFading = rc.info(0).ticks;
rc.over(ctx, 9400, cam, W, Hh, []);
out.tickFaded = rc.info(0).ticks;
const obst = [{ x: 340, y: 0, w: 120, h: 60 }];
rc.over(ctx, 500, cam, W, Hh, obst);
out.obstacleRespected = rc
  .info(0)
  .tickList.every((t) => !(t.x > 340 && t.x < 460 && t.y > 0 && t.y < 60));
out.obstacleMoved = rc
  .info(0)
  .tickList.some((t) => t.y < 30 && Math.abs(t.x - 400) > 20);
rc.over(ctx, 500, cam, W, Hh, [], () => obst);
out.extraRespected = rc
  .info(0)
  .tickList.every((t) => !(t.x > 340 && t.x < 460 && t.y > 0 && t.y < 60));
out.extraMoved = rc
  .info(0)
  .tickList.some((t) => t.y < 30 && Math.abs(t.x - 400) > 20);
rc.over(ctx, 400000, cam, W, Hh, []);
rc.under(ctx, 400000, cam, W, Hh);
out.expired = rc.waves.length;
const recorder = () => {
  const calls = [];
  const c = new Proxy(
    {},
    {
      get(o, k) {
        if (k in o) return o[k];
        return (...a) => {
          calls.push([k, ...a]);
        };
      },
      set(o, k, v) {
        o[k] = v;
        return true;
      },
    },
  );
  return { ctx: c, calls };
};
rc.clear();
reduced = false;
{
  const g = [];
  const nd = (topic, x, y) => {
    const n = mkNode(g.length + 5000, topic, x, y);
    n.gi = g.length;
    g.push(n);
    return n;
  };
  const lk = (a, b) => {
    gl.push({ a, b, s: a, t: b });
    a.deg++;
    b.deg++;
  };
  var gl = [];
  const seed = nd(5, 0, 0),
    mid = nd(5, 10, 0);
  lk(seed, mid);
  const same = [],
    cross = [];
  for (let i = 0; i < 12; i++) {
    const x = nd(5, 20 + i, 10);
    same.push(x);
    lk(mid, x);
  }
  for (let i = 0; i < 12; i++) {
    const x = nd(6, 20 + i, 20);
    cross.push(x);
    lk(mid, x);
  }
  const r2 = G.recall.create(host);
  r2.setGraph(g, gl);
  const w2 = r2.fire(1, [{ node: seed, t: 0 }], "");
  const h2 = w2.hops.filter((h) => h.lv === 2);
  out.xcapHop2 = h2.length;
  out.xcapCross = h2.filter((h) => h.node.topic === 6).length;
  out.xcapSame = h2.filter((h) => h.node.topic === 5).length;
}
{
  const g = [],
    gl = [];
  const nd = (topic, x, y) => {
    const n = mkNode(g.length, topic, x, y);
    n.gi = g.length;
    g.push(n);
    return n;
  };
  const lk = (a, b) => {
    gl.push({ a, b, s: a, t: b });
    a.deg++;
    b.deg++;
  };
  const hubs = [];
  for (let h = 0; h < 8; h++) {
    const hub = nd(h % 4, h * 50, 0);
    hubs.push(hub);
    for (let i = 0; i < 40; i++) {
      const leaf = nd(h % 4, h * 50 + i, 30);
      lk(hub, leaf);
      for (let j = 0; j < 3; j++) lk(leaf, nd(h % 4, h * 50 + i, 60 + j));
    }
  }
  const r3 = G.recall.create(host);
  r3.setGraph(g, gl);
  const w3 = r3.fire(
    1,
    hubs.map((n) => ({ node: n, t: 0 })),
    "",
  );
  out.bigHop1 = w3.hops.filter((h) => h.lv === 1).length;
  out.bigHop2 = w3.hops.filter((h) => h.lv === 2).length;
  out.bigEdges = w3.edges.length;
}
{
  const g = [],
    gl = [];
  const nd = (topic, x, y) => {
    const n = mkNode(g.length, topic, x, y);
    n.gi = g.length;
    g.push(n);
    return n;
  };
  const lk = (a, b) => {
    gl.push({ a, b, s: a, t: b });
    a.deg++;
    b.deg++;
  };
  const seed = nd(0, 0, 0);
  for (let t = 1; t <= 14; t++) lk(seed, nd(t, t * 10, 10));
  const r4 = G.recall.create(host);
  r4.setGraph(g, gl);
  r4.fire(1, [{ node: seed, t: 0 }], "");
  const arr4 = new Float32Array(20),
    lit = new Set();
  for (let t = 0; t < 4000; t += 20) {
    r4.breath(t, arr4);
    arr4.forEach((v, i) => {
      if (v > 0) lit.add(i);
    });
  }
  out.breathTopics = lit.size;
  out.breathKeepsSeed = lit.has(0);
  const seeds6 = [];
  for (let t = 0; t < 6; t++) {
    const sd = nd(20 + t, 500 + t * 5, 0);
    seeds6.push(sd);
    for (let u = 0; u < 3; u++) lk(sd, nd(30 + t * 3 + u, 500 + t * 5, 20 + u));
  }
  r4.setGraph(g, gl);
  r4.clear();
  r4.fire(
    2,
    seeds6.map((n) => ({ node: n, t: 0 })),
    "",
  );
  out.breathMany = r4.info(200).breathing;
  r4.clear();
  r4.fire(1, [{ node: seed, t: 0 }], "");
  out.infoBreathing = r4.info(200).breathing;
  out.infoBreathingLate = r4.info(5000).breathing;
}
{
  const g = [],
    gl = [];
  const nd = (topic, x, y) => {
    const n = mkNode(g.length, topic, x, y);
    n.gi = g.length;
    g.push(n);
    return n;
  };
  const A1 = nd(0, 100, 100),
    B1 = nd(0, 300, 200);
  gl.push({ a: A1, b: B1, s: A1, t: B1 });
  A1.deg++;
  B1.deg++;
  const r5 = G.recall.create(host);
  r5.setGraph(g, gl);
  r5.fire(1, [{ node: A1, t: 1000 }], "");
  const flat = { x: 0, y: 0, k: 1 };
  const at = (t) => {
    const R = recorder();
    r5.under(R.ctx, t, flat, 800, 600);
    return { n: r5.info(0).comets, lit: r5.info(0).lit, calls: R.calls };
  };
  out.cometAtIgnite = at(1000).n;
  out.cometJustAfter = at(1001).n;
  out.cometEmberAlive = at(1000 + hop + 890).n;
  out.cometEmberGone = at(1000 + hop + 910).n;
  out.litAlive = at(1000 + hop + 1590).lit;
  out.litGone = at(1000 + hop + 1610).lit;
  const c = G.theme.curve,
    dx = 200,
    dy = 100;
  const mx = 200 - dy * c,
    my = 150 + dx * c;
  const R = at(1070);
  const head = R.calls.find((x) => x[0] === "arc" && x[3] === 2.3);
  const ex = 0.25 * 100 + 0.5 * mx + 0.25 * 300,
    ey = 0.25 * 100 + 0.5 * my + 0.25 * 200;
  out.cometHead = head ? [head[1], head[2]] : null;
  out.cometExpect = [ex, ey];
  out.curve = c;
  const E = at(1000 + hop + 100).calls;
  const mv = E.find((x) => x[0] === "moveTo"),
    qd = E.find((x) => x[0] === "quadraticCurveTo");
  out.emberMove = mv ? [mv[1], mv[2]] : null;
  out.emberQuad = qd ? qd.slice(1) : null;
  out.emberExpect = [mx, my, 300, 200];
  r5.clear();
  r5.fire(2, [{ node: B1, t: 1000 }], "");
  const Rv = at(1035);
  const hv = Rv.calls.find((x) => x[0] === "arc" && x[3] === 2.3);
  const tt = 0.75,
    uu = 0.25;
  out.reverseHead = hv ? [hv[1], hv[2]] : null;
  out.reverseExpect = [
    uu * uu * 100 + 2 * uu * tt * mx + tt * tt * 300,
    uu * uu * 100 + 2 * uu * tt * my + tt * tt * 200,
  ];
  const trail = Rv.calls.filter((x) => x[0] === "quadraticCurveTo");
  out.reverseTrail = trail.length;
}
{
  const r6 = G.recall.create(host, () => {});
  const before = timers.length;
  r6.fire(
    1,
    [
      { node: S, t: 1000 },
      { node: S2, t: 1600 },
    ],
    "",
  );
  out.timerCount = timers.length - before;
  out.timerDelay = timers.length ? timers[timers.length - 1].d : -1;
  const cl = cleared.length;
  r6.clear();
  out.timerCleared = cleared.length - cl;
}
out.badNumbers = bad;
out.draws = calls;
process.stdout.write(JSON.stringify(out));
