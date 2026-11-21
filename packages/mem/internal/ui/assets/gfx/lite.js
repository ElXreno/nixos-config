"use strict";
(() => {
  const G = (window.MemGfx = window.MemGfx || {});
  const T = G.theme;
  const C = T.c;
  const TAU = 6.283185307179586;
  const create = (canvas) => {
    const ctx = canvas.getContext("2d");
    let nodes = [],
      links = [],
      topics = [];
    const setTopics = (ts) => {
      topics = ts;
    };
    const pushTopicState = () => {};
    const setData = (ns, ls) => {
      nodes = ns;
      links = ls;
      ns.forEach((n, i) => {
        n.gi = i;
      });
    };
    const pushState = () => {};
    const pushPos = () => {};
    const setHulls = () => {};
    const draw = (v) => {
      const cam = v.cam,
        k = cam.k,
        W = v.W,
        H = v.H;
      const sc = v.W ? canvas.width / v.W : 1;
      ctx.setTransform(sc, 0, 0, sc, 0, 0);
      ctx.fillStyle = C.canvas;
      ctx.fillRect(0, 0, W, H);
      ctx.lineWidth = 1;
      for (const t of topics) {
        if (!(t.R > 0) || !t.members.length) continue;
        const X = t.ax * k + cam.x,
          Y = t.ay * k + cam.y,
          R = t.R * k;
        if (X + R < 0 || X - R > W || Y + R < 0 || Y - R > H) continue;
        ctx.beginPath();
        ctx.arc(X, Y, R, 0, TAU);
        ctx.fillStyle = C.fill;
        ctx.fill();
        ctx.strokeStyle = C.contour;
        ctx.stroke();
      }
      const fn = v.focus >= 0 ? nodes[v.focus] : null;
      if (fn) {
        ctx.lineWidth = 1.3;
        for (const l of links) {
          if (l.a !== fn && l.b !== fn) continue;
          const o = l.a === fn ? l.b : l.a;
          ctx.strokeStyle = T.rgba(o.color, 0.65 * fn.hl + 0.1);
          ctx.beginPath();
          ctx.moveTo(l.a.x * k + cam.x, l.a.y * k + cam.y);
          ctx.lineTo(l.b.x * k + cam.x, l.b.y * k + cam.y);
          ctx.stroke();
        }
      }
      for (const n of nodes) {
        if (n.hl < 0.01) continue;
        const X = n.x * k + cam.x,
          Y = n.y * k + cam.y;
        if (X < -30 || X > W + 30 || Y < -30 || Y > H + 30) continue;
        const r = Math.max(n.r * (0.35 + 0.65 * n.sc) * k, 1.5);
        const fl = v.now - n.flashAt,
          fa = fl >= 0 && fl < 9000 ? Math.pow(1 - fl / 9000, 1.4) : 0;
        ctx.globalAlpha = Math.max(n.hl, fa);
        if (n.status !== "current") {
          ctx.fillStyle = C.canvas;
          ctx.beginPath();
          ctx.arc(X, Y, r, 0, TAU);
          ctx.fill();
          ctx.strokeStyle = n.status === "retired" ? C.m2 : n.color;
          ctx.lineWidth = Math.max(1.3, r * 0.2);
          ctx.setLineDash([Math.max(r * 0.55, 1.6), Math.max(r * 0.42, 1.3)]);
          ctx.stroke();
          ctx.setLineDash([]);
        } else {
          ctx.fillStyle = n.color;
          ctx.beginPath();
          ctx.arc(X, Y, r, 0, TAU);
          ctx.fill();
        }
        if (fa > 0) {
          ctx.fillStyle = T.rgba(C.live, fa * 0.85);
          ctx.beginPath();
          ctx.arc(X, Y, r, 0, TAU);
          ctx.fill();
        }
        if (n.dangerous || n.core) {
          ctx.strokeStyle = n.dangerous ? C.danger : C.bright;
          ctx.lineWidth = Math.max(1.2, r * 0.2);
          ctx.beginPath();
          ctx.arc(X, Y, r + Math.max(1.5, r * 0.28), 0, TAU);
          ctx.stroke();
        }
        if (v.sel === n.gi) {
          ctx.strokeStyle = C.bright;
          ctx.lineWidth = 1.6;
          ctx.beginPath();
          ctx.arc(X, Y, r + (n.dangerous ? 6 + r * 0.3 : 4), 0, TAU);
          ctx.stroke();
        }
      }
      ctx.globalAlpha = 1;
    };
    return {
      name: "lite",
      setData,
      setTopics,
      setHulls,
      pushTopicState,
      pushState,
      pushPos,
      draw,
      sync() {},
    };
  };
  G.lite = { create };
})();
