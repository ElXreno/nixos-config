"use strict";
(() => {
  const G = (window.MemGfx = window.MemGfx || {});
  const T = G.theme;
  const C = T.c;
  const SEG = 144,
    HULL = 48,
    TAU = Math.PI * 2,
    PAD = 12,
    HW = 1 + SEG / 4,
    BIN = TAU / HULL;
  const smooth = (a, b, x) => {
    const t = x <= a ? 0 : x >= b ? 1 : (x - a) / (b - a);
    return t * t * (3 - 2 * t);
  };
  const hullAt = (hull, ang) => {
    const n = hull.length;
    let u = ((ang + Math.PI) / TAU) * n - 0.5;
    u -= Math.floor(u / n) * n;
    const i = Math.floor(u),
      f = u - i;
    const p0 = hull[(i + n - 1) % n],
      p1 = hull[i % n],
      p2 = hull[(i + 1) % n],
      p3 = hull[(i + 2) % n];
    return (
      0.5 *
      (2 * p1 +
        (p2 - p0) * f +
        (2 * p0 - 5 * p1 + 4 * p2 - p3) * f * f +
        (3 * p1 - p0 - 3 * p2 + p3) * f * f * f)
    );
  };
  const smoothHull = (hull) => {
    if (hull.__s) return hull.__s;
    const n = hull.length;
    let a = new Array(n);
    for (let i = 0; i < n; i++)
      a[i] = Math.max(
        hull[(i + n - 1) % n] * 0.97,
        hull[i],
        hull[(i + 1) % n] * 0.97,
      );
    for (let pass = 0; pass < 2; pass++) {
      const b = new Array(n);
      for (let i = 0; i < n; i++)
        b[i] = (a[(i + n - 1) % n] + 2 * a[i] + a[(i + 1) % n]) / 4;
      a = b;
    }
    hull.__s = a;
    return a;
  };
  const radiusAt = (t, ang) =>
    t.hull && t.hull.length === HULL ? hullAt(smoothHull(t.hull), ang) : t.R;
  const levels = (Rs, k) => {
    const spread = smooth(0.85, 1.25, k);
    return {
      full: Math.max(smooth(55, 100, Rs), smooth(1, 1.6, k)),
      edge: Math.max(smooth(70, 140, Rs), spread),
      cross: spread,
    };
  };
  const COSB = new Float32Array(HULL),
    SINB = new Float32Array(HULL),
    RAW = new Float32Array(HULL),
    TMP = new Float32Array(HULL),
    ENV = new Float32Array(HULL);
  for (let b = 0; b < HULL; b++) {
    const a = -Math.PI + (b + 0.5) * BIN;
    COSB[b] = Math.cos(a);
    SINB[b] = Math.sin(a);
  }
  const LEASH = 18;
  let PX = new Float64Array(0),
    PY = new Float64Array(0);
  const fit = (t) => {
    const ms = t.members,
      n = ms.length;
    if (!n) return false;
    if (PX.length < n) {
      PX = new Float64Array(n * 2);
      PY = new Float64Array(n * 2);
    }
    for (let i = 0; i < n; i++) {
      const m = ms[i],
        hx = m.hx,
        hy = m.hy;
      if (!(isFinite(hx) && isFinite(hy))) {
        PX[i] = m.x;
        PY[i] = m.y;
        continue;
      }
      const dx = m.x - hx,
        dy = m.y - hy,
        d = Math.sqrt(dx * dx + dy * dy),
        f = d > LEASH ? LEASH / d : 1;
      PX[i] = hx + dx * f;
      PY[i] = hy + dy * f;
    }
    let cx = 0,
      cy = 0;
    for (let i = 0; i < n; i++) {
      cx += PX[i];
      cy += PY[i];
    }
    cx /= n;
    cy /= n;
    if (n > 4) {
      let sd = 0;
      for (let i = 0; i < n; i++) {
        const dx = PX[i] - cx,
          dy = PY[i] - cy;
        sd += Math.sqrt(dx * dx + dy * dy);
      }
      const lim = (1.7 * sd) / n;
      let tx = 0,
        ty = 0,
        tc = 0;
      for (let i = 0; i < n; i++) {
        const dx = PX[i] - cx,
          dy = PY[i] - cy;
        if (Math.sqrt(dx * dx + dy * dy) <= lim) {
          tx += PX[i];
          ty += PY[i];
          tc++;
        }
      }
      if (tc) {
        cx = tx / tc;
        cy = ty / tc;
      }
    }
    const raw = RAW;
    raw.fill(0);
    for (let i = 0; i < n; i++) {
      const m = ms[i],
        dx = PX[i] - cx,
        dy = PY[i] - cy,
        dist = Math.sqrt(dx * dx + dy * dy),
        rho = m.r * (m.shape ? 1.35 : 1) + PAD,
        rr = rho * rho;
      const full = dist <= rho;
      const span = full
        ? HULL
        : Math.min(
            HULL,
            Math.ceil(Math.min(Math.PI, (rho / dist) * 1.58) / BIN) + 1,
          );
      const b0 = full ? 0 : Math.floor((Math.atan2(dy, dx) + Math.PI) / BIN);
      const from = full ? 0 : -span,
        to = full ? HULL - 1 : span;
      for (let o = from; o <= to; o++) {
        const b = full ? o : (b0 + o + HULL * 2) % HULL;
        const ux = COSB[b],
          uy = SINB[b],
          perp = dx * uy - dy * ux,
          q = rr - perp * perp;
        if (q <= 0) continue;
        const ext = dx * ux + dy * uy + Math.sqrt(q);
        if (ext > raw[b]) raw[b] = ext;
      }
    }
    let any = false;
    for (let b = 0; b < HULL; b++)
      if (raw[b] > 0) {
        any = true;
        break;
      }
    if (!any) return false;
    for (let b = 0; b < HULL;) {
      if (raw[b] > 0) {
        b++;
        continue;
      }
      let e = b;
      while (e < HULL && raw[e] <= 0) e++;
      const lo = raw[(b + HULL - 1) % HULL],
        hi = raw[e % HULL],
        len = e - b + 1;
      for (let q = b; q < e; q++) raw[q] = lo + ((hi - lo) * (q - b + 1)) / len;
      b = e;
    }
    for (let i = 0; i < HULL; i++)
      ENV[i] = Math.max(
        raw[(i + HULL - 1) % HULL] * 0.96,
        raw[i],
        raw[(i + 1) % HULL] * 0.96,
      );
    for (let pass = 0; pass < 2; pass++) {
      for (let i = 0; i < HULL; i++)
        TMP[i] =
          (ENV[(i + HULL - 1) % HULL] + 2 * ENV[i] + ENV[(i + 1) % HULL]) / 4;
      ENV.set(TMP);
    }
    let fin = t.hull;
    if (!(fin && fin.__s === fin && fin.length === HULL)) {
      fin = new Array(HULL);
      fin.__s = fin;
    }
    let sq = 0;
    for (let i = 0; i < HULL; i++) {
      fin[i] = Math.max(8, ENV[i], raw[i] * 0.985);
      sq += fin[i] * fin[i];
    }
    t.hull = fin;
    t.ax = t.lx = cx;
    t.ay = t.ly = cy;
    t.R = Math.sqrt(sq / HULL);
    return true;
  };
  const bounds = (t) => {
    const h = t.hull;
    if (!(h && h.length === HULL))
      return [t.ax - t.R, t.ay - t.R, t.ax + t.R, t.ay + t.R];
    let x0 = Infinity,
      y0 = Infinity,
      x1 = -Infinity,
      y1 = -Infinity;
    for (let b = 0; b < HULL; b++) {
      const x = t.ax + h[b] * COSB[b],
        y = t.ay + h[b] * SINB[b];
      if (x < x0) x0 = x;
      if (x > x1) x1 = x;
      if (y < y0) y0 = y;
      if (y > y1) y1 = y;
    }
    return [x0, y0, x1, y1];
  };
  const fitAll = (topics) => {
    let any = false;
    for (const t of topics) if (fit(t)) any = true;
    return any;
  };
  const tintOf = (members) => {
    const cnt = {};
    let n = 0;
    for (const m of members) {
      cnt[m.type] = (cnt[m.type] || 0) + 1;
      n++;
    }
    let best = "",
      bc = 0;
    for (const k in cnt)
      if (cnt[k] > bc) {
        bc = cnt[k];
        best = k;
      }
    if (!n || !T.kind[best]) return [0, 0, 0];
    const w = smooth(0.3, 0.62, bc / n),
      u = T.unit(T.kind[best]);
    return [u[0] * w, u[1] * w, u[2] * w];
  };
  const ringGeometry = (topics) => {
    const fill = [],
      line = [];
    topics.forEach((t, ti) => {
      if (!(t.R > 0) || (!t.members.length && t.size === 0)) return;
      const dir = [];
      for (let j = 0; j < SEG; j++) {
        const a = -Math.PI + (j / SEG) * TAU;
        dir.push([Math.cos(a), Math.sin(a)]);
      }
      for (let j = 0; j < SEG; j++) {
        const j1 = (j + 1) % SEG,
          A = [dir[j][0], dir[j][1], j],
          B = [dir[j1][0], dir[j1][1], j1];
        fill.push(0, 0, -1, 0, ti, ...A, 1, ti, ...B, 1, ti);
        line.push(
          ...A,
          -1,
          ti,
          ...A,
          1,
          ti,
          ...B,
          -1,
          ti,
          ...B,
          -1,
          ti,
          ...A,
          1,
          ti,
          ...B,
          1,
          ti,
        );
      }
    });
    return { fill: new Float32Array(fill), line: new Float32Array(line) };
  };
  const hullRows = (topics, out) => {
    const rows = topics.length,
      need = HW * 4 * Math.max(1, rows);
    const d = out && out.length === need ? out : new Float32Array(need);
    topics.forEach((t, ti) => {
      const o = ti * HW * 4;
      let mean = 0;
      for (let j = 0; j < SEG; j++) {
        const r = radiusAt(t, -Math.PI + (j / SEG) * TAU);
        d[o + 4 + j] = r;
        mean += r / SEG;
      }
      d[o] = t.ax;
      d[o + 1] = t.ay;
      d[o + 2] = mean;
    });
    return d;
  };

  const SHADERS = (P) => {
    const HULLFN = `
uniform sampler2D u_hull;
vec4 hullHead(float ti) { return texelFetch(u_hull, ivec2(0, int(ti + .5)), 0); }
float hullRad(float ti, float seg) {
  int s = int(seg + .5);
  vec4 t = texelFetch(u_hull, ivec2(1 + (s >> 2), int(ti + .5)), 0);
  return t[s & 3];
}
`;
    const FILL_VS =
      P.HEAD +
      P.COMMON +
      HULLFN +
      `
layout(location = 0) in vec3 a_dr;
layout(location = 1) in vec2 a_ut;
out vec2 v_u;
flat out vec4 v_s;
flat out vec3 v_tint;
void main() {
  vec4 ts = tsOf(a_ut.y, 0), tn = tsOf(a_ut.y, 1);
  vec4 hd = hullHead(a_ut.y);
  float rs = (a_dr.z < 0. ? hd.z : hullRad(a_ut.y, a_dr.z)) * u_view.z;
  vec2 p = hd.xy * u_view.z + u_view.xy + a_dr.xy * rs * a_ut.x;
  v_u = vec2(a_ut.x, rs);
  v_s = vec4(ts.z, ts.w, tn.w, 0.);
  v_tint = tn.rgb;
  gl_Position = toClip(p);
}
`;
    const FILL_FS =
      P.HEAD +
      P.COMMON +
      `
uniform vec3 u_fill;
uniform vec3 u_fillIn;
uniform vec3 u_edge;
uniform vec3 u_live;
in vec2 v_u;
flat in vec4 v_s;
flat in vec3 v_tint;
out vec4 o;
void main() {
  float u = v_u.x, din = (1. - u) * v_u.y;
  float dim = v_s.x, emph = v_s.y;
  vec3 col = mix(u_fillIn, u_fill, smoothstep(.15, 1., u));
  col += v_tint * .03;
  float band = exp(-din / 9.);
  col += (u_edge * .5 + v_tint * .12) * band * (.2 + .22 * emph);
  col += vec3(.02) * emph;
  float br = v_s.z;
  col = mix(col, u_live * .5, br * .16 * (.55 + .45 * u));
  col += u_live * band * br * .3;
  float a = clamp(dim, 0., 1.) * smoothstep(0., 1.2, din);
  o = vec4(col * a, a);
}
`;
    const LINE_VS =
      P.HEAD +
      P.COMMON +
      HULLFN +
      `
layout(location = 0) in vec3 a_dr;
layout(location = 1) in vec2 a_ut;
uniform float u_off;
uniform float u_hw;
out float v_e;
flat out vec4 v_s;
flat out vec3 v_tint;
void main() {
  vec4 ts = tsOf(a_ut.y, 0), tn = tsOf(a_ut.y, 1);
  vec4 hd = hullHead(a_ut.y);
  float rs = hullRad(a_ut.y, a_dr.z) * u_view.z;
  vec2 p = hd.xy * u_view.z + u_view.xy + a_dr.xy * (rs + u_off + a_ut.x * u_hw);
  v_e = a_ut.x * u_hw;
  v_s = vec4(ts.z, ts.w, tn.w, 0.);
  v_tint = tn.rgb;
  gl_Position = toClip(p);
}
`;
    const LINE_FS =
      P.HEAD +
      P.COMMON +
      `
uniform vec3 u_col;
uniform float u_core;
uniform float u_alpha;
uniform float u_soft;
uniform vec3 u_live;
in float v_e;
flat in vec4 v_s;
flat in vec3 v_tint;
out vec4 o;
void main() {
  float a = cov(abs(v_e) - u_core);
  if (u_soft > 0.) {
    float t = clamp((v_e + u_soft) / (2. * u_soft), 0., 1.);
    a = (1. - t) * (1. - t) * smoothstep(0., .08, t);
  }
  a *= u_alpha * clamp(v_s.x, 0., 1.) * (1. + .9 * v_s.y + 1.4 * v_s.z);
  vec3 c = mix(u_col + v_tint * .2 + vec3(.08) * v_s.y, u_live, clamp(v_s.z * .85, 0., 1.));
  o = vec4(c * a, a);
}
`;
    const VIG_VS =
      P.HEAD +
      `
void main() {
  vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
  gl_Position = vec4(p * 2. - 1., 0., 1.);
}
`;
    const VIG_FS =
      P.HEAD +
      `
uniform vec2 u_res;
uniform vec3 u_vig;
out vec4 o;
void main() {
  vec2 q = gl_FragCoord.xy / u_res * 2. - 1.;
  float a = smoothstep(.55, 1.5, length(q * vec2(.9, 1.1))) * .85;
  o = vec4(u_vig * a, a);
}
`;
    return { FILL_VS, FILL_FS, LINE_VS, LINE_FS, VIG_VS, VIG_FS };
  };

  const gl2 = (gl, P) => {
    const S = SHADERS(P);
    const fillProg = P.program(gl, S.FILL_VS, S.FILL_FS);
    const lineProg = P.program(gl, S.LINE_VS, S.LINE_FS);
    const vigProg = P.program(gl, S.VIG_VS, S.VIG_FS);
    const fillVao = gl.createVertexArray(),
      lineVao = gl.createVertexArray();
    let fillBuf = null,
      lineBuf = null,
      fillN = 0,
      lineN = 0,
      rows = 0,
      hullData = null;
    const hullTex = gl.createTexture();
    const attr = (loc, size, stride, off) => {
      gl.enableVertexAttribArray(loc);
      gl.vertexAttribPointer(loc, size, gl.FLOAT, false, stride, off);
      gl.vertexAttribDivisor(loc, 0);
    };
    const setTopics = (topics) => {
      const g = ringGeometry(topics);
      fillN = g.fill.length / 5;
      lineN = g.line.length / 5;
      if (fillBuf) gl.deleteBuffer(fillBuf);
      if (lineBuf) gl.deleteBuffer(lineBuf);
      fillBuf = P.buffer(gl, gl.ARRAY_BUFFER, g.fill);
      lineBuf = P.buffer(gl, gl.ARRAY_BUFFER, g.line);
      for (const [vao, buf] of [
        [fillVao, fillBuf],
        [lineVao, lineBuf],
      ]) {
        gl.bindVertexArray(vao);
        gl.bindBuffer(gl.ARRAY_BUFFER, buf);
        attr(0, 3, 20, 0);
        attr(1, 2, 20, 12);
      }
      gl.bindVertexArray(null);
      setHulls(topics, true);
    };
    const setHulls = (topics, realloc) => {
      hullData = hullRows(topics, hullData);
      gl.activeTexture(gl.TEXTURE3);
      gl.bindTexture(gl.TEXTURE_2D, hullTex);
      if (realloc || rows !== topics.length) {
        rows = topics.length;
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
        gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
        gl.texImage2D(
          gl.TEXTURE_2D,
          0,
          gl.RGBA32F,
          HW,
          Math.max(1, rows),
          0,
          gl.RGBA,
          gl.FLOAT,
          hullData,
        );
      } else
        gl.texSubImage2D(
          gl.TEXTURE_2D,
          0,
          0,
          0,
          HW,
          Math.max(1, rows),
          gl.RGBA,
          gl.FLOAT,
          hullData,
        );
      gl.activeTexture(gl.TEXTURE0);
    };
    const uni3 = (loc, h) => {
      const u = T.unit(h);
      gl.uniform3f(loc, u[0], u[1], u[2]);
    };
    const backdrop = (v) => {
      gl.useProgram(vigProg.p);
      gl.uniform2f(vigProg.loc.u_res, v.W * v.ds, v.H * v.ds);
      uni3(vigProg.loc.u_vig, C.vignette);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
    };
    const bindHull = (L) => {
      gl.activeTexture(gl.TEXTURE3);
      gl.bindTexture(gl.TEXTURE_2D, hullTex);
      gl.activeTexture(gl.TEXTURE0);
      gl.uniform1i(L.u_hull, 3);
    };
    const fills = (v, common) => {
      if (!fillN) return;
      common(fillProg, v);
      const L = fillProg.loc;
      bindHull(L);
      uni3(L.u_fill, C.fill);
      uni3(L.u_fillIn, C.fillIn);
      uni3(L.u_edge, C.contour);
      uni3(L.u_live, C.live);
      gl.bindVertexArray(fillVao);
      gl.drawArrays(gl.TRIANGLES, 0, fillN);
    };
    const contours = (v, common) => {
      if (!lineN) return;
      common(lineProg, v);
      const L = lineProg.loc;
      bindHull(L);
      gl.bindVertexArray(lineVao);
      uni3(L.u_live, C.live);
      uni3(L.u_col, C.contour);
      gl.uniform1f(L.u_soft, 7);
      gl.uniform1f(L.u_off, 7);
      gl.uniform1f(L.u_hw, 7);
      gl.uniform1f(L.u_core, 0);
      gl.uniform1f(L.u_alpha, 0.16);
      uni3(L.u_col, C.glow);
      gl.drawArrays(gl.TRIANGLES, 0, lineN);
      gl.uniform1f(L.u_soft, 0);
      uni3(L.u_col, C.contour);
      gl.uniform1f(L.u_off, 0);
      gl.uniform1f(L.u_core, 0.55);
      gl.uniform1f(L.u_hw, 1.3);
      gl.uniform1f(L.u_alpha, 1);
      gl.drawArrays(gl.TRIANGLES, 0, lineN);
      gl.uniform1f(L.u_off, -3.5);
      gl.uniform1f(L.u_core, 0.45);
      gl.uniform1f(L.u_hw, 1.2);
      gl.uniform1f(L.u_alpha, 0.45);
      gl.drawArrays(gl.TRIANGLES, 0, lineN);
    };
    return { setTopics, setHulls, backdrop, fills, contours };
  };

  G.terrain = {
    smooth,
    hullAt,
    radiusAt,
    levels,
    tintOf,
    fit,
    fitAll,
    bounds,
    gl: gl2,
    SEG,
    HULL,
    PAD,
  };
})();
