#!/usr/bin/env node
// Bundle-size budget check. Runs after `vite build` and fails CI if the
// generated assets exceed the defined thresholds. Keeps both raw and gzipped
// budgets so regressions on either axis trip the gate.
//
// Budgets are set with ~25% headroom over the current build; tighten as the
// project matures. Update intentionally, not incidentally.
import { promises as fs } from 'node:fs';
import { gzipSync } from 'node:zlib';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// Use fileURLToPath — `.pathname` on a URL leaves spaces/`+` percent-encoded,
// which breaks on paths like "Codex + Code Projects/".
const HERE = path.dirname(fileURLToPath(import.meta.url));
const DIST = path.resolve(HERE, '..', 'dist', 'assets');

// Budgets in bytes. Bumped at v0.1.6 to absorb the firmware-page rewrite
// (dual-channel cache, sortable table, install job modal), the auto-update
// surfaces (Firmware bulk buttons, Devices column, Provision form, Compliance
// rule), and the configurable gen-badge settings. Pre-v0.1.6 baseline was
// 260 kB raw / 75 kB gzip; v0.1.6 measured 263.06 kB raw / 72.62 kB gzip.
const BUDGETS = {
  js: { raw: 280 * 1024, gzip: 80 * 1024 },
  css: { raw: 30 * 1024, gzip: 8 * 1024 },
};

function fmt(bytes) {
  return `${(bytes / 1024).toFixed(2)} kB`;
}

async function collect() {
  const entries = await fs.readdir(DIST);
  const buckets = { js: [], css: [] };
  for (const name of entries) {
    const ext = path.extname(name).toLowerCase();
    if (ext === '.js') buckets.js.push(name);
    else if (ext === '.css') buckets.css.push(name);
  }
  return buckets;
}

async function measure(files) {
  let raw = 0;
  let gzip = 0;
  for (const name of files) {
    const buf = await fs.readFile(path.join(DIST, name));
    raw += buf.length;
    gzip += gzipSync(buf).length;
  }
  return { raw, gzip };
}

async function main() {
  let files;
  try {
    files = await collect();
  } catch (err) {
    console.error(`[bundle-size] could not read ${DIST}: ${err.message}`);
    console.error('[bundle-size] did you run `vite build` first?');
    process.exit(2);
  }

  const failures = [];
  for (const kind of ['js', 'css']) {
    if (files[kind].length === 0) {
      console.warn(`[bundle-size] no ${kind} assets found — skipping ${kind} budget`);
      continue;
    }
    const sizes = await measure(files[kind]);
    const budget = BUDGETS[kind];
    const rawOK = sizes.raw <= budget.raw;
    const gzipOK = sizes.gzip <= budget.gzip;
    const icon = rawOK && gzipOK ? 'OK ' : 'FAIL';
    console.log(
      `[bundle-size] ${icon} ${kind}: ${fmt(sizes.raw)} raw (budget ${fmt(budget.raw)}), ${fmt(sizes.gzip)} gzip (budget ${fmt(budget.gzip)})`,
    );
    if (!rawOK) failures.push(`${kind} raw ${fmt(sizes.raw)} > budget ${fmt(budget.raw)}`);
    if (!gzipOK) failures.push(`${kind} gzip ${fmt(sizes.gzip)} > budget ${fmt(budget.gzip)}`);
  }

  if (failures.length > 0) {
    console.error('[bundle-size] BUDGET EXCEEDED:');
    for (const msg of failures) console.error(`  - ${msg}`);
    console.error(
      '[bundle-size] If the growth is intentional, raise the budget in scripts/check-bundle-size.mjs.',
    );
    process.exit(1);
  }
}

main().catch((err) => {
  console.error(`[bundle-size] unexpected error: ${err.stack || err.message}`);
  process.exit(2);
});
