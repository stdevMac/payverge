#!/usr/bin/env node
/* eslint-disable no-console */
"use strict";

// Operator-tier locale validator. The operator dashboard is served by
// SimpleTranslationProvider for exactly { en, es, es-AR }. en is the source of
// truth; es is a full bundle; es-AR is a thin voseo OVERRIDE layer deep-merged
// over es. This script replicates the provider's tree construction
// (src/i18n/SimpleTranslationProvider.tsx) and enforces structural parity:
//   - es must contain every en key (missing => English fallback for operators)
//   - es must contain NO key absent from en (extra => dead cruft; this is what
//     hid 28 genuine gaps before 2026-06-01)
//   - the es-AR EFFECTIVE tree (es base + es-AR overrides) must satisfy both too
//     (an es-AR override of a key en lacks reintroduces an orphan)
//
// Unlike the guest tier this has no coverage/same-as-en notion — operator es is
// expected to differ from en, and brand terms legitimately coincide. We only
// guard the structural key set, which is the failure mode that actually drifts.
//
// Usage: node scripts/check-operator-locales.js [--report]
// Exit: 0 = parity holds, 1 = hard errors (missing/extra keys).

const fs = require("fs");
const path = require("path");

const MESSAGES_DIR = path.resolve(__dirname, "..", "src", "i18n", "messages");
const LOCALES_DIR = path.resolve(__dirname, "..", "src", "i18n", "locales");
// es-AR lives at messages/es-ar and locales/es-ar on disk (case-insensitive FS).
const DISK = { en: "en", es: "es", "es-AR": "es-ar" };

function loadJson(p) {
  return JSON.parse(fs.readFileSync(p, "utf8"));
}

function flatten(obj, prefix, out) {
  for (const [key, value] of Object.entries(obj)) {
    const p = prefix ? `${prefix}.${key}` : key;
    if (value && typeof value === "object" && !Array.isArray(value)) {
      flatten(value, p, out);
    } else {
      out[p] = value;
    }
  }
  return out;
}

function isPlainObject(v) {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}
function deepMerge(base, override) {
  const result = { ...base };
  for (const [key, ov] of Object.entries(override)) {
    const bv = result[key];
    result[key] = isPlainObject(bv) && isPlainObject(ov) ? deepMerge(bv, ov) : ov;
  }
  return result;
}

// Load every namespace JSON in messages/<disk> keyed by filename (mirrors the
// index.ts barrel, which imports each file under its basename).
function loadMessages(disk) {
  const dir = path.join(MESSAGES_DIR, disk);
  const tree = {};
  for (const f of fs.readdirSync(dir).filter((x) => x.endsWith(".json"))) {
    tree[f.replace(/\.json$/, "")] = loadJson(path.join(dir, f));
  }
  return tree;
}
function localeCommon(disk) {
  const p = path.join(LOCALES_DIR, disk, "common.json");
  return fs.existsSync(p) ? loadJson(p) : {};
}

// Replicate SimpleTranslationProvider.tsx tree construction.
function buildTrees() {
  const enMsg = loadMessages("en");
  const esMsg = loadMessages("es");
  const esArMsg = loadMessages("es-ar");

  const enTree = { ...enMsg, common: { ...enMsg.common, ...localeCommon("en") } };
  const esTree = { ...esMsg, common: { ...esMsg.common, ...localeCommon("es") } };
  const esArTree = deepMerge(deepMerge(esTree, esArMsg), { common: localeCommon("es-ar") });

  return { en: enTree, es: esTree, "es-AR": esArTree };
}

function runValidator() {
  const trees = buildTrees();
  const en = flatten(trees.en, "", {});
  const enKeys = Object.keys(en);
  const enSet = new Set(enKeys);

  const hardErrors = [];
  const summary = {};
  for (const locale of ["es", "es-AR"]) {
    const flat = flatten(trees[locale], "", {});
    const set = new Set(Object.keys(flat));
    const missing = enKeys.filter((k) => !set.has(k));
    const extra = Object.keys(flat).filter((k) => !enSet.has(k));
    missing.forEach((k) => hardErrors.push({ locale, type: "missing_key", key: k }));
    extra.forEach((k) => hardErrors.push({ locale, type: "extra_key", key: k }));
    summary[locale] = { total: enKeys.length, missing: missing.length, extra: extra.length };
  }
  return { exitCode: hardErrors.length ? 1 : 0, hardErrors, summary, enTotal: enKeys.length };
}

function main() {
  const report = process.argv.includes("--report");
  const r = runValidator();

  console.log("Operator-locale validator (en is source of truth; es full, es-AR override)\n");
  console.log(`en keys: ${r.enTotal}`);
  for (const [loc, s] of Object.entries(r.summary)) {
    console.log(`  ${loc}: missing=${s.missing} extra=${s.extra}`);
  }
  if (r.hardErrors.length) {
    console.log(`\nHard errors (${r.hardErrors.length}):`);
    const show = report ? r.hardErrors : r.hardErrors.slice(0, 40);
    for (const e of show) console.log(`  [${e.locale}] ${e.type} ${e.key}`);
    if (!report && r.hardErrors.length > show.length) {
      console.log(`  ... and ${r.hardErrors.length - show.length} more (run with --report)`);
    }
  } else {
    console.log("\nOK — operator locales are at full parity.");
  }
  process.exit(r.exitCode);
}

if (require.main === module) main();
module.exports = { runValidator, buildTrees, flatten, deepMerge };
