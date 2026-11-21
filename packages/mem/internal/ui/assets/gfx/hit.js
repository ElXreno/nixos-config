"use strict";
(() => {
  const G = (window.MemGfx = window.MemGfx || {});
  const CELL = 28;
  const create = () => {
    let list = [],
      start = new Int32Array(1),
      fill = new Int32Array(1),
      idx = new Int32Array(0),
      cellOf = new Int32Array(0);
    let gw = 0,
      gh = 0,
      ox = 0,
      oy = 0,
      rmax = 0;
    const build = (nodes) => {
      list = nodes;
      const n = nodes.length;
      let x0 = Infinity,
        y0 = Infinity,
        x1 = -Infinity,
        y1 = -Infinity;
      rmax = 0;
      for (let i = 0; i < n; i++) {
        const m = nodes[i];
        if (!isFinite(m.x) || !isFinite(m.y)) continue;
        if (m.x < x0) x0 = m.x;
        if (m.x > x1) x1 = m.x;
        if (m.y < y0) y0 = m.y;
        if (m.y > y1) y1 = m.y;
        if (m.r > rmax) rmax = m.r;
      }
      if (!(x0 <= x1)) {
        gw = gh = 0;
        return;
      }
      ox = x0;
      oy = y0;
      gw = Math.floor((x1 - x0) / CELL) + 1;
      gh = Math.floor((y1 - y0) / CELL) + 1;
      if (gw * gh > 4e6) {
        gw = gh = 0;
        return;
      }
      if (start.length < gw * gh + 1) {
        start = new Int32Array(gw * gh + 1);
        fill = new Int32Array(gw * gh + 1);
      } else start.fill(0, 0, gw * gh + 1);
      if (cellOf.length < n) {
        cellOf = new Int32Array(n);
        idx = new Int32Array(n);
      }
      for (let i = 0; i < n; i++) {
        const m = nodes[i];
        if (!isFinite(m.x) || !isFinite(m.y)) {
          cellOf[i] = -1;
          continue;
        }
        const c =
          Math.floor((m.y - oy) / CELL) * gw + Math.floor((m.x - ox) / CELL);
        cellOf[i] = c;
        start[c + 1]++;
      }
      for (let c = 0; c < gw * gh; c++) start[c + 1] += start[c];
      fill.set(start.subarray(0, gw * gh));
      for (let i = 0; i < n; i++)
        if (cellOf[i] >= 0) idx[fill[cellOf[i]]++] = i;
    };
    const query = (wx, wy, k) => {
      if (!gw) return null;
      const reach = rmax + 5 / k + 1.5 / k;
      const x0 = Math.max(0, Math.floor((wx - reach - ox) / CELL)),
        x1 = Math.min(gw - 1, Math.floor((wx + reach - ox) / CELL));
      const y0 = Math.max(0, Math.floor((wy - reach - oy) / CELL)),
        y1 = Math.min(gh - 1, Math.floor((wy + reach - oy) / CELL));
      let best = null,
        bd = Infinity;
      for (let cy = y0; cy <= y1; cy++) {
        for (let cx = x0; cx <= x1; cx++) {
          const c = cy * gw + cx;
          for (let j = start[c], e = start[c + 1]; j < e; j++) {
            const m = list[idx[j]];
            if (m.hidden) continue;
            const dx = m.x - wx,
              dy = m.y - wy,
              d = dx * dx + dy * dy;
            const rr = Math.max(m.r, 1.5 / k) + 5 / k;
            if (d < rr * rr && d < bd) {
              bd = d;
              best = m;
            }
          }
        }
      }
      return best;
    };
    return { build, query };
  };
  G.hit = { create };
})();
