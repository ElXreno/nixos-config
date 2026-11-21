import { deflateRawSync, inflateRawSync, constants } from 'node:zlib';

export const COLORS_ENTRY = 'colors.tdesktop-theme';
const LFH = Buffer.from([0x50, 0x4b, 0x03, 0x04]);
const EOCD = Buffer.from([0x50, 0x4b, 0x05, 0x06]);

const crcTable = (() => {
  const table = new Int32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
    table[n] = c;
  }
  return table;
})();

export function crc32(buf) {
  let c = -1;
  for (let i = 0; i < buf.length; i++) c = crcTable[(c ^ buf[i]) & 0xff] ^ (c >>> 8);
  return (c ^ -1) >>> 0;
}

function readCentralDirectory(blob) {
  const eocd = blob.lastIndexOf(EOCD);
  if (eocd === -1 || eocd + 22 > blob.length) return null;
  const count = blob.readUInt16LE(eocd + 10);
  let offset = blob.readUInt32LE(eocd + 16);
  const entries = [];
  for (let i = 0; i < count; i++) {
    if (offset + 46 > blob.length || blob.readUInt32LE(offset) !== 0x02014b50) return null;
    const nameLen = blob.readUInt16LE(offset + 28);
    const extraLen = blob.readUInt16LE(offset + 30);
    const commentLen = blob.readUInt16LE(offset + 32);
    entries.push({
      name: blob.subarray(offset + 46, offset + 46 + nameLen).toString('latin1'),
      method: blob.readUInt16LE(offset + 10),
      time: blob.readUInt16LE(offset + 12),
      date: blob.readUInt16LE(offset + 14),
      crc: blob.readUInt32LE(offset + 16),
      compressedSize: blob.readUInt32LE(offset + 20),
      uncompressedSize: blob.readUInt32LE(offset + 24),
      localOffset: blob.readUInt32LE(offset + 42),
    });
    offset += 46 + nameLen + extraLen + commentLen;
  }
  return entries;
}

function entryData(blob, entry) {
  const local = entry.localOffset;
  if (blob.readUInt32LE(local) !== 0x04034b50) return null;
  const nameLen = blob.readUInt16LE(local + 26);
  const extraLen = blob.readUInt16LE(local + 28);
  const start = local + 30 + nameLen + extraLen;
  return blob.subarray(start, start + entry.compressedSize);
}

export function entryBytes(blob, entry) {
  const data = entryData(blob, entry);
  if (entry.method === 0) return data;
  return inflateRawSync(data, { finishFlush: constants.Z_SYNC_FLUSH });
}

function decode(entry, data) {
  if (entry.method === 0) return data.toString('utf8');
  return inflateRawSync(data, { finishFlush: constants.Z_SYNC_FLUSH }).toString('utf8');
}

const toLinear = (v) => (v <= 0.04045 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4));
const toGamma = (v) => (v <= 0.0031308 ? v * 12.92 : 1.055 * Math.pow(v, 1 / 2.4) - 0.055);

export function hexToLab(hex) {
  const [r, g, b] = [0, 2, 4].map((k) => toLinear(parseInt(hex.slice(k, k + 2), 16) / 255));
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
  return [
    0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s,
    1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s,
    0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s,
  ];
}

