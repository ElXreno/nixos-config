"use strict";
(() => {
  const G = (window.MemGfx = window.MemGfx || {});
  const TAU = Math.PI * 2;
  const CELL = 22;
  const TUNE = {
    home: 10,
    damp: 6.5,
    linkSame: 70,
    linkCross: 16,
    collide: 80,
    squash: 0.8,
    vmax: 1400,
    pinMax: 480,
    wander: 3.5,
    restV: 0.04,
    restD: 0.02,
  };
  const clamp = (v, a, b) => (v < a ? a : v > b ? b : v);

  const create = (tune) => {
    const P = Object.assign({}, TUNE, tune);
    let nodes = [],
      n = 0,
      links = [],
      m = 0;
    let x = new Float64Array(0),
      y = new Float64Array(0),
      vx = new Float64Array(0),
      vy = new Float64Array(0);
    let hx = new Float64Array(0),
      hy = new Float64Array(0),
      rr = new Float64Array(0),
      ax = new Float64Array(0),
      ay = new Float64Array(0);
    let tx = new Float64Array(0),
      ty = new Float64Array(0),
      ph = new Float64Array(0),
      hid = new Uint8Array(0);
    let la = new Int32Array(0),
      lb = new Int32Array(0),
      lk = new Float64Array(0),
      lr = new Float64Array(0);
    let cellOf = new Int32Array(0),
      start = new Int32Array(1),
      fill = new Int32Array(1),
      order = new Int32Array(0);
    const nb = new Int32Array(4);
    let pinned = -1,
      pinX = 0,
      pinY = 0;
    let awake = true,
      amp = P.wander,
      version = 0;

    const setGraph = (ns, ls) => {
      const held = pinned >= 0 ? nodes[pinned] : null;
      nodes = ns;
      n = ns.length;
      links = ls;
      x = new Float64Array(n);
      y = new Float64Array(n);
      vx = new Float64Array(n);
      vy = new Float64Array(n);
      hx = new Float64Array(n);
      hy = new Float64Array(n);
      rr = new Float64Array(n);
      ax = new Float64Array(n);
      ay = new Float64Array(n);
      tx = new Float64Array(n);
      ty = new Float64Array(n);
      ph = new Float64Array(n);
      hid = new Uint8Array(n);
      cellOf = new Int32Array(n);
      order = new Int32Array(n);
      const deg = new Int32Array(n);
      ns.forEach((nd, i) => {
        nd.li = i;
        x[i] = nd.x;
        y[i] = nd.y;
        hx[i] = isFinite(nd.hx) ? nd.hx : nd.x;
        hy[i] = isFinite(nd.hy) ? nd.hy : nd.y;
        rr[i] = nd.r || 3;
        ph[i] = (((i * 2654435761) >>> 0) / 4294967296) * TAU;
      });
      let k = 0;
      la = new Int32Array(ls.length);
      lb = new Int32Array(ls.length);
      lk = new Float64Array(ls.length);
      lr = new Float64Array(ls.length);
      for (const l of ls) {
        const a = l.a && l.a.li,
          b = l.b && l.b.li;
        if (
          a == null ||
          b == null ||
          a === b ||
          a >= n ||
          b >= n ||
          ns[a] !== l.a ||
          ns[b] !== l.b
        )
          continue;
        la[k] = a;
        lb[k] = b;
        lr[k] = Math.hypot(hx[a] - hx[b], hy[a] - hy[b]);
        lk[k] = l.a.topic === l.b.topic ? P.linkSame : P.linkCross;
        deg[a]++;
        deg[b]++;
        k++;
      }
      m = k;
      for (let i = 0; i < m; i++)
        lk[i] /= Math.sqrt(Math.max(1, (deg[la[i]] * deg[lb[i]]) / 9));
      pinned = held && held.li != null && ns[held.li] === held ? held.li : -1;
      awake = true;
      version++;
    };

    const wanderAt = (i, t) => {
      const px = hx[i],
        py = hy[i],
        p = ph[i];
      tx[i] =
        hx[i] +
        amp *
          (0.68 * Math.sin(px * 0.0113 + py * 0.0071 + t * 0.9) +
            0.32 * Math.sin(t * 0.47 + p));
      ty[i] =
        hy[i] +
        amp *
          (0.68 * Math.sin(py * 0.0127 - px * 0.0059 + t * 0.8 + 1.7) +
            0.32 * Math.sin(t * 0.53 + p * 1.7 + 2.1));
    };

    const collide = () => {
      let x0 = Infinity,
        y0 = Infinity,
        x1 = -Infinity,
        y1 = -Infinity,
        live = 0;
      for (let i = 0; i < n; i++) {
        if (hid[i]) continue;
        live++;
        if (x[i] < x0) x0 = x[i];
        if (x[i] > x1) x1 = x[i];
        if (y[i] < y0) y0 = y[i];
        if (y[i] > y1) y1 = y[i];
      }
      if (live < 2) return;
      const gw = Math.floor((x1 - x0) / CELL) + 1,
        gh = Math.floor((y1 - y0) / CELL) + 1;
      if (!(gw * gh < 4e6)) return;
      if (start.length < gw * gh + 1) start = new Int32Array(gw * gh + 1);
      else start.fill(0, 0, gw * gh + 1);
      for (let i = 0; i < n; i++) {
        if (hid[i]) {
          cellOf[i] = -1;
          continue;
        }
        const c =
          Math.floor((y[i] - y0) / CELL) * gw + Math.floor((x[i] - x0) / CELL);
        cellOf[i] = c;
        start[c + 1]++;
      }
      for (let c = 0; c < gw * gh; c++) start[c + 1] += start[c];
      if (fill.length < gw * gh) fill = new Int32Array(gw * gh);
      fill.set(start.subarray(0, gw * gh));
      for (let i = 0; i < n; i++)
        if (cellOf[i] >= 0) order[fill[cellOf[i]]++] = i;
      const kc = P.collide,
        sq = P.squash;
      const pair = (i, j) => {
        const dx = x[j] - x[i],
          dy = y[j] - y[i],
          md = (rr[i] + rr[j]) * sq,
          d2 = dx * dx + dy * dy;
        if (d2 >= md * md) return;
        let d = Math.sqrt(d2),
          ux,
          uy;
        if (d < 1e-6) {
          const a = (((i * 7 + j * 13) % 360) * Math.PI) / 180;
          ux = Math.cos(a);
          uy = Math.sin(a);
          d = 0;
        } else {
          ux = dx / d;
          uy = dy / d;
        }
        const f = kc * (md - d);
        ax[i] -= ux * f;
        ay[i] -= uy * f;
        ax[j] += ux * f;
        ay[j] += uy * f;
      };
      for (let cy = 0; cy < gh; cy++) {
        for (let cx = 0; cx < gw; cx++) {
          const c = cy * gw + cx,
            s = start[c],
            e = start[c + 1];
          if (s === e) continue;
          nb[0] = cx + 1 < gw ? c + 1 : -1;
          nb[1] = cy + 1 < gh && cx > 0 ? c + gw - 1 : -1;
          nb[2] = cy + 1 < gh ? c + gw : -1;
          nb[3] = cy + 1 < gh && cx + 1 < gw ? c + gw + 1 : -1;
          for (let a = s; a < e; a++) {
            const i = order[a];
            for (let b = a + 1; b < e; b++) pair(i, order[b]);
            for (let q = 0; q < 4; q++) {
              if (nb[q] < 0) continue;
              for (let b = start[nb[q]], be = start[nb[q] + 1]; b < be; b++)
                pair(i, order[b]);
            }
          }
        }
      }
    };

    const step = (now, dtMs, o) => {
      if (!n) return false;
      const wander = !!(o && o.wander);
      const dt = clamp(dtMs, 1, 34) / 1000,
        sub = dt > 0.02 ? 2 : 1,
        h = dt / sub;
      const t = now / 1000;
      for (let i = 0; i < n; i++) {
        hid[i] = nodes[i].hidden ? 1 : 0;
        if (wander) wanderAt(i, t);
        else {
          tx[i] = hx[i];
          ty[i] = hy[i];
        }
      }
      let moved = false;
      const vmax = P.vmax,
        kh = P.home;
      const decay = Math.exp(-P.damp * h);
      let maxV = 0,
        maxD = 0;
      for (let s = 0; s < sub; s++) {
        for (let i = 0; i < n; i++) {
          ax[i] = kh * (tx[i] - x[i]);
          ay[i] = kh * (ty[i] - y[i]);
        }
        for (let l = 0; l < m; l++) {
          const a = la[l],
            b = lb[l];
          if (hid[a] || hid[b]) continue;
          const dx = x[b] - x[a],
            dy = y[b] - y[a],
            d = Math.sqrt(dx * dx + dy * dy);
          if (d < 1e-6) continue;
          const f = (lk[l] * (d - lr[l])) / d;
          ax[a] += f * dx;
          ay[a] += f * dy;
          ax[b] -= f * dx;
          ay[b] -= f * dy;
        }
        collide();
        maxV = 0;
        maxD = 0;
        for (let i = 0; i < n; i++) {
          if (i === pinned) {
            if (x[i] !== pinX || y[i] !== pinY) moved = true;
            x[i] = pinX;
            y[i] = pinY;
            vx[i] = 0;
            vy[i] = 0;
            continue;
          }
          let u = (vx[i] + ax[i] * h) * decay,
            w = (vy[i] + ay[i] * h) * decay;
          const sp = Math.sqrt(u * u + w * w);
          if (sp > vmax) {
            u *= vmax / sp;
            w *= vmax / sp;
          }
          vx[i] = u;
          vy[i] = w;
          const nx = x[i] + u * h,
            ny = y[i] + w * h;
          if (Math.abs(nx - x[i]) > 2e-4 || Math.abs(ny - y[i]) > 2e-4)
            moved = true;
          x[i] = nx;
          y[i] = ny;
          if (sp > maxV) maxV = sp;
          const dd = Math.abs(nx - tx[i]) + Math.abs(ny - ty[i]);
          if (dd > maxD) maxD = dd;
        }
      }
      let rest = false;
      if (!wander && pinned < 0 && maxV < P.restV && maxD < P.restD) {
        rest = true;
        for (let i = 0; i < n; i++) {
          if (x[i] !== hx[i] || y[i] !== hy[i]) moved = true;
          x[i] = hx[i];
          y[i] = hy[i];
          vx[i] = 0;
          vy[i] = 0;
        }
      }
      for (let i = 0; i < n; i++) {
        const nd = nodes[i];
        nd.x = x[i];
        nd.y = y[i];
      }
      awake = !rest;
      if (moved) version++;
      return moved;
    };

    const pin = (nd, px, py) => {
      const i = nd && nd.li != null && nodes[nd.li] === nd ? nd.li : -1;
      if (i < 0) return false;
      const dx = px - hx[i],
        dy = py - hy[i],
        d = Math.hypot(dx, dy),
        f = d > P.pinMax ? P.pinMax / d : 1;
      pinned = i;
      pinX = hx[i] + dx * f;
      pinY = hy[i] + dy * f;
      awake = true;
      return true;
    };
    const release = () => {
      pinned = -1;
      awake = true;
    };
    const wake = () => {
      awake = true;
    };
    const setAmp = (v) => {
      amp = v;
    };
    return {
      setGraph,
      step,
      pin,
      release,
      wake,
      setAmp,
      awake: () => awake,
      pinned: () => (pinned >= 0 ? nodes[pinned] : null),
      version: () => version,
      tune: P,
    };
  };
  G.live = { create, TUNE };
})();
