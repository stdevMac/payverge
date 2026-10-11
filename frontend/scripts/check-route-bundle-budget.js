#!/usr/bin/env node
/**
 * Frontend route-bundle performance gate (Session Q / Root A).
 *
 * Sibling to the Backend Performance Gate (CLAUDE.md): a committed script that
 * fails CI when a watched route exceeds its KiB budget. Evidence lives under
 * docs/performance/ — never summary.md (backend-only).
 *
 * Metric (in-repo, disk-honest): after `npm run build`, read
 * `.next/app-build-manifest.json` and `statSync` each chunk listed for a route.
 * Sums are raw (ungzipped) on-disk bytes — the same shape the 2026-08-06 scout
 * used (dashboard ~5.7 MiB / 46 chunks). We deliberately do NOT try to
 * reproduce Next's "First Load JS" reporter number (that de-dups shared chunks
 * differently); this gate is a regression signal on the full referenced graph.
 *
 * Usage:
 *   npm run build && node scripts/check-route-bundle-budget.js
 *   node scripts/check-route-bundle-budget.js --write   # reseed budgets (+headroom)
 *   node scripts/check-route-bundle-budget.js --json    # machine-readable measure
 *
 * Zero extra deps (Node builtins only).
 */
"use strict";

const fs = require("fs");
const path = require("path");

const FRONTEND_DIR = path.join(__dirname, "..");
const NEXT_DIR = path.join(FRONTEND_DIR, ".next");
const MANIFEST = path.join(NEXT_DIR, "app-build-manifest.json");
const BUDGET_FILE = path.join(FRONTEND_DIR, "route-bundle-budget.json");

/** Headroom applied when (re)seeding budgets with --write. */
const HEADROOM = 1.1;

/** Route id we treat as the primary operator surface (L3-1). */
const DASHBOARD_ROUTE =
  "/(shop)/business/[businessId]/dashboard/page";

/**
 * Measure one route's full referenced chunk graph.
 * @param {string} nextDir  path to .next
 * @param {string[]} files  relative paths from app-build-manifest pages[route]
 * @returns {{ totalBytes: number, chunkCount: number, missing: number, files: string[] }}
 */
function measureRouteFiles(nextDir, files) {
  let totalBytes = 0;
  let missing = 0;
  const seen = new Set();
  const resolved = [];
  for (const rel of files || []) {
    if (seen.has(rel)) continue;
    seen.add(rel);
    const abs = path.join(nextDir, rel);
    if (!fs.existsSync(abs)) {
      missing += 1;
      continue;
    }
    totalBytes += fs.statSync(abs).size;
    resolved.push(rel);
  }
  return {
    totalBytes,
    chunkCount: resolved.length,
    missing,
    files: resolved,
  };
}

/**
 * Load app-build-manifest and measure every page route.
 * @param {string} [nextDir]
 * @returns {{
 *   routes: Array<{ route: string, totalBytes: number, totalKiB: number, chunkCount: number, missing: number }>,
 *   dashboard: object | null,
 *   medianPageKiB: number,
 *   pageRouteCount: number,
 * }}
 */
function measureAllRoutes(nextDir = NEXT_DIR) {
  const manifestPath = path.join(nextDir, "app-build-manifest.json");
  if (!fs.existsSync(manifestPath)) {
    throw new Error(
      `no app-build-manifest at ${manifestPath} — run \`npm run build\` first`,
    );
  }
  const manifest = JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  const pages = manifest.pages || {};
  const routes = [];
  for (const [route, files] of Object.entries(pages)) {
    // Page + root routes only (skip layouts/loading/error for median).
    if (!(route.endsWith("/page") || route === "/page")) continue;
    const m = measureRouteFiles(nextDir, files);
    routes.push({
      route,
      totalBytes: m.totalBytes,
      totalKiB: Math.round((m.totalBytes / 1024) * 10) / 10,
      chunkCount: m.chunkCount,
      missing: m.missing,
    });
  }
  routes.sort((a, b) => b.totalBytes - a.totalBytes);

  const sizes = routes.map((r) => r.totalBytes).sort((a, b) => a - b);
  const medianBytes =
    sizes.length === 0 ? 0 : sizes[Math.floor(sizes.length / 2)];
  const dashboard = routes.find((r) => r.route === DASHBOARD_ROUTE) || null;

  return {
    routes,
    dashboard,
    medianPageKiB: Math.round((medianBytes / 1024) * 10) / 10,
    pageRouteCount: routes.length,
  };
}

