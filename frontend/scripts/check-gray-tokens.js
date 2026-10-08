#!/usr/bin/env node
"use strict";

/**
 * Guard: reject NEW `*-gray-*` Tailwind utility classes. `gray` aliases the
 * same warm `neutralScale` as `ink`/`warm` (tailwind.config.ts), so this is a
 * naming guard — prefer `ink` (text) / `warm` (surface). Pre-existing offenders
 * are snapshotted in scripts/audit-guards-allowlist.json (grayTokens) so this
 * fails only on net-new usage. (Q-7)
 */
const fs = require("fs");
const path = require("path");

// text-/bg-/border-/ring-/from-/via-/to- gray, incl. dark: and hover: prefixes.
const GRAY_RE = /(?:text|bg|border|ring|from|via|to|fill|stroke|divide|placeholder|decoration)-gray-\d/;

function walk(dir, out) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, e.name);
    if (e.isDirectory()) walk(p, out);
    else if (/\.(ts|tsx)$/.test(e.name) && !/\.test\.(ts|tsx)$/.test(e.name)) out.push(p);
  }
  return out;
}

function runGrayGuard({ dir, allowlist }) {
  const violations = [];
  for (const file of walk(dir, [])) {
    const rel = path.relative(dir, file).replace(/\\/g, "/");
    if (allowlist[rel]) continue;
    const lines = fs.readFileSync(file, "utf8").split("\n");
    lines.forEach((line, i) => {
      if (GRAY_RE.test(line)) violations.push(`${rel}:${i + 1}: ${line.trim().slice(0, 120)}`);
    });
  }
  return { violations };
}

module.exports = { runGrayGuard, GRAY_RE };

if (require.main === module) {
  const SRC = path.join(__dirname, "..", "src");
  const allowlist =
    JSON.parse(fs.readFileSync(path.join(__dirname, "audit-guards-allowlist.json"), "utf8")).grayTokens || {};
  const { violations } = runGrayGuard({ dir: SRC, allowlist });
  if (violations.length) {
    console.error(`gray-token guard: ${violations.length} NEW *-gray-* class(es). Use ink/warm instead.`);
    for (const v of violations) console.error(`  ${v}`);
    process.exit(1);
  }
  console.log("gray-token guard: clean.");
}
