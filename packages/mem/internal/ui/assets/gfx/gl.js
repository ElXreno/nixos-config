"use strict";
(() => {
  const G = (window.MemGfx = window.MemGfx || {});
  const STATE_W = 1024;
  const TS_W = 256;
  const HEAD =
    "#version 300 es\nprecision highp float;\nprecision highp int;\nprecision highp sampler2D;\n";
  const COMMON = `
uniform vec4 u_view;
uniform vec2 u_res;
uniform sampler2D u_state;
uniform sampler2D u_ts;
uniform sampler2D u_pos;
uniform float u_now;
uniform float u_reduced;
uniform float u_ds;
vec4 stateOf(float idx) {
  int i = int(idx + .5);
  return texelFetch(u_state, ivec2(i & ${STATE_W - 1}, i >> 10), 0);
}
vec2 posOf(float idx) {
  int i = int(idx + .5);
  return texelFetch(u_pos, ivec2(i & ${STATE_W - 1}, i >> 10), 0).xy;
}
vec4 tsOf(float t, int row) {
  return texelFetch(u_ts, ivec2(int(t + .5), row), 0);
}
vec4 toClip(vec2 s) { return vec4(s.x / u_res.x * 2. - 1., 1. - s.y / u_res.y * 2., 0., 1.); }
float cov(float x) { return clamp(.5 - x * u_ds, 0., 1.); }
`;
  const NODE_VS =
    HEAD +
    COMMON +
    `
layout(location = 0) in vec2 a_q;
layout(location = 1) in vec2 a_rs;
layout(location = 2) in vec4 a_col;
layout(location = 3) in vec4 a_fl;
uniform float u_sel;
out vec2 v_d;
flat out vec4 v_a;
flat out vec4 v_b;
flat out vec3 v_col;
flat out float v_shape;
void main() {
  vec4 st = stateOf(a_rs.y);
  vec4 ts = tsOf(st.w, 0);
  float hl = st.x;
  float fl = (u_now - st.z) * 1000.;
  float fa = (fl >= 0. && fl < 9000.) ? (u_reduced > .5 ? 1. : pow(1. - fl / 9000., 1.4)) : 0.;
  float pulse = (u_reduced < .5 && fl >= 0. && fl < 1400.) ? fl / 1400. : -1.;
  float rf = a_rs.x * .78 * (.35 + .65 * st.y) * u_view.z;
  float lm = (a_fl.w > .5 || a_fl.y > .5) ? 1. : 0.;
  float rpx = lm > .5 ? max(rf, 1.5 + 1.9 * st.y) : mix(clamp(rf, 1.1, 1.7), max(rf, 1.5), ts.x);
  float quiet = lm > .5 ? 1. : mix(.6, 1., ts.x);
  float sel = abs(a_rs.y - u_sel) < .5 ? 1. : 0.;
  v_a = vec4(rpx, max(hl * quiet, fa), fa, pulse);
  v_b = vec4(a_fl.x, a_fl.y, a_fl.z, sel);
  v_shape = a_fl.w;
  v_col = a_col.rgb;
  float ext = rpx * 1.5 + 9. + (pulse >= 0. ? 26. : 0.);
  v_d = a_q * ext;
  if (hl < .01) { gl_Position = vec4(2., 2., 2., 1.); return; }
  vec2 c = posOf(a_rs.y) * u_view.z + u_view.xy;
  gl_Position = toClip(c + v_d);
}
`;
  const NODE_FS =
    HEAD +
    COMMON +
    `
uniform vec3 u_bg;
uniform vec3 u_live;
uniform vec3 u_danger;
uniform vec3 u_bright;
uniform vec3 u_m2;
in vec2 v_d;
flat in vec4 v_a;
flat in vec4 v_b;
flat in vec3 v_col;
flat in float v_shape;
out vec4 o;
float sdShape(vec2 p, float r, float shape) {
  if (shape < .5) return length(p) - r;
  if (shape < 1.5) return (abs(p.x) + abs(p.y)) * .70710678 - r * .92;
  float ri = r * .74;
  return max(p.y - ri, max(dot(p, vec2(.8660254, -.5)) - ri, dot(p, vec2(-.8660254, -.5)) - ri));
}
vec4 over(vec4 dst, vec3 rgb, float a) { return dst * (1. - a) + vec4(rgb * a, a); }
float disc(float d, float r) { return cov(d - r); }
float ring(float d, float r, float w) { return cov(abs(d - r) - w * .5); }
void main() {
  float r = v_a.x;
  float d = r + sdShape(v_d, r, v_shape);
  float alpha = v_a.y, fa = v_a.z, pulse = v_a.w;
  float status = v_b.x, danger = v_b.y, core = v_b.z, sel = v_b.w;
  vec4 c = vec4(0.);
  vec3 vc = v_shape > 1.5 ? mix(v_col, u_bright, .55) : v_col;
  if (status < .5) {
    c = over(c, vc, disc(d, r));
  } else {
    c = over(c, u_bg, disc(d, r));
    float w = max(1.3, r * .2);
    float on = status > 1.5 && status < 2.5 ? max(r * .25, 1.) : max(r * .55, 1.6);
    float off = status > 1.5 && status < 2.5 ? max(r * .4, 1.4) : max(r * .42, 1.3);
    float n = max(3., floor(6.2831853 * r / (on + off) + .5));
    float ph = fract(atan(v_d.y, v_d.x) / 6.2831853 * n);
    float dash = smoothstep(0., .06, ph) * (1. - smoothstep(on / (on + off) - .06, on / (on + off), ph));
    vec3 rc = status > 2.5 ? u_m2 : vc;
    c = over(c, rc, ring(d, r, w) * dash);
  }
  if (fa > 0.) {
    if (u_reduced > .5) {
      c = over(c, u_live, ring(d, r + 3.5, 1.8) * .9);
    } else {
      c = over(c, u_live, disc(d, r) * fa * .85);
      c = over(c, u_live, ring(d, r + 3.5, 1.4) * fa * .55);
    }
  }
  if (pulse >= 0.) {
    c = over(c, u_live, ring(d, r + pulse * 22., 1.6) * (1. - pulse) * .8);
  }
  if (danger > .5) {
    c = over(c, u_danger, ring(d, r + max(1.6, r * .3), max(1.3, r * .24)));
  } else if (core > .5) {
    c = over(c, u_bright, ring(d, r + max(1.4, r * .26), max(1., r * .16)));
  }
  if (sel > .5) {
    c = over(c, u_bright, ring(d, r + (danger > .5 ? 6. + r * .3 : 4.), 1.6));
  }
  o = c * alpha;
}
`;
  const EDGE_VS =
    HEAD +
    COMMON +
    `
layout(location = 0) in vec2 a_ij;
layout(location = 1) in vec4 a_c0;
layout(location = 2) in vec4 a_c1;
layout(location = 3) in vec4 a_ty;
uniform float u_focus;
uniform float u_focusHl;
uniform vec3 u_link;
uniform float u_linkA;
uniform float u_cross;
out vec4 v_col;
out vec3 v_e;
flat out float v_ty;
void main() {
  int seg = gl_VertexID >> 1;
  float side = (gl_VertexID & 1) == 0 ? -1. : 1.;
  float t = float(seg) / 8.;
  vec4 s0 = stateOf(a_ij.x), s1 = stateOf(a_ij.y);
  float m = min(s0.x, s1.x);
  bool hot = abs(a_ij.x - u_focus) < .5 || abs(a_ij.y - u_focus) < .5;
  vec2 p0 = posOf(a_ij.x) * u_view.z + u_view.xy;
  vec2 p1 = posOf(a_ij.y) * u_view.z + u_view.xy;
  vec2 dd = p1 - p0;
  float len = length(dd) + 1e-4;
  vec2 nrm = vec2(-dd.y, dd.x) / len;
  vec2 mid = (p0 + p1) * .5 + nrm * len * ${G.theme.curve.toFixed(3)};
  float u = 1. - t;
  vec2 pos = u * u * p0 + 2. * u * t * mid + t * t * p1;
  vec2 tg = normalize(2. * u * (mid - p0) + 2. * t * (p1 - mid) + vec2(1e-6));
  vec2 nn = vec2(-tg.y, tg.x);
  float ty = a_ty.x;
  float w = .75;
  float lvl = abs(s0.w - s1.w) < .5 ? tsOf(s0.w, 0).y : u_cross * .32;
  float a = u_linkA * lvl * (.13 + .87 * m) * smoothstep(0., .02, m) * (1. + .8 * smoothstep(.8, 3., u_view.z));
  vec3 rgb = u_link;
  if (ty > 1.5) w = mix(.5, 2.1, t);
  else if (ty > .5) w = .95;
  if (hot) {
    bool first = abs(a_ij.x - u_focus) < .5;
    rgb = (first ? a_c1.rgb : a_c0.rgb);
    w = 1.3;
    a = .65 * u_focusHl + .1;
    a *= smoothstep(0., .02, m);
  }
  float hw = w * .5 + .75;
  v_col = vec4(rgb, a);
  v_e = vec3(side * hw, w * .5, t * len * 1.08);
  v_ty = hot ? 0. : ty;
  gl_Position = toClip(pos + nn * side * hw);
}
`;
  const EDGE_FS =
    HEAD +
    COMMON +
    `
in vec4 v_col;
in vec3 v_e;
flat in float v_ty;
out vec4 o;
void main() {
  float a = v_col.a * cov(abs(v_e.x) - v_e.y);
  if (v_ty > .5 && v_ty < 1.5) a *= step(.45, fract(v_e.z / 4.));
  o = vec4(v_col.rgb * a, a);
}
`;
  const compile = (gl, type, src) => {
    const s = gl.createShader(type);
    gl.shaderSource(s, src);
    gl.compileShader(s);
    if (!gl.getShaderParameter(s, gl.COMPILE_STATUS)) {
      const log = gl.getShaderInfoLog(s);
      gl.deleteShader(s);
      throw new Error("shader: " + log);
    }
    return s;
  };
  const program = (gl, vs, fs) => {
    const p = gl.createProgram();
    const a = compile(gl, gl.VERTEX_SHADER, vs),
      b = compile(gl, gl.FRAGMENT_SHADER, fs);
    gl.attachShader(p, a);
    gl.attachShader(p, b);
    gl.linkProgram(p);
    gl.deleteShader(a);
    gl.deleteShader(b);
    if (!gl.getProgramParameter(p, gl.LINK_STATUS))
      throw new Error("link: " + gl.getProgramInfoLog(p));
    const loc = {};
    const n = gl.getProgramParameter(p, gl.ACTIVE_UNIFORMS);
    for (let i = 0; i < n; i++) {
      const info = gl.getActiveUniform(p, i);
      loc[info.name] = gl.getUniformLocation(p, info.name);
    }
    return { p, loc };
  };
  const context = (canvas) => {
    let gl = null;
    try {
      gl = canvas.getContext("webgl2", {
        antialias: false,
        alpha: false,
        depth: false,
        stencil: false,
        powerPreference: "low-power",
        preserveDrawingBuffer: false,
      });
    } catch (e) {
      gl = null;
    }
    return gl || null;
  };
  const buffer = (gl, target, data, usage) => {
    const b = gl.createBuffer();
    gl.bindBuffer(target, b);
    gl.bufferData(target, data, usage || gl.STATIC_DRAW);
    return b;
  };
  G.gl = {
    STATE_W,
    TS_W,
    HEAD,
    COMMON,
    context,
    program,
    buffer,
    SRC: { NODE_VS, NODE_FS, EDGE_VS, EDGE_FS },
  };
})();
