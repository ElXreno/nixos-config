"use strict";
const vm = require("vm");
const fs = require("fs");
const [jsPath, topicsPath] = process.argv.slice(2);
const rows = JSON.parse(fs.readFileSync(topicsPath, "utf8"));
const sandbox = {
  Math,
  Map,
  Set,
  Array,
  Object,
  String,
  Number,
  Float32Array,
  parseInt,
  parseFloat,
  performance: { now: () => Date.now() },
  console,
};
sandbox.window = sandbox;
vm.createContext(sandbox);
vm.runInContext(fs.readFileSync(jsPath, "utf8"), sandbox);
const G = sandbox.MemGfx;
const ctx = new Proxy(
  { font: "10px x" },
  {
    get(o, k) {
      if (k === "measureText")
        return (s) => ({
          width:
            String(s).length *
            parseFloat(/(\d+(?:\.\d+)?)px/.exec(o.font)[1]) *
            0.55,
        });
      if (k in o) return o[k];
      return () => {};
    },
    set(o, k, v) {
      o[k] = v;
      return true;
    },
  },
);
const topics = rows.map((r, i) => ({
  i,
  id: r.id,
  label: r.label,
  size: r.n,
  members: Array.from({ length: r.n }, () => ({ hidden: false })),
  ax: r.x,
  ay: r.y,
  R: r.R,
  lx: r.lx,
  ly: r.ly,
}));
const host = {
  reduced: () => false,
  selected: () => null,
  turnFocus: () => null,
  topics: () => topics,
  active: () => [],
  byDeg: () => [],
};
const W = 980,
  H = 768;
let x0 = Infinity,
  y0 = Infinity,
  x1 = -Infinity,
  y1 = -Infinity;
for (const t of topics) {
  x0 = Math.min(x0, t.ax - t.R);
  y0 = Math.min(y0, t.ay - t.R);
  x1 = Math.max(x1, t.ax + t.R);
  y1 = Math.max(y1, t.ay + t.R);
}
const kFit = Math.min(W / ((x1 - x0) * 1.12), H / ((y1 - y0) * 1.12));
const camAt = (cx, cy, k) => ({ k, x: W / 2 - cx * k, y: H / 2 - cy * k });
const settle = (lbl, cam, t0, n) => {
  let now = t0;
  for (let i = 0; i < n; i++) {
    lbl.draw(now, cam, W, H, null, null, []);
    now += 16;
  }
  return now;
};
const overlaps = (list) => {
  let n = 0;
  for (let i = 0; i < list.length; i++) {
    for (let j = i + 1; j < list.length; j++) {
      const a = list[i],
        b = list[j];
      if (
        a.x < b.x + b.w &&
        a.x + a.w > b.x &&
        a.y < b.y + b.h &&
        a.y + a.h > b.y
      )
        n++;
    }
  }
  return n;
};
const placed = (lbl) => lbl.debug().filter((d) => d.st === 1 && d.x != null);
const out = {};
const lbl = G.labels.create(ctx, host);
const cam = camAt((x0 + x1) / 2, (y0 + y1) / 2, kFit);
let now = settle(lbl, cam, 1000, 40);
out.kFit = +kFit.toFixed(4);
out.count = lbl.count();
const names = lbl.names();
out.names = names.length;
out.uniqueNames = new Set(names).size;
out.emptyNames = names.filter((n) => !n).length;
out.overlaps = overlaps(placed(lbl));
out.outside = placed(lbl).filter(
  (d) => d.x < 0 || d.y < 0 || d.x + d.w > W || d.y + d.h > H,
).length;
const base = lbl.shownIds();
const pans = [
  [1, 0],
  [0, 1],
  [-1, 0],
  [0, -1],
  [1, 1],
];
out.panStable = pans.every(([dx, dy]) => {
  const c = { k: cam.k, x: cam.x + dx, y: cam.y + dy };
  now = settle(lbl, c, now, 30);
  const same = JSON.stringify(lbl.shownIds()) === JSON.stringify(base);
  now = settle(lbl, cam, now, 30);
  return same;
});
const zc = {
  k: cam.k * 1.004,
  x: cam.x - (W / 2 - cam.x) * 0.004,
  y: cam.y - (H / 2 - cam.y) * 0.004,
};
now = settle(lbl, zc, now, 30);
out.zoomStable = JSON.stringify(lbl.shownIds()) === JSON.stringify(base);
const lbl2 = G.labels.create(ctx, host);
settle(lbl2, cam, 1000, 40);
out.deterministic = JSON.stringify(lbl2.shownIds()) === JSON.stringify(base);
const first = topics.filter((t) => t.size >= 3).slice(0, 1)[0];
const c1 = camAt(first.ax, first.ay, 1);
const lbl3 = G.labels.create(ctx, host);
settle(lbl3, c1, 1000, 40);
out.l1Shown = lbl3.count();
out.l1Outside = placed(lbl3).filter(
  (d) => d.x < 0 || d.y < 0 || d.x + d.w > W || d.y + d.h > H,
).length;
out.l1Overlaps = overlaps(placed(lbl3));
const detached = (lb, cm) =>
  placed(lb)
    .filter((d) => d.id[0] === "t")
    .filter((d) => {
      const t = topics.find((q) => "t" + q.id === d.id);
      const cx = t.ax * cm.k + cm.x,
        cy = t.ay * cm.k + cm.y,
        Rs = t.R * cm.k;
      const dx = Math.max(d.x - cx, 0, cx - d.x - d.w),
        dy = Math.max(d.y - cy, 0, cy - d.y - d.h);
      const gap = Math.hypot(dx, dy) - Rs;
      return gap > 48 || (gap > 16 && !d.lead);
    }).length;
