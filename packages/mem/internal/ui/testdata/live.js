"use strict";
const vm = require("vm");
const fs = require("fs");
const [jsPath] = process.argv.slice(2);
const sandbox = {
  Math,
  Map,
  Set,
  Array,
  Object,
  String,
  Number,
  Float32Array,
  Float64Array,
  Uint8Array,
  Int32Array,
  isFinite,
  parseInt,
  parseFloat,
  performance: { now: () => 0 },
  console,
};
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(fs.readFileSync(jsPath, "utf8"), sandbox);
const G = sandbox.MemGfx;
const out = {};
const COLS = 25,
  ROWS = 25,
  GAP = 24;

const lattice = () => {
  const nodes = [],
    links = [];
  for (let j = 0; j < ROWS; j++) {
    for (let i = 0; i < COLS; i++) {
      const x = i * GAP,
        y = j * GAP;
      nodes.push({
        id: nodes.length,
        topic: i < COLS / 2 ? 0 : 1,
        x,
        y,
        hx: x,
        hy: y,
        r: 4,
        hidden: false,
        members: null,
      });
    }
  }
  const at = (i, j) => nodes[j * COLS + i];
  for (let j = 0; j < ROWS; j++) {
    for (let i = 0; i < COLS; i++) {
      if (i + 1 < COLS) links.push({ a: at(i, j), b: at(i + 1, j) });
      if (j + 1 < ROWS) links.push({ a: at(i, j), b: at(i, j + 1) });
    }
  }
  return { nodes, links, at };
};
const disp = (n) => Math.hypot(n.x - n.hx, n.y - n.hy);
const maxDisp = (nodes) => nodes.reduce((m, n) => Math.max(m, disp(n)), 0);
const run = (live, from, frames, wander) => {
  let t = from;
  for (let f = 0; f < frames; f++) {
    live.step(t, 16, { wander });
    t += 16;
  }
  return t;
};

{
  const g = lattice();
  const live = G.live.create();
  live.setGraph(g.nodes, g.links);
  let t = run(live, 1000, 200, false);
  out.restMax = maxDisp(g.nodes);
  out.restSleeps = !live.awake();
  out.restExact = g.nodes.every((n) => n.x === n.hx && n.y === n.hy);
  live.setAmp(3.5);
  t = run(live, t, 600, true);
  out.wanderMax = maxDisp(g.nodes);
  out.wanderAwake = live.awake();
  let alive = 0;
  for (const n of g.nodes) if (disp(n) > 1) alive++;
  out.wanderAliveShare = alive / g.nodes.length;
  out.wanderLinkStretch = g.links.reduce(
    (m, l) =>
      Math.max(
        m,
        Math.abs(
          Math.hypot(l.a.x - l.b.x, l.a.y - l.b.y) -
            Math.hypot(l.a.hx - l.b.hx, l.a.hy - l.b.hy),
        ),
      ),
    0,
  );
  t = run(live, t, 400, false);
  out.afterWanderRestMax = maxDisp(g.nodes);
  out.afterWanderSleeps = !live.awake();
}

{
  const g = lattice();
  const live = G.live.create();
  live.setGraph(g.nodes, g.links);
  const c = g.at(12, 12),
    near = g.at(13, 12),
    two = g.at(14, 12),
    far = g.at(22, 12),
    other = g.at(2, 3);
  let t = run(live, 1000, 5, false);
  for (let f = 0; f < 12; f++) {
    live.pin(c, c.hx + (100 * (f + 1)) / 12, c.hy);
    live.step(t, 16, { wander: false });
    t += 16;
  }
  t = run(live, t, 90, false);
  out.dragSelf = Math.hypot(c.x - (c.hx + 100), c.y - c.hy);
  out.dragNear = disp(near);
  out.dragTwo = disp(two);
  out.dragFar = disp(far);
  out.dragOther = disp(other);
  out.dragPinned = live.pinned() === c;
  out.dragAwake = live.awake();
  live.release();
  t = run(live, t, 6, false);
  out.releaseEarly = disp(c);
  t = run(live, t, 24, false);
  out.releaseHalfSecond = disp(c);
  t = run(live, t, 90, false);
  out.releaseTwoSeconds = maxDisp(g.nodes);
  t = run(live, t, 200, false);
  out.releaseDone = maxDisp(g.nodes);
  out.releaseSleeps = !live.awake();
  out.releaseExact = g.nodes.every((n) => n.x === n.hx && n.y === n.hy);
}

{
  const nodes = [],
    links = [];
  const mk = (x, y, topic) => {
    const n = {
      id: nodes.length,
      topic,
      x,
      y,
      hx: x,
      hy: y,
      r: 4,
      hidden: false,
    };
    nodes.push(n);
    return n;
  };
  const center = mk(0, 0, 0),
    same = mk(30, 0, 0),
    cross = mk(-30, 0, 1);
  links.push({ a: center, b: same }, { a: center, b: cross });
  const live = G.live.create();
  live.setGraph(nodes, links);
  let t = 1000;
  for (let f = 0; f < 12; f++) {
    live.pin(center, 0, (30 * (f + 1)) / 12);
    live.step(t, 16, { wander: false });
    t += 16;
  }
  run(live, t, 90, false);
  out.sameFollows = disp(same);
  out.crossFollows = disp(cross);
}

