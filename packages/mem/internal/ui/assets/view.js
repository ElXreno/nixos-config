"use strict";
MemGfx.theme.apply(document.documentElement);
const cv = $("#cv"),
  ov = $("#ov");
const cam = MemGfx.camera.create(() => GFX.invalidate());
const HIT = MemGfx.hit.create();
let autoFit = false,
  booted = false;
const GFX = MemGfx.scene.create({
  canvas: cv,
  overlay: ov,
  cam,
  host: {
    reduced: () => REDUCED,
    visible: () => booted && S.tab === "graph",
    focus: focusInfo,
    selected: () => S.selected,
    turnFocus: () => S.turnFocus,
    active: () => ACTIVE,
    alinks: () => ALINKS,
    byDeg: () => byDeg,
    topics: () => TOPICS,
    arcs,
    posVer: () => GFX.posVer(),
    focusKey: () => {
      const D = S.drag;
      const f = (D && D.n) || (D && D.moved ? null : S.hover) || S.selected;
      return (
        (f ? f.id : "") +
        "|" +
        (S.turnFocus ? S.turnFocus.n : "") +
        "|" +
        (S.topicFocus == null ? "" : S.topicFocus) +
        "|" +
        GVER +
        "|" +
        (f && f === S.selected ? EGO : 1)
      );
    },
  },
});
window.__gfx = GFX;
window.__frames = GFX.frames;
window.__memBench = (o) => GFX.memBench(o);
const camTo = (cx, cy, k, dur, track) => {
  autoFit = false;
  cam.flyTo(cx, cy, k, dur, REDUCED || dur === 0, track);
  NAV.poke();
};
function resize() {
  const m = GFX.measure();
  if (!m) return false;
  cam.resize(m.W, m.H, false);
  return true;
}
new ResizeObserver(() => {
  const w0 = cam.W,
    h0 = cam.H;
  if (!resize()) return;
  if (!(w0 && h0)) return;
  if (autoFit && booted) fitView(0);
  else {
    cam.x += (cam.W - w0) / 2;
    cam.y += (cam.H - h0) / 2;
  }
  GFX.invalidate();
}).observe(ov);
{
  const lg = $("#legend");
  if (lg)
    new ResizeObserver(() => {
      if (autoFit && booted) {
        fitView(0);
        GFX.invalidate();
      }
    }).observe(lg);
}
function fitNodes(list, pad = 60, dur) {
  list = list.filter((n) => !n.hidden);
  if (!list.length) return;
  let x0 = Infinity,
    y0 = Infinity,
    x1 = -Infinity,
    y1 = -Infinity;
  for (const n of list) {
    x0 = Math.min(x0, n.x - n.r);
    y0 = Math.min(y0, n.y - n.r);
    x1 = Math.max(x1, n.x + n.r);
    y1 = Math.max(y1, n.y + n.r);
  }
  const k = Math.min(
    (cam.W - pad * 2) / Math.max(1, x1 - x0),
    (cam.H - pad * 2) / Math.max(1, y1 - y0),
  );
  camTo(
    (x0 + x1) / 2,
    (y0 + y1) / 2,
    Math.min(k, list.length === 1 ? 2.2 : 2.1),
    dur,
  );
}
function fitMap(dur, pad = 0.06) {
  const seen = new Set();
  for (const n of ACTIVE) seen.add(n.topic);
  if (!seen.size) return;
  let x0 = Infinity,
    y0 = Infinity,
    x1 = -Infinity,
    y1 = -Infinity;
  for (const i of seen) {
    const b = MemGfx.terrain.bounds(TOPICS[i]);
    x0 = Math.min(x0, b[0]);
    y0 = Math.min(y0, b[1]);
    x1 = Math.max(x1, b[2]);
    y1 = Math.max(y1, b[3]);
  }
  const lg = $("#legend"),
    lr = lg && lg.offsetParent ? lg.getBoundingClientRect() : null,
    or = ov.getBoundingClientRect();
  const res =
    lr && lr.height > 0 && or.height > 0
      ? Math.min(or.height * 0.18, Math.max(0, or.bottom - lr.top + 4))
      : 0;
  const k = Math.min(
    cam.W / Math.max(1, (x1 - x0) * (1 + 2 * pad)),
    (cam.H - res) / Math.max(1, (y1 - y0) * (1 + 2 * pad)),
  );
  camTo((x0 + x1) / 2, (y0 + y1) / 2 + res / 2 / k, k, dur);
}
const fitView = (dur, pad) => {
  const filtered =
    FILTER.hideType.size ||
    FILTER.hideStatus.size ||
    FILTER.projects.size ||
    FILTER.hosts.size;
  if (filtered) fitNodes(ACTIVE, 40, dur);
  else fitMap(dur, pad);
  autoFit = true;
};
function zoomAt(sx, sy, f) {
  autoFit = false;
  cam.zoomAt(sx, sy, f);
}
let hitVer = -1;
function pushGraph() {
  GFX.setTopics(TOPICS);
  GFX.setData(NODES, LINKS);
  hitVer = -1;
}

