"use strict";
(() => {
  const G = (window.MemGfx = window.MemGfx || {});
  const T = G.theme;
  const C = T.c;
  const SYS = 'system-ui, "Liberation Sans", "DejaVu Sans", sans-serif';
  const MONO = '"JetBrains Mono", ui-monospace, monospace';
  const FONT_K = "11px " + MONO;
  const FONT_B = "500 11px " + MONO;
  const FONT_S = "11px " + SYS;
  const CELL = 60,
    HOLD = 300,
    FADE = 160,
    CAP = 400,
    HUB_TOP = 3,
    KEY_LEN = 28;
  const MARGIN = 12,
    VC = { f: 0, g: 3, F: 9, G: 12, h: 70, H: 74, i: 78, I: 82 },
    DISC_W = 40,
    TERR_W = 2.5,
    SELF_W = 1.5;
  const PAN = 8,
    ZOOM = 0.015,
    KEY_AREA = 19000,
    LM_W = 30,
    CONTAIN_K = 1.4,
    HUB_RS = 105;
  const ANG = [-90, 90, -60, -120, 60, 120, 0, 180, -30, -150, 30, 150].map(
    (a) => (a * Math.PI) / 180,
  );
  const COS = ANG.map(Math.cos),
    SIN = ANG.map(Math.sin);
  const clamp = (v, a, b) => (v < a ? a : v > b ? b : v);
  const smooth = (a, b, x) => G.terrain.smooth(a, b, x);
  const base = (s) => s.replace(/\s*\([^)]*\)/g, "").trim();
  const words = (s, n) => s.split(/\s+/).slice(0, n).join(" ");
  const head = (s) => s.slice(0, KEY_LEN - 3) + "...";
  const middle = (s) => s.slice(0, KEY_LEN - 12) + "..." + s.slice(-9);
  const headOf = (label) => {
    const s = base(label),
      colon = s.indexOf(":");
    let h;
    if (colon > 0) h = s.slice(0, colon).trim();
    else {
      const parts = s.split(",").map((x) => x.trim());
      h = parts[0];
      if (h.length < 9 && parts.length > 1) h = parts[0] + ", " + parts[1];
    }
    if (h.length > 26) {
      const cut = h.split(/\s+and\s+/)[0];
      h = cut.length >= 8 && cut.length < h.length ? cut : words(h, 3);
    }
    return h;
  };
  const balance = (text) => {
    const ws = text.split(" ");
    if (ws.length < 2 || (ws.length === 2 && text.length < 15)) return null;
    let best = null,
      bw = Infinity;
    for (let i = 1; i < ws.length; i++) {
      const a = ws.slice(0, i).join(" "),
        b = ws.slice(i).join(" "),
        w = Math.max(a.length, b.length);
      if (w < bw) {
        bw = w;
        best = [a, b];
      }
    }
    return best;
  };
  const tailOf = (label) => {
    const s = base(label),
      colon = s.indexOf(":");
    if (colon < 0) return "";
    return words(
      s
        .slice(colon + 1)
        .split(/\s+and\s+|,/)[0]
        .trim(),
      3,
    );
  };
  const shortNames = (topics) => {
    const heads = topics.map((t) => headOf(t.label || t.id || ""));
    const count = new Map();
    for (const h of heads) count.set(h, (count.get(h) || 0) + 1);
    return topics.map((t, i) => {
      const h = heads[i];
      if (count.get(h) < 2) return h;
      const tail = tailOf(t.label || "");
      return tail ? h + ": " + tail : h;
    });
  };

  const create = (ctx, host, opts) => {
    let dead = opts && opts.dead != null ? opts.dead : 1;
    const lw = {};
    const lgrid = new Map(),
      disc = { src: null, n: 0, maxR: 0 };
    let cK = 1,
      cX = 0,
      cY = 0;
    const ents = new Map();
    const terrs = [],
      lms = [];
    const lmc = { src: null, n: 0, list: [] },
      dup = { src: null, n: 0, set: new Set() };
    let cap = CAP,
      names = [],
      namesFor = null,
      count = 0,
      lastMs = 0,
      placeMs = 0,
      lastItems = 0,
      prevNow = 0,
      ref = null,
      held = false;
    const textW = (s, font) => {
      const k = font + "|" + s;
      let w = lw[k];
      if (w == null) w = lw[k] = ((ctx.font = font), ctx.measureText(s).width);
      return w;
    };
    const resetText = () => {
      for (const k in lw) delete lw[k];
    };
    const cells = (x0, y0, x1, y1, fn) => {
      const cx0 = Math.floor(x0 / CELL),
        cx1 = Math.floor(x1 / CELL),
        cy0 = Math.floor(y0 / CELL),
        cy1 = Math.floor(y1 / CELL);
      for (let cx = cx0; cx <= cx1; cx++)
        for (let cy = cy0; cy <= cy1; cy++)
          if (fn(cx * 10007 + cy)) return true;
      return false;
    };
    const hit = (list, x0, y0, x1, y1) => {
      if (!list) return false;
      for (const r of list)
        if (x0 < r[2] && x1 > r[0] && y0 < r[3] && y1 > r[1]) return true;
      return false;
    };
    const DCELL = 24;
    const discGrid = (act) => {
      let x0 = Infinity,
        y0 = Infinity,
        x1 = -Infinity,
        y1 = -Infinity;
      for (const n of act) {
        if (n.x < x0) x0 = n.x;
        if (n.x > x1) x1 = n.x;
        if (n.y < y0) y0 = n.y;
        if (n.y > y1) y1 = n.y;
      }
      if (!act.length) {
        x0 = y0 = x1 = y1 = 0;
      }
      const gw = Math.floor((x1 - x0) / DCELL) + 1,
        gh = Math.floor((y1 - y0) / DCELL) + 1,
        cells = gw * gh;
      if (!disc.start || disc.start.length < cells + 1) {
        disc.start = new Int32Array(cells + 1);
        disc.fill = new Int32Array(cells + 1);
      } else disc.start.fill(0, 0, cells + 1);
      const start = disc.start,
        cell = disc.cell,
        px = disc.px,
        py = disc.py;
      for (let i = 0; i < act.length; i++) {
        const n = act[i];
        px[i] = n.x;
        py[i] = n.y;
        const c =
          Math.floor((n.y - y0) / DCELL) * gw + Math.floor((n.x - x0) / DCELL);
        cell[i] = c;
        start[c + 1]++;
      }
      for (let i = 0; i < cells; i++) start[i + 1] += start[i];
      const fill = disc.fill;
      fill.set(start.subarray(0, cells));
      const idx = disc.idx;
      for (let i = 0; i < act.length; i++) idx[fill[cell[i]]++] = i;
      disc.gw = gw;
      disc.gh = gh;
      disc.ox = x0;
      disc.oy = y0;
    };
    const ensureDiscs = () => {
      const act = host.active(),
        ver = host.posVer ? host.posVer() : 0;
      if (disc.src === act && disc.n === act.length) {
        for (let i = 0; i < act.length; i++) disc.hl[i] = act[i].hl;
        if (disc.ver !== ver) {
          disc.ver = ver;
          discGrid(act);
        }
        return;
      }
      disc.src = act;
      disc.n = act.length;
      disc.ver = ver;
      let mr = 0;
      for (const n of act) mr = Math.max(mr, n.r * (n.shape ? 1.35 : 1));
      disc.cell = new Int32Array(act.length);
      disc.idx = new Int32Array(act.length);
      disc.px = new Float32Array(act.length);
      disc.py = new Float32Array(act.length);
      discGrid(act);
      disc.ix = new Map(act.map((n, i) => [n, i]));
      disc.hl = Float32Array.from(act, (n) => n.hl);
      disc.pd = Uint8Array.from(act, (n) => (n.dangerous ? 1 : 0));
      disc.pr = Float32Array.from(act, (n) => n.r * (n.shape ? 1.35 : 1));
      disc.maxR = mr;
    };
    const discOv = (c, own, lim) => {
      const k = cK,
        wr = disc.maxR + 6 / k,
        x0 = c.x - 1,
        y0 = c.y - 1,
        x1 = c.x + c.w + 1,
        y1 = c.y + c.h + 1,
        area = (x1 - x0) * (y1 - y0),
        cut = (lim * area) / 0.785;
      const rk = 1.5 / k,
        gw = disc.gw,
        start = disc.start,
        idx = disc.idx,
        px = disc.px,
        py = disc.py,
        pr = disc.pr,
        hl = disc.hl,
        pd = disc.pd,
        oi = disc.ix.get(own);
      const cx0 = Math.max(
          0,
          Math.floor(((x0 - cX) / k - wr - disc.ox) / DCELL),
        ),
        cx1 = Math.min(
          gw - 1,
          Math.floor(((x1 - cX) / k + wr - disc.ox) / DCELL),
        );
      const cy0 = Math.max(
          0,
          Math.floor(((y0 - cY) / k - wr - disc.oy) / DCELL),
        ),
        cy1 = Math.min(
          disc.gh - 1,
          Math.floor(((y1 - cY) / k + wr - disc.oy) / DCELL),
        );
      let s = 0;
      for (let cy = cy0; cy <= cy1; cy++) {
        for (let cx = cx0; cx <= cx1; cx++) {
          const cell = cy * gw + cx;
          for (let j = start[cell], e = start[cell + 1]; j < e; j++) {
            const i = idx[j];
            if (i === oi || hl[i] < 0.3) continue;
            const X = px[i] * k + cX,
              Y = py[i] * k + cY,
              rr = Math.max(pr[i], rk) * k + (pd[i] ? 4 : 1);
            const ox = Math.min(X + rr, x1) - Math.max(X - rr, x0),
              oy = Math.min(Y + rr, y1) - Math.max(Y - rr, y0);
            if (ox > 0 && oy > 0) {
              s += ox * oy;
              if (s > cut) return (s * 0.785) / area;
            }
          }
        }
      }
      return (s * 0.785) / area;
    };
    const blocked = (x0, y0, x1, y1) =>
      cells(x0, y0, x1, y1, (key) => hit(lgrid.get(key), x0, y0, x1, y1));
    const reserve = (x0, y0, x1, y1) => {
      const r = [x0, y0, x1, y1];
      cells(x0, y0, x1, y1, (key) => {
        const c = lgrid.get(key);
        if (c) c.push(r);
        else lgrid.set(key, [r]);
        return false;
      });
    };
    const landmarks = () => {
      const act = host.active();
      if (lmc.src !== act || lmc.n !== act.length) {
        lmc.src = act;
        lmc.n = act.length;
        lmc.list = act.filter((n) => n.shape || n.dangerous);
      }
      return lmc.list;
    };
    const dups = () => {
      const act = host.active();
      if (dup.src !== act || dup.n !== act.length) {
        const seen = new Set(),
          set = new Set();
        for (const n of act) {
          if (n.key.length <= KEY_LEN) continue;
          const h = head(n.key);
          if (seen.has(h)) set.add(h);
          else seen.add(h);
        }
        dup.src = act;
        dup.n = act.length;
        dup.set = set;
      }
      return dup.set;
    };
    const clip = (key) =>
      key.length <= KEY_LEN
        ? key
        : dups().has(head(key))
          ? middle(key)
          : head(key);
    const synced = (topics) => {
      if (namesFor === topics && names.length === topics.length) return;
      namesFor = topics;
      names = shortNames(topics);
    };

    const nodeItem = (id, n, pri, a, color, bold, cam, W, H, k, showTitle) => {
      const X = n.x * k + cam.x,
        Y = n.y * k + cam.y;
      const font = bold ? FONT_B : FONT_K,
        text = clip(n.key),
        w1 = textW(text, font);
      const shape = n.shape ? 1.35 : 1;
      const rr = Math.max(n.r * shape, 1.5 / k) * k + (n.dangerous ? 6 : 4);
      if (X + w1 < -20 || X - w1 > W + 20 || Y < -90 || Y > H + 90) return null;
      const lines = [{ text, font, color, w: w1 }];
      let w = w1,
        h = 14;
      if (showTitle > 0) {
        const s = (n.summary || "").replace(/`/g, ""),
          t = s.length > 36 ? s.slice(0, 36).trimEnd() + "..." : s;
        const tw = textW(t, FONT_S);
        lines.push({
          text: t,
          font: FONT_S,
          color: C.text2,
          a: showTitle,
          w: tw,
        });
        w = Math.max(w, tw);
        h = 28;
      }
      const d = rr * 0.72;
      const spec = [
        ["b", X - w / 2, Y + rr + 3, "c"],
        ["a", X - w / 2, Y - rr - 3 - h, "c"],
        ["r", X + rr + 5, Y - h / 2, "l"],
        ["l", X - rr - 5 - w, Y - h / 2, "r"],
        ["br", X + d, Y + d + 1, "l"],
        ["bl", X - d - w, Y + d + 1, "r"],
        ["ar", X + d, Y - d - 1 - h, "l"],
        ["al", X - d - w, Y - d - 1 - h, "r"],
      ];
      const lead =
        pri >= 4 ? [X, Y, Math.max(n.r * shape, 1.5 / k) * k + 2.6] : null;
      const cands = spec.map(([key, x, y, al], i) => ({
        k: key,
        x,
        y,
        w,
        h,
        lines,
        al,
        lead,
        base: i * 0.5,
      }));
      const byKey = {};
      for (const c of cands) byKey[c.k] = c;
      const tol = pri < 5 ? 1e9 : 0.2;
      let fit = null;
      if (pri >= 4 && k < CONTAIN_K) {
        const own = n.topic;
        fit = (c) => {
          const x1 = c.x + c.w,
            y1 = c.y + c.h;
          for (const o of terrs) {
            if (o.i === own) continue;
            const dx = o.cx < c.x ? c.x - o.cx : o.cx > x1 ? o.cx - x1 : 0,
              dy = o.cy < c.y ? c.y - o.cy : o.cy > y1 ? o.cy - y1 : 0;
            if (dx * dx + dy * dy < o.r * o.r) return false;
          }
          return true;
        };
      }
      return {
        id,
        pri,
        a,
        nv: 1,
        gen: () => cands,
        find: (ck) => byKey[ck] || null,
        node: n,
        tol,
        fit,
        weight: n.deg || 0,
        halo: 3.5,
        key: pri >= 5,
        ring: pri >= 4,
      };
    };

    const topicItem = (t, i, cam, W, H, k, F, tdim) => {
      const Rs = t.R * k,
        cx = t.ax * k + cam.x,
        cy = t.ay * k + cam.y;
      if (
        cx + Rs < -60 ||
        cx - Rs > W + 60 ||
        cy + Rs < -60 ||
        cy - Rs > H + 60
      )
        return null;
      let vis = 0;
      for (const m of t.members)
        if (!m.hidden) {
          vis++;
          break;
        }
      if (!vis) return null;
      const sc = clamp(Math.sqrt((W * H) / (980 * 768)), 0.78, 1);
      const size =
        Math.min(18, Math.max(11, 8 + (1.1 * Math.sqrt(t.size)) / 2)) * sc;
      const p = smooth(55, 90, Rs);
      const focusT = F && F.topic === i;
      const dim = tdim ? tdim[i * 2] : 1;
      const color = focusT ? C.bright : C.text;
      const mono = "10px " + MONO;
      const fontOf = (z) => "500 " + (size * z).toFixed(2) + "px " + SYS;
      const mk = (parts, f, lh, id) => {
        const ls = parts.map((text) => ({
          text,
          font: f,
          color,
          w: textW(text, f),
        }));
        return {
          id,
          lines: ls,
          w: Math.max(...ls.map((l) => l.w)),
          h: lh * ls.length,
        };
      };
      const two = balance(names[i]);
      const first = words(names[i], 1).replace(/[:,]$/, "");
      const vs = [mk([names[i]], fontOf(1), size * 1.25, "f")];
      if (two) vs.push(mk(two, fontOf(1), size * 1.25, "g"));
      const small = [mk([names[i]], fontOf(0.82), size * 0.82 * 1.25, "F")];
      if (two) small.push(mk(two, fontOf(0.82), size * 0.82 * 1.25, "G"));
      const tiny = [];
      if (first.length >= 4 && first !== names[i])
        tiny.push(
          mk([first], fontOf(1), size * 1.25, "h"),
          mk([first], fontOf(0.82), size * 0.82 * 1.25, "H"),
        );
      if (t.id && t.id.length < names[i].length)
        tiny.push(
          mk([t.id], mono, 13, "i"),
          mk([t.id], "9px " + MONO, 11, "I"),
        );
      const variants = vs.concat(small, tiny);
      const lx = (t.lx == null ? t.ax : t.lx) * k + cam.x,
        ly = (t.ly == null ? t.ay : t.ly) * k + cam.y;
      const gaps = Rs < 90 ? [10, 22, 38] : [5, 14];
      const lead = [cx, cy, Rs];
      const sticky =
        cy - Rs < 6 &&
        cy + Rs > 60 &&
        cx - Rs < W / 2 &&
        cx + Rs > W / 2 &&
        cy < H / 2 + Rs;
      const clipped =
        cx - Rs < 6 || cx + Rs > W - 6 || cy - Rs < 6 || cy + Rs > H - 6;
      const inside = (v) => v.w + 10 <= 1.85 * Rs && v.h + 6 <= 1.6 * Rs;
      const box = (v, x, y, key, ld, bs, own) => ({
        k: key + v.id,
        x,
        y,
        w: v.w,
        h: v.h,
        lines: v.lines,
        al: "c",
        lead: ld ? lead : null,
        base: bs,
        own,
      });
      const dist = (c) =>
        Math.hypot(
          Math.max(c.x - cx, 0, cx - c.x - c.w),
          Math.max(c.y - cy, 0, cy - c.y - c.h),
        ) - Rs;
      const cornersIn = (c) => {
        const r = Rs * 0.97;
        return (
          Math.hypot(c.x - cx, c.y - cy) <= r &&
          Math.hypot(c.x + c.w - cx, c.y - cy) <= r &&
          Math.hypot(c.x - cx, c.y + c.h - cy) <= r &&
          Math.hypot(c.x + c.w - cx, c.y + c.h - cy) <= r
        );
      };
      const vw = Math.min(cx + Rs, W) - Math.max(cx - Rs, 0),
        vh = Math.min(cy + Rs, H) - Math.max(cy - Rs, 0);
      const was = ents.get("t" + t.id),
        lim = was && was.shown ? 22 : 34;
      const sliver = clipped && Rs > 40 && (vw < lim || vh < lim);
      const special = (v, vi) => {
        const out = [],
          tp = vi < vs.length + small.length;
        if (sticky && v === vs[0]) {
          const x = clamp(cx - v.w / 2, 8, Math.max(8, W - v.w - 8)),
            y = clamp(8, 8, Math.max(8, H - v.h - 8));
          if (
            Math.hypot(x + v.w / 2 - cx, y + v.h / 2 - cy) <=
            Rs - Math.min(8, v.h / 2)
          )
            out.push(box(v, x, y, "s", 0, 0, true));
        }
        if (clipped && tp) {
          const x0 = Math.max(cx - Rs, 6),
            x1 = Math.min(cx + Rs, W - 6),
            y0 = Math.max(cy - Rs, 6),
            y1 = Math.min(cy + Rs, H - 6);
          if (x1 > x0 && y1 > y0) {
            const c = box(
              v,
              (x0 + x1) / 2 - v.w / 2,
              (y0 + y1) / 2 - v.h / 2,
              "v",
              0,
              0.5,
              true,
            );
            if (
              c.x >= 4 &&
              c.x + c.w <= W - 4 &&
              c.y >= 2 &&
              c.y + c.h <= H - 2 &&
              cornersIn(c)
            )
              out.push(c);
          }
        }
        if (tp && inside(v)) {
          const c = box(v, lx - v.w / 2, ly - v.h / 2, "c", 0, 0, true),
            top = box(
              v,
              cx - v.w / 2,
              cy - Rs - gaps[0] - v.h,
              "t",
              1,
              0,
              false,
            );
          const x = clamp(c.x + (top.x - c.x) * p, 8, Math.max(8, W - v.w - 8)),
            y = clamp(c.y + (top.y - c.y) * p, 8, Math.max(8, H - v.h - 8));
          if (
            Math.hypot(x + v.w / 2 - cx, y + v.h / 2 - cy) <=
            Rs - Math.min(8, v.h / 2)
          )
            out.push({ ...c, x, y, lead: p > 0.5 ? lead : null });
        }
        return out;
      };
      const around = (v, ai, gi) => {
        if (sliver) return null;
        const g = gaps[gi],
          dx = COS[ai],
          dy = SIN[ai],
          off = Rs + g + (Math.abs(dx) * v.w) / 2 + (Math.abs(dy) * v.h) / 2;
        const x = clamp(cx + dx * off - v.w / 2, 4, Math.max(4, W - v.w - 4)),
          y = clamp(cy + dy * off - v.h / 2, 2, Math.max(2, H - v.h - 2));
        const c = box(
          v,
          x,
          y,
          "o" + ai + gi,
          1,
          (gi * ANG.length + ai) * 0.3,
          false,
        );
        return dist(c) > g + 6 ? null : c;
      };
      const gen = (vi, bound) => {
        const v = variants[vi],
          vc = VC[v.id],
          out = special(v, vi);
        for (let gi = 0; gi < gaps.length; gi++) {
          if (gi * ANG.length * 0.3 + vc >= bound) break;
          for (let ai = 0; ai < ANG.length; ai++) {
            if ((gi * ANG.length + ai) * 0.3 + vc >= bound) break;
            const c = around(v, ai, gi);
            if (c) out.push(c);
          }
        }
        for (const c of out) c.vc = vc;
        return out;
      };
      const find = (ck) => {
        const id = ck[ck.length - 1],
          vi = variants.findIndex((v) => v.id === id);
        if (vi < 0) return null;
        const v = variants[vi];
        let c = null;
        if (ck[0] === "o") {
          const num = ck.slice(1, -1);
          c = around(v, +num.slice(0, -1), +num.slice(-1));
        } else c = special(v, vi).find((q) => q.k === ck) || null;
        if (c) c.vc = VC[id];
        return c;
      };
      const cost = (c, bound) => {
        let s = 0;
        const x1 = c.x + c.w,
          y1 = c.y + c.h;
        for (const r of lms) {
          if (r[0] < x1 && r[2] > c.x && r[1] < y1 && r[3] > c.y) {
            s += LM_W;
            if (s > bound) return s;
          }
        }
        for (const o of terrs) {
          if (
            o.cx + o.r < c.x ||
            o.cx - o.r > x1 ||
            o.cy + o.r < c.y ||
            o.cy - o.r > y1
          )
            continue;
          if (o.i === i && c.own) continue;
          const d = Math.hypot(
            (o.cx < c.x ? c.x : o.cx > x1 ? x1 : o.cx) - o.cx,
            (o.cy < c.y ? c.y : o.cy > y1 ? y1 : o.cy) - o.cy,
          );
          if (d < o.r) {
            s += (o.i === i ? SELF_W : TERR_W) * (o.r - d);
            if (s > bound) return s;
          }
        }
        return s;
      };
      return {
        id: "t" + t.id,
        pri: focusT ? 2.5 : 3,
        a: dim,
        nv: variants.length,
        vcs: variants.map((v) => VC[v.id]),
        gen,
        find,
        cost,
        weight: t.size,
        halo: 4,
        topic: true,
      };
    };

    const buildItems = (now, cam, W, H, F, tdim) => {
      const k = cam.k,
        items = [],
        seen = new Set();
      const sel = host.selected();
      const add = (it) => {
        if (it) items.push(it);
      };
      const addNode = (n, pri, a, color, bold, title) => {
        if (seen.has(n) || a < 0.03) return;
        seen.add(n);
        add(nodeItem("n" + n.id, n, pri, a, color, bold, cam, W, H, k, title));
      };
      const titleA = smooth(3, 3.5, k);
      if (F && F.node) {
        addNode(F.node, 0, 1, C.bright, true, titleA);
        for (const o of F.node.nbr)
          if (o !== F.node && !o.hidden)
            addNode(o, 1, clamp(o.hl, 0, 1), C.text, false, 0);
      }
      if (sel && (!F || F.node !== sel) && sel.hl > 0.3)
        addNode(sel, 1.2, sel.hl, C.bright, true, titleA);
      const tf = host.turnFocus();
      if (tf)
        for (const o of tf.set)
          if (!o.hidden) addNode(o, 2, 1, C.bright, false, 0);
      const topics = host.topics();
      synced(topics);
      terrs.length = 0;
      lms.length = 0;
      for (const n of landmarks()) {
        if (n.hidden || n.hl < 0.3) continue;
        const X = n.x * k + cam.x,
          Y = n.y * k + cam.y;
        if (X < -40 || X > W + 40 || Y < -40 || Y > H + 40) continue;
        const rr = Math.max(n.r * 1.35, 1.5 / k) * k + 2;
        lms.push([X - rr, Y - rr, X + rr, Y + rr]);
      }
      for (let i = 0; i < topics.length; i++) {
        const t = topics[i];
        if (!(t.R > 0) || !t.members.length) continue;
        const Rs = t.R * k,
          cx = t.ax * k + cam.x,
          cy = t.ay * k + cam.y;
        if (
          cx + Rs > -80 &&
          cx - Rs < W + 80 &&
          cy + Rs > -80 &&
          cy - Rs < H + 80
        )
          terrs.push({ i, cx, cy, r: Rs * 0.94 });
      }
      for (let i = 0; i < topics.length; i++) {
        const t = topics[i];
        if (!(t.R > 0) || !t.members.length) continue;
        if (t.size < 1 && !(F && F.topic === i)) continue;
        add(topicItem(t, i, cam, W, H, k, F, tdim));
      }
      const zoomG = smooth(0.9, 1.2, k);
      const gate = (ti) =>
        Math.max(smooth(HUB_RS, HUB_RS + 35, topics[ti].R * k), zoomG);
      if (k > 0.3) {
        const top = new Set(),
          per = new Map();
        if (k < 1.4) {
          for (const n of host.byDeg()) {
            if (!(n.shape || n.dangerous) || n.hidden || n.hl < 0.3) continue;
            const c = per.get(n.topic) || 0;
            if (c < HUB_TOP) {
              per.set(n.topic, c + 1);
              top.add(n);
            }
          }
        }
        let nBase = 0;
        for (const n of host.byDeg()) {
          if (n.hidden || n.hl < 0.3 || seen.has(n)) continue;
          const X = n.x * k + cam.x,
            Y = n.y * k + cam.y;
          if (X < -100 || X > W + 100 || Y < -80 || Y > H + 80) continue;
          if (n.shape || n.dangerous) {
            const g = gate(n.topic);
            if (g > 0.3 && (k >= 1.4 || top.has(n))) {
              addNode(
                n,
                4,
                clamp((g - 0.3) / 0.4, 0, 1) * n.hl,
                C.bright,
                true,
                titleA,
              );
              continue;
            }
          }
          if (k < 0.6) continue;
          const thr =
              (F && F.topic === n.topic ? 0.6 : 1) *
              (1 + 1.9 * Math.pow(1 - n.rank, 1.5)),
            was = ents.get("n" + n.id);
          if (k < thr * (was && was.shown ? 0.94 : 1)) continue;
          const a = n.hl;
          if (a < 0.2) continue;
          if (++nBase > cap) {
            const e = ents.get("n" + n.id);
            if (!(e && e.state)) continue;
          }
          addNode(n, 5, a, n.hl > 0.9 ? C.text : C.m2, false, titleA);
        }
      }
      const old = (it) => {
        const e = ents.get(it.id);
        return e && e.shown ? 0 : 1;
      };
      for (const it of items) it.old = old(it);
      items.sort(
        (a, b) => a.pri - b.pri || a.old - b.old || b.weight - a.weight,
      );
      return items;
    };

    const moved = (cam, r, W, H) => {
      const q = cam.k / r.k;
      let m = 0;
      for (const sx of [0, W]) {
        for (const sy of [0, H])
          m = Math.max(
            m,
            Math.abs((sx - r.x) * q + cam.x - sx),
            Math.abs((sy - r.y) * q + cam.y - sy),
          );
      }
      return m;
    };

    const signature = (cam, W, H, F, obst) => {
      const act = host.active(),
        sel = host.selected(),
        tf = host.turnFocus();
      let hs = 0;
      for (const n of act) hs += n.hl;
      let o = "";
      if (obst)
        for (const r of obst)
          o +=
            (r.x | 0) +
            "," +
            (r.y | 0) +
            "," +
            (r.w | 0) +
            "," +
            (r.h | 0) +
            ";";
      return [
        W,
        H,
        cap,
        act.length,
        Math.round(hs * 4),
        F && F.node ? F.node.id : "-",
        F && F.topic != null ? F.topic : "-",
        sel ? sel.id : "-",
        tf ? tf.set.size : "-",
        host.reduced() ? 1 : 0,
        o,
      ].join("|");
    };

    const draw = (now, cam, W, H, F, tdim, obst) => {
      const t0 = performance.now();
      const reduced = host.reduced();
      const dt = reduced ? 1e6 : clamp(prevNow ? now - prevNow : 16, 1, 64);
      prevNow = now;
      lgrid.clear();
      cK = cam.k;
      cX = cam.x;
      cY = cam.y;
      if (obst)
        for (const r of obst)
          reserve(r.x - 4, r.y - 4, r.x + r.w + 4, r.y + r.h + 4);
      const items = buildItems(now, cam, W, H, F, tdim);
      lastItems = items.length;
      if (items.some((it) => it.node)) ensureDiscs();
      const sig = signature(cam, W, H, F, obst),
        tfNow = host.turnFocus(),
        selNow = host.selected();
      held =
        !!ref &&
        ref.sig === sig &&
        ref.tf === tfNow &&
        ref.sel === selNow &&
        moved(cam, ref, W, H) <= PAN * dead &&
        Math.abs(Math.log(cam.k / ref.k)) <= ZOOM * dead;
      if (!held)
        ref = { x: cam.x, y: cam.y, k: cam.k, sig, tf: tfNow, sel: selNow };
      const keyMax = Math.max(12, Math.round((W * H) / KEY_AREA));
      let keys = 0,
        tries = 0;
      const live = new Set();
      const free = (c, pad) =>
        c.x >= 2 + pad &&
        c.x + c.w <= W - 2 - pad &&
        c.y >= 1 + pad &&
        c.y + c.h <= H - 1 - pad &&
        !blocked(
          c.x - 2 - pad,
          c.y - 1 - pad,
          c.x + c.w + 2 + pad,
          c.y + c.h + 1 + pad,
        );
      const score = (it, c, pad, bound) => {
        const low = c.base + (c.vc || 0);
        if (low >= bound || !free(c, pad)) return -1;
        if (it.fit && !it.fit(c)) return -1;
        if (it.node) {
          const lim = Math.min(it.tol, (bound - low) / DISC_W),
            ov = discOv(c, it.node, lim);
          return ov > lim ? -1 : low + ov * DISC_W;
        }
        const s = it.cost(c, bound - low);
        return low + s >= bound ? -1 : low + s;
      };
      for (const it of items) {
        let e = ents.get(it.id);
        const fresh = !e;
        if (fresh) {
          if (held) continue;
          e = {
            alpha: 0,
            shown: false,
            last: 0,
            ck: null,
            c: null,
            a: 0,
            halo: it.halo,
            state: 0,
            up: 0,
          };
          ents.set(it.id, e);
        }
        live.add(it.id);
        e.a = it.a;
        e.halo = it.halo;
        e.node = it.ring ? it.node : null;
        if (held && e.state !== 2 && !e.up && !fresh) {
          if (e.state === 1) {
            let c = it.find(e.ck);
            if (!c && e.c) {
              const sx = cam.x - e.px,
                sy = cam.y - e.py,
                l = e.c.lead;
              c = {
                ...e.c,
                x: e.c.x + sx,
                y: e.c.y + sy,
                lead: l ? [l[0] + sx, l[1] + sy, l[2]] : null,
              };
            }
            e.c = c;
            e.px = cam.x;
            e.py = cam.y;
            reserve(c.x - 2, c.y - 1, c.x + c.w + 2, c.y + c.h + 1);
            if (it.key) keys++;
            continue;
          } else continue;
        }
        const padNew = e.shown ? 0 : 3;
        let best = null,
          bs = Infinity;
        if (!(
          it.key &&
          !e.shown &&
          (keys >= keyMax || tries++ >= keyMax * 3)
        )) {
          for (let vi = 0; vi < it.nv; vi++) {
            if (it.vcs && it.vcs[vi] >= bs) break;
            for (const c of it.gen(vi, bs)) {
              const s = score(it, c, padNew, bs);
              if (s >= 0) {
                bs = s;
                best = c;
              }
            }
          }
        }
        const pc = e.shown && e.ck != null ? it.find(e.ck) : null;
        let keep = null,
          ks = 0;
        if (pc) {
          const s = score(it, pc, -1, Infinity);
          if (s >= 0) {
            keep = pc;
            ks = s;
          }
        }
        let pick = best;
        if (keep) {
          pick = keep;
          if (best && best !== keep && bs + MARGIN < ks) {
            if (it.topic) {
              if (!e.up) e.up = now;
              if (now - e.up >= HOLD) {
                pick = best;
                e.up = 0;
              }
            } else pick = best;
          } else e.up = 0;
        } else e.up = 0;
        if (pick) {
          e.shown = true;
          e.state = 1;
          e.ck = pick.k;
          e.last = now;
          e.c = pick;
          e.px = cam.x;
          e.py = cam.y;
        } else if (pc && now - e.last <= HOLD) {
          e.state = 2;
          e.c = pc;
          e.px = cam.x;
          e.py = cam.y;
        } else {
          e.shown = false;
          e.state = 0;
          e.up = 0;
        }
        if (e.state && e.c)
          reserve(e.c.x - 2, e.c.y - 1, e.c.x + e.c.w + 2, e.c.y + e.c.h + 1);
        if (e.state === 1 && it.key) keys++;
      }
      placeMs = performance.now() - t0;
      let animating = false,
        shown = 0;
      ctx.textBaseline = "middle";
      ctx.lineJoin = "round";
      ctx.strokeStyle = C.canvas;
      let curFont = "";
      for (const [id, e] of ents) {
        const want = e.state && live.has(id) ? 1 : 0;
        if (e.alpha < want) e.alpha = Math.min(want, e.alpha + dt / FADE);
        else if (e.alpha > want) e.alpha = Math.max(want, e.alpha - dt / FADE);
        if (e.alpha !== want) animating = true;
        if (e.state === 2 || e.up) animating = true;
        if (e.alpha <= 0 && !want) {
          ents.delete(id);
          continue;
        }
        if (!e.c) continue;
        if (want) shown++;
        const A = e.alpha * e.a;
        if (A < 0.01) continue;
        const c = e.c;
        if (e.node && want) {
          const n = e.node,
            rr =
              Math.max(n.r * (n.shape ? 1.35 : 1), 1.5 / cam.k) * cam.k + 2.6;
          ctx.globalAlpha = A * 0.6;
          ctx.strokeStyle = C.text2;
          ctx.lineWidth = 1;
          ctx.beginPath();
          ctx.arc(n.x * cam.k + cam.x, n.y * cam.k + cam.y, rr, 0, 6.283);
          ctx.stroke();
          ctx.strokeStyle = C.canvas;
        }
        if (c.lead && want) {
          const [ox, oy, orr] = c.lead,
            mx = c.x + c.w / 2 - ox,
            my = c.y + c.h / 2 - oy,
            md = Math.hypot(mx, my);
          if (md > 1) {
            const px = ox + (mx / md) * orr,
              py = oy + (my / md) * orr;
            const qx = clamp(px, c.x, c.x + c.w),
              qy = clamp(py, c.y, c.y + c.h),
              len = Math.hypot(qx - px, qy - py);
            if (len > 3.5) {
              ctx.globalAlpha = A * 0.85;
              ctx.strokeStyle = C.text2;
              ctx.lineWidth = 1;
              ctx.beginPath();
              ctx.moveTo(px, py);
              ctx.lineTo(
                px + ((qx - px) * (len - 2)) / len,
                py + ((qy - py) * (len - 2)) / len,
              );
              ctx.stroke();
              ctx.fillStyle = C.text2;
              ctx.beginPath();
              ctx.arc(px, py, 1.8, 0, 6.283);
              ctx.fill();
              ctx.strokeStyle = C.canvas;
            }
          }
        }
        for (let i = 0; i < c.lines.length; i++) {
          const ln = c.lines[i],
            la = A * (ln.a == null ? 1 : ln.a);
          if (la < 0.01) continue;
          const y = c.y + (c.h / c.lines.length) * (i + 0.5),
            x =
              c.x +
              (c.al === "c" ? (c.w - ln.w) / 2 : c.al === "r" ? c.w - ln.w : 0);
          if (ln.font !== curFont) {
            ctx.font = ln.font;
            curFont = ln.font;
          }
          ctx.globalAlpha = la * 0.9;
          ctx.lineWidth = e.halo;
          ctx.strokeText(ln.text, x, y);
          ctx.globalAlpha = la;
          ctx.fillStyle = ln.color;
          ctx.fillText(ln.text, x, y);
        }
      }
      ctx.globalAlpha = 1;
      count = shown;
      lastMs = performance.now() - t0;
      return animating;
    };
    const reset = () => {
      ents.clear();
      namesFor = null;
      ref = null;
    };
    const boxes = () => {
      const out = [];
      for (const e of ents.values())
        if (e.state && e.c && e.alpha > 0.3)
          out.push({ x: e.c.x, y: e.c.y, w: e.c.w, h: e.c.h });
      return out;
    };
    const shownIds = () => {
      const out = [];
      for (const [id, e] of ents) if (e.state && e.alpha > 0) out.push(id);
      return out.sort();
    };
    const debug = () =>
      [...ents].map(([id, e]) => ({
        id,
        ck: e.ck,
        st: e.state,
        a: +e.alpha.toFixed(2),
        ia: +(e.a == null ? 1 : e.a).toFixed(2),
        x: e.c && Math.round(e.c.x),
        y: e.c && Math.round(e.c.y),
        w: e.c && Math.round(e.c.w),
        h: e.c && Math.round(e.c.h),
        lead: !!(e.c && e.c.lead),
        ring: !!e.node,
        text: e.c ? e.c.lines.map((l) => l.text).join(" ") : "",
      }));
    const setCap = (v) => {
      cap = v > 0 ? v : CAP;
      ref = null;
    };
    const setDead = (v) => {
      dead = v;
      ref = null;
    };
    return {
      draw,
      debug,
      boxes,
      setCap,
      setDead,
      resetText,
      textW,
      reset,
      count: () => count,
      ms: () => lastMs,
      placeMs: () => placeMs,
      items: () => lastItems,
      held: () => held,
      shownIds,
      names: () => names,
    };
  };
  G.labels = { create, shortNames };
})();
