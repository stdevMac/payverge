#!/usr/bin/env node
/**
 * Production-dependency npm audit gate with a reviewed allowlist.
 *
 * Runs `npm audit --omit=dev --json` and fails when any advisory at or above
 * the threshold (default: high) is not listed in npm-audit-allowlist.json.
 * Each allowlist entry needs a reason and a `reviewBy` date; an expired entry
 * fails the gate so accepted risks are re-checked instead of forgotten.
 * Entries that npm no longer reports are printed so they can be removed.
 *
 * Usage:
 *   node scripts/check-npm-audit.js                 # runs npm audit itself
 *   node scripts/check-npm-audit.js --input a.json  # evaluate a saved report
 *   node scripts/check-npm-audit.js --level critical
 *
 * Zero extra deps (Node builtins only).
 */
"use strict";

const fs = require("fs");
const path = require("path");
const { spawnSync } = require("child_process");

const FRONTEND_DIR = path.join(__dirname, "..");
const ALLOWLIST_PATH = path.join(__dirname, "npm-audit-allowlist.json");
const SEVERITY_RANK = { info: 0, low: 1, moderate: 2, high: 3, critical: 4 };

function advisoryId(via) {
  const match = /GHSA-[0-9a-z]{4}-[0-9a-z]{4}-[0-9a-z]{4}/i.exec(via.url || "");
  return match ? match[0] : `npm-${via.source}`;
}

/** Collect advisories (not the packages that merely depend on them). */
function collectAdvisories(report) {
  const found = new Map();
  for (const vuln of Object.values(report.vulnerabilities || {})) {
    for (const via of vuln.via || []) {
      if (!via || typeof via !== "object") continue;
      const id = advisoryId(via);
      const entry = found.get(id) || {
        id,
        title: via.title,
        severity: via.severity,
        packages: new Set(),
      };
      entry.packages.add(via.name);
      found.set(id, entry);
    }
  }
  return [...found.values()];
}

function evaluate(
  report,
  allowlist,
  { level = "high", today = new Date() } = {},
) {
  const threshold = SEVERITY_RANK[level];
  if (threshold === undefined) throw new Error(`unknown level: ${level}`);
  const allowed = new Map((allowlist.advisories || []).map((a) => [a.id, a]));
  const todayKey = today.toISOString().slice(0, 10);

  const failures = [];
  const accepted = [];
  const reported = new Set();
  for (const adv of collectAdvisories(report)) {
    reported.add(adv.id);
    if ((SEVERITY_RANK[adv.severity] ?? 0) < threshold) continue;
    const entry = allowed.get(adv.id);
    if (!entry) {
      failures.push(
        `${adv.id} (${adv.severity}) ${adv.title} in ${[...adv.packages].join(", ")}`,
      );
    } else if (!entry.reviewBy || entry.reviewBy < todayKey) {
      failures.push(
        `${adv.id} allowlist entry expired (reviewBy ${entry.reviewBy || "missing"}); re-review it`,
      );
    } else {
      accepted.push(
        `${adv.id} (${adv.severity}) accepted until ${entry.reviewBy}: ${entry.reason}`,
      );
    }
  }
  const stale = [...allowed.keys()].filter((id) => !reported.has(id));
  return { failures, accepted, stale };
}

function runAudit() {
  const npm = process.platform === "win32" ? "npm.cmd" : "npm";
  const res = spawnSync(npm, ["audit", "--omit=dev", "--json"], {
    cwd: FRONTEND_DIR,
    encoding: "utf8",
    maxBuffer: 64 * 1024 * 1024,
  });
  // npm audit exits non-zero when it finds advisories; only a missing or
  // unparsable report is an error.
  try {
    const report = JSON.parse(res.stdout);
    if (report.error) throw new Error(JSON.stringify(report.error));
    return report;
  } catch (err) {
    process.stderr.write(res.stderr || "");
    throw new Error(`npm audit did not return a usable report: ${err.message}`);
  }
}

function main(argv) {
  const arg = (name) => {
    const i = argv.indexOf(name);
    return i >= 0 ? argv[i + 1] : undefined;
  };
  const input = arg("--input");
  const level = arg("--level") || "high";
  const report = input
    ? JSON.parse(fs.readFileSync(input, "utf8"))
    : runAudit();
  const allowlist = JSON.parse(fs.readFileSync(ALLOWLIST_PATH, "utf8"));
  const { failures, accepted, stale } = evaluate(report, allowlist, { level });

  for (const line of accepted) console.log(`accepted: ${line}`);
  for (const id of stale)
    console.log(`stale allowlist entry (npm no longer reports it): ${id}`);
  if (failures.length) {
    console.error(`npm audit (production dependencies, ${level}+) failed:`);
    for (const line of failures) console.error(`  - ${line}`);
    return 1;
  }
  console.log(
    `npm audit (production dependencies, ${level}+): no unreviewed advisories.`,
  );
  return 0;
}

module.exports = { collectAdvisories, evaluate };

if (require.main === module) {
  try {
    process.exitCode = main(process.argv.slice(2));
  } catch (err) {
    console.error(err.message);
    process.exitCode = 2;
  }
}