export function labToRgb([L, A, B]) {
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
const toHex = (rgb) =>
  rgb.map((v) => Math.round(Math.min(1, Math.max(0, v)) * 255).toString(16).padStart(2, '0')).join('');

function lchToHex(L, C, h) {
  const at = (c) => labToRgb([L, c * Math.cos(h), c * Math.sin(h)]);
  if (inGamut(at(C))) return toHex(at(C));
  let lo = 0;
  let hi = C;
  for (let i = 0; i < 24; i++) {
    const mid = (lo + hi) / 2;
    if (inGamut(at(mid))) lo = mid;
    else hi = mid;
  }
  return toHex(at(lo));
}

export const lightness = (hex) => hexToLab(hex.replace('#', ''))[0];

export function parseKeys(text) {
  return [...text.matchAll(/^([a-zA-Z][a-zA-Z0-9_]*):\s*([^;]+);/gm)];
}

export function scanThemes(buf) {
  const themes = [];
  for (let i = 0; (i = buf.indexOf(LFH, i)) !== -1; i++) {
    if (i < 4) continue;
    const declared = buf.readUInt32BE(i - 4);
    if (declared < 256 || declared > 1 << 20 || i + declared > buf.length) continue;
    const blob = buf.subarray(i, i + declared);
    const entries = readCentralDirectory(blob);
    if (!entries) continue;
    const colors = entries.find((e) => e.name === COLORS_ENTRY);
    if (!colors) continue;
    let text;
    try {
      text = decode(colors, entryData(blob, colors));
    } catch {
      continue;
    }
    const keys = parseKeys(text);
    const windowBg = keys.find((m) => m[1] === 'windowBg')?.[2]?.trim();
    if (keys.length < 300 || !/^#[0-9a-fA-F]{6}$/.test(windowBg ?? '')) continue;
    themes.push({
      offset: i,
      declared,
      entries,
      keys: keys.length,
      windowBg,
      L: lightness(windowBg),
      text,
    });
  }
  return themes;
}

export function rebuildZip(buf, theme, replacements, deflate = (raw) => deflateRawSync(raw, { level: 9 })) {
  const blob = buf.subarray(theme.offset, theme.offset + theme.declared);
  const parts = [];
  const central = [];
  let offset = 0;

  for (const entry of theme.entries) {
    const raw = replacements[entry.name] ?? null;
    const data = raw ? deflate(raw) : entryData(blob, entry);
    const method = raw ? 8 : entry.method;
    const crc = raw ? crc32(raw) : entry.crc;
    const uncompressed = raw ? raw.length : entry.uncompressedSize;
    const name = Buffer.from(entry.name, 'latin1');

    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(20, 4);
    local.writeUInt16LE(method, 8);
    local.writeUInt16LE(entry.time, 10);
    local.writeUInt16LE(entry.date, 12);
    local.writeUInt32LE(crc, 14);
    local.writeUInt32LE(data.length, 18);
    local.writeUInt32LE(uncompressed, 22);
    local.writeUInt16LE(name.length, 26);
    parts.push(local, name, data);

    const cd = Buffer.alloc(46);
    cd.writeUInt32LE(0x02014b50, 0);
    cd.writeUInt16LE(20, 4);
    cd.writeUInt16LE(20, 6);
    cd.writeUInt16LE(method, 10);
    cd.writeUInt16LE(entry.time, 12);
    cd.writeUInt16LE(entry.date, 14);
    cd.writeUInt32LE(crc, 16);
    cd.writeUInt32LE(data.length, 20);
    cd.writeUInt32LE(uncompressed, 24);
    cd.writeUInt16LE(name.length, 28);
    cd.writeUInt32LE(offset, 42);
    central.push(cd, name);

    offset += local.length + name.length + data.length;
  }

  const centralBuf = Buffer.concat(central);
  const eocd = Buffer.alloc(22);
  eocd.writeUInt32LE(0x06054b50, 0);
  eocd.writeUInt16LE(theme.entries.length, 8);
  eocd.writeUInt16LE(theme.entries.length, 10);
  eocd.writeUInt32LE(centralBuf.length, 12);
  eocd.writeUInt32LE(offset, 16);

  return Buffer.concat([...parts, centralBuf, eocd]);
}

export function createMapper(palette, chromaPull) {
  const NEUTRAL_CHROMA = 0.055;
  const LIFTABLE_LIGHTNESS = 0.5;
  const neutrals = ['base00', 'base01', 'base02', 'base03', 'base04', 'base05', 'base06', 'base07'];
  const accentNames = ['base08', 'base09', 'base0A', 'base0B', 'base0C', 'base0D', 'base0E', 'base0F'];

  const lab = Object.fromEntries(
    Object.entries(palette).map(([k, v]) => [k, hexToLab(v.replace('#', ''))]),
  );
  const lch = (k) => {
    const [L, A, B] = lab[k];
    return { L, C: Math.hypot(A, B), h: Math.atan2(B, A) };
  };

  const ramp = neutrals
    .map((k) => ({ k, ...lch(k), lab: lab[k] }))
    .concat([{ k: 'black', L: 0, C: 0, h: 0, lab: [0, 0, 0] }])
    .sort((a, b) => a.L - b.L);
  const accents = accentNames.map((k) => ({ k, ...lch(k) }));

  const hueDistance = (a, b) => {
    const d = Math.abs(a - b) % (2 * Math.PI);
    return d > Math.PI ? 2 * Math.PI - d : d;
  };

  const rampAt = (L) => {
    if (L <= ramp[0].L) return null;
    if (L >= ramp[ramp.length - 1].L) return ramp[ramp.length - 1].lab;
    for (let i = 0; i < ramp.length - 1; i++) {
      const a = ramp[i];
      const b = ramp[i + 1];
      if (L >= a.L && L <= b.L) {
        const t = (L - a.L) / (b.L - a.L);
        return [L, a.lab[1] + (b.lab[1] - a.lab[1]) * t, a.lab[2] + (b.lab[2] - a.lab[2]) * t];
      }
    }
    return null;
  };

  return (hex, stats) => {
    const rgb = hex.slice(0, 6);
    const alpha = hex.length === 8 ? hex.slice(6, 8) : '';
    const [L, A, B] = hexToLab(rgb);
    const C = Math.hypot(A, B);

    if (C < NEUTRAL_CHROMA) {
      const mapped = rampAt(L);
      if (!mapped) {
        if (stats) stats.black++;
        return `#${hex}`;
      }
      if (stats) stats.neutral++;
      return `#${toHex(labToRgb(mapped))}${alpha}`;
    }

    const h = Math.atan2(B, A);
    const near = accents.reduce((best, a) => (hueDistance(a.h, h) < hueDistance(best.h, h) ? a : best));
    const pulled = C * (1 - chromaPull) + near.C * chromaPull;
    const lift = L >= LIFTABLE_LIGHTNESS;
    if (stats) stats[lift ? 'lifted' : 'chromatic']++;
    return `#${lchToHex(L, lift ? pulled : Math.min(C, pulled), near.h)}${alpha}`;
  };
}

export function remap(text, mapHex) {
  const stats = { neutral: 0, chromatic: 0, lifted: 0, black: 0, alias: 0 };

  const out = text
    .split('\n')
    .map((line) => {
      const m = line.match(/^([a-zA-Z][a-zA-Z0-9_]*):\s*([^;]+);(.*)$/);
      if (!m) return line;
      const [, key, value, rest] = m;
      const v = value.trim();
      if (!v.startsWith('#')) {
        stats.alias++;
        return `${key}: ${v};${rest}`;
      }
      if (!/^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$/.test(v)) {
        throw new Error(`unparsable value for ${key}: ${v}`);
      }
      return `${key}: ${mapHex(v.slice(1), stats)};${rest}`;
    })
    .join('\n');

  if (out.includes('|')) {
    throw new Error('output contains the codegen-only "|" fallback syntax, Telegram would reject the theme');
  }

  return { text: out, stats };
}
