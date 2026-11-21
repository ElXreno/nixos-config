"use strict";
(() => {
  const G = (window.MemGfx = window.MemGfx || {});
  const kind = {
    rule: "#4f9dfc",
    fact: "#21cec6",
    state: "#f5ae46",
    incident: "#ff8fa3",
    howto: "#8bd17c",
    log: "#7b7975",
  };
  const c = {
    canvas: "#151514",
    bg: "#1a1a19",
    panel: "#2a2927",
    line: "#393835",
    m1: "#62615d",
    m2: "#7b7975",
    text: "#cccac3",
    text2: "#9b9992",
    bright: "#d9d7d0",
    live: "#ff7c4a",
    head: "#ffd2b8",
    danger: "#ff6366",
    link: "#4a4944",
    fill: "#1b1b19",
    fillIn: "#262624",
    glow: "#8a8c86",
    contour: "#3e3d38",
    vignette: "#0f0f0e",
    aged: "#5d5a52",
  };
  const hex = (h) => {
    const n = parseInt(h.slice(1), 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  };
  const mix = (a, b, t) => {
    const A = hex(a),
      B = hex(b);
    return (
      "#" +
      A.map((v, i) =>
        Math.round(v + (B[i] - v) * t)
          .toString(16)
          .padStart(2, "0"),
      ).join("")
    );
  };
  const rgba = (h, a) => {
    const [r, g, b] = hex(h);
    return `rgba(${r},${g},${b},${a})`;
  };
  const unit = (h) => hex(h).map((v) => v / 255);
  const vars = {
    "--canvas": c.canvas,
    "--bg": c.bg,
    "--panel": c.panel,
    "--line": c.line,
    "--mute1": c.m1,
    "--mute2": c.m2,
    "--text": c.text,
    "--text2": c.text2,
    "--bright": c.bright,
    "--live": c.live,
    "--danger": c.danger,
    "--link": c.link,
    "--rule": kind.rule,
    "--fact": kind.fact,
    "--state": kind.state,
    "--incident": kind.incident,
    "--howto": kind.howto,
    "--log": kind.log,
  };
  const apply = (root) => {
    if (!root || !root.style) return;
    for (const k in vars) root.style.setProperty(k, vars[k]);
  };
  const legendItems = () => [
    [
      "опасная: core-правило",
      `<svg width="14" height="14"><circle cx="7" cy="7" r="3.5" fill="${kind.rule}"/><circle cx="7" cy="7" r="6" fill="none" stroke="${c.danger}" stroke-width="1.5"/></svg>`,
    ],
    [
      "core",
      `<svg width="14" height="14"><circle cx="7" cy="7" r="3.5" fill="${kind.fact}"/><circle cx="7" cy="7" r="6" fill="none" stroke="${c.bright}" stroke-width="1.2"/></svg>`,
    ],
    [
      "заменена или снята",
      `<svg width="14" height="14"><circle cx="7" cy="7" r="5" fill="none" stroke="${kind.fact}" stroke-width="1.4" stroke-dasharray="2.2 2"/></svg>`,
    ],
    [
      "срок истёк",
      `<svg width="14" height="14"><circle cx="7" cy="7" r="5" fill="none" stroke="${kind.state}" stroke-width="1.4" stroke-dasharray="1 2"/></svg>`,
    ],
    [
      "статус стареет к своему ttl",
      `<svg width="30" height="10"><circle cx="5" cy="5" r="4" fill="${kind.state}"/><circle cx="15" cy="5" r="4" fill="${mix(kind.state, c.aged, 0.45)}"/><circle cx="25" cy="5" r="4" fill="${mix(kind.state, c.aged, 0.9)}"/></svg>`,
    ],
    [
      "подана в ходе",
      `<svg width="14" height="14"><circle cx="7" cy="7" r="3.5" fill="${c.live}"/><circle cx="7" cy="7" r="6" fill="none" stroke="${c.live}" stroke-opacity=".5"/></svg>`,
    ],
    [
      "указатель",
      `<svg width="22" height="10"><path d="M1 5h20" stroke="${c.live}" stroke-width="1.3" stroke-dasharray="5 3"/></svg>`,
    ],
    [
      "тень, не подана",
      `<svg width="22" height="10"><path d="M1 5h20" stroke="${c.m2}" stroke-width="1.2" stroke-dasharray="1.5 3"/></svg>`,
    ],
    [
      "триггер инструмента",
      `<svg width="12" height="12"><rect x="2.5" y="2.5" width="7" height="7" fill="${c.live}"/></svg>`,
    ],
  ];
  const legendHtml = () =>
    legendItems()
      .map(([text, svg]) => `<span>${svg}${text}</span>`)
      .join("");
  G.theme = {
    kind,
    c,
    curve: 0.16,
    hex,
    mix,
    rgba,
    unit,
    vars,
    apply,
    legendHtml,
  };
})();
