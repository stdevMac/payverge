#!/usr/bin/env node
"use strict";

/**
 * Guard: forbid UTC calendar-day idioms in UI code.
 * `toISOString().split("T")[0]` / `.slice(0, 10)` yield the UTC day — wrong
 * near midnight for non-UTC users. Use src/lib/localDate.ts instead.
 * Allowlist: scripts/audit-guards-allowlist.json (utcDateKeys).
 */

const fs = require("fs");
const path = require("path");

const SRC = path.join(__dirname, "..", "src");
const ALLOWLIST = JSON.parse(
  fs.readFileSync(path.join(__dirname, "audit-guards-allowlist.json"), "utf8"),
).utcDateKeys;

const PATTERN =
  /toISOString\(\)\s*\.\s*(split\(\s*["']T["']\s*\)\s*\[\s*0\s*\]|slice\(\s*0\s*,\s*10\s*\))/;

function walk(dir, out) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(p, out);
    else if (/\.(ts|tsx)$/.test(entry.name) && !/\.test\.(ts|tsx)$/.test(entry.name))
      out.push(p);
  }
  return out;
}

const violations = [];
for (const file of walk(SRC, [])) {
  const rel = path.relative(path.join(__dirname, ".."), file).replace(/\\/g, "/");
  if (ALLOWLIST[rel]) continue;
  const lines = fs.readFileSync(file, "utf8").split("\n");
  lines.forEach((line, i) => {
    if (PATTERN.test(line)) violations.push(`${rel}:${i + 1}: ${line.trim()}`);
  });
}

if (violations.length > 0) {
  console.error(
    `UTC date-key guard: ${violations.length} violation(s). Use localDateKey()/localDateTimeInputValue() from src/lib/localDate.ts, or allowlist with a reason.`,
  );
  for (const v of violations) console.error(`  ${v}`);
  process.exit(1);
}
console.log("UTC date-key guard: clean.");
