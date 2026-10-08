#!/usr/bin/env node
/* eslint-disable no-console */
"use strict";

const fs = require("fs");
const path = require("path");

const DEFAULT_DIR = path.resolve(
  __dirname,
  "..",
  "src",
  "i18n",
  "guest-messages",
);

function loadJson(filePath) {
  return JSON.parse(fs.readFileSync(filePath, "utf8"));
}

// Walk en + locale in parallel using en's key order. For any key absent in
// locale, take the en value. The output preserves en's order so diffs are
// stable across runs.
function mergeUsingEnAsTemplate(en, locale, insertedKeys, prefix) {
  if (en === null || typeof en !== "object" || Array.isArray(en)) {
    return locale === undefined ? en : locale;
  }
  const out = {};
  for (const key of Object.keys(en)) {
    const fullKey = prefix ? `${prefix}.${key}` : key;
    const enValue = en[key];
    const localeValue = locale && typeof locale === "object" ? locale[key] : undefined;
    if (enValue && typeof enValue === "object" && !Array.isArray(enValue)) {
      out[key] = mergeUsingEnAsTemplate(
        enValue,
        localeValue,
        insertedKeys,
        fullKey,
      );
    } else if (localeValue === undefined) {
      out[key] = enValue;
      insertedKeys.push(fullKey);
    } else {
      out[key] = localeValue;
    }
  }
  return out;
}

function runBackfill(opts) {
  const dir = opts.dir || DEFAULT_DIR;
  const enPath = path.join(dir, "en.json");
  const en = loadJson(enPath);
  const inserted = {};
  const files = fs
    .readdirSync(dir)
    .filter((f) => f.endsWith(".json") && f !== "en.json" && !f.startsWith("."));
  for (const file of files) {
    const locale = path.basename(file, ".json");
    const filePath = path.join(dir, file);
    const current = loadJson(filePath);
    const keys = [];
    const merged = mergeUsingEnAsTemplate(en, current, keys, "");
    inserted[locale] = keys;
    if (keys.length > 0) {
      fs.writeFileSync(filePath, JSON.stringify(merged, null, 2) + "\n");
    }
  }
  return { inserted };
}

if (require.main === module) {
  const result = runBackfill({});
  let total = 0;
  for (const [locale, keys] of Object.entries(result.inserted)) {
    if (keys.length > 0) {
      console.log(`  ${locale}: backfilled ${keys.length} keys`);
      total += keys.length;
    }
  }
  console.log(`Total: ${total} keys inserted across ${Object.keys(result.inserted).length} locales.`);
}

module.exports = { runBackfill, mergeUsingEnAsTemplate };
