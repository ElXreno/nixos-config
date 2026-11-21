"use strict";
(() => {
  const G = (window.MemGfx = window.MemGfx || {});
  const T = G.theme;
  const C = T.c;
  const STATUS_CODE = { current: 0, superseded: 1, expired: 2, retired: 3 };
  const TY_CODE = { r: 0, d: 1, s: 2 };
  const NODE_STRIDE = 16,
    EDGE_STRIDE = 20,
    EDGE_VERTS = 18;

  const glBackend = (canvas, gl) => {
    const P = G.gl;
    const nodeProg = P.program(gl, P.SRC.NODE_VS, P.SRC.NODE_FS);
    const edgeProg = P.program(gl, P.SRC.EDGE_VS, P.SRC.EDGE_FS);
    const quad = P.buffer(
      gl,
      gl.ARRAY_BUFFER,
      new Float32Array([-1, -1, 1, -1, -1, 1, 1, 1]),
    );
    const nodeVao = gl.createVertexArray();
    const edgeVao = gl.createVertexArray();
    const tex = gl.createTexture();
    const posTex = gl.createTexture();
    const tsTex = gl.createTexture();
    const terr = G.terrain.gl(gl, P),
      brg = G.bridges.gl(gl, P);
    let tsData = new Float32Array(P.TS_W * 2 * 4);
    let nodeBuf = null,
      edgeBuf = null,
      nNodes = 0,
      nEdges = 0,
      rows = 1,
      stateData = new Float32Array(P.STATE_W * 4),
      posData = new Float32Array(P.STATE_W * 4);
    const bg = T.unit(C.canvas);
    const attr = (loc, size, type, norm, stride, off, div) => {
      gl.enableVertexAttribArray(loc);
      gl.vertexAttribPointer(loc, size, type, norm, stride, off);
      gl.vertexAttribDivisor(loc, div);
    };
    const allocState = (n) => {
      rows = Math.max(1, Math.ceil(n / P.STATE_W));
      stateData = new Float32Array(P.STATE_W * rows * 4);
      gl.bindTexture(gl.TEXTURE_2D, tex);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      gl.texImage2D(
        gl.TEXTURE_2D,
        0,
        gl.RGBA32F,
        P.STATE_W,
        rows,
        0,
        gl.RGBA,
        gl.FLOAT,
        stateData,
      );
      posData = new Float32Array(P.STATE_W * rows * 4);
      gl.bindTexture(gl.TEXTURE_2D, posTex);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      gl.texImage2D(
        gl.TEXTURE_2D,
        0,
        gl.RGBA32F,
        P.STATE_W,
        rows,
        0,
        gl.RGBA,
        gl.FLOAT,
        posData,
      );
    };
    const pushPos = (nodes) => {
      const d = posData;
      for (let i = 0; i < nodes.length; i++) {
        const n = nodes[i],
          o = n.gi * 4;
        d[o] = n.x;
        d[o + 1] = n.y;
      }
      gl.bindTexture(gl.TEXTURE_2D, posTex);
      gl.texSubImage2D(
        gl.TEXTURE_2D,
        0,
        0,
        0,
        P.STATE_W,
        rows,
        gl.RGBA,
        gl.FLOAT,
        d,
      );
    };
    const setData = (nodes, links) => {
      nNodes = nodes.length;
      nEdges = links.length;
      nodes.forEach((n, i) => {
        n.gi = i;
      });
      const order = nodes.slice().sort((a, b) => a.r - b.r);
      const nb = new ArrayBuffer(Math.max(1, nNodes) * NODE_STRIDE),
        nf = new Float32Array(nb),
        nu = new Uint8Array(nb);
      order.forEach((n, i) => {
        const o = i * 4,
          rgb = T.hex(n.color),
          b = i * NODE_STRIDE;
        nf[o] = n.r;
        nf[o + 1] = n.gi;
        nu[b + 8] = rgb[0];
        nu[b + 9] = rgb[1];
        nu[b + 10] = rgb[2];
        nu[b + 11] = 255;
        nu[b + 12] = STATUS_CODE[n.status] || 0;
        nu[b + 13] = n.dangerous ? 1 : 0;
        nu[b + 14] = n.core ? 1 : 0;
        nu[b + 15] = n.shape || 0;
      });
      const eb = new ArrayBuffer(Math.max(1, nEdges) * EDGE_STRIDE),
        ef = new Float32Array(eb),
        eu = new Uint8Array(eb);
      links.forEach((l, i) => {
        const o = i * 5,
          b = i * EDGE_STRIDE,
          A = l.a,
          B = l.b,
          ca = T.hex(A.color),
          cb = T.hex(B.color);
        ef[o] = A.gi;
        ef[o + 1] = B.gi;
        eu[b + 8] = ca[0];
        eu[b + 9] = ca[1];
        eu[b + 10] = ca[2];
        eu[b + 11] = 255;
        eu[b + 12] = cb[0];
        eu[b + 13] = cb[1];
        eu[b + 14] = cb[2];
        eu[b + 15] = 255;
        eu[b + 16] = TY_CODE[l.rel] || 0;
      });
      if (nodeBuf) gl.deleteBuffer(nodeBuf);
      if (edgeBuf) gl.deleteBuffer(edgeBuf);
      nodeBuf = P.buffer(gl, gl.ARRAY_BUFFER, nb);
      edgeBuf = P.buffer(gl, gl.ARRAY_BUFFER, eb);
      gl.bindVertexArray(nodeVao);
      gl.bindBuffer(gl.ARRAY_BUFFER, quad);
      attr(0, 2, gl.FLOAT, false, 0, 0, 0);
      gl.bindBuffer(gl.ARRAY_BUFFER, nodeBuf);
      attr(1, 2, gl.FLOAT, false, NODE_STRIDE, 0, 1);
      attr(2, 4, gl.UNSIGNED_BYTE, true, NODE_STRIDE, 8, 1);
      attr(3, 4, gl.UNSIGNED_BYTE, false, NODE_STRIDE, 12, 1);
      gl.bindVertexArray(edgeVao);
      gl.bindBuffer(gl.ARRAY_BUFFER, edgeBuf);
      attr(0, 2, gl.FLOAT, false, EDGE_STRIDE, 0, 1);
      attr(1, 4, gl.UNSIGNED_BYTE, true, EDGE_STRIDE, 8, 1);
      attr(2, 4, gl.UNSIGNED_BYTE, true, EDGE_STRIDE, 12, 1);
      attr(3, 4, gl.UNSIGNED_BYTE, false, EDGE_STRIDE, 16, 1);
      gl.bindVertexArray(null);
      allocState(nNodes);
      pushState(nodes);
      pushPos(nodes);
    };
    function pushState(nodes) {
      const d = stateData;
      for (let i = 0; i < nodes.length; i++) {
        const n = nodes[i],
          o = n.gi * 4;
        d[o] = n.hl;
        d[o + 1] = n.sc;
        d[o + 2] = n.flashAt / 1000;
        d[o + 3] = n.topic;
      }
      gl.bindTexture(gl.TEXTURE_2D, tex);
      gl.texSubImage2D(
        gl.TEXTURE_2D,
        0,
        0,
        0,
        P.STATE_W,
        rows,
        gl.RGBA,
        gl.FLOAT,
        d,
      );
    }
    const allocTs = () => {
      gl.bindTexture(gl.TEXTURE_2D, tsTex);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      gl.texImage2D(
        gl.TEXTURE_2D,
        0,
        gl.RGBA32F,
        P.TS_W,
        2,
        0,
        gl.RGBA,
        gl.FLOAT,
        tsData,
      );
    };
    allocTs();
    const setTopics = (topics, bridges) => {
      terr.setTopics(topics);
      brg.set(bridges);
    };
    const setHulls = (topics, bridges) => {
      terr.setHulls(topics, false);
      brg.update(bridges);
    };
    const pushTopicState = (data) => {
      tsData = data;
      gl.bindTexture(gl.TEXTURE_2D, tsTex);
      gl.texSubImage2D(
        gl.TEXTURE_2D,
        0,
        0,
        0,
        P.TS_W,
        2,
        gl.RGBA,
        gl.FLOAT,
        data,
      );
    };
    const common = (prog, v) => {
      const L = prog.loc;
      gl.useProgram(prog.p);
      gl.uniform4f(L.u_view, v.cam.x, v.cam.y, v.cam.k, 0);
      gl.uniform2f(L.u_res, v.W, v.H);
      gl.uniform1f(L.u_now, v.now / 1000);
      gl.uniform1f(L.u_reduced, v.reduced ? 1 : 0);
      gl.uniform1f(L.u_ds, v.ds);
      gl.uniform1i(L.u_state, 0);
      gl.uniform1i(L.u_ts, 1);
      gl.uniform1i(L.u_pos, 2);
      gl.uniform1f(L.u_cross, v.cross);
    };
    const draw = (v) => {
      gl.viewport(0, 0, canvas.width, canvas.height);
      gl.clearColor(bg[0], bg[1], bg[2], 1);
      gl.clear(gl.COLOR_BUFFER_BIT);
      gl.enable(gl.BLEND);
      gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
      gl.activeTexture(gl.TEXTURE1);
      gl.bindTexture(gl.TEXTURE_2D, tsTex);
      gl.activeTexture(gl.TEXTURE2);
      gl.bindTexture(gl.TEXTURE_2D, posTex);
      gl.activeTexture(gl.TEXTURE0);
      gl.bindTexture(gl.TEXTURE_2D, tex);
      terr.backdrop(v);
      terr.fills(v, common);
      brg.draw(v, common);
      terr.contours(v, common);
      if (nEdges) {
        common(edgeProg, v);
        const L = edgeProg.loc,
          k = T.unit(C.link);
        gl.uniform1f(L.u_focus, v.focus);
        gl.uniform1f(L.u_focusHl, v.focusHl);
        gl.uniform3f(L.u_link, k[0], k[1], k[2]);
        gl.uniform1f(L.u_linkA, 0.4);
        gl.bindVertexArray(edgeVao);
        gl.drawArraysInstanced(gl.TRIANGLE_STRIP, 0, EDGE_VERTS, nEdges);
      }
      if (nNodes) {
        common(nodeProg, v);
        const L = nodeProg.loc,
          set = (name, h) => {
            const u = T.unit(h);
            gl.uniform3f(L[name], u[0], u[1], u[2]);
          };
        set("u_bg", C.canvas);
        set("u_live", C.live);
        set("u_danger", C.danger);
        set("u_bright", C.bright);
        set("u_m2", C.m2);
        gl.uniform1f(L.u_sel, v.sel);
        gl.bindVertexArray(nodeVao);
        gl.drawArraysInstanced(gl.TRIANGLE_STRIP, 0, 4, nNodes);
      }
      gl.bindVertexArray(null);
    };
    const sync = () => {
      const px = new Uint8Array(4);
      gl.readPixels(0, 0, 1, 1, gl.RGBA, gl.UNSIGNED_BYTE, px);
    };
    return {
      name: "webgl2",
      setData,
      setTopics,
      setHulls,
      pushTopicState,
      pushState,
      pushPos,
      draw,
      sync,
    };
  };

  const create = (opts) => {
    const host = opts.host;
    let cv = opts.canvas;
    const ov = opts.overlay;
    const cam = opts.cam;
    const overlay = G.overlay.create(ov, host, () => invalidate());
    const FRAMES = { n: 0, sum: 0, max: 0, on: false, ms: [], iv: [] };
    const live = G.live.create();
    const clamp = (v, a, b) => (v < a ? a : v > b ? b : v);
    const S = {
      raf: 0,
      frames: 0,
      dpr: 1,
      scale: 1,
      W: 0,
      H: 0,
      drawMs: 0,
      flashUntil: 0,
      key: null,
      nodes: [],
      links: [],
      topics: [],
      bridges: [],
      tint: [],
      tcur: new Float32Array(0),
      ttgt: new Float32Array(0),
      tsData: new Float32Array(G.gl.TS_W * 8),
      cross: 0,
      bridgeA: 1,
      levels: null,
      tgtHl: new Float32Array(0),
      tgtSc: new Float32Array(0),
      dirty: true,
      stateDirty: true,
      overlayOn: true,
      lost: false,
      slow: [],
      slowAt: 0,
      labels: 0,
      bench: false,
      pin: null,
      breath: new Float32Array(0),
      simT: 0,
      posVer: 0,
      lastAt: 0,
    };
    let backend = null;

    const freshCanvas = () => {
      const nc = document.createElement("canvas");
      nc.id = cv.id;
      nc.setAttribute("aria-hidden", "true");
      cv.replaceWith(nc);
      cv = nc;
    };
    const makeBackend = () => {
      const gl = G.gl.context(cv);
      if (gl) {
        try {
          return glBackend(cv, gl);
        } catch (e) {
          console.warn(
            "mem: webgl2 failed, using the 2d fallback: " + e.message,
          );
          freshCanvas();
        }
      }
      return G.lite.create(cv);
    };
    const bind = () => {
      backend = makeBackend();
      cv.addEventListener("webglcontextlost", onLost);
      cv.addEventListener("webglcontextrestored", onRestored);
    };
    let lostTimer = 0;
    const onLost = (e) => {
      e.preventDefault();
      S.lost = true;
      overlay.ctx.setTransform(1, 0, 0, 1, 0, 0);
      overlay.ctx.clearRect(0, 0, ov.width, ov.height);
      clearTimeout(lostTimer);
      lostTimer = setTimeout(() => {
        if (S.lost) fallback();
      }, 3000);
    };
    const onRestored = () => {
      clearTimeout(lostTimer);
      S.lost = false;
      try {
        backend = glBackend(cv, G.gl.context(cv));
        backend.setTopics(S.topics, S.bridges);
        backend.setData(S.nodes, S.links);
        S.stateDirty = true;
        measure();
        invalidate();
      } catch (e) {
        fallback();
      }
    };
    const fallback = () => {
      freshCanvas();
      backend = G.lite.create(cv);
      S.lost = false;
      backend.setTopics(S.topics, S.bridges);
      backend.setData(S.nodes, S.links);
      S.stateDirty = true;
      measure();
      invalidate();
    };

    const measure = () => {
      const r = ov.getBoundingClientRect();
      if (!r.width) return null;
      S.W = r.width;
      S.H = r.height;
      S.dpr = Math.min(2, window.devicePixelRatio || 1);
      const w = Math.max(1, Math.round(S.W * S.dpr * S.scale)),
        h = Math.max(1, Math.round(S.H * S.dpr * S.scale));
      obCache = null;
      if (cv.width !== w || cv.height !== h) {
        cv.width = w;
        cv.height = h;
      }
      const ow = Math.max(1, Math.round(S.W * S.dpr)),
        oh = Math.max(1, Math.round(S.H * S.dpr));
      if (ov.width !== ow || ov.height !== oh) {
        ov.width = ow;
        ov.height = oh;
      }
      return { W: S.W, H: S.H };
    };

    let obCache = null,
      obAt = 0,
      obKey = "";
    const obstacles = (key) => {
      const t = performance.now();
      if (obCache && key === obKey && t - obAt < 250) return obCache;
      obKey = key;
      obAt = t;
      const r0 = ov.getBoundingClientRect(),
        out = [];
      for (const el of document.querySelectorAll(
        ".legend,.zoom,.gchip,.stale,.tip",
      )) {
        const r = el.getBoundingClientRect();
        if (r.width > 0 && r.height > 0)
          out.push({
            x: r.left - r0.left,
            y: r.top - r0.top,
            w: r.width,
            h: r.height,
          });
      }
      obCache = out;
      return out;
    };
    const invalidate = () => {
      if (S.bench || S.raf) return;
      S.raf = requestAnimationFrame(frame);
    };
    const watchDpr = () => {
      if (typeof matchMedia !== "function") return;
      const mq = matchMedia(
        "(resolution: " + (window.devicePixelRatio || 1) + "dppx)",
      );
      if (!mq || !mq.addEventListener) return;
      mq.addEventListener(
        "change",
        () => {
          measure();
          S.stateDirty = true;
          invalidate();
          watchDpr();
        },
        { once: true },
      );
    };
    watchDpr();
    const retarget = (F) => {
      const nodes = S.nodes;
      if (S.tgtHl.length !== nodes.length) {
        S.tgtHl = new Float32Array(nodes.length);
        S.tgtSc = new Float32Array(nodes.length);
      }
      for (let i = 0; i < nodes.length; i++) {
        const n = nodes[i];
        S.tgtHl[i] = n.hidden
          ? 0
          : !F
            ? 1
            : F.lv
              ? F.lv.get(n) || 0.06
              : F.set.has(n)
                ? 1
                : 0.09;
        S.tgtSc[i] = n.hidden ? 0 : 1;
      }
      const tn = S.topics.length,
        vis = new Array(tn).fill(0),
        touched = new Set();
      for (const n of nodes) if (!n.hidden) vis[n.topic]++;
      if (F) for (const m of F.lv ? F.lv.keys() : F.set) touched.add(m.topic);
      for (let i = 0; i < tn; i++) {
        S.ttgt[i * 2] = !vis[i] ? 0.12 : !F || touched.has(i) ? 1 : 0.38;
        S.ttgt[i * 2 + 1] = F && F.topic === i ? 1 : 0;
      }
    };
    const ease = (dt, reduced) => {
      const nodes = S.nodes,
        th = S.tgtHl,
        ts = S.tgtSc;
      const eh = reduced ? 1 : 1 - Math.exp(-dt / 130),
        es = reduced ? 1 : 1 - Math.exp(-dt / 220);
      let moving = false;
      for (let i = 0; i < nodes.length; i++) {
        const n = nodes[i];
        let d = th[i] - n.hl;
        if (d !== 0) {
          if (Math.abs(d) < 0.002) n.hl = th[i];
          else {
            n.hl += d * eh;
            moving = true;
          }
        }
        d = ts[i] - n.sc;
        if (d !== 0) {
          if (Math.abs(d) < 0.004) n.sc = ts[i];
          else {
            n.sc += d * es;
            moving = true;
          }
        }
      }
      const tc = S.tcur,
        tt = S.ttgt;
      for (let i = 0; i < tc.length; i++) {
        const d = tt[i] - tc[i];
        if (d !== 0) {
          if (Math.abs(d) < 0.004) tc[i] = tt[i];
          else {
            tc[i] += d * eh;
            moving = true;
          }
        }
      }
      return moving;
    };
    const topicState = (camera, now) => {
      const d = S.tsData,
        k = camera.k,
        W = G.gl.TS_W,
        n = Math.min(S.topics.length, W);
      overlay.recall.breath(now, S.breath);
      for (let i = 0; i < n; i++) {
        const t = S.topics[i],
          lv = G.terrain.levels(t.R * k, k),
          o = i * 4,
          o2 = (W + i) * 4,
          c = S.tint[i] || [0, 0, 0];
        d[o] = lv.full;
        d[o + 1] = lv.edge;
        d[o + 2] = S.tcur[i * 2];
        d[o + 3] = S.tcur[i * 2 + 1];
        d[o2] = c[0];
        d[o2 + 1] = c[1];
        d[o2 + 2] = c[2];
        d[o2 + 3] = S.breath[i] || 0;
      }
      S.cross = G.terrain.levels(0, k).cross;
      let l0 = 0,
        l1 = 0,
        big = 0;
      for (let i = 0; i < n; i++) {
        const Rs = S.topics[i].R * k;
        if (Rs < 70) l0++;
        else if (Rs < 260) l1++;
        else big++;
      }
      S.levels = {
        l0,
        l1,
        l2: k >= 0.97 ? 1 : 0,
        l3: k >= 3 ? 1 : 0,
        rs260: big,
      };
      return d;
    };
    const view = (now, camera, F) => {
      const fn = F && F.node ? F.node : null,
        sel = host.selected();
      return {
        cam: camera,
        W: S.W,
        H: S.H,
        now,
        reduced: host.reduced(),
        ds: S.dpr * S.scale,
        focus: fn ? fn.gi : -1,
        focusHl: fn ? fn.hl : 0,
        sel: sel ? sel.gi : -1,
        cross: S.cross,
        bridgeA: (1 - G.terrain.smooth(0.75, 1.15, cam.k)) * (F ? 0.45 : 1),
      };
    };
    const noteFlash = (until) => {
      if (until > S.flashUntil) S.flashUntil = until;
      S.stateDirty = true;
      invalidate();
    };

    const adapt = (now, interval) => {
      if (interval > 250) return;
      S.slow.push(interval);
      if (now - S.slowAt < 2000 || S.slow.length < 30) return;
      const s = S.slow.slice().sort((a, b) => a - b),
        p95 = s[Math.floor(0.95 * (s.length - 1))];
      S.slow = [];
      S.slowAt = now;
      if (p95 > 20 && S.scale > 0.8) {
        S.scale = 0.75;
        measure();
      } else if (p95 < 12 && S.scale < 1) {
        S.scale = 1;
        measure();
      }
    };
    const refit = () => {
      if (!G.terrain.fitAll(S.topics)) return;
      G.bridges.place(S.bridges, S.topics);
      backend.setHulls(S.topics, S.bridges);
    };
    const stepLive = (now, reduced) => {
      if (S.pin != null) return false;
      const wander = !reduced && backend.name === "webgl2";
      if (!wander && !live.awake()) return false;
      const dt = S.simT ? now - S.simT : 16;
      S.simT = now;
      live.setAmp(
        live.tune.wander * (1 + 0.9 * clamp((1 - cam.k) / 0.8, 0, 1)),
      );
      if (live.step(now, dt, { wander })) {
        S.posVer++;
        backend.pushPos(S.nodes);
        refit();
      }
      return wander || live.awake();
    };
    let prevAnimated = 0;
    function frame(now) {
      S.raf = 0;
      if (S.pin != null) now = S.pin;
      if (S.bench || !host.visible()) return;
      const t0 = performance.now();
      S.frames++;
      const dt = prevAnimated ? Math.min(64, now - prevAnimated) : 16;
      const reduced = host.reduced();
      if (S.lost) return;
      let animating = cam.step(now);
      if (stepLive(now, reduced)) animating = true;
      const key = host.focusKey();
      const F = host.focus();
      if (key !== S.key) {
        S.key = key;
        retarget(F);
        S.dirty = true;
      }
      if (S.dirty) {
        S.stateDirty = true;
        if (ease(dt, reduced)) animating = true;
        else S.dirty = false;
      }
      if (now < S.flashUntil) {
        if (!reduced) animating = true;
        else if (!S.flashT)
          S.flashT = setTimeout(
            () => {
              S.flashT = 0;
              S.stateDirty = true;
              invalidate();
            },
            S.flashUntil - now + 20,
          );
      }
      if (S.stateDirty) {
        backend.pushState(S.nodes);
        S.stateDirty = false;
      }
      backend.pushTopicState(topicState(cam, now));
      backend.draw(view(now, cam, F));
      if (
        S.overlayOn &&
        overlay.draw(now, cam, S.W, S.H, S.dpr, F, S.tcur, obstacles(key))
      )
        animating = true;
      S.labels = overlay.labels();
      const ms = performance.now() - t0;
      S.drawMs = ms;
      if (FRAMES.on) {
        FRAMES.n++;
        FRAMES.sum += ms;
        if (ms > FRAMES.max) FRAMES.max = ms;
        if (FRAMES.ms.length < 20000) FRAMES.ms.push(ms);
        if (S.lastAt && now - S.lastAt < 250 && FRAMES.iv.length < 20000)
          FRAMES.iv.push(now - S.lastAt);
      }
      S.lastAt = now;
      if (S.pin != null) animating = false;
      if (animating) {
        if (prevAnimated) adapt(now, now - prevAnimated);
        prevAnimated = now;
        invalidate();
      } else {
        prevAnimated = 0;
        S.simT = 0;
      }
    }
    const setTopics = (topics) => {
      S.topics = topics;
      S.tint = topics.map((t) => G.terrain.tintOf(t.members));
      S.breath = new Float32Array(topics.length);
      S.tcur = new Float32Array(topics.length * 2).fill(1);
      for (let i = 0; i < topics.length; i++) S.tcur[i * 2 + 1] = 0;
      S.ttgt = S.tcur.slice();
    };
    const setData = (nodes, links) => {
      S.nodes = nodes;
      S.links = links;
      G.terrain.fitAll(S.topics);
      S.bridges = G.bridges.compute(S.topics, links);
      backend.setTopics(S.topics, S.bridges);
      backend.setData(nodes, links);
      live.setGraph(nodes, links);
      S.posVer++;
      overlay.recall.setGraph(nodes, links);
      S.key = null;
      S.dirty = true;
      S.stateDirty = true;
      overlay.reset();
      invalidate();
    };
    const state = () => {
      S.stateDirty = true;
      invalidate();
    };

    const percentile = (a, p) => {
      const s = a.slice().sort((x, y) => x - y);
      return +s[Math.floor(p * (s.length - 1))].toFixed(2);
    };
    const memBench = async (o = {}) => {
      if (backend.name !== "webgl2")
        return { error: "no webgl2", backend: backend.name };
      const tile = o.tile || 2,
        frames = o.frames || 240,
        saved = { nodes: S.nodes, links: S.links };
      const base = saved.nodes;
      if (!base.length) return { error: "empty" };
      let x0 = Infinity,
        y0 = Infinity,
        x1 = -Infinity,
        y1 = -Infinity;
      for (const n of base) {
        x0 = Math.min(x0, n.x);
        y0 = Math.min(y0, n.y);
        x1 = Math.max(x1, n.x);
        y1 = Math.max(y1, n.y);
      }
      const w = x1 - x0 + 160,
        h = y1 - y0 + 160;
      const nodes = [],
        links = [];
      for (let ty = 0; ty < tile; ty++) {
        for (let tx = 0; tx < tile; tx++) {
          const map = new Map();
          base.forEach((n) => {
            const c = {
              x: n.x + tx * w,
              y: n.y + ty * h,
              r: n.r,
              color: n.color,
              status: n.status,
              core: n.core,
              dangerous: n.dangerous,
              shape: n.shape,
              topic: n.topic,
              hl: 1,
              sc: 1,
              flashAt: -1e9,
              hidden: false,
            };
            map.set(n, c);
            nodes.push(c);
          });
          for (const l of saved.links) {
            const a = map.get(l.a),
              b = map.get(l.b);
            if (a && b) links.push({ a, b, rel: l.rel });
          }
        }
      }
      S.bench = true;
      if (S.raf) {
        cancelAnimationFrame(S.raf);
        S.raf = 0;
      }
      backend.setData(nodes, links);
      const bcam = G.camera.create();
      bcam.W = S.W;
      bcam.H = S.H;
      const cx = (x1 - x0 + w * (tile - 1)) / 2 + x0,
        cy = (y1 - y0 + h * (tile - 1)) / 2 + y0;
      const k0 = Math.min(S.W / (w * tile), S.H / (h * tile)) * 0.95;
      const intervals = [],
        js = [];
      let last = 0,
        f = 0;
      await new Promise((done) => {
        const loop = (now) => {
          if (last) intervals.push(now - last);
          last = now;
          const t0 = performance.now(),
            ph = (f / frames) * Math.PI * 2;
          const k = k0 * Math.pow(2, 2.2 * (Math.sin(ph) * 0.5 + 0.5));
          bcam.k = k;
          bcam.x = S.W / 2 - (cx + (Math.cos(ph) * w * 0.15 * k0) / k) * k;
          bcam.y = S.H / 2 - cy * k;
          backend.draw({
            cam: bcam,
            W: S.W,
            H: S.H,
            now,
            reduced: false,
            ds: S.dpr * S.scale,
            focus: -1,
            focusHl: 0,
            sel: -1,
            cross: S.cross,
            bridgeA: 0,
          });
          js.push(performance.now() - t0);
          f++;
          if (f < frames) requestAnimationFrame(loop);
          else done();
        };
        requestAnimationFrame(loop);
      });
      const gpu = [];
      for (let i = 0; i < 60; i++) {
        const t0 = performance.now();
        backend.draw({
          cam: bcam,
          W: S.W,
          H: S.H,
          now: 0,
          reduced: false,
          ds: S.dpr * S.scale,
          focus: -1,
          focusHl: 0,
          sel: -1,
          cross: S.cross,
          bridgeA: 0,
        });
        backend.sync();
        gpu.push(performance.now() - t0);
      }
      S.bench = false;
      backend.setData(saved.nodes, saved.links);
      S.stateDirty = true;
      S.key = null;
      invalidate();
      const iv = intervals.slice(5),
        jw = js.slice(5);
      return {
        nodes: nodes.length,
        edges: links.length,
        frames,
        interval: {
          p50: percentile(iv, 0.5),
          p95: percentile(iv, 0.95),
          p99: percentile(iv, 0.99),
        },
        js: {
          p50: percentile(jw, 0.5),
          p95: percentile(jw, 0.95),
          mean: +(jw.reduce((a, b) => a + b, 0) / jw.length).toFixed(3),
        },
        synced: { p50: percentile(gpu, 0.5), p95: percentile(gpu, 0.95) },
      };
    };

    const pc = (a, p) =>
      a.length
        ? +a
            .slice()
            .sort((x, y) => x - y)
            [Math.floor(p * (a.length - 1))].toFixed(2)
        : 0;
    const frameReport = (reset) => {
      const r = {
        n: FRAMES.n,
        js: {
          p50: pc(FRAMES.ms, 0.5),
          p95: pc(FRAMES.ms, 0.95),
          max: +FRAMES.max.toFixed(2),
        },
        interval: {
          p50: pc(FRAMES.iv, 0.5),
          p95: pc(FRAMES.iv, 0.95),
          n: FRAMES.iv.length,
        },
      };
      if (reset) {
        FRAMES.n = 0;
        FRAMES.sum = 0;
        FRAMES.max = 0;
        FRAMES.ms.length = 0;
        FRAMES.iv.length = 0;
        S.lastAt = 0;
      }
      return r;
    };
    const grab = (n, x, y) => {
      if (!live.pin(n, x, y)) return false;
      invalidate();
      return true;
    };
    const drop = () => {
      live.release();
      invalidate();
    };

    bind();
    const api = {
      setTopics,
      setData,
      state,
      invalidate,
      measure,
      noteFlash,
      resetText: overlay.resetText,
      memBench,
      frameReport,
      grab,
      drop,
      live,
      posVer: () => S.posVer,
      awake: () => live.awake(),
      get backend() {
        return backend.name;
      },
      set overlayOn(v) {
        S.overlayOn = v;
        invalidate();
      },
      stats: () => ({
        backend: backend.name,
        levels: S.levels,
        topics: S.topics.length,
        bridges: S.bridges.length,
        nodes: S.nodes.length,
        edges: S.links.length,
        frames: S.frames,
        labels: S.labels,
        drawMs: +S.drawMs.toFixed(3),
        dpr: S.dpr,
        scale: S.scale,
        W: S.W,
        H: S.H,
        lost: S.lost,
        canvas: [cv.width, cv.height],
      }),
      frames: FRAMES,
      fireRecall: (n, seeds, label) => {
        const w = overlay.recall.fire(n, seeds, label);
        invalidate();
        return !!w;
      },
      clearRecall: () => {
        overlay.recall.clear();
        invalidate();
      },
      sonar: (n) => {
        overlay.sonar(n);
        invalidate();
      },
      sonarOn: () => overlay.sonarOn(),
      tickAt: (x, y) => overlay.recall.tickAt(x, y),
      recallInfo: () =>
        overlay.recall.info(S.pin != null ? S.pin : performance.now()),
      pin: (t) => {
        S.pin = t == null ? null : t;
        invalidate();
      },
      labelBench: (n = 100, items) => {
        const F = host.focus(),
          full = [],
          place = [];
        overlay.lbl.setCap(items);
        overlay.lbl.setDead(0);
        for (let i = 0; i < n; i++) {
          overlay.lbl.draw(
            performance.now() + i * 16,
            { k: cam.k, x: cam.x + (i & 1), y: cam.y },
            S.W,
            S.H,
            F,
            S.tcur,
            obstacles(),
          );
          full.push(overlay.lbl.ms());
          place.push(overlay.lbl.placeMs());
        }
        overlay.lbl.setCap(0);
        overlay.lbl.setDead(1);
        invalidate();
        const pc = (a, p) => {
          const s = a.slice().sort((x, y) => x - y);
          return +s[Math.floor(p * (s.length - 1))].toFixed(3);
        };
        return {
          items: overlay.lbl.items(),
          count: overlay.lbl.count(),
          full: { p50: pc(full, 0.5), p95: pc(full, 0.95) },
          place: { p50: pc(place, 0.5), p95: pc(place, 0.95) },
        };
      },
      labelInfo: () => ({
        count: overlay.lbl.count(),
        ms: +overlay.lbl.ms().toFixed(3),
        items: overlay.lbl.items(),
        shown: overlay.lbl.shownIds(),
        names: overlay.lbl.names(),
      }),
    };
    return api;
  };
  G.scene = { create };
})();
