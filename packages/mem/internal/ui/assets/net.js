"use strict";
(() => {
  const G = (window.MemGfx = window.MemGfx || {});
  const OPEN_MS = 8000,
    RETRY_MS = 20000,
    HIDDEN_MS = 30000,
    MAX_ERRORS = 2;
  const byKey = (a, b) => (a.k < b.k ? -1 : a.k > b.k ? 1 : 0);
  const applyDiff = (old, d) => {
    if (!old || !d || !Array.isArray(d.topics) || !Array.isArray(old.topics))
      throw new Error("diff shape");
    const oldId = old.topics.map((t) => t.id),
      keep = new Map(old.topics.map((t) => [t.id, t]));
    const at = new Map();
    const topics = d.topics.map((t, i) => {
      at.set(t.id, i);
      if (!t.same) return t;
      const prev = keep.get(t.id);
      if (!prev) throw new Error("diff topic " + t.id);
      return prev;
    });
    const gone = new Set(d.del),
      next = new Map();
    for (const n of d.add) next.set(n.k, n);
    for (const n of d.set) next.set(n.k, n);
    const records = [];
    for (const r of old.records) {
      if (gone.has(r.k) || next.has(r.k)) continue;
      const g = at.get(oldId[r.g]);
      if (g === undefined) throw new Error("diff node " + r.k);
      records.push(g === r.g ? r : Object.assign({}, r, { g }));
    }
    for (const n of next.values()) records.push(n);
    records.sort(byKey);
    return Object.assign({}, old, {
      rev: d.rev,
      built: d.built,
      links: d.links,
      rows: d.rows,
      layout: d.layout,
      topics,
      records,
    });
  };
  const parse = (s) => {
    try {
      return JSON.parse(s);
    } catch (e) {
      return null;
    }
  };
  const create = (o) => {
    const ES = o.EventSource,
      setT = o.setTimeout || setTimeout,
      clearT = o.clearTimeout || clearTimeout;
    const url = o.url || "/api/events";
    let mode = "off",
      es = null,
      opened = false,
      errors = 0,
      openT = 0,
      retryT = 0,
      hiddenT = 0,
      stopped = false;
    const set = (m) => {
      if (m === mode) return;
      mode = m;
      if (o.onMode) o.onMode(m);
    };
    const drop = () => {
      clearT(openT);
      openT = 0;
      if (es) {
        es.onopen = es.onerror = null;
        es.close();
        es = null;
      }
      opened = false;
    };
    const fail = () => {
      drop();
      if (stopped) return;
      set("poll");
      clearT(retryT);
      retryT = setT(() => {
        retryT = 0;
        if (!stopped && !es && !(o.hidden && o.hidden())) connect();
      }, RETRY_MS);
    };
    const connect = () => {
      if (stopped) return;
      drop();
      clearT(retryT);
      retryT = 0;
      errors = 0;
      const src = new ES(url);
      es = src;
      openT = setT(() => {
        if (es === src && !opened) fail();
      }, OPEN_MS);
      src.onopen = () => {
        if (es !== src) return;
        opened = true;
        errors = 0;
        clearT(openT);
        openT = 0;
        set("sse");
        if (o.onOpen) o.onOpen();
      };
      src.onerror = () => {
        if (es !== src) return;
        errors++;
        if (errors >= MAX_ERRORS || src.readyState === 2) fail();
      };
      src.addEventListener("graph", (e) => {
        const j = parse(e.data);
        if (es === src && j && typeof j.rev === "string" && j.rev && o.onGraph)
          o.onGraph(j.rev);
      });
      src.addEventListener("turn", (e) => {
        const j = parse(e.data);
        if (es === src && j && typeof j === "object" && o.onTurn)
          o.onTurn(j, e.lastEventId || "");
      });
    };
    const start = () => {
      stopped = false;
      if (!ES) {
        set("poll");
        return;
      }
      connect();
    };
    const pause = () => {
      drop();
      clearT(retryT);
      retryT = 0;
      if (mode === "sse") set("poll");
    };
    const hide = () => {
      clearT(hiddenT);
      hiddenT = setT(() => {
        hiddenT = 0;
        pause();
      }, HIDDEN_MS);
    };
    const show = () => {
      clearT(hiddenT);
      hiddenT = 0;
      if (ES && !stopped && !es) connect();
    };
    const stop = () => {
      stopped = true;
      clearT(hiddenT);
      hiddenT = 0;
      clearT(retryT);
      retryT = 0;
      drop();
      set("off");
    };
    return {
      start,
      stop,
      hide,
      show,
      live: () => mode === "sse",
      mode: () => mode,
      errors: () => errors,
    };
  };
  G.net = { create, applyDiff, OPEN_MS, RETRY_MS, HIDDEN_MS, MAX_ERRORS };
})();
