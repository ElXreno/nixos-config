import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import { catppuccinNames, makeSnapper } from './palette.mjs';

const [, , srcDir, palettePath, varsPath, outPath, ...styles] = process.argv;
const { global: globalOverrides, perStyle } = JSON.parse(readFileSync(varsPath, 'utf8'));
const scratch = 'work';
mkdirSync(scratch, { recursive: true });

const die = (message) => {
  console.error(`userstyles: ${message}`);
  process.exit(1);
};

const base = JSON.parse(readFileSync(palettePath, 'utf8'));
const colours = catppuccinNames(base);
const FLAVOURS = ['latte', 'frappe', 'macchiato', 'mocha'];
const NAMES = Object.keys(colours);

const resolveLib = () => {
  const entry = readFileSync(join(srcDir, 'lib/lib.less'), 'utf8');
  if (entry.includes('#lib {')) return entry;
  const versioned = /@import\s+"[^"]*\/(lib\/std\/v\d+\.less)"\s*;/.exec(entry);
  if (!versioned) die('lib/lib.less holds neither the mixins nor an import of a versioned standard library');
  return readFileSync(join(srcDir, versioned[1]), 'utf8');
};

const upstreamLib = resolveLib();
const mixinStart = upstreamLib.indexOf('#lib {');
if (mixinStart === -1) die('cannot find the "#lib {" mixin block in the upstream standard library');

const upstreamFlavour = (map, flavour) => {
  const block = new RegExp(`@${flavour}:\\s*\\{([^}]*)\\}`).exec(map);
  if (!block) die(`cannot read the upstream ${flavour} block`);
  return block[1].trim();
};
const filtersMap = /@catppuccin-filters:\s*\{([\s\S]*?)\n\};/.exec(upstreamLib);
if (!filtersMap) die('cannot find the @catppuccin-filters map');
const darkFilters = upstreamFlavour(filtersMap[1], 'mocha');

const upstreamPalette = /@catppuccin:\s*\{([\s\S]*?)\n\};/.exec(upstreamLib);
if (!upstreamPalette) die('cannot find the @catppuccin map');
for (const name of NAMES) {
  if (!upstreamFlavour(upstreamPalette[1], 'mocha').includes(`@${name}:`)) {
    die(`upstream lib.less no longer defines @${name}, the palette mapping needs updating`);
  }
}
const upstreamNames = [...upstreamFlavour(upstreamPalette[1], 'mocha').matchAll(/@([a-z0-9]+):/g)].map((m) => m[1]);
const missing = upstreamNames.filter((n) => !NAMES.includes(n));
if (missing.length) die(`upstream defines colours we do not map: ${missing.join(', ')}`);

const entries = NAMES.map((name) => `@${name}: ${colours[name]};`).join(' ');
const generatedLib = [
  '/* generated from the stylix palette, mixins below are upstream verbatim */',
  '@catppuccin: {',
  ...FLAVOURS.map((flavour) => `  @${flavour}: { ${entries} };`),
  '};',
  '',
  '@catppuccin-filters: {',
  ...FLAVOURS.map((flavour) => `  @${flavour}: { ${darkFilters} };`),
  '};',
  '',
  upstreamLib.slice(mixinStart),
].join('\n');
const libPath = join(scratch, 'lib.less');
writeFileSync(libPath, generatedLib);

