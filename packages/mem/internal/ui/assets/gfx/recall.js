"use strict";
(() => {
  const G = (window.MemGfx = window.MemGfx || {});
  const T = G.theme;
  const C = T.c;
  const TAU = 6.283185307179586;
  const HOP_MS = 140,
    MAX_WAVES = 3,
    TICK_MS = 9000,
    EMBER_MS = 900,
    GLOW_MS = 1600,
    BREATH_MS = 3300;
  const FAN1 = 40,
    FAN2 = 10,
    XCAP2 = 3,
    MAX1 = 260,
    MAX2 = 420,
    MAX_BREATH = 8,
    EXTRA_BREATH = 3;
  const MONO = '600 10px "JetBrains Mono", ui-monospace, monospace';
  const clamp = (v, a, b) => (v < a ? a : v > b ? b : v);
  const LIVE = T.hex(C.live),
    HEADC = T.hex(C.head);
  const rgba = (c, a) =>
    "rgba(" +
    c[0] +
    "," +
    c[1] +
    "," +
    c[2] +
    "," +
    (a < 0 ? 0 : a > 1 ? 1 : a).toFixed(3) +
    ")";
  const byDeg = (a, b) => b.node.deg - a.node.deg || a.node.gi - b.node.gi;

  const create = (host, wake) => {
    let nodes = [],
      links = [],
      off = new Int32Array(1),
      nbr = new Int32Array(0),
      eid = new Int32Array(0);
    const waves = [];
    let ticks = [];

    const setGraph = (ns, ls) => {
      nodes = ns;
      links = ls;
      const n = ns.length,
        deg = new Int32Array(n + 1);
      const ends = new Int32Array(ls.length * 2).fill(-1);
      ls.forEach((l, i) => {
        const a = l.a ? l.a.gi : -1,
          b = l.b ? l.b.gi : -1;
        if (
          a < 0 ||
          b < 0 ||
          a >= n ||
          b >= n ||
          ns[a] !== l.a ||
          ns[b] !== l.b
        )
          return;
        ends[i * 2] = a;
        ends[i * 2 + 1] = b;
        deg[a + 1]++;
        deg[b + 1]++;
      });
      for (let i = 0; i < n; i++) deg[i + 1] += deg[i];
      off = deg;
      nbr = new Int32Array(deg[n]);
      eid = new Int32Array(deg[n]);
      const at = deg.slice(0, n);
      for (let i = 0; i < ls.length; i++) {
        const a = ends[i * 2],
          b = ends[i * 2 + 1];
        if (a < 0) continue;
        nbr[at[a]] = b;
        eid[at[a]++] = i;
        nbr[at[b]] = a;
        eid[at[b]++] = i;
      }
    };

    const neighbours = (src, seen, cap, xcap) => {
      const gi = src.gi;
      let out = [];
      if (gi < 0 || gi + 1 >= off.length) return out;
      for (let j = off[gi]; j < off[gi + 1]; j++) {
        const v = nodes[nbr[j]];
        if (!v || v.hidden || seen.has(v)) continue;
        out.push({ node: v, e: eid[j] });
      }
      if (xcap < cap) {
        out.sort(byDeg);
        const same = out.filter((c) => c.node.topic === src.topic),
          cross = out.filter((c) => c.node.topic !== src.topic);
        cross.length = Math.min(cross.length, xcap);
        same.length = Math.min(same.length, cap - cross.length);
        out = same.concat(cross);
      } else if (out.length > cap) {
        out.sort(byDeg);
        out.length = cap;
      }
      return out;
    };

    const spread = (w, from, seen, level, fan, xcap, max) => {
      const best = new Map();
      for (const s of from) {
        for (const c of neighbours(s.node, seen, fan, xcap)) {
          const t = s.t + HOP_MS,
            cur = best.get(c.node);
          if (!cur || t < cur.t)
            best.set(c.node, { node: c.node, t, e: c.e, from: s.node });
        }
      }
      const list = [...best.values()].sort(
        (a, b) => a.t - b.t || b.node.deg - a.node.deg || a.node.gi - b.node.gi,
      );
      if (list.length > max) list.length = max;
      for (const h of list) {
        seen.add(h.node);
        const l = links[h.e];
        w.hops.push({ node: h.node, t: h.t, lv: level });
        w.edges.push({
          a: l.a,
          b: l.b,
          fwd: l.a === h.from,
          t: h.t - HOP_MS,
          lv: level,
        });
        w.end = Math.max(w.end, h.t);
      }
      return list;
    };

    const stop = (w) => {
      if (w.timer) clearTimeout(w.timer);
      w.timer = 0;
    };
    const drop = (i) => {
      stop(waves[i]);
      waves.splice(i, 1);
    };

    const fire = (n, seedList, label) => {
      const w = {
        n,
        label: label || "",
        seeds: [],
        hops: [],
        edges: [],
        topics: new Map(),
        end: 0,
        t0: Infinity,
        tmax: -Infinity,
        timer: 0,
      };
      const seen = new Set();
      for (const s of seedList) {
        const nd = s.node;
        if (!nd || nd.hidden || seen.has(nd) || !isFinite(s.t)) continue;
        seen.add(nd);
        w.seeds.push({ node: nd, t: s.t });
        w.t0 = Math.min(w.t0, s.t);
        w.tmax = Math.max(w.tmax, s.t);
        w.end = Math.max(w.end, s.t);
      }
      if (!w.seeds.length) return null;
      if (!host.reduced()) {
        const l1 = spread(w, w.seeds, seen, 1, FAN1, FAN1, MAX1);
        spread(w, l1, seen, 2, FAN2, XCAP2, MAX2);
        const note = (nd, t, amp) => {
          const cur = w.topics.get(nd.topic);
          if (!cur) w.topics.set(nd.topic, { t, amp, hits: 1 });
          else {
            cur.hits++;
            if (t < cur.t) cur.t = t;
            if (amp > cur.amp) cur.amp = amp;
          }
        };
        for (const s of w.seeds) note(s.node, s.t, 1);
        for (const h of w.hops) note(h.node, h.t, h.lv === 1 ? 0.8 : 0.5);
        const cap = Math.min(
          MAX_BREATH,
          new Set(w.seeds.map((s) => s.node.topic)).size + EXTRA_BREATH,
        );
        if (w.topics.size > cap) {
          const ranked = [...w.topics].sort(
            (a, b) =>
              b[1].amp - a[1].amp ||
              b[1].hits - a[1].hits ||
              a[1].t - b[1].t ||
              a[0] - b[0],
          );
          ranked.length = cap;
          w.topics = new Map(ranked);
        }
      }
      w.end += BREATH_MS;
      if (wake)
        w.timer = setTimeout(
          wake,
          Math.max(0, w.tmax + TICK_MS + 60 - performance.now()),
        );
      waves.push(w);
      while (waves.length > MAX_WAVES) drop(0);
      return w;
    };

    const clear = () => {
      for (const w of waves) stop(w);
      waves.length = 0;
      ticks = [];
    };

    const curve = (ctx, ax, ay, bx, by, fwd, s0, s1) => {
      const dx = bx - ax,
        dy = by - ay,
        mx = (ax + bx) / 2 - dy * T.curve,
        my = (ay + by) / 2 + dx * T.curve;
      const lo = fwd ? s0 : 1 - s1,
        hi = fwd ? s1 : 1 - s0,
        ul = 1 - lo,
        uh = 1 - hi;
      const w0 = ul * uh,
        w1 = lo * uh + hi * ul,
        w2 = lo * hi;
      ctx.moveTo(
        ul * ul * ax + 2 * ul * lo * mx + lo * lo * bx,
        ul * ul * ay + 2 * ul * lo * my + lo * lo * by,
      );
      ctx.quadraticCurveTo(
        w0 * ax + w1 * mx + w2 * bx,
        w0 * ay + w1 * my + w2 * by,
        uh * uh * ax + 2 * uh * hi * mx + hi * hi * bx,
        uh * uh * ay + 2 * uh * hi * my + hi * hi * by,
      );
    };
    const HP = [0, 0];
    const headAt = (ax, ay, bx, by, fwd, s) => {
      const dx = bx - ax,
        dy = by - ay,
        mx = (ax + bx) / 2 - dy * T.curve,
        my = (ay + by) / 2 + dx * T.curve;
      const t = fwd ? s : 1 - s,
        u = 1 - t;
      HP[0] = u * u * ax + 2 * u * t * mx + t * t * bx;
      HP[1] = u * u * ay + 2 * u * t * my + t * t * by;
      return HP;
    };

    const stats = { comets: 0, lit: 0 };
    const LV = [0, 1, 0.62],
      QN = 10;
    const flight = [[], [], []],
      fills = [[], []],
      rings = [];
    for (let i = 0; i <= QN; i++) {
      fills[0].push([]);
      fills[1].push([]);
      rings.push([]);
    }
    const emberQ = [];
    for (let i = 0; i <= QN; i++) emberQ.push([]);
    const LAYERS = [
      [0.2, 1.1, LIVE],
      [0.45, 1.6, LIVE],
      [0.85, 2.1, HEADC],
    ];
    const under = (ctx, now, cam, W, H) => {
      stats.comets = 0;
      stats.lit = 0;
      for (let i = waves.length - 1; i >= 0; i--)
        if (now > waves[i].end && now > waves[i].tmax + TICK_MS) drop(i);
      if (host.reduced()) return false;
      const k = cam.k,
        cx = cam.x,
        cy = cam.y,
        M = 40;
      let animating = false;
      flight[1].length = 0;
      flight[2].length = 0;
      for (const q of emberQ) q.length = 0;
      for (let i = 0; i <= QN; i++) {
        fills[0][i].length = 0;
        fills[1][i].length = 0;
        rings[i].length = 0;
      }
      for (const w of waves) {
        if (now < w.end) animating = true;
        for (const e of w.edges) {
          const a = now - e.t;
          if (a <= 0 || a > HOP_MS + EMBER_MS || (a >= HOP_MS && e.lv > 1))
            continue;
          const A = e.a,
            B = e.b;
          if (A.hidden || B.hidden) continue;
          const ax = A.x * k + cx,
            ay = A.y * k + cy,
            bx = B.x * k + cx,
            by = B.y * k + cy;
          if (
            Math.max(ax, bx) < -M ||
            Math.min(ax, bx) > W + M ||
            Math.min(ay, by) > H + M ||
            Math.max(ay, by) < -M
          )
            continue;
          stats.comets++;
          if (a < HOP_MS)
            flight[e.lv].push(ax, ay, bx, by, e.fwd ? 1 : 0, a / HOP_MS);
          else {
            const f = 1 - (a - HOP_MS) / EMBER_MS,
              q = Math.round(f * f * QN);
            if (q > 0) emberQ[q].push(ax, ay, bx, by, e.fwd ? 1 : 0);
          }
        }
        for (const h of w.hops) {
          const a = now - h.t;
          if (a < 0 || a > GLOW_MS) continue;
          const n = h.node;
          if (n.hidden || n.sc < 0.05) continue;
          const x = n.x * k + cx,
            y = n.y * k + cy;
          if (x < -M || x > W + M || y < -M || y > H + M) continue;
          stats.lit++;
          const r = Math.max(1.7, n.r * 0.78 * k * (0.35 + 0.65 * n.sc)),
            lv = h.lv === 1 ? 1 : 0.6,
            dim = 0.5 + 0.5 * clamp(n.hl, 0, 1);
          const qa = Math.round(0.85 * Math.exp(-a / 520) * lv * dim * QN);
          if (qa > 0) fills[a < 90 ? 1 : 0][qa].push(x, y, r + 1.2);
          if (a < 420 && h.lv === 1) {
            const p = a / 420,
              qr = Math.round((1 - p) * 0.6 * dim * QN);
            if (qr > 0) rings[qr].push(x, y, r + 2 + p * 12);
          }
        }
      }
      ctx.lineCap = "butt";
      ctx.lineWidth = 1.1;
      for (let q = 1; q <= QN; q++) {
        const list = emberQ[q];
        if (!list.length) continue;
        ctx.strokeStyle = rgba(LIVE, (0.5 * q) / QN);
        ctx.beginPath();
        for (let i = 0; i < list.length; i += 5)
          curve(
            ctx,
            list[i],
            list[i + 1],
            list[i + 2],
            list[i + 3],
            list[i + 4] === 1,
            0,
            1,
          );
        ctx.stroke();
      }
      ctx.lineCap = "round";
      for (let l = 2; l >= 1; l--) {
        const list = flight[l];
        if (!list.length) continue;
        for (let j = l === 1 ? 0 : 1; j < 3; j++) {
          const lay = LAYERS[j];
          ctx.strokeStyle = rgba(lay[2], lay[0] * LV[l]);
          ctx.lineWidth = lay[1];
          ctx.beginPath();
          let any = false;
          for (let i = 0; i < list.length; i += 6) {
            const p = list[i + 5],
              tail = Math.max(0, p - 0.5),
              c0 = j === 0 ? tail : tail + (p - tail) * (j === 1 ? 0.45 : 0.75),
              c1 =
                j === 0
                  ? tail + (p - tail) * 0.45
                  : j === 1
                    ? tail + (p - tail) * 0.75
                    : p;
            if (c1 - c0 < 1e-4) continue;
            any = true;
            curve(
              ctx,
              list[i],
              list[i + 1],
              list[i + 2],
              list[i + 3],
              list[i + 4] === 1,
              c0,
              c1,
            );
          }
          if (any) ctx.stroke();
        }
      }
      ctx.lineCap = "butt";
      for (let l = 2; l >= 1; l--) {
        const list = flight[l];
        if (!list.length) continue;
        if (l === 1) {
          ctx.fillStyle = rgba(LIVE, 0.28);
          ctx.beginPath();
          for (let i = 0; i < list.length; i += 6) {
            const h = headAt(
              list[i],
              list[i + 1],
              list[i + 2],
              list[i + 3],
              list[i + 4] === 1,
              list[i + 5],
            );
            ctx.moveTo(h[0] + 5.5, h[1]);
            ctx.arc(h[0], h[1], 5.5, 0, TAU);
          }
          ctx.fill();
        }
        ctx.fillStyle = rgba(HEADC, 1);
        ctx.beginPath();
        for (let i = 0; i < list.length; i += 6) {
          const h = headAt(
            list[i],
            list[i + 1],
            list[i + 2],
            list[i + 3],
            list[i + 4] === 1,
            list[i + 5],
          );
          ctx.moveTo(h[0] + 2.3, h[1]);
          ctx.arc(h[0], h[1], 2.3, 0, TAU);
        }
        ctx.fill();
      }
      for (let c = 0; c < 2; c++) {
        for (let q = 1; q <= QN; q++) {
          const list = fills[c][q];
          if (!list.length) continue;
          ctx.fillStyle = rgba(c ? HEADC : LIVE, q / QN);
          ctx.beginPath();
          for (let i = 0; i < list.length; i += 3) {
            ctx.moveTo(list[i] + list[i + 2], list[i + 1]);
            ctx.arc(list[i], list[i + 1], list[i + 2], 0, TAU);
          }
          ctx.fill();
        }
      }
      ctx.lineWidth = 1.2;
      for (let q = 1; q <= QN; q++) {
        const list = rings[q];
        if (!list.length) continue;
        ctx.strokeStyle = rgba(LIVE, q / QN);
        ctx.beginPath();
        for (let i = 0; i < list.length; i += 3) {
          ctx.moveTo(list[i] + list[i + 2], list[i + 1]);
          ctx.arc(list[i], list[i + 1], list[i + 2], 0, TAU);
        }
        ctx.stroke();
      }
      return animating;
    };

    const breath = (now, out) => {
      out.fill(0);
      if (host.reduced()) return false;
      let any = false;
      for (const w of waves) {
        for (const [ti, v] of w.topics) {
          const a = now - v.t;
          if (a < 0 || a > BREATH_MS || ti >= out.length) continue;
          const e = (a < 140 ? a / 140 : Math.exp(-(a - 140) / 650)) * v.amp;
          if (e > out[ti]) out[ti] = e;
          any = true;
        }
      }
      return any;
    };

    const pill = (n, now) => {
      let g = 0;
      for (const w of waves) {
        if (w.n !== n) continue;
        for (const s of w.seeds) {
          const a = now - s.t;
          if (a >= 0 && a < 520) g = Math.max(g, Math.pow(1 - a / 520, 2));
        }
      }
      return g;
    };

    const side = (x, y, W, H, m) => {
      const dx = x - W / 2,
        dy = y - H / 2,
        sx = Math.abs(dx) < 1e-6 ? Infinity : (W / 2 - m) / Math.abs(dx),
        sy = Math.abs(dy) < 1e-6 ? Infinity : (H / 2 - m) / Math.abs(dy);
      const t = Math.min(sx, sy);
      return {
        x: W / 2 + dx * t,
        y: H / 2 + dy * t,
        s: sx < sy ? (dx < 0 ? 0 : 1) : dy < 0 ? 2 : 3,
      };
    };
    const NORMAL = [
      [-1, 0],
      [1, 0],
      [0, -1],
      [0, 1],
    ];
    const over = (ctx, now, cam, W, H, obst, extra) => {
      const k = cam.k,
        cx = cam.x,
        cy = cam.y,
        groups = new Map();
      for (const w of waves) {
        for (const s of w.seeds) {
          const a = now - s.t,
            n = s.node;
          if (a < 0 || a > TICK_MS || n.hidden || n.sc < 0.05) continue;
          const x = n.x * k + cx,
            y = n.y * k + cy;
          if (x >= 0 && x <= W && y >= 0 && y <= H) continue;
          const p = side(x, y, W, H, 15),
            al = Math.min(
              clamp(a / 220, 0, 1),
              clamp((TICK_MS - a) / 1500, 0, 1),
            );
          const key = p.s + ":" + Math.round((p.s < 2 ? p.y : p.x) / 34);
          let g = groups.get(key);
          if (!g) {
            g = { s: p.s, x: 0, y: 0, nodes: [], al: 0, a0: a, label: w.label };
            groups.set(key, g);
          }
          g.x += p.x;
          g.y += p.y;
          g.nodes.push(n);
          g.al = Math.max(g.al, al);
          if (a < g.a0) {
            g.a0 = a;
            g.label = w.label;
          }
        }
      }
      ticks = [];
      let animating = false;
      const blocks = groups.size && extra ? obst.concat(extra()) : obst;
      for (const g of groups.values()) {
        const c = g.nodes.length;
        let x = g.x / c,
          y = g.y / c;
        const nrm = NORMAL[g.s],
          tx = nrm[1] === 0 ? 0 : 1,
          ty = nrm[1] === 0 ? 1 : 0;
        for (let tries = 0; tries < 10; tries++) {
          const sgn = tries & 1 ? -1 : 1,
            d = sgn * Math.ceil(tries / 2) * 30;
          const px = x + tx * d,
            py = y + ty * d;
          if (px < 14 || px > W - 14 || py < 14 || py > H - 14) continue;
          if (
            !blocks.some(
              (o) =>
                px > o.x - 14 &&
                px < o.x + o.w + 14 &&
                py > o.y - 14 &&
                py < o.y + o.h + 14,
            )
          ) {
            x = px;
            y = py;
            break;
          }
        }
        const pulse = g.a0 < 700 ? 1 + 0.55 * Math.pow(1 - g.a0 / 700, 2) : 1;
        if (g.a0 < 700) animating = true;
        if (g.al < 1 && g.a0 > 1000) animating = true;
        ticks.push({ x, y, r: 16, nodes: g.nodes, label: g.label, n: c });
        const ox = nrm[0],
          oy = nrm[1],
          px = -oy,
          py = ox;
        ctx.fillStyle = rgba(LIVE, 0.22 * g.al);
        ctx.beginPath();
        ctx.arc(x, y, 13 * pulse, 0, TAU);
        ctx.fill();
        ctx.fillStyle = rgba(LIVE, 0.95 * g.al);
        ctx.beginPath();
        ctx.moveTo(x + ox * 8 * pulse, y + oy * 8 * pulse);
        ctx.lineTo(
          x - ox * 5 * pulse + px * 9 * pulse,
          y - oy * 5 * pulse + py * 9 * pulse,
        );
        ctx.lineTo(
          x - ox * 5 * pulse - px * 9 * pulse,
          y - oy * 5 * pulse - py * 9 * pulse,
        );
        ctx.closePath();
        ctx.fill();
        if (c > 1) {
          ctx.font = MONO;
          ctx.textBaseline = "middle";
          ctx.textAlign = "center";
          ctx.fillStyle = rgba(HEADC, g.al);
          ctx.fillText(String(c), x - ox * 17, y - oy * 17);
          ctx.textAlign = "start";
        }
      }
      return animating;
    };

    const tickAt = (px, py) => {
      for (const t of ticks)
        if (Math.hypot(px - t.x, py - t.y) <= t.r) return t;
      return null;
    };

    const breathing = (now) => {
      const on = new Set();
      for (const w of waves)
        for (const [ti, v] of w.topics)
          if (now - v.t >= 0 && now - v.t <= BREATH_MS) on.add(ti);
      return on.size;
    };

    const info = (now) => ({
      breathing: breathing(now),
      waves: waves.length,
      seeds: waves.map((w) => w.seeds.length),
      hop1: waves.map((w) => w.hops.filter((h) => h.lv === 1).length),
      hop2: waves.map((w) => w.hops.filter((h) => h.lv === 2).length),
      edges: waves.map((w) => w.edges.length),
      comets: stats.comets,
      lit: stats.lit,
      ticks: ticks.length,
      tickList: ticks.map((t) => ({
        x: Math.round(t.x),
        y: Math.round(t.y),
        n: t.n,
      })),
      t0: waves.length ? waves[waves.length - 1].t0 : null,
      end: waves.length ? waves[waves.length - 1].end : null,
      active: waves.some((w) => now < w.end),
    });

    return {
      setGraph,
      fire,
      clear,
      under,
      over,
      breath,
      pill,
      tickAt,
      info,
      waves,
      HOP_MS,
    };
  };

  G.recall = { create, HOP_MS, TICK_MS };
})();
