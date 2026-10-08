#!/usr/bin/env node
/**
 * R19 stable-Jest gate.
 *
 * Runs the Jest suite with --detectOpenHandles and FAILS if Jest either exits
 * non-zero or reports leaked open handles. A plain `jest --detectOpenHandles`
 * step is not a gate: Jest still exits 0 when it only *detects* handles (it just
 * prints a warning), so a leak would pass silently. This wrapper turns the
 * warning into a non-zero exit.
 *
 * Cadence: nightly (see .github/workflows/e2e-nightly.yml), not per-PR — the
 * --detectOpenHandles run forces --runInBand and is ~30% slower. The fast
 * per-PR jest run in ci-gates.yml already catches process-blocking leaks
 * (it runs without --forceExit, so a blocking handle hangs the job to timeout).
 *
 * Usage:
 *   node scripts/check-jest-open-handles.js            # full suite (CI)
 *   node scripts/check-jest-open-handles.js <path>     # subset (local smoke)
 *   node scripts/check-jest-open-handles.js --detect-only <logfile>  # test the detector
 */
"use strict";

const { spawnSync } = require("child_process");
const fs = require("fs");

// Patterns that indicate Jest could not cleanly shut down.
const OPEN_HANDLE_PATTERNS = [
  /Jest has detected the following \d+ open handle/i,
  /Jest did not exit one second after the test run/i,
  /A worker process has failed to exit gracefully/i,
];

/**
 * @param {string} output combined stdout+stderr from a jest --detectOpenHandles run
 * @returns {string|null} the matched warning line, or null if clean
 */
function detectOpenHandles(output) {
  for (const re of OPEN_HANDLE_PATTERNS) {
    const m = output.match(re);
    if (m) return m[0];
  }
  return null;
}

module.exports = { detectOpenHandles, OPEN_HANDLE_PATTERNS };

if (require.main === module) {
  const args = process.argv.slice(2);

  // Detector-only mode for tests: validate the predicate against a captured log.
  if (args[0] === "--detect-only") {
    const file = args[1];
    const text = fs.readFileSync(file, "utf8");
    const hit = detectOpenHandles(text);
    if (hit) {
      console.error(`check-jest-open-handles: open-handle warning found: ${hit}`);
      process.exit(1);
    }
    console.log("check-jest-open-handles: no open-handle warning in log");
    process.exit(0);
  }

  const passthrough = args; // optional test-path filter for local smoke runs
  const jestArgs = [
    "jest",
    ...passthrough,
    "--ci",
    "--watchman=false",
    "--runInBand",
    "--detectOpenHandles",
  ];

  console.log(`check-jest-open-handles: npx ${jestArgs.join(" ")}`);
  const res = spawnSync("npx", jestArgs, {
    cwd: __dirname + "/..",
    encoding: "utf8",
    maxBuffer: 64 * 1024 * 1024,
    env: { ...process.env, NODE_OPTIONS: "--max-old-space-size=4096" },
  });

  const combined = `${res.stdout || ""}\n${res.stderr || ""}`;
  // Jest writes its normal reporter output to stderr; echo everything through.
  process.stdout.write(res.stdout || "");
  process.stderr.write(res.stderr || "");

  if (res.status !== 0) {
    console.error(`check-jest-open-handles: FAILED — jest exited ${res.status}`);
    process.exit(res.status || 1);
  }
  const hit = detectOpenHandles(combined);
  if (hit) {
    console.error(`check-jest-open-handles: FAILED — leaked open handles: ${hit}`);
    process.exit(1);
  }
  console.log("check-jest-open-handles: OK — suite passed with no open handles");
}