{
  const nodes = [];
  const mk = (x, y) => {
    const n = {
      id: nodes.length,
      topic: 0,
      x,
      y,
      hx: x,
      hy: y,
      r: 4,
      hidden: false,
    };
    nodes.push(n);
    return n;
  };
  const a = mk(0, 0),
    b = mk(40, 0),
    h = mk(80, 0);
  const live = G.live.create();
  live.setGraph(nodes, [
    { a, b },
    { a: b, b: h },
  ]);
  h.hidden = true;
  let t = 1000;
  for (let f = 0; f < 12; f++) {
    live.pin(a, (30 * (f + 1)) / 12, 0);
    live.step(t, 16, { wander: false });
    t += 16;
  }
  run(live, t, 60, false);
  out.hiddenStays = disp(h) < 0.01;
}

{
  const nodes = [];
  const mk = (x, y, r) => {
    const n = {
      id: nodes.length,
      topic: 0,
      x,
      y,
      hx: x,
      hy: y,
      r,
      hidden: false,
    };
    nodes.push(n);
    return n;
  };
  const a = mk(0, 0, 8),
    b = mk(2, 0, 8);
  const live = G.live.create();
  live.setGraph(nodes, []);
  run(live, 1000, 60, false);
  out.collisionPushes = Math.hypot(a.x - b.x, a.y - b.y) > 2.5;
}

{
  const T = G.terrain;
  const members = [];
  const mk = (x, y, r) => members.push({ x, y, r, shape: 0 });
  mk(100, 100, 4);
  mk(120, 108, 6);
  mk(110, 130, 3);
  mk(140, 120, 5);
  mk(95, 128, 4);
  mk(130, 98, 3);
  const t = {
    members,
    hull: new Array(48).fill(40),
    ax: 0,
    ay: 0,
    R: 40,
    lx: 0,
    ly: 0,
  };
  out.fitOk = T.fit(t);
  let inside = 0,
    worst = Infinity;
  for (const m of members) {
    const dx = m.x - t.ax,
      dy = m.y - t.ay,
      d = Math.hypot(dx, dy),
      margin = T.radiusAt(t, Math.atan2(dy, dx)) - d - m.r;
    worst = Math.min(worst, margin);
    if (margin >= 0) inside++;
  }
  out.hullInside = inside;
  out.hullMembers = members.length;
  out.hullWorstMargin = worst;
  let far = 0,
    maxGap = 0;
  for (let j = 0; j < 144; j++) {
    const a = -Math.PI + (j / 144) * Math.PI * 2,
      r = T.radiusAt(t, a),
      px = t.ax + r * Math.cos(a),
      py = t.ay + r * Math.sin(a);
    let g = Infinity;
    for (const m of members)
      g = Math.min(g, Math.hypot(px - m.x, py - m.y) - m.r);
    maxGap = Math.max(maxGap, g);
    if (g > T.PAD + 24) far++;
  }
  out.hullMaxGap = maxGap;
  out.hullFarSamples = far;
  const b = T.bounds(t);
  out.hullBoundsContain = members.every(
    (m) => m.x >= b[0] && m.x <= b[2] && m.y >= b[1] && m.y <= b[3],
  );
  out.hullCenterNearCloud = Math.hypot(t.ax - 115, t.ay - 115) < 25;
  out.hullR = t.R;
  const solo = {
    members: [{ x: 5, y: 5, r: 3, shape: 0 }],
    hull: [],
    ax: 0,
    ay: 0,
    R: 50,
    lx: 0,
    ly: 0,
  };
  T.fit(solo);
  out.soloR = solo.R;
  members.forEach((m) => {
    m.x += 60;
    m.y -= 30;
  });
  const before = t.ax;
  T.fit(t);
  out.hullFollows = Math.abs(t.ax - before - 60) < 3;
}

{
  const nodes = [
    { x: 10, y: 10, r: 4, hidden: false, key: "a" },
    { x: 400, y: 300, r: 4, hidden: false, key: "b" },
    { x: 410, y: 300, r: 3, hidden: true, key: "c" },
  ];
  const h = G.hit.create();
  h.build(nodes);
  out.hitFinds =
    h.query(11, 11, 1) === nodes[0] && h.query(400, 301, 1) === nodes[1];
  out.hitSkipsHidden =
    h.query(410, 300, 4) === nodes[1] || h.query(410, 300, 4) === null;
  nodes[0].x = 200;
  nodes[0].y = 200;
  out.hitStale = h.query(200, 200, 1) === null;
  h.build(nodes);
  out.hitFollows =
    h.query(201, 200, 1) === nodes[0] && h.query(10, 10, 1) === null;
}

process.stdout.write(JSON.stringify(out));
