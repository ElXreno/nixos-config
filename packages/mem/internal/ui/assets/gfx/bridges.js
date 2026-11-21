"use strict";
(() => {
  const G = (window.MemGfx = window.MemGfx || {});
  const T = G.theme;
  const C = T.c;
  const STRIDE = 8;
  const compute = (topics, links) => {
    const pair = new Map();
    for (const l of links) {
      const a = l.a.topic,
        b = l.b.topic;
      if (a === b) continue;
      const lo = Math.min(a, b),
        hi = Math.max(a, b),
        key = lo * 4096 + hi;
      pair.set(key, (pair.get(key) || 0) + 1);
    }
    let wmax = 1;
    for (const w of pair.values()) if (w > wmax) wmax = w;
    const out = [];
    for (const [key, w] of pair) {
      const A = topics[Math.floor(key / 4096)],
        B = topics[key % 4096];
      if (!A || !B) continue;
      const lw = Math.log2(1 + w),
        share = lw / Math.log2(1 + wmax);
      out.push({
        a: A.i,
        b: B.i,
        w,
        on: false,
        x0: 0,
        y0: 0,
        x1: 0,
        y1: 0,
        px: 0.45 + 0.8 * lw,
        alpha: 0.05 + 0.22 * share,
      });
    }
    out.sort((p, q) => p.w - q.w);
    place(out, topics);
    return out;
  };
  const place = (list, topics) => {
    for (const b of list) {
      const A = topics[b.a],
        B = topics[b.b];
      b.on = false;
      if (!A || !B || !(A.R > 0) || !(B.R > 0)) continue;
      const dx = B.ax - A.ax,
        dy = B.ay - A.ay,
        d = Math.hypot(dx, dy);
      if (d < 1) continue;
      const ang = Math.atan2(dy, dx),
        ra = G.terrain.radiusAt(A, ang) * 0.98,
        rb = G.terrain.radiusAt(B, ang + Math.PI) * 0.98;
      if (d <= ra + rb + 4) continue;
      const ux = dx / d,
        uy = dy / d;
      b.on = true;
      b.x0 = A.ax + ux * ra;
      b.y0 = A.ay + uy * ra;
      b.x1 = B.ax - ux * rb;
      b.y1 = B.ay - uy * rb;
    }
  };
  const SHADERS = (P) => {
    const VS =
      P.HEAD +
      P.COMMON +
      `
layout(location = 0) in vec4 a_p;
layout(location = 1) in vec4 a_w;
uniform float u_bridgeA;
out vec2 v_e;
flat out float v_a;
void main() {
  int seg = gl_VertexID >> 1;
  float side = (gl_VertexID & 1) == 0 ? -1. : 1.;
  float t = float(seg) / 16.;
  vec2 p0 = a_p.xy * u_view.z + u_view.xy;
  vec2 p1 = a_p.zw * u_view.z + u_view.xy;
  vec2 dd = p1 - p0;
  float len = length(dd) + 1e-4;
  vec2 nrm = vec2(-dd.y, dd.x) / len;
  vec2 mid = (p0 + p1) * .5 + nrm * len * .13 * a_w.z;
  float u = 1. - t;
  vec2 pos = u * u * p0 + 2. * u * t * mid + t * t * p1;
  vec2 tg = normalize(2. * u * (mid - p0) + 2. * t * (p1 - mid) + vec2(1e-6));
  vec2 nn = vec2(-tg.y, tg.x);
  float w = a_w.x * pow(sin(3.14159265 * t), .8);
  float hw = w * .5 + .75;
  v_e = vec2(side * hw, w * .5);
  v_a = a_w.y * u_bridgeA;
  gl_Position = toClip(pos + nn * side * hw);
}
`;
    const FS =
      P.HEAD +
      P.COMMON +
      `
uniform vec3 u_link;
in vec2 v_e;
flat in float v_a;
out vec4 o;
void main() {
  float a = v_a * cov(abs(v_e.x) - v_e.y);
  o = vec4(u_link * a, a);
}
`;
    return { VS, FS };
  };
  const gl2 = (gl, P) => {
    const S = SHADERS(P);
    const prog = P.program(gl, S.VS, S.FS);
    const vao = gl.createVertexArray();
    let buf = null,
      n = 0,
      data = null;
    const pack = (list) => {
      n = list.length;
      const f =
        data && data.length === Math.max(1, n) * STRIDE
          ? data
          : new Float32Array(Math.max(1, n) * STRIDE);
      list.forEach((b, i) => {
        const o = i * STRIDE;
        f[o] = b.x0;
        f[o + 1] = b.y0;
        f[o + 2] = b.x1;
        f[o + 3] = b.y1;
        f[o + 4] = b.px;
        f[o + 5] = b.on ? b.alpha : 0;
        f[o + 6] = b.a < b.b ? 1 : -1;
      });
      data = f;
      return f;
    };
    const update = (list) => {
      if (!buf || list.length !== n) {
        set(list);
        return;
      }
      const f = pack(list);
      gl.bindBuffer(gl.ARRAY_BUFFER, buf);
      gl.bufferSubData(gl.ARRAY_BUFFER, 0, f);
    };
    const set = (list) => {
      const f = pack(list);
      if (buf) gl.deleteBuffer(buf);
      buf = P.buffer(gl, gl.ARRAY_BUFFER, f, gl.DYNAMIC_DRAW);
      gl.bindVertexArray(vao);
      gl.bindBuffer(gl.ARRAY_BUFFER, buf);
      for (const [loc, off] of [
        [0, 0],
        [1, 16],
      ]) {
        gl.enableVertexAttribArray(loc);
        gl.vertexAttribPointer(loc, 4, gl.FLOAT, false, STRIDE * 4, off);
        gl.vertexAttribDivisor(loc, 1);
      }
      gl.bindVertexArray(null);
    };
    const draw = (v, common) => {
      if (!n || v.bridgeA < 0.01) return;
      common(prog, v);
      const L = prog.loc,
        k = T.unit(C.m1);
      gl.uniform1f(L.u_bridgeA, v.bridgeA);
      gl.uniform3f(L.u_link, k[0], k[1], k[2]);
      gl.bindVertexArray(vao);
      gl.drawArraysInstanced(gl.TRIANGLE_STRIP, 0, 34, n);
    };
    return { set, update, draw };
  };
  G.bridges = { compute, place, gl: gl2 };
})();
