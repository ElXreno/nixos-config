"use strict";
(() => {
  const G = (window.MemGfx = window.MemGfx || {});
  const T = G.theme;
  const C = T.c;
  const FONT_K = '11px "JetBrains Mono", ui-monospace, monospace';
  const DOTS_MS = 5000;
  const clamp = (v, a, b) => (v < a ? a : v > b ? b : v);
  const create = (canvas, host, wake) => {
    const ctx = canvas.getContext("2d");
    const lbl = G.labels.create(ctx, host);
    const textW = lbl.textW;
    const rc = G.recall.create(host, wake);
    let dotsUntil = 0,
      lastFocus = null,
      hot = [];
    const bez = (l, t) => {
      const A = l.a,
        B = l.b;
      const dx = B.x - A.x,
        dy = B.y - A.y;
      const mx = (A.x + B.x) / 2 - dy * T.curve,
        my = (A.y + B.y) / 2 + dx * T.curve;
      const u = 1 - t;
      return [
        u * u * A.x + 2 * u * t * mx + t * t * B.x,
        u * u * A.y + 2 * u * t * my + t * t * B.y,
      ];
    };
    let sonar = null;
    const SONAR_MS = 1500,
      SONAR_R = 56;
    const sonarDraw = (now, cam, W, H, reduced) => {
      const n = sonar.n;
      if (n.hidden) return false;
      if (sonar.t0 < 0) sonar.t0 = now;
      const k = cam.k,
        x = n.x * k + cam.x,
        y = n.y * k + cam.y,
        r0 = Math.max(n.r, 1.5 / k) * k + 5;
      const inside = x > 6 && y > 6 && x < W - 6 && y < H - 6;
      ctx.lineWidth = 1.2;
      if (!inside) {
        const cx = clamp(x, 18, W - 18),
          cy = clamp(y, 18, H - 18),
          a = Math.atan2(y - H / 2, x - W / 2);
        ctx.fillStyle = T.rgba(C.bright, 0.85);
        ctx.beginPath();
        ctx.moveTo(cx + Math.cos(a) * 9, cy + Math.sin(a) * 9);
        ctx.lineTo(cx + Math.cos(a + 2.5) * 7, cy + Math.sin(a + 2.5) * 7);
        ctx.lineTo(cx + Math.cos(a - 2.5) * 7, cy + Math.sin(a - 2.5) * 7);
        ctx.closePath();
        ctx.fill();
        return false;
      }
      const ring = (r, a) => {
        ctx.beginPath();
        ctx.arc(x, y, r, 0, 6.283);
        ctx.lineWidth = 3.6;
        ctx.strokeStyle = T.rgba(C.canvas, 0.55 * a);
        ctx.stroke();
        ctx.lineWidth = 1.3;
        ctx.strokeStyle = T.rgba(C.bright, a);
        ctx.stroke();
      };
      ring(r0, 0.9);
      if (reduced) return false;
      for (let i = 0; i < 2; i++) {
        const p = ((now - sonar.t0) / SONAR_MS + i * 0.5) % 1,
          e = 1 - Math.pow(1 - p, 2.2);
        ring(r0 + e * SONAR_R, 0.75 * (1 - p));
      }
      return true;
    };
    const draw = (now, cam, W, H, DPR, F, tdim, obst) => {
      const k = cam.k,
        reduced = host.reduced();
      const sx = (n) => n.x * k + cam.x,
        sy = (n) => n.y * k + cam.y;
      ctx.setTransform(DPR, 0, 0, DPR, 0, 0);
      ctx.clearRect(0, 0, W, H);
      ctx.textBaseline = "middle";
      let animating = false;
      const fnode = (F && F.node) || null;
      if (fnode !== lastFocus) {
        lastFocus = fnode;
        dotsUntil = fnode ? now + DOTS_MS : 0;
        hot = fnode
          ? host.alinks().filter((l) => l.s === fnode || l.t === fnode)
          : [];
      }
      if (fnode && !reduced && now < dotsUntil) {
        animating = true;
        const fade = clamp((dotsUntil - now) / 700, 0, 1);
        for (const l of hot) {
          const o = l.s === fnode ? l.t : l.s;
          const len = Math.hypot(l.t.x - l.s.x, l.t.y - l.s.y);
          const per = 900 + len * 9;
          ctx.fillStyle = T.rgba(
            o.color === C.m2 ? C.bright : o.color,
            0.95 * fade * Math.min(fnode.hl, 1),
          );
          for (let p = 0; p < 2; p++) {
            const t = (now / per + l.phase + p * 0.5) % 1;
            const [x, y] = bez(l, t);
            ctx.beginPath();
            ctx.arc(x * k + cam.x, y * k + cam.y, 1.9, 0, 6.283);
            ctx.fill();
          }
        }
      }
      if (rc.under(ctx, now, cam, W, H)) animating = true;
      if (lbl.draw(now, cam, W, H, F, tdim, obst)) animating = true;
      ctx.globalAlpha = 1;
      if (sonar && sonarDraw(now, cam, W, H, reduced)) animating = true;
      const arcs = host.arcs;
      for (let i = arcs.length - 1; i >= 0; i--) {
        const A = arcs[i],
          e = now - A.t0;
        if (e > 8200) {
          arcs.splice(i, 1);
          continue;
        }
        animating = true;
        if (e < 0) continue;
        const my = A.slot * H,
          mx = 12,
          label = A.label;
        ctx.font = "500 " + FONT_K;
        const mk = A.tool ? 11 : 0,
          w = textW(label, "500 " + FONT_K) + 18 + mk;
        const pa =
          e < 300 ? e / 300 : e > 6800 ? Math.max(0, 1 - (e - 6800) / 1400) : 1;
        for (let j = 0; j < A.items.length; j++) {
          const it = A.items[j],
            n = it.node;
          if (n.hidden) continue;
          const st = 250 + j * 150,
            dur = reduced ? 1 : 780,
            p = clamp((e - st) / dur, 0, 1);
          if (p <= 0) continue;
          const fade =
            e > st + dur ? Math.max(0, 1 - (e - st - dur) / 2600) : 1;
          if (fade <= 0) continue;
          const ghost = it.tier === "ghost",
            ptr = it.tier === "pointer";
          const x0 = mx + w,
            y0 = my,
            x1 = sx(n),
            y1 = sy(n);
          const cxp = x0 + (x1 - x0) * 0.45,
            cyp =
              Math.min(y0, y1) - Math.min(140, Math.abs(x1 - x0) * 0.22) - 20;
          ctx.strokeStyle = ghost
            ? T.rgba(C.m2, 0.55 * fade)
            : T.rgba(C.live, (ptr ? 0.62 : 0.75) * fade);
          ctx.lineWidth = ghost ? 1 : ptr ? 1.25 : 1.5;
          ctx.setLineDash(ghost ? [1.5, 4] : ptr ? [6, 4] : []);
          ctx.beginPath();
          ctx.moveTo(x0, y0);
          const u = 1 - p,
            hx = u * u * x0 + 2 * u * p * cxp + p * p * x1,
            hy = u * u * y0 + 2 * u * p * cyp + p * p * y1;
          ctx.quadraticCurveTo(u * x0 + p * cxp, u * y0 + p * cyp, hx, hy);
          ctx.stroke();
          ctx.setLineDash([]);
          if (p < 1) {
            ctx.fillStyle = ghost ? C.m2 : C.live;
            ctx.beginPath();
            ctx.arc(hx, hy, ghost ? 2 : 3, 0, 6.283);
            ctx.fill();
          } else if (ghost) {
            ctx.strokeStyle = T.rgba(C.m2, 0.7 * fade);
            ctx.lineWidth = 1;
            ctx.setLineDash([2, 3]);
            ctx.beginPath();
            ctx.arc(x1, y1, Math.max(n.r, 1.5 / k) * k + 5, 0, 6.283);
            ctx.stroke();
            ctx.setLineDash([]);
          }
        }
        const glow = reduced ? 0 : rc.pill(A.n, now);
        if (glow > 0.01) {
          ctx.globalAlpha = pa * glow * 0.55;
          ctx.strokeStyle = C.live;
          ctx.lineWidth = 2;
          ctx.beginPath();
          ctx.roundRect(mx - 3, my - 15, w + 6, 30, A.tool ? 5 : 15);
          ctx.stroke();
        }
        ctx.globalAlpha = pa;
        ctx.fillStyle = C.panel;
        ctx.strokeStyle = C.live;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.roundRect(mx, my - 12, w, 24, A.tool ? 3 : 12);
        ctx.fill();
        ctx.stroke();
        if (A.tool) {
          ctx.fillStyle = C.live;
          ctx.fillRect(mx + 9, my - 3.5, 7, 7);
        }
        ctx.fillStyle = C.live;
        ctx.fillText(label, mx + 9 + mk, my + 0.5);
        ctx.globalAlpha = 1;
      }
      if (rc.over(ctx, now, cam, W, H, obst, lbl.boxes)) animating = true;
      return animating;
    };
    const reset = () => {
      lbl.reset();
      hot = lastFocus
        ? host.alinks().filter((l) => l.s === lastFocus || l.t === lastFocus)
        : [];
    };
    const setSonar = (n) => {
      sonar = n ? { n, t0: -1 } : null;
    };
    return {
      draw,
      resetText: lbl.resetText,
      reset,
      labels: lbl.count,
      lbl,
      ctx,
      recall: rc,
      sonar: setSonar,
      sonarOn: () => !!sonar,
    };
  };
  G.overlay = { create };
})();