out.detached = 0;
for (const big of topics.filter((t) => t.size >= 20).slice(0, 8)) {
  for (const z of [1.2, 1.8]) {
    for (const [ox, oy] of [
      [0, 0],
      [big.R * 0.9, 0],
      [0, -big.R * 0.9],
      [-big.R * 1.2, big.R * 0.6],
    ]) {
      const cm = camAt(big.ax + ox, big.ay + oy, z),
        lb = G.labels.create(ctx, host);
      settle(lb, cm, 1000, 30);
      out.detached += detached(lb, cm);
    }
  }
}
let seed = 7;
const rnd = () => {
  seed = (seed * 1664525 + 1013904223) >>> 0;
  return seed / 4294967296;
};
out.poses = 0;
out.poseFlips = 0;
const same = (p, q) => p.length === q.length && p.every((v, i) => v === q[i]);
const stab = G.labels.create(ctx, host);
for (let i = 0; i < 60; i++) {
  const k = [0.44, 0.65, 1, 1.5, 2.4][i % 5],
    cx = x0 + rnd() * (x1 - x0),
    cy = y0 + rnd() * (y1 - y0);
  now = settle(stab, camAt(cx, cy, k), now, 20);
  const a = stab.shownIds();
  out.poses++;
  for (const [dx, dy, dk] of [
    [1, 0, 0],
    [0, 1, 0],
    [-1, 0, 0],
    [0, -1, 0],
    [0, 0, 0.004],
  ]) {
    const c = camAt(cx + dx / k, cy + dy / k, k * (1 + dk));
    now = settle(stab, c, now, 20);
    if (!same(a, stab.shownIds())) out.poseFlips++;
    now = settle(stab, camAt(cx, cy, k), now, 20);
  }
}
const straddle = (lb, cm) =>
  placed(lb)
    .filter((d) => d.id[0] === "t")
    .filter((d) => {
      const me = topics.find((q) => "t" + q.id === d.id);
      return topics.some((o) => {
        if (o === me) return false;
        const ox = o.ax * cm.k + cm.x,
          oy = o.ay * cm.k + cm.y,
          r = o.R * cm.k * 0.94;
        const dx = Math.max(d.x - ox, 0, ox - d.x - d.w),
          dy = Math.max(d.y - oy, 0, oy - d.y - d.h);
        return r - Math.hypot(dx, dy) > 10;
      });
    }).length;
out.straddle = straddle(lbl, cam);
const lbl4 = G.labels.create(ctx, host);
const hostRM = Object.assign({}, host, { reduced: () => true });
const lblRM = G.labels.create(ctx, hostRM);
lblRM.draw(1000, cam, W, H, null, null, []);
out.reducedFirstFrame = lblRM
  .debug()
  .filter((d) => d.st)
  .every((d) => d.a === 1);
const t0 = Date.now();
for (let i = 0; i < 200; i++)
  lbl4.draw(1000 + i * 16, cam, W, H, null, null, []);
out.msPerFrame = +((Date.now() - t0) / 200).toFixed(3);
console.log(JSON.stringify(out));
