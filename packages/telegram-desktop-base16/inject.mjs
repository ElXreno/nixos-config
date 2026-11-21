import { readFileSync, writeFileSync, readdirSync, statSync, chmodSync, mkdtempSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { COLORS_ENTRY, createMapper, entryBytes, parseKeys, remap, rebuildZip, scanThemes } from './theme.mjs';

const [, , outDir, originalPath, palettePath, pullArg] = process.argv;
const chromaPull = Number(pullArg);
const palette = JSON.parse(readFileSync(palettePath, 'utf8'));
const mapHex = createMapper(palette, chromaPull);
const scratch = mkdtempSync(join(tmpdir(), 'tg-inject-'));

const die = (message) => {
  console.error(`inject: ${message}`);
  process.exit(1);
};

if (outDir.length !== originalPath.length) {
  die(`store path lengths differ, cannot patch the wrapper's baked-in path:\n  ${originalPath}\n  ${outDir}`);
}

const files = [];
const walk = (dir) => {
  for (const item of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, item.name);
    if (item.isDirectory()) walk(path);
    else if (item.isFile()) files.push(path);
  }
};
walk(outDir);

const binary = files
  .map((path) => ({ path, size: statSync(path).size }))
  .sort((a, b) => b.size - a.size)[0];
if (!binary || binary.size < 1 << 20) die('no binary larger than 1 MiB found in the package');
console.error(`inject: patching ${binary.path} (${(binary.size / 1024 / 1024).toFixed(0)} MiB)`);

const buf = readFileSync(binary.path);
const themes = scanThemes(buf);
if (!themes.length) {
  die('no embedded theme found; Qt may have started compressing the qrc entries');
}
for (const theme of themes) {
  console.error(
    `inject: found theme offset=${theme.offset} bytes=${theme.declared} keys=${theme.keys} windowBg=${theme.windowBg} L=${theme.L.toFixed(3)}`,
  );
}

const dark = themes.filter((theme) => theme.L < 0.5).sort((a, b) => a.L - b.L || a.offset - b.offset);
if (!dark.length) die('every embedded theme is light, refusing to patch');

const solidTile = (hex, index) => {
  const path = join(scratch, `tile-${index}.png`);
  execFileSync('magick', ['-size', '1x1', `xc:${hex}`, '-strip', `PNG24:${path}`]);
  return readFileSync(path);
};

const zopfli = (raw) => {
  const path = join(scratch, 'entry');
  writeFileSync(path, raw);
  return execFileSync('zopfli', ['--deflate', '-c', path], { maxBuffer: 64 << 20 });
};

const tileColour = (path) => {
  const pixel = execFileSync('magick', [path, '-resize', '1x1!', '-depth', '8', '-format', '%[hex:p{0,0}]', 'info:'])
    .toString()
    .trim();
  if (!/^[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$/.test(pixel)) return null;
  return pixel.slice(0, 6).toLowerCase();
};

const expected = new Map();

for (const [index, theme] of dark.entries()) {
  const { text: remapped, stats } = remap(theme.text, mapHex);
  const replacements = { [COLORS_ENTRY]: Buffer.from(remapped, 'utf8') };

  const background = theme.entries.find((entry) => /^background\./.test(entry.name));
  if (background) {
    const blob = buf.subarray(theme.offset, theme.offset + theme.declared);
    const sourcePath = join(scratch, `bg-${index}-${background.name}`);
    writeFileSync(sourcePath, entryBytes(blob, background));
    const source = tileColour(sourcePath);
    if (!source) die(`cannot read the background colour of ${background.name} in theme ${theme.offset}`);
    const mapped = mapHex(source);
    replacements[background.name] = solidTile(mapped, index);
    console.error(`inject: background ${background.name} #${source} -> ${mapped}`);
  }

  const zip = rebuildZip(buf, theme, replacements, zopfli);
  if (zip.length > theme.declared) {
    die(`rebuilt theme is ${zip.length} bytes but only ${theme.declared} are available at offset ${theme.offset}`);
  }
  buf.writeUInt32BE(zip.length, theme.offset - 4);
  zip.copy(buf, theme.offset);
  buf.fill(0, theme.offset + zip.length, theme.offset + theme.declared);
  expected.set(theme.offset, remapped);
  console.error(
    `inject: offset=${theme.offset} windowBg=${theme.windowBg} -> ${zip.length} of ${theme.declared} bytes, remap ${JSON.stringify(stats)}`,
  );
}

const replaceStorePath = (target) => {
  const from = Buffer.from(originalPath, 'utf8');
  const to = Buffer.from(outDir, 'utf8');
  let count = 0;
  for (let i = 0; (i = target.indexOf(from, i)) !== -1; i += to.length) {
    to.copy(target, i);
    count++;
  }
  return count;
};

console.error(`inject: rewrote ${replaceStorePath(buf)} self-references in the binary`);
writeFileSync(binary.path, buf);

for (const path of files) {
  if (path === binary.path) continue;
  const mode = statSync(path).mode;
  const content = readFileSync(path);
  const count = replaceStorePath(content);
  if (count === 0) continue;
  writeFileSync(path, content);
  chmodSync(path, mode);
  console.error(`inject: rewrote ${count} self-references in ${path}`);
}

const verify = readFileSync(binary.path);
if (verify.indexOf(Buffer.from(originalPath, 'utf8')) !== -1) {
  die('the binary still references the unpatched store path');
}
if (verify.length !== binary.size) die('binary size changed, offsets would be invalid');

const patched = scanThemes(verify).filter((theme) => theme.L < 0.5);
if (patched.length !== dark.length) {
  die(`expected ${dark.length} dark themes after patching, found ${patched.length}`);
}
for (const theme of patched) {
  if (theme.text !== expected.get(theme.offset)) {
    die(`theme at offset ${theme.offset} does not read back as the palette generated for it`);
  }
  const keys = parseKeys(theme.text).length;
  if (keys < 300) die(`theme at offset ${theme.offset} only has ${keys} keys`);
  console.error(`inject: verified offset=${theme.offset} keys=${keys} windowBg=${theme.windowBg}`);
}

console.error(`inject: patched ${patched.length} dark themes, each remapped from its own source`);
