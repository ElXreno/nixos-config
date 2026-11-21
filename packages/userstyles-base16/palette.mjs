const toLinear = (v) => (v <= 0.04045 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4));
const toGamma = (v) => (v <= 0.0031308 ? v * 12.92 : 1.055 * Math.pow(v, 1 / 2.4) - 0.055);

export function hexToLab(hex) {
  const [r, g, b] = [0, 2, 4].map((k) => toLinear(parseInt(hex.replace('#', '').slice(k, k + 2), 16) / 255));
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ];
}

function labToRgb([L, A, B]) {
  const l = (L + 0.3963377774 * A + 0.2158037573 * B) ** 3;
  const m = (L - 0.1055613458 * A - 0.0638541728 * B) ** 3;
  const s = (L - 0.0894841775 * A - 1.291485548 * B) ** 3;
  return [
    toGamma(4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s),
    toGamma(-1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s),
    toGamma(-0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s),
  ];
}

const inGamut = (rgb) => rgb.every((v) => v >= -0.0005 && v <= 1.0005);

function labToHex(labColour) {
  const [L, A, B] = labColour;
  const C = Math.hypot(A, B);
  const h = Math.atan2(B, A);
  let lo = 0;
  let hi = C;
  if (!inGamut(labToRgb(labColour))) {
    for (let i = 0; i < 24; i++) {
      const mid = (lo + hi) / 2;
      if (inGamut(labToRgb([L, mid * Math.cos(h), mid * Math.sin(h)]))) lo = mid;
      else hi = mid;
    }
  } else {
    lo = C;
  }
  return (
    '#' +
    labToRgb([L, lo * Math.cos(h), lo * Math.sin(h)])
      .map((v) => Math.round(Math.min(1, Math.max(0, v)) * 255).toString(16).padStart(2, '0'))
      .join('')
  );
}

const mix = (a, b, t) => labToHex(hexToLab(a).map((v, i) => v + (hexToLab(b)[i] - v) * t));
const shiftLightness = (hex, delta) => {
  const [L, A, B] = hexToLab(hex);
  return labToHex([Math.max(0, Math.min(1, L + delta)), A, B]);
};

export function makeSnapper(colours) {
  const targets = Object.entries(colours).map(([name, hex]) => ({ name, hex, lab: hexToLab(hex) }));
  const known = new Set(Object.values(colours).map((h) => h.toLowerCase()));
  const cache = new Map();

  return (hex) => {
    const lower = `#${hex.toLowerCase()}`;
    if (known.has(lower)) return null;
    if (cache.has(lower)) return cache.get(lower);
    const [L, A, B] = hexToLab(hex);
    let best = null;
    let bestDistance = Infinity;
    for (const target of targets) {
      const distance = Math.hypot(L - target.lab[0], (A - target.lab[1]) * 1.5, (B - target.lab[2]) * 1.5);
      if (distance < bestDistance) {
        bestDistance = distance;
        best = target;
      }
    }
    cache.set(lower, best.hex);
    return best.hex;
  };
}

export function catppuccinNames(base) {
  return {
    crust: shiftLightness(base.base00, -0.05),
    mantle: base.base01,
    base: base.base00,
    surface0: base.base02,
    surface1: mix(base.base02, base.base03, 0.45),
    surface2: mix(base.base02, base.base03, 0.8),
    overlay0: base.base03,
    overlay1: mix(base.base03, base.base04, 0.5),
    overlay2: base.base04,
    subtext0: mix(base.base04, base.base05, 0.4),
    subtext1: mix(base.base04, base.base05, 0.7),
    text: base.base05,

    red: base.base08,
    maroon: mix(base.base08, base.base0F, 0.35),
    peach: base.base09,
    yellow: base.base0A,
    green: base.base0B,
    teal: base.base0C,
    sky: mix(base.base0C, base.base0D, 0.35),
    sapphire: mix(base.base0C, base.base0D, 0.65),
    blue: base.base0D,
    lavender: mix(base.base0D, base.base0E, 0.6),
    mauve: base.base0E,
    pink: base.base0F,
    flamingo: mix(base.base0F, base.base06, 0.45),
    rosewater: mix(base.base0F, base.base06, 0.75),
  };
}