const varDefaults = (source, overrides) => {
  const meta = /==UserStyle==([\s\S]*?)==\/UserStyle==/.exec(source);
  if (!meta) return [];
  const definitions = [];
  const seen = new Set();
  for (const line of meta[1].split('\n')) {
    const m = /^\s*@var\s+(\S+)\s+(\S+)\s+(?:"[^"]*"|'[^']*')\s+(.*)$/.exec(line);
    if (!m) continue;
    const [, kind, name, rest] = m;
    const value = rest.trim();
    seen.add(name);
    if (overrides[name] !== undefined) {
      definitions.push(`@${name}: ${overrides[name]};`);
    } else if (kind === 'select' || kind === 'dropdown') {
      const options = [...value.matchAll(/"([^":]+):([^"]*)"/g)].map((o) => ({ key: o[1], label: o[2] }));
      if (!options.length) continue;
      const chosen = options.find((o) => o.label.endsWith('*')) ?? options[0];
      definitions.push(`@${name}: ${chosen.key};`);
    } else if (kind === 'checkbox') {
      definitions.push(`@${name}: ${value.replace(/[^01]/g, '') || 0};`);
    } else if (kind === 'range' || kind === 'number') {
      const items = value.replace(/^\[|\]$/g, '').split(',').map((item) => item.trim());
      const last = items[items.length - 1];
      const unit = /^['"].*['"]$/.test(last) ? last.slice(1, -1) : '';
      if (!/^-?[\d.]+$/.test(items[0])) die(`cannot read the default of @var ${kind} ${name}: ${value}`);
      definitions.push(`@${name}: ${items[0]}${unit};`);
    } else {
      definitions.push(`@${name}: ${value.replace(/;+$/, '')};`);
    }
  }
  for (const [name, value] of Object.entries(overrides)) {
    if (!seen.has(name)) definitions.push(`@${name}: ${value};`);
  }
  return definitions;
};

const snap = makeSnapper(colours);

const stripComments = (css) => {
  let out = '';
  let quote = null;
  for (let i = 0; i < css.length; i++) {
    const ch = css[i];
    if (quote) {
      out += ch;
      if (ch === quote && css[i - 1] !== '\\') quote = null;
      continue;
    }
    if (ch === '"' || ch === "'") {
      quote = ch;
      out += ch;
      continue;
    }
    if (ch === '/' && css[i + 1] === '*') {
      const end = css.indexOf('*/', i + 2);
      i = end === -1 ? css.length : end + 1;
      continue;
    }
    out += ch;
  }
  return out;
};

const transform = (css) => {
  let out = '';
  let depth = 0;
  let buffer = '';
  let parens = 0;
  let quote = null;
  let added = 0;
  let snapped = 0;
  const examples = [];

  const snapValue = (property, value) =>
    value.replace(/#([0-9a-fA-F]{6})\b/g, (match, hex) => {
      const replacement = snap(hex);
      if (!replacement) return match;
      snapped++;
      if (examples.length < 5) examples.push(`${property.trim()}: #${hex.toLowerCase()} -> ${replacement}`);
      return replacement;
    });

  const flush = (terminator) => {
    let declaration = buffer.trim();
    buffer = '';
    if (!declaration) {
      out += terminator;
      return;
    }
    const isDeclaration = depth > 0 && !declaration.startsWith('@') && /^[-a-zA-Z][^{}]*:/.test(declaration);
    if (!isDeclaration) {
      out += `${declaration}${terminator}`;
      return;
    }

    const colon = declaration.indexOf(':');
    declaration = declaration.slice(0, colon + 1) + snapValue(declaration.slice(0, colon), declaration.slice(colon + 1));

    if (/!important\s*$/i.test(declaration)) {
      out += `${declaration}${terminator}`;
      return;
    }
    out += `${declaration} !important${terminator}`;
    added++;
  };

  for (const ch of css) {
    if (quote) {
      buffer += ch;
      if (ch === quote) quote = null;
      continue;
    }
    if (ch === '"' || ch === "'") {
      quote = ch;
      buffer += ch;
      continue;
    }
    if (ch === '(') parens++;
    if (ch === ')') parens--;
    if (parens > 0) {
      buffer += ch;
      continue;
    }
    if (ch === ';') {
      flush(';');
      continue;
    }
    if (ch === '{') {
      out += buffer + ch;
      buffer = '';
      depth++;
      continue;
    }
    if (ch === '}') {
      flush('');
      out += '}';
      depth--;
      continue;
    }
    buffer += ch;
  }
  out += buffer;
  return { css: out, added, snapped, examples };
};

const chunks = [];
let totalAdded = 0;
let totalSnapped = 0;

for (const style of styles) {
  const stylePath = join(srcDir, 'styles', style, 'catppuccin.user.less');
  if (!existsSync(stylePath)) die(`no such style in the collection: ${style}`);
  const source = readFileSync(stylePath, 'utf8');

  const patched = source.replace(
    /@import\s+"[^"]*\/lib\/(?:lib\.less|std\/v\d+\.less)"\s*;/,
    `@import "${libPath}";`,
  );
  if (patched === source) die(`${style}: the upstream lib import is not where it used to be`);

  const entry = join(scratch, `${style.replace(/[^a-z0-9.-]/gi, '_')}.less`);
  const overrides = { ...globalOverrides, ...(perStyle[style] ?? {}) };
  writeFileSync(entry, [...varDefaults(source, overrides), patched].join("\n"));

  let css;
  try {
    css = execFileSync('lessc', ['--no-color', entry], { maxBuffer: 1 << 28 }).toString();
  } catch (error) {
    die(`${style}: lessc failed\n${error.stderr?.toString() ?? error.message}`);
  }

  const { css: important, added, snapped, examples } = transform(stripComments(css));
  totalAdded += added;
  totalSnapped += snapped;
  for (const example of examples) console.error(`userstyles:   snapped ${example}`);
  chunks.push(`/* ${style} */\n${important.trim()}\n`);
  console.error(`userstyles: ${style} -> ${css.length} bytes, ${added} important, ${snapped} foreign colours snapped`);
}

const result = [
  '/* generated by packages/userstyles-base16 from catppuccin/userstyles */',
  ...chunks,
].join('\n');
writeFileSync(outPath, result);
console.error(`userstyles: ${styles.length} styles, ${result.length} bytes, ${totalAdded} important, ${totalSnapped} foreign colours snapped`);
