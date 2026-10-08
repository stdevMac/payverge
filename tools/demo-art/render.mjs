#!/usr/bin/env node
// Renders the project-made demo illustrations (art.mjs) into the JPEGs the
// demo seed embeds under backend/internal/demo/assets/.
//
//   cd frontend && npm ci            # provides playwright
//   node tools/demo-art/render.mjs   # writes every image
//   node tools/demo-art/render.mjs carta/flan-casero.jpg   # just one
//
// Uses the system Chrome (channel "chrome") when Playwright's bundled
// Chromium is not installed.

import { createRequire } from 'node:module';
import { mkdirSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { ART } from './art.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const repo = resolve(here, '..', '..');
const require = createRequire(join(repo, 'frontend', 'package.json'));
const { chromium } = require('playwright');

const OUT = join(repo, 'backend', 'internal', 'demo', 'assets');
const only = process.argv.slice(2);

async function launch() {
  try {
    return await chromium.launch();
  } catch {
    return chromium.launch({ channel: 'chrome' });
  }
}

const browser = await launch();
try {
  const page = await browser.newPage();
  for (const [path, { width, height, svg }] of Object.entries(ART)) {
    if (only.length && !only.includes(path)) continue;
    await page.setViewportSize({ width, height });
    await page.setContent(
      `<!doctype html><html><head><style>html,body{margin:0;padding:0;background:#000}svg{display:block}</style></head><body>${svg()}</body></html>`,
    );
    const file = join(OUT, path);
    mkdirSync(dirname(file), { recursive: true });
    await page.screenshot({ path: file, type: 'jpeg', quality: 84, clip: { x: 0, y: 0, width, height } });
    console.log(`wrote ${path} (${width}x${height})`);
  }
} finally {
  await browser.close();
}
