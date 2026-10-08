#!/usr/bin/env node
// Contract: every environment variable the backend, the frontend or the
// deploy stack reads must be documented in docs/self-hosting/configuration.md,
// and every backend server flag must be listed there too.
//
// Run from the repository root:
//   node --test scripts/docs/env-inventory.test.mjs
//
// When this fails, add the variable to the right table in configuration.md
// (name, default, required?, effect). Only add a name to ALLOW below when an
// operator can never usefully set it on a running instance, and say why.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import test from "node:test";

import { ROOT, inventory, serverFlags } from "./env-inventory.mjs";

const DOC = "docs/self-hosting/configuration.md";

// Names the scanner finds that configuration.md deliberately does not list.
const ALLOW = new Map([
  // Test harness only (backend/internal/config test helpers, perf suites).
  ["TEST_DATABASE_URL", "unit-test config helper"],
  ["TEST_EMAIL_PROVIDER", "unit-test config helper"],
  ["TEST_JWT_SECRET", "unit-test config helper"],
  ["TEST_S3_BUCKET", "unit-test config helper"],
  ["TEST_S3_REGION", "unit-test config helper"],
  ["TEST_TELEGRAM_TOKEN", "unit-test config helper"],
  ["TESTPERF_DATABASE_URL", "perf benchmark suite"],
  // Set by Next.js itself, never by the operator.
  ["NEXT_RUNTIME", "set by Next.js per runtime (nodejs or edge)"],
  // Keys of the browser config object (src/config/publicConfig.ts), not env
  // names: the operator sets FRONTEND_SENTRY_* or NEXT_PUBLIC_SENTRY_* instead.
  ["SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE", "browser config key, not an env name"],
  ["SENTRY_REPLAYS_SESSION_SAMPLE_RATE", "browser config key, not an env name"],
]);

const docText = readFileSync(path.join(ROOT, DOC), "utf8");

test("every scanned variable is documented in configuration.md", () => {
  const missing = inventory()
    .map((row) => row.name)
    .filter((name) => !ALLOW.has(name) && !docText.includes("`" + name + "`"));
  assert.deepEqual(missing, [], `document these in ${DOC}: ${missing.join(", ")}`);
});

test("every allow-listed name is still read somewhere", () => {
  const names = new Set(inventory().map((row) => row.name));
  const stale = [...ALLOW.keys()].filter((name) => !names.has(name));
  assert.deepEqual(stale, [], `remove stale ALLOW entries: ${stale.join(", ")}`);
});

test("every backend server flag is documented in configuration.md", () => {
  const missing = serverFlags().filter((flag) => !docText.includes("`--" + flag + "`"));
  assert.deepEqual(missing, [], `document these flags in ${DOC}: ${missing.join(", ")}`);
});

test("the scanner still finds the core variables", () => {
  const names = new Set(inventory().map((row) => row.name));
  for (const name of ["JWT_SECRET_KEY", "PLUGIN_SECRET_KEY", "DB_PASSWORD", "DOMAIN", "TRUSTED_PROXIES", "FRONTEND_TRUSTED_PROXIES", "DB_HOST"]) {
    assert.ok(names.has(name), `scanner lost ${name}`);
  }
});