function fail(msg) {
  console.error(`check-route-bundle-budget: ${msg}`);
  process.exit(1);
}

function main(argv = process.argv.slice(2)) {
  let measured;
  try {
    measured = measureAllRoutes(NEXT_DIR);
  } catch (err) {
    fail(err.message || String(err));
  }

  if (argv.includes("--json")) {
    process.stdout.write(JSON.stringify(measured, null, 2) + "\n");
    return 0;
  }

  if (!measured.dashboard) {
    fail(
      `dashboard route ${DASHBOARD_ROUTE} missing from app-build-manifest — unexpected Next output shape`,
    );
  }

  if (argv.includes("--write")) {
    const budget = {
      _comment:
        "Per-route raw KiB budgets from app-build-manifest + statSync. Enforced by scripts/check-route-bundle-budget.js. Reseed with --write ONLY after intentional size change. Sibling to scripts/check-bundle-budget.js (gzip totals).",
      dashboardRoute: DASHBOARD_ROUTE,
      dashboardTotalKiB: Math.ceil(measured.dashboard.totalKiB * HEADROOM),
      dashboardChunkCountMax: Math.ceil(
        measured.dashboard.chunkCount * HEADROOM,
      ),
      medianPageKiB: Math.ceil(measured.medianPageKiB * HEADROOM),
    };
    fs.writeFileSync(BUDGET_FILE, JSON.stringify(budget, null, 2) + "\n");
    console.log(
      `check-route-bundle-budget: wrote budgets (+${Math.round((HEADROOM - 1) * 100)}% headroom) to ${BUDGET_FILE}`,
    );
    console.log(budget);
    console.log(
      `  measured dashboard: ${measured.dashboard.totalKiB} KiB / ${measured.dashboard.chunkCount} chunks`,
    );
    console.log(`  measured median page: ${measured.medianPageKiB} KiB`);
    return 0;
  }

  if (!fs.existsSync(BUDGET_FILE)) {
    fail(
      `no budget file at ${BUDGET_FILE} — seed once with: node scripts/check-route-bundle-budget.js --write`,
    );
  }

  const budget = JSON.parse(fs.readFileSync(BUDGET_FILE, "utf8"));
  const breaches = [];

  console.log("check-route-bundle-budget: measured route graph (raw KiB)");
  console.log(
    `  dashboard  ${measured.dashboard.totalKiB} KiB  ${measured.dashboard.chunkCount} chunks  (budget ${budget.dashboardTotalKiB} KiB / ≤${budget.dashboardChunkCountMax} chunks)`,
  );
  console.log(
    `  median page ${measured.medianPageKiB} KiB  across ${measured.pageRouteCount} page routes  (budget ${budget.medianPageKiB} KiB)`,
  );

  if (measured.dashboard.totalKiB > budget.dashboardTotalKiB) {
    breaches.push(
      `dashboard total: ${measured.dashboard.totalKiB} KiB > budget ${budget.dashboardTotalKiB} KiB`,
    );
  }
  if (measured.dashboard.chunkCount > budget.dashboardChunkCountMax) {
    breaches.push(
      `dashboard chunks: ${measured.dashboard.chunkCount} > budget ${budget.dashboardChunkCountMax}`,
    );
  }
  if (
    budget.medianPageKiB != null &&
    measured.medianPageKiB > budget.medianPageKiB
  ) {
    breaches.push(
      `median page: ${measured.medianPageKiB} KiB > budget ${budget.medianPageKiB} KiB`,
    );
  }

  if (breaches.length) {
    console.error("check-route-bundle-budget: FAILED — route budget exceeded:");
    for (const b of breaches) console.error(`  - ${b}`);
    console.error(
      "Investigate the increase. If intentional and reviewed, reseed: node scripts/check-route-bundle-budget.js --write",
    );
    process.exit(1);
  }

  console.log("check-route-bundle-budget: OK — all route budgets satisfied");
  return 0;
}

module.exports = {
  DASHBOARD_ROUTE,
  HEADROOM,
  measureRouteFiles,
  measureAllRoutes,
  main,
  BUDGET_FILE,
};

if (require.main === module) {
  main();
}
