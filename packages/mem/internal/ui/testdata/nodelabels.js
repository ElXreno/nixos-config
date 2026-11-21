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
  Uint8Array,
  Int32Array,
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
            (/mono/i.test(o.font) ? 0.6 : 0.52),
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
let seed = 4242;
const rnd = () => {
  seed = (seed * 1664525 + 1013904223) >>> 0;
  return seed / 4294967296;
};
const topics = rows.map((r, i) => ({
  i,
  id: r.id,
  label: r.label,
  size: 0,
  members: [],
  ax: r.x,
  ay: r.y,
  R: r.R,
  lx: r.lx,
  ly: r.ly,
}));
const nodes = [];
const mk = (key, ti, x, y, extra) => {
  const n = Object.assign(
    {
      id: nodes.length,
      key,
      type: "fact",
      topic: ti,
      summary: key,
      core: false,
      dangerous: false,
      hl: 1,
      hidden: false,
      x,
      y,
      deg: 1,
      rank: 0.5,
      shape: 0,
      nbr: null,
    },
    extra,
  );
  n.nbr = new Set([n]);
  nodes.push(n);
  topics[ti].members.push(n);
  topics[ti].size++;
  return n;
};
topics.forEach((t, ti) => {
  const cnt = Math.min(rows[ti].n, 70);
  for (let i = 0; i < cnt; i++) {
    const a = rnd() * 6.283,
      r = Math.sqrt(rnd()) * t.R * 0.85;
    mk(
      "k_" + t.id + "_" + i + "_long_record_name_for_labels",
      ti,
      t.ax + Math.cos(a) * r,
      t.ay + Math.sin(a) * r,
      { deg: 1 + Math.floor(rnd() * 8) },
    );
  }
  if (cnt >= 12) {
    for (let h = 0; h < 3; h++) {
      const a = rnd() * 6.283,
        r = Math.sqrt(rnd()) * t.R * 0.6;
      mk(
        "hub_" + t.id + "_" + h + "_with_a_rather_long_name",
        ti,
        t.ax + Math.cos(a) * r,
        t.ay + Math.sin(a) * r,
        { deg: 25 + h, hub: true, shape: 2 },
      );
    }
  }
  if (cnt >= 40) {
    for (let c = 0; c < 4; c++)
      mk(
        "core_" + t.id + "_" + c,
        ti,
        t.lx + (c - 1.5) * 9,
        t.ly + (c % 2) * 6,
        { core: true, shape: 1, deg: 3, dangerous: c === 0 },
      );
  }
});
for (const n of nodes)
  n.r = Math.min(
    12,
    Math.max(2.6, 2.6 + Math.sqrt(n.deg) * 1.35 + (n.core ? 1.2 : 0)),
  );
const byDeg = nodes.slice().sort((a, b) => b.deg - a.deg);
byDeg.forEach((n, i) => {
  n.rank = 1 - i / nodes.length;
});
const host = {
  reduced: () => false,
  selected: () => null,
  turnFocus: () => null,
  topics: () => topics,
  active: () => nodes,
  byDeg: () => byDeg,
};
const out = {};
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
const tdim = new Float32Array(topics.length * 2).fill(1);
for (let i = 0; i < topics.length; i++) tdim[i * 2 + 1] = 0;
const run = (lbl, W, H, cam, F, t0, n) => {
  let now = t0;
  for (let i = 0; i < n; i++) {
    lbl.draw(now, cam, W, H, F || null, tdim, []);
    now += 16;
  }
  return now;
};
const camFit = (W, H) => {
  const k = Math.min(W / ((x1 - x0) * 1.12), H / ((y1 - y0) * 1.12));
  return { k, x: W / 2 - ((x0 + x1) / 2) * k, y: H / 2 - ((y0 + y1) / 2) * k };
};
const placed = (lbl) => lbl.debug().filter((d) => d.st === 1 && d.x != null);

{
  const W = 2200,
    H = 1400,
    cam = camFit(W, H),
    lbl = G.labels.create(ctx, host);
  run(lbl, W, H, cam, null, 1000, 40);
  const nodeLabels = placed(lbl).filter((d) => d.id[0] === "n");
  out.bigOverviewNodeLabels = nodeLabels.length;
  const spill = nodeLabels.filter((d) => {
    const own = topics[nodes[+d.id.slice(1)].topic];
    return topics.some((t) => {
      if (t === own || !t.members.length) return false;
      const cx = t.ax * cam.k + cam.x,
        cy = t.ay * cam.k + cam.y,
        r = t.R * cam.k * 0.94;
      const dx = Math.max(d.x - cx, 0, cx - d.x - d.w),
        dy = Math.max(d.y - cy, 0, cy - d.y - d.h);
      return dx * dx + dy * dy < r * r;
    });
  });
  out.bigOverviewSpill = spill.length;
}

{
  const W = 980,
    H = 768,
    cam = camFit(W, H),
    lbl = G.labels.create(ctx, host);
  run(lbl, W, H, cam, null, 1000, 40);
  const lm = nodes
    .filter((n) => n.shape)
    .map((n) => {
      const X = n.x * cam.k + cam.x,
        Y = n.y * cam.k + cam.y,
        r = Math.max(n.r * 1.35, 1.5 / cam.k) * cam.k;
      return [X - r, Y - r, X + r, Y + r];
    });
  const tl = placed(lbl).filter((d) => d.id[0] === "t");
  out.topicLabels = tl.length;
  out.topicOnLandmark = tl.filter((d) =>
    lm.some(
      (r) => d.x < r[2] && d.x + d.w > r[0] && d.y < r[3] && d.y + d.h > r[1],
    ),
  ).length;
}

