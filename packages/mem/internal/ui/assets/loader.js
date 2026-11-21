"use strict";
(() => {
  const boot = document.getElementById("boot");
  const say = (text) => {
    if (boot) boot.textContent = text;
  };
  const get = (url) =>
    fetch(url, { cache: "no-store" }).then((r) =>
      r.ok ? r.json() : Promise.reject(new Error("HTTP " + r.status)),
    );
  const SCRIPTS = __SCRIPTS__;
  const load = (src) =>
    new Promise((ok, bad) => {
      const s = document.createElement("script");
      s.src = src;
      s.async = false;
      s.onload = ok;
      s.onerror = () => bad(new Error(src));
      document.body.appendChild(s);
    });
  const start = () =>
    Promise.all([get("/api/graph"), get("/api/hygiene").catch(() => null)])
      .then(([data, hyg]) => {
        if (data.layout && data.layout.state === "building") {
          say("раскладываю граф...");
          return new Promise((r) => setTimeout(r, 700)).then(start);
        }
        if (!data.records || !data.records.length) {
          say("В базе нет записей");
          return null;
        }
        window.__MEMDATA = data;
        window.__MEMHYG = hyg;
        return Promise.all(SCRIPTS.map(load));
      })
      .catch((e) => say("База недоступна: " + e.message));
  start();
})();
