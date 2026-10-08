#!/usr/bin/env node
/**
 * R20 frontend bundle-size budget gate.
 *
 * Zero-dependency (Node builtins only) so it adds nothing to the dependency
 * surface the `npm audit` gate watches. Measures the *gzipped* client JS the
 * Next.js build emits and compares against committed budgets in
 * `frontend/bundle-budget.json`. Exits non-zero on any breach.
 *
 * Two honest, directly-measurable metrics (both gzipped, since gzip is the real
 * network cost and zlib is a builtin):
 *
 *   - sharedRootJsGzipKB : the app-router shared entry (`rootMainFiles` from
 *     .next/build-manifest.json) — the baseline every route pays on first load.
 *   - totalClientJsGzipKB : gzip of every .js under .next/static/chunks — the
 *     total client JS the deployed app can ship. Best single regression signal.
 *
 * We deliberately do NOT try to reproduce Next's per-route "First Load JS"
 * number: app-build-manifest lists a route's full referenced chunk graph
 * (async + shared, de-dup only in Next's own reporter), so summing it
 * over-counts. The two metrics above correspond to quantities we can measure
 * exactly on disk.
 *
 * Usage:
 *   npm run build && node scripts/check-bundle-budget.js
 *   node scripts/check-bundle-budget.js --write   # reseed budgets from current build
 */
"use strict";

const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const FRONTEND_DIR = path.join(__dirname, "..");
const NEXT_DIR = path.join(FRONTEND_DIR, ".next");
const BUDGET_FILE = path.join(FRONTEND_DIR, "bundle-budget.json");
const BUILD_MANIFEST = path.join(NEXT_DIR, "build-manifest.json");
const CHUNKS_DIR = path.join(NEXT_DIR, "static", "chunks");

// Headroom applied when (re)seeding budgets with --write.
const HEADROOM = 1.1;

function fail(msg) {
  console.error(`check-bundle-budget: ${msg}`);
  process.exit(1);
}

function gzipLen(absPath) {
  return zlib.gzipSync(fs.readFileSync(absPath)).length;
}

function kb(bytes) {
  return Math.round((bytes / 1024) * 10) / 10;
}

function walkJs(dir) {
  const out = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...walkJs(p));
    else if (entry.name.endsWith(".js")) out.push(p);
  }
  return out;
}

function measure() {
  if (!fs.existsSync(BUILD_MANIFEST)) {
    fail(`no build manifest at ${BUILD_MANIFEST} — run \`npm run build\` first`);
  }
  if (!fs.existsSync(CHUNKS_DIR)) {
    fail(`no chunks dir at ${CHUNKS_DIR} — run \`npm run build\` first`);
  }

  const manifest = JSON.parse(fs.readFileSync(BUILD_MANIFEST, "utf8"));
  const rootMainFiles = Array.isArray(manifest.rootMainFiles)
    ? manifest.rootMainFiles
    : [];
  if (rootMainFiles.length === 0) {
    fail("build-manifest.json has no rootMainFiles (unexpected Next.js output shape)");
  }

  const sharedBytes = rootMainFiles.reduce(
    (sum, rel) => sum + gzipLen(path.join(NEXT_DIR, rel)),
    0,
  );

  const allChunks = walkJs(CHUNKS_DIR);
  const totalBytes = allChunks.reduce((sum, abs) => sum + gzipLen(abs), 0);

  return {
    sharedRootJsGzipKB: kb(sharedBytes),
    totalClientJsGzipKB: kb(totalBytes),
    chunkFileCount: allChunks.length,
  };
}

const measured = measure();

if (process.argv.includes("--write")) {
  const budget = {
    _comment:
      "Gzipped-KB client-JS budgets enforced by scripts/check-bundle-budget.js. Reseed with --write ONLY after an intentional, reviewed size change — investigate before raising.",
    sharedRootJsGzipKB: Math.ceil(measured.sharedRootJsGzipKB * HEADROOM),
    totalClientJsGzipKB: Math.ceil(measured.totalClientJsGzipKB * HEADROOM),
  };
  fs.writeFileSync(BUDGET_FILE, JSON.stringify(budget, null, 2) + "\n");
  console.log(
    `check-bundle-budget: wrote budgets (+${Math.round((HEADROOM - 1) * 100)}% headroom) to ${BUDGET_FILE}`,
  );
  console.log(budget);
  process.exit(0);
}

if (!fs.existsSync(BUDGET_FILE)) {
  fail(`no budget file at ${BUDGET_FILE} — seed once with: node scripts/check-bundle-budget.js --write`);
}

const budget = JSON.parse(fs.readFileSync(BUDGET_FILE, "utf8"));
const checks = [
  ["sharedRootJsGzipKB", "shared root JS (every route)"],
  ["totalClientJsGzipKB", `total client JS (${measured.chunkFileCount} chunks)`],
];

const breaches = [];
console.log("check-bundle-budget: measured client JS (gzipped KB)");
for (const [key, label] of checks) {
  const actual = measured[key];
  const limit = budget[key];
  const status = limit == null ? "no-budget" : actual > limit ? "OVER" : "ok";
  console.log(`  ${status.padEnd(9)} ${label}: ${actual} KB (budget ${limit ?? "—"} KB)`);
  if (limit != null && actual > limit) {
    breaches.push(`${label}: ${actual} KB > budget ${limit} KB`);
  }
}

if (breaches.length) {
  console.error("check-bundle-budget: FAILED — bundle budget exceeded:");
  for (const b of breaches) console.error(`  - ${b}`);
  console.error(
    "Investigate the increase. If intentional and reviewed, reseed: node scripts/check-bundle-budget.js --write",
  );
  process.exit(1);
}

console.log("check-bundle-budget: OK — all bundle budgets satisfied");