{
  const W = 980,
    H = 768,
    lbl = G.labels.create(ctx, host);
  const a = nodes.find((n) => n.key.startsWith("k_archive_3_")),
    b = mk(
      "k_archive_3_long_record_name_for_labels_other",
      a.topic,
      a.x + 60,
      a.y + 10,
      { deg: 2, r: 5 },
    );
  b.r = 5;
  a.nbr.add(b);
  b.nbr.add(a);
  a.key = "duplicate_prefix_that_is_rather_long_alpha_one";
  b.key = "duplicate_prefix_that_is_rather_long_alpha_two";
  const cam = { k: 3, x: W / 2 - a.x * 3 - 90, y: H / 2 - a.y * 3 };
  run(lbl, W, H, cam, { node: a, topic: a.topic }, 1000, 40);
  const ds = placed(lbl).filter(
    (d) => d.id === "n" + a.id || d.id === "n" + b.id,
  );
  out.dupShown = ds.length;
  out.dupDistinct = new Set(ds.map((d) => d.text)).size;
  out.dupClipped = ds.every((d) => d.text.length <= 28);
  a.nbr.delete(b);
}

{
  const W = 980,
    H = 768,
    cam0 = camFit(W, H);
  const hubs = nodes.filter((n) => n.hub).slice(0, 40);
  let bad = 0,
    shown = 0,
    noRing = 0;
  for (const h of hubs) {
    const lbl = G.labels.create(ctx, host),
      cam = { k: 5.5, x: W / 2 - h.x * 5.5, y: H / 2 - h.y * 5.5 };
    run(lbl, W, H, cam, null, 1000, 40);
    const d = lbl.debug().find((e) => e.id === "n" + h.id);
    if (!d || !d.st) {
      bad++;
      continue;
    }
    shown++;
    if (!d.ring) noRing++;
  }
  out.hubPoses = hubs.length;
  out.hubShown = shown;
  out.hubNoRing = noRing;
  void cam0;
}

{
  const W = 980,
    H = 768,
    ks = [0.8, 1, 1.2, 1.6, 2.2, 3, 4.5];
  let tog3 = 0,
    poses = 0,
    togAll = 0;
  for (let p = 0; p < 28; p++) {
    const k = ks[p % ks.length],
      pick = nodes[Math.floor(rnd() * nodes.length)];
    const dx = (rnd() < 0.5 ? -1 : 1) * (1 + rnd() * 2),
      dy = (rnd() < 0.5 ? -1 : 1) * rnd() * 2;
    const lbl = G.labels.create(ctx, host);
    const at = (s) => ({
      k,
      x: W / 2 - pick.x * k - dx * s,
      y: H / 2 - pick.y * k - dy * s,
    });
    let now = run(lbl, W, H, at(0), null, 1000, 40);
    let prev = new Set(lbl.shownIds());
    const tog = {};
    for (let s = 1; s <= 80; s++) {
      now = run(lbl, W, H, at(s), null, now, 3);
      const cur = new Set(lbl.shownIds());
      for (const id of cur) if (!prev.has(id)) tog[id] = (tog[id] || 0) + 1;
      for (const id of prev) if (!cur.has(id)) tog[id] = (tog[id] || 0) + 1;
      prev = cur;
    }
    poses++;
    for (const id in tog) {
      togAll++;
      if (tog[id] >= 3) tog3++;
    }
  }
  out.panPoses = poses;
  out.panToggles = togAll;
  out.panFlicker = tog3;
}
{
  const W = 980,
    H = 768;
  const solo = {
    id: 9001,
    key: "solo_record_for_the_zoom_reveal",
    type: "fact",
    topic: 0,
    summary: "s",
    core: false,
    dangerous: false,
    hl: 1,
    hidden: false,
    x: 5e4,
    y: 5e4,
    deg: 1,
    rank: 0.5,
    shape: 0,
    r: 4,
  };
  solo.nbr = new Set([solo]);
  const h = {
    reduced: () => false,
    selected: () => null,
    turnFocus: () => null,
    topics: () => topics,
    active: () => [solo],
    byDeg: () => [solo],
  };
  const thr = 1 + 1.9 * Math.pow(0.5, 1.5);
  const at = (k) => ({ k, x: W / 2 - solo.x * k, y: H / 2 - solo.y * k });
  const lbl = G.labels.create(ctx, h);
  const look = () => lbl.debug().find((d) => d.id === "n9001");
  run(lbl, W, H, at(thr * 0.9), null, 1000, 30);
  out.revealBelow = !!look() && look().st === 1;
  let now = run(lbl, W, H, at(thr + 0.03), null, 2000, 40);
  const d = look();
  out.revealShown = !!d && d.st === 1;
  out.revealOpaque = !!d && d.a === 1 && d.ia === 1;
  now = run(lbl, W, H, at(thr * 0.97), null, now, 40);
  out.revealHysteresis = !!look() && look().st === 1;
  run(lbl, W, H, at(thr * 0.85), null, now, 40);
  out.revealGone = !look() || look().st === 0;
}
console.log(JSON.stringify(out));
