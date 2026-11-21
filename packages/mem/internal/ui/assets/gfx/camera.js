"use strict";
(() => {
  const G = (window.MemGfx = window.MemGfx || {});
  const KMIN = 0.05,
    KMAX = 6,
    RHO = Math.SQRT2;
  const clamp = (v, a, b) => (v < a ? a : v > b ? b : v);
  const zoomPath = (x0, y0, w0, x1, y1, w1) => {
    const dx = x1 - x0,
      dy = y1 - y0,
      d2 = dx * dx + dy * dy;
    if (d2 < 1e-9) {
      const S = Math.log(w1 / w0) / RHO;
      return {
        S,
        at: (t) => [x0 + t * dx, y0 + t * dy, w0 * Math.exp(RHO * t * S)],
      };
    }
    const d1 = Math.sqrt(d2);
    const b0 = (w1 * w1 - w0 * w0 + 4 * d2) / (2 * w0 * 2 * d1);
    const b1 = (w1 * w1 - w0 * w0 - 4 * d2) / (2 * w1 * 2 * d1);
    const r0 = Math.log(Math.sqrt(b0 * b0 + 1) - b0),
      r1 = Math.log(Math.sqrt(b1 * b1 + 1) - b1);
    const S = (r1 - r0) / RHO;
    return {
      S,
      at: (t) => {
        const s = t * S,
          ch = Math.cosh(r0),
          u = (w0 / (2 * d1)) * (ch * Math.tanh(RHO * s + r0) - Math.sinh(r0));
        return [x0 + u * dx, y0 + u * dy, (w0 * ch) / Math.cosh(RHO * s + r0)];
      },
    };
  };
  const create = (onChange) => {
    const cam = { x: 0, y: 0, k: 1, W: 800, H: 600, anim: null };
    const ping = () => {
      if (onChange) onChange();
    };
    cam.toWorld = (sx, sy) => ({
      x: (sx - cam.x) / cam.k,
      y: (sy - cam.y) / cam.k,
    });
    cam.center = () => cam.toWorld(cam.W / 2, cam.H / 2);
    cam.set = (cx, cy, k) => {
      cam.k = clamp(k, KMIN, KMAX);
      cam.x = cam.W / 2 - cx * cam.k;
      cam.y = cam.H / 2 - cy * cam.k;
      cam.anim = null;
      ping();
    };
    cam.cancel = () => {
      cam.anim = null;
    };
    cam.flyTo = (cx, cy, k, dur, instant, track) => {
      k = clamp(k, KMIN, KMAX);
      if (instant) {
        cam.set(cx, cy, k);
        return;
      }
      const c0 = cam.center();
      const path = zoomPath(c0.x, c0.y, cam.W / cam.k, cx, cy, cam.W / k);
      const span = isFinite(path.S) ? Math.abs(path.S) : 0;
      cam.anim = {
        t0: -1,
        dur: dur > 0 ? dur : clamp(420 + span * 260, 450, 900),
        path,
        k1: k,
        cx,
        cy,
        cx0: cx,
        cy0: cy,
        track: track || null,
      };
      ping();
    };
    cam.step = (now) => {
      const a = cam.anim;
      if (!a) return false;
      if (a.t0 < 0) a.t0 = now;
      const p = clamp((now - a.t0) / a.dur, 0, 1);
      const e = p < 0.5 ? 4 * p * p * p : 1 - Math.pow(-2 * p + 2, 3) / 2;
      if (a.track && isFinite(a.track.x) && isFinite(a.track.y)) {
        a.cx = a.track.x;
        a.cy = a.track.y;
      }
      if (p >= 1) {
        cam.k = a.k1;
        cam.x = cam.W / 2 - a.cx * cam.k;
        cam.y = cam.H / 2 - a.cy * cam.k;
        cam.anim = null;
        return false;
      }
      const [px, py, w] = a.path.at(e);
      const x = px + (a.cx - a.cx0) * e,
        y = py + (a.cy - a.cy0) * e;
      cam.k = clamp(cam.W / w, KMIN, KMAX);
      cam.x = cam.W / 2 - x * cam.k;
      cam.y = cam.H / 2 - y * cam.k;
      return true;
    };
    cam.zoomAt = (sx, sy, f) => {
      cam.anim = null;
      const w = cam.toWorld(sx, sy);
      cam.k = clamp(cam.k * f, KMIN, KMAX);
      cam.x = sx - w.x * cam.k;
      cam.y = sy - w.y * cam.k;
      ping();
    };
    cam.panBy = (dx, dy) => {
      cam.x += dx;
      cam.y += dy;
      ping();
    };
    cam.resize = (W, H, keepCenter) => {
      const dw = W - cam.W,
        dh = H - cam.H;
      cam.W = W;
      cam.H = H;
      if (keepCenter) {
        cam.x += dw / 2;
        cam.y += dh / 2;
      }
      ping();
    };
    return cam;
  };
  G.camera = { create, KMIN, KMAX };
})();
