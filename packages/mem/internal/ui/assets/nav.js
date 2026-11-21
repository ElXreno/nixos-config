"use strict";
const NAV = (() => {
  const H =
    typeof history !== "undefined" && history && history.pushState
      ? history
      : null;
  const KEYS = ["t", "n", "turn"];
  const POSES = 24,
    TICK_MS = 200,
    PAL_MAX = 8;
  let applying = 0,
    lastHash = "",
    pokeT = 0,
    still = 0,
    seen = null,
    pendingTurn = null,
    stepReplace = false;
  const stack = [];

  const parse = (h) => {
    const o = {};
    for (const part of String(h).replace(/^#/, "").split("&")) {
      const i = part.indexOf("=");
      if (i < 0) continue;
      const k = part.slice(0, i);
      if (!KEYS.includes(k) && k !== "cam" && k !== "l") continue;
      let v = part.slice(i + 1);
      try {
        v = decodeURIComponent(v);
      } catch (e) {
        v = "";
      }
      if (!(k in o)) o[k] = v;
    }
    return o;
  };
  const nav = (s) =>
    KEYS.filter((k) => s[k] != null)
      .map((k) => k + "=" + encodeURIComponent(s[k]))
      .join("&");
  const state = () => {
    const T = S.turnFocus ? TURNS.find((x) => x.n === S.turnFocus.n) : null;
    return {
      t:
        S.topicFocus != null && TOPICS[S.topicFocus]
          ? TOPICS[S.topicFocus].id
          : null,
      n: S.selected ? S.selected.key : null,
      turn: T && T.id ? T.id : null,
    };
  };
  const layoutHash = () => {
    let h = 2166136261;
    const mix = (s) => {
      for (let i = 0; i < s.length; i++) {
        h ^= s.charCodeAt(i);
        h = Math.imul(h, 16777619) >>> 0;
      }
    };
    for (const t of TOPICS)
      mix(
        t.id +
          "|" +
          Math.round(t.sx) +
          "," +
          Math.round(t.sy) +
          "," +
          Math.round(t.sR) +
          ";",
      );
    return h.toString(16).padStart(8, "0");
  };
  const pose = () => {
    const c = cam.center();
    return { x: c.x, y: c.y, k: cam.k };
  };
  const camStr = (p) =>
    p.x.toFixed(1) + "," + p.y.toFixed(1) + "," + p.k.toFixed(3);
  const same = (a, b) =>
    Math.abs(a.x - b.x) * cam.k < 0.5 &&
    Math.abs(a.y - b.y) * cam.k < 0.5 &&
    Math.abs(Math.log(a.k / b.k)) < 0.003;
  const far = (a, b) =>
    Math.hypot(a.x - b.x, a.y - b.y) * cam.k > cam.W * 0.12 ||
    Math.abs(Math.log(a.k / b.k)) > 0.2;
  const say = (text) => {
    const el = $("#sr");
    if (el) el.textContent = text;
  };

  const describe = (kind) => {
    if (kind === "node" && S.selected) {
      const n = S.selected;
      return `${n.key}. ${TYPES[n.type].one}, тема ${TOPICS[n.topic].label}. Связей: ${n.deg}.${n.core ? " Core." : ""}${n.dangerous ? " Опасная." : ""}${n.status !== "current" ? " " + (STATUSES[n.status] || n.status) + "." : ""}`;
    }
    if (kind === "topic" && S.topicFocus != null) {
      const t = TOPICS[S.topicFocus];
      return `Тема ${t.label}, ${t.size} ${plural(t.size, ["запись", "записи", "записей"])}`;
    }
    if (kind === "turn" && S.turnFocus) {
      const T = TURNS.find((x) => x.n === S.turnFocus.n);
      return T
        ? `Ход ${T.time} ${T.sid}, записей: ${T.items.filter((x) => x.node).length}`
        : "Ход";
    }
    return "Вся карта";
  };

  const writeCam = () => {
    if (!H || applying || !booted) return;
    const nv = nav(state());
    const cm = autoFit ? "" : "cam=" + camStr(pose()) + "&l=" + layoutHash();
    const hash = [nv, cm].filter(Boolean).join("&");
    if ((hash ? "#" + hash : "") === location.hash) return;
    H.replaceState(
      H.state,
      "",
      hash ? "#" + hash : location.pathname + location.search,
    );
    lastHash = location.hash;
  };
  const settled = (p) => {
    seen = null;
    const top = stack[stack.length - 1];
    if (!top || far(p, top)) stack.push(p);
    else stack[stack.length - 1] = p;
    if (stack.length > POSES) stack.shift();
    writeCam();
  };
  const tick = () => {
    pokeT = 0;
    if (!booted || document.hidden) return;
    const p = pose();
    if (seen && same(p, seen) && !cam.anim) {
      if (++still >= 2) {
        settled(p);
        return;
      }
    } else still = 0;
    seen = p;
    pokeT = setTimeout(tick, TICK_MS);
  };
  const poke = () => {
    still = 0;
    if (!pokeT) pokeT = setTimeout(tick, TICK_MS);
  };

  const step = (kind) => {
    if (applying || !booted) return;
    say(describe(kind));
    if (!H) return;
    const nv = nav(state());
    if (nv === nav(parse(location.hash))) {
      poke();
      return;
    }
    (stepReplace ? H.replaceState : H.pushState).call(
      H,
      null,
      "",
      nv ? "#" + nv : location.pathname + location.search,
    );
    lastHash = location.hash;
    poke();
  };

  const camFrom = (p) => {
    const m = p.cam ? /^(-?[\d.]+),(-?[\d.]+),([\d.]+)$/.exec(p.cam) : null;
    if (!m || (p.l != null && p.l !== layoutHash())) return null;
    const c = { x: +m[1], y: +m[2], k: +m[3] };
    return Number.isFinite(c.x) &&
      Number.isFinite(c.y) &&
      Number.isFinite(c.k) &&
      c.k > 0
      ? c
      : null;
  };
  const apply = (p, instant) => {
    const ti = p.t != null ? TOPICS.findIndex((t) => t.id === p.t) : -1;
    const n = p.n != null ? BYKEY.get(p.n) : null;
    let focused = false;
    applying++;
    try {
      if (n) {
        if (ti >= 0 && S.topicFocus !== ti)
          focusTopic(ti, { force: true, noFit: true });
        select(n);
        focused = true;
      } else if (p.turn) {
        const T = TURNS.find((x) => x.id === p.turn);
        if (T) {
          if (!(S.turnFocus && S.turnFocus.n === T.n)) focusTurn(T.n);
        } else {
          pendingTurn = { id: p.turn, cam: camFrom(p) };
          if (S.selected || S.topicFocus != null || S.turnFocus) clearFocus();
        }
        focused = true;
      } else if (ti >= 0) {
        focusTopic(ti, { force: true });
        focused = true;
      } else if (S.selected || S.topicFocus != null || S.turnFocus)
        clearFocus();
      const c = camFrom(p);
      if (c) {
        autoFit = false;
        cam.flyTo(c.x, c.y, c.k, 0, instant || REDUCED);
      } else if (n)
        camTo(n.x, n.y, Math.max(cam.k, 1.5), instant ? 0 : undefined, n);
      else if (!focused) fitView(instant ? 0 : undefined);
    } finally {
      applying--;
    }
    if (!instant)
      say(describe(n ? "node" : p.turn ? "turn" : ti >= 0 ? "topic" : "map"));
    poke();
  };
  const start = () => {
    wire();
    lastHash = location.hash;
    const p = parse(location.hash);
    if (KEYS.some((k) => p[k] != null) || p.cam) apply(p, true);
    poke();
  };
  const onHash = () => {
    if (!booted || location.hash === lastHash) return;
    lastHash = location.hash;
    apply(parse(location.hash), false);
  };
  const turns = () => {
    if (!pendingTurn) return;
    const T = TURNS.find((x) => x.id === pendingTurn.id),
      want = pendingTurn;
    pendingTurn = null;
    if (!T) return;
    applying++;
    try {
      focusTurn(T.n);
      if (want.cam) {
        autoFit = false;
        cam.flyTo(want.cam.x, want.cam.y, want.cam.k, 0, true);
      }
    } finally {
      applying--;
    }
    poke();
  };

  const back = () => {
    const cur = pose(),
      top = stack[stack.length - 1];
    let to = null;
    if (top && far(cur, top)) to = top;
    else if (stack.length >= 2) {
      stack.pop();
      to = stack[stack.length - 1];
    }
    if (!to) {
      say("Это первая поза камеры");
      return false;
    }
    autoFit = false;
    cam.flyTo(to.x, to.y, to.k, 0, REDUCED);
    say("Камера: предыдущая поза");
    poke();
    return true;
  };
  const up = () => {
    if (S.selected) {
      focusTopic(S.selected.topic, { force: true });
      return true;
    }
    if (S.turnFocus) {
      focusTurn(S.turnFocus.n);
      return true;
    }
    if (S.topicFocus != null) {
      clearFocus();
      fitView();
      return true;
    }
    return false;
  };

  const neighbour = (n, dx, dy) => {
    for (const cone of [0.5, 0]) {
      let best = null,
        bs = Infinity;
      for (const o of n.nbr) {
        if (o === n || o.hidden) continue;
        const vx = o.x - n.x,
          vy = o.y - n.y,
          d = Math.hypot(vx, vy) || 1e-6,
          c = (vx * dx + vy * dy) / d;
        if (c < cone) continue;
        const s = d * (2 - c);
        if (s < bs) {
          bs = s;
          best = o;
        }
      }
      if (best) return best;
    }
    return null;
  };
  const nearestCenter = () => {
    const c = cam.center();
    let best = null,
      bd = Infinity;
    for (const n of ACTIVE) {
      const d = (n.x - c.x) * (n.x - c.x) + (n.y - c.y) * (n.y - c.y);
      if (d < bd) {
        bd = d;
        best = n;
      }
    }
    return best;
  };
  const keepVisible = (n) => {
    const sx = n.x * cam.k + cam.x,
      sy = n.y * cam.k + cam.y;
    if (
      cam.k < 1 ||
      sx < cam.W * 0.12 ||
      sx > cam.W * 0.88 ||
      sy < cam.H * 0.12 ||
      sy > cam.H * 0.88
    ) {
      autoFit = false;
      cam.flyTo(n.x, n.y, Math.max(cam.k, 1.2), 0, REDUCED);
      poke();
    }
  };
  const arrow = (dx, dy) => {
    const cur = S.selected;
    if (!cur) {
      const n = nearestCenter();
      if (n) {
        select(n);
        keepVisible(n);
      }
      return;
    }
    const next = neighbour(cur, dx, dy);
    if (!next) {
      say(cur.deg ? "В этом направлении связей нет" : "У записи нет связей");
      return;
    }
    stepReplace = true;
    try {
      select(next);
    } finally {
      stepReplace = false;
    }
    keepVisible(next);
  };
  const cycle = (dir) => {
    const list = TOPICS.filter((t) => t.members.some((m) => !m.hidden));
    const cur =
      S.topicFocus != null
        ? list.findIndex((t) => t.i === S.topicFocus)
        : S.selected
          ? list.findIndex((t) => t.i === S.selected.topic)
          : -1;
    const next = cur < 0 ? (dir > 0 ? 0 : -1) : cur + dir;
    if (next < 0 || next >= list.length) {
      if (S.topicFocus != null || S.selected) clearFocus();
      return false;
    }
    focusTopic(list[next].i, { force: true });
    return true;
  };
  const toDetail = () => {
    D.setAttribute("tabindex", "-1");
    D.focus({ preventScroll: true });
    say("Панель записи " + S.selected.key);
  };
  const setEgo = (v) => {
    v = clamp(v, 1, 3);
    if (v === EGO) {
      say("Глубина " + EGO);
      return;
    }
    EGO = v;
    renderChip();
    GFX.invalidate();
    say("Глубина " + EGO);
  };
  const DIRS = {
    ArrowUp: [0, -1],
    ArrowDown: [0, 1],
    ArrowLeft: [-1, 0],
    ArrowRight: [1, 0],
  };
  const onKey = (e) => {
    if (e.defaultPrevented) return;
    const f = document.activeElement;
    if (/INPUT|TEXTAREA|SELECT/.test((f && f.tagName) || "")) return;
    if (e.altKey && e.key === "ArrowLeft" && !e.ctrlKey && !e.metaKey) {
      e.preventDefault();
      back();
      return;
    }
    if (S.tab !== "graph" || e.ctrlKey || e.metaKey || e.altKey) return;
    if ((e.key === "[" || e.key === "]") && S.selected) {
      e.preventDefault();
      setEgo(EGO + (e.key === "]" ? 1 : -1));
      return;
    }
    if (f !== ov) return;
    if (DIRS[e.key]) {
      e.preventDefault();
      arrow(DIRS[e.key][0], DIRS[e.key][1]);
      return;
    }
    if (e.key === "Tab") {
      if (cycle(e.shiftKey ? -1 : 1)) e.preventDefault();
      return;
    }
    if (e.key === "Enter" && S.selected) {
      e.preventDefault();
      toDetail();
    }
  };

  let palOpen = false,
    palList = [],
    palTotal = 0,
    palActive = -1;
  const gs = () => $("#gsearch");
  const palRender = () => {
    const el = gs();
    if (!el) return;
    if (!palList.length) {
      el.innerHTML = `<div class="gs-empty">${q1.value.trim() ? "Ищу..." : ""}</div>`;
      return;
    }
    el.innerHTML =
      palList
        .map(
          ({ n }, i) =>
            `<button class="gsr${i === palActive ? " on" : ""}" role="option" aria-selected="${i === palActive}" data-gi="${i}"><span class="dot" style="--c:${n.color}"></span><span class="k">${esc(n.key)}</span><span class="s">${esc(n.summary.replace(/`/g, ""))}</span></button>`,
        )
        .join("") +
      `<button class="gsr all" data-gall>Все результаты: ${palTotal}</button>`;
  };
  const palSonar = () => {
    const it = palList[palActive];
    GFX.sonar(it ? it.n : null);
  };
  const pal = {
    open() {
      const el = gs();
      if (!el || !q1.value.trim()) {
        if (palOpen) pal.close();
        return;
      }
      const r = $(".search").getBoundingClientRect();
      el.style.left = r.left + "px";
      el.style.top = r.bottom + 4 + "px";
      el.style.width = Math.max(320, r.width) + "px";
      el.hidden = false;
      if (!palOpen) {
        palOpen = true;
        palActive = -1;
        palList = [];
        palRender();
      }
    },
    close() {
      const el = gs();
      if (el) el.hidden = true;
      if (palOpen && GFX.sonarOn()) GFX.sonar(null);
      palOpen = false;
      palList = [];
      palActive = -1;
    },
    results(res) {
      if (!palOpen) return;
      if (!q1.value.trim()) {
        pal.close();
        return;
      }
      palList = res.list.filter((x) => x.n).slice(0, PAL_MAX);
      palTotal = res.list.length;
      palActive = palList.length ? 0 : -1;
      palRender();
      palSonar();
    },
    pick(list) {
      if (!palOpen || palActive < 0 || !lastSearch || lastSearch.list !== list)
        return null;
      return palList[palActive] || null;
    },
    key(e) {
      if (!palOpen) return false;
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        if (!palList.length) return true;
        e.preventDefault();
        palActive =
          (palActive + (e.key === "ArrowDown" ? 1 : -1) + palList.length) %
          palList.length;
        palRender();
        palSonar();
        const on = gs().querySelector(".on");
        if (on && on.scrollIntoView) on.scrollIntoView({ block: "nearest" });
        say(palList[palActive].n.key);
        return true;
      }
      if (e.key === "Tab") pal.close();
      return false;
    },
    go(i) {
      const it = palList[i];
      if (!it) return;
      pal.close();
      q1.blur();
      select(it.n, { reveal: true });
      ov.focus({ preventScroll: true });
    },
    active: () => palActive,
    isOpen: () => palOpen,
  };
  const wire = () => {
    const el = gs();
    if (el) {
      el.addEventListener("mousedown", (e) => e.preventDefault());
      el.addEventListener("mousemove", (e) => {
        const b = e.target.closest("[data-gi]");
        if (!b || +b.dataset.gi === palActive) return;
        palActive = +b.dataset.gi;
        palRender();
        palSonar();
      });
      el.addEventListener("click", (e) => {
        const b = e.target.closest("[data-gi],[data-gall]");
        if (!b) return;
        if (b.hasAttribute("data-gall")) {
          pal.close();
          setTab("search");
          return;
        }
        pal.go(+b.dataset.gi);
      });
    }
    q1.addEventListener("blur", () => pal.close());
    D.addEventListener("keydown", (e) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        ov.focus({ preventScroll: true });
      }
    });
    ov.setAttribute("tabindex", "0");
    ov.setAttribute("role", "application");
  };
  addEventListener("popstate", onHash);
  addEventListener("hashchange", onHash);
  document.addEventListener("keydown", onKey);

  window.__nav = {
    state,
    pose,
    stack,
    parse,
    layoutHash,
    back,
    up,
    apply,
    poke,
    say,
    key: onKey,
    palette: pal,
  };
  return { start, step, poke, up, turns, back, palette: pal, layoutHash };
})();