const tip = $("#tip");
function hit(px, py) {
  const v = GFX.posVer();
  if (hitVer !== v) {
    hitVer = v;
    HIT.build(NODES);
  }
  const w = cam.toWorld(px, py);
  return HIT.query(w.x, w.y, cam.k);
}
const ptr = (e) => {
  const r = ov.getBoundingClientRect();
  return { x: e.clientX - r.left, y: e.clientY - r.top };
};
ov.addEventListener("pointerdown", (e) => {
  const p = ptr(e);
  const n = GFX.tickAt(p.x, p.y) ? null : hit(p.x, p.y);
  ov.setPointerCapture(e.pointerId);
  const w = cam.toWorld(p.x, p.y);
  S.drag = {
    n,
    x: p.x,
    y: p.y,
    moved: false,
    cx: cam.x,
    cy: cam.y,
    gx: n ? n.x - w.x : 0,
    gy: n ? n.y - w.y : 0,
  };
  cam.cancel();
  hideTip();
  ov.classList.add("drag");
  GFX.invalidate();
});
ov.addEventListener("pointermove", (e) => {
  const p = ptr(e);
  const D = S.drag;
  if (D) {
    if (!D.moved && Math.hypot(p.x - D.x, p.y - D.y) > 3) {
      D.moved = true;
      if (!D.n) autoFit = false;
    }
    if (D.moved) {
      if (D.n) {
        const w = cam.toWorld(p.x, p.y);
        GFX.grab(D.n, w.x + D.gx, w.y + D.gy);
      } else {
        cam.x = D.cx + p.x - D.x;
        cam.y = D.cy + p.y - D.y;
        GFX.invalidate();
      }
    }
    return;
  }
  const tk = GFX.tickAt(p.x, p.y),
    n = tk ? null : hit(p.x, p.y);
  if (n !== S.hover) {
    S.hover = n;
    GFX.invalidate();
  }
  ov.classList.toggle("pt", !!n || !!tk);
  if (n) showTip(n, p);
  else if (tk) showTickTip(tk, p);
  else {
    const t = topicAt(p.x, p.y);
    if (t) showTopicTip(t, p);
    else hideTip();
  }
});
function endDrag(e) {
  const D = S.drag;
  if (!D) return;
  S.drag = null;
  ov.classList.remove("drag");
  GFX.invalidate();
  if (D.moved && e && e.type === "pointerup") {
    const p = ptr(e);
    S.hover = D.n ? null : hit(p.x, p.y);
    ov.classList.toggle("pt", !!S.hover);
  }
  if (D.n) {
    if (D.moved) GFX.drop();
    else select(D.n);
  } else if (!D.moved) {
    const tk = GFX.tickAt(D.x, D.y);
    if (tk) {
      hideTip();
      fitNodes(tk.nodes, 120);
    } else clearFocus();
  }
}
ov.addEventListener("pointerup", (e) => {
  endDrag(e);
  NAV.poke();
});
ov.addEventListener("pointercancel", (e) => {
  endDrag(e);
  NAV.poke();
});
ov.addEventListener("pointerleave", () => {
  if (!S.drag) {
    S.hover = null;
    hideTip();
    GFX.invalidate();
  }
});
ov.addEventListener(
  "wheel",
  (e) => {
    e.preventDefault();
    const p = ptr(e);
    zoomAt(p.x, p.y, Math.exp(-e.deltaY * (e.deltaMode ? 0.05 : 0.0017)));
    NAV.poke();
  },
  { passive: false },
);
ov.addEventListener("dblclick", (e) => {
  const p = ptr(e);
  const n = hit(p.x, p.y);
  if (n) camTo(n.x, n.y, Math.max(cam.k, 2.4), undefined, n);
});
function topicAt(px, py) {
  const w = cam.toWorld(px, py);
  let best = null,
    bd = Infinity;
  for (const t of TOPICS) {
    if (!(t.R > 0) || !t.members.length) continue;
    const dx = w.x - t.ax,
      dy = w.y - t.ay,
      d = Math.hypot(dx, dy),
      r = MemGfx.terrain.radiusAt(t, Math.atan2(dy, dx));
    if (d <= r && d / r < bd) {
      bd = d / r;
      best = t;
    }
  }
  return best;
}
function placeTip(p) {
  tip.style.display = "block";
  const tw = tip.offsetWidth,
    th = tip.offsetHeight;
  let x = p.x + 16,
    y = p.y + 16;
  if (x + tw > cam.W - 8) x = p.x - tw - 16;
  if (y + th > cam.H - 8) y = p.y - th - 16;
  tip.style.left = x + "px";
  tip.style.top = y + "px";
}
function showTickTip(tk, p) {
  tip.innerHTML = `<div class="k">${esc(tk.label)}</div><div class="m"><span>${tk.n} ${plural(tk.n, ["запись", "записи", "записей"])} вне экрана</span><span>клик: перелететь</span></div>`;
  placeTip(p);
}
function showTopicTip(t, p) {
  tip.innerHTML = `<div class="k">${esc(t.label)}</div><div class="m"><span>${t.size} ${plural(t.size, ["запись", "записи", "записей"])}</span><span>${esc(t.id)}</span></div>`;
  placeTip(p);
}
function showTip(n, p) {
  const t = TOPICS[n.topic];
  tip.innerHTML = `<div class="k">${esc(n.key)}</div><div class="s">${esc(n.summary.replace(/`/g, ""))}</div><div class="m"><span>${TYPES[n.type].one}</span><span>${esc(t.label)}</span><span>${ageStr(n.age)}</span><span>${n.deg} ${plural(n.deg, ["связь", "связи", "связей"])}</span>${n.core ? "<span>core</span>" : ""}</div>`;
  placeTip(p);
}
function hideTip() {
  tip.style.display = "none";
}
$("#zIn").onclick = () => {
  zoomAt(cam.W / 2, cam.H / 2, 1.4);
  NAV.poke();
};
$("#zOut").onclick = () => {
  zoomAt(cam.W / 2, cam.H / 2, 1 / 1.4);
  NAV.poke();
};
$("#zFit").onclick = () => {
  clearFocus();
  fitView();
};
