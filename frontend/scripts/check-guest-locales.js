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
const DEFAULT_CRITICAL_PATH = path.resolve(
  __dirname,
  "..",
  "src",
  "i18n",
  "critical-guest-keys.json",
);
const DEFAULT_INVARIANTS_PATH = path.resolve(
  __dirname,
  "..",
  "src",
  "i18n",
  "translation-invariants.json",
);
const PLACEHOLDER_RE = /\{[a-zA-Z_][a-zA-Z0-9_]*\}/g;
// ICU plural/select escape blocks like `{{count, plural, one {item} other {items}}}`.
// The inner `{item}` `{items}` are ICU plural form options, not placeholders that
// translations must preserve verbatim. Strip these blocks before placeholder extraction.
const ICU_BLOCK_RE = /\{\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}\}/g;

function flatten(obj, prefix, out) {
  for (const [key, value] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (value && typeof value === "object" && !Array.isArray(value)) {
      flatten(value, path, out);
    } else {
      out[path] = value;
    }
  }
  return out;
}

function loadJson(filePath) {
  const raw = fs.readFileSync(filePath, "utf8");
  return JSON.parse(raw);
}

function loadAllowlist(dir) {
  const allowlistPath = path.join(dir, ".same-as-en-allowlist.json");
  if (!fs.existsSync(allowlistPath)) {
    return { all: [], perLocale: {} };
  }
  const raw = loadJson(allowlistPath);
  const all = Array.isArray(raw.all) ? raw.all : [];
  const perLocale = {};
  for (const [key, value] of Object.entries(raw)) {
    if (key === "all" || key === "_comment") continue;
    perLocale[key] = Array.isArray(value) ? value : [];
  }
  return { all, perLocale };
}

function placeholdersIn(value) {
  if (typeof value !== "string") return [];
  const stripped = value.replace(ICU_BLOCK_RE, "");
  return stripped.match(PLACEHOLDER_RE) || [];
}

function placeholderCounts(value) {
  const counts = {};
  for (const token of placeholdersIn(value)) {
    counts[token] = (counts[token] || 0) + 1;
  }
  return Object.fromEntries(
    Object.entries(counts).sort(([a], [b]) => a.localeCompare(b)),
  );
}

function guestErrorCodeIsConsumed(repoRoot, code) {
  const mappingFiles = [
    "frontend/src/lib/guestOrderErrors.ts",
    "frontend/src/lib/guestPaymentErrors.ts",
  ];
  let sawMappingFile = false;
  for (const relative of mappingFiles) {
    const file = path.join(repoRoot, relative);
    if (!fs.existsSync(file)) continue;
    sawMappingFile = true;
    const source = fs.readFileSync(file, "utf8");
    const escaped = code.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    if (
      source.includes(`"${code}"`) ||
      source.includes(`'${code}'`) ||
      new RegExp(`\\b${escaped}\\s*:`).test(source)
    )
      return true;
  }
  // Synthetic checker fixtures may intentionally omit the application tree.
  return !sawMappingFile;
}

function discoverPublicErrorCodes(repoRoot) {
  const sourceFiles = [
    "backend/internal/services/order_validation.go",
    "backend/internal/handlers/payments.go",
    // Guest USDC payer-binding responses (amount_mismatch, crypto_quote_*).
    "backend/internal/handlers/crypto_quote_binding.go",
    "backend/internal/handlers/splitting.go",
    "backend/internal/handlers/plugin_handlers.go",
  ];
  const codes = new Set();
  for (const relative of sourceFiles) {
    const file = path.join(repoRoot, relative);
    if (!fs.existsSync(file)) continue;
    const source = fs.readFileSync(file, "utf8");
    const patterns = [
      /OrderErrCode\w+\s*=\s*"([^"]+)"/g,
      /RespondWithError\([^\n]*?,\s*"([A-Za-z0-9_]+)"\s*,/g,
      /"code"\s*:\s*"([A-Za-z0-9_]+)"/g,
    ];
    for (const pattern of patterns) {
      for (const match of source.matchAll(pattern)) codes.add(match[1]);
    }
  }
  return [...codes].sort();
}

/**
 * Blocking launch-locale validator. Unlike the broad 21-locale parity check,
 * this intentionally covers only the reviewed launch-critical catalog and the
 * three launch locales. es-AR is an override layer at runtime, but every key in
 * this manifest must be explicit so inheritance cannot silently replace a
 * reviewed Argentine customer-facing string.
 */
function runCriticalValidator(opts = {}) {
  const dir = opts.dir || DEFAULT_DIR;
  const configPath = opts.configPath || DEFAULT_CRITICAL_PATH;
  const invariantsPath = opts.invariantsPath || DEFAULT_INVARIANTS_PATH;
  const hardErrors = [];

  let config;
  let invariantsFile;
  try {
    config = loadJson(configPath);
    invariantsFile = loadJson(invariantsPath);
  } catch (error) {
    return {
      exitCode: 1,
      hardErrors: [{ type: "critical_config_invalid", error: error.message }],
    };
  }

  const locales = Array.isArray(config.locales)
    ? config.locales
    : ["en", "es", "es-AR"];
  const domainEntries = Object.entries(config.domains || {});
  const criticalKeys = domainEntries.flatMap(([, keys]) =>
    Array.isArray(keys) ? keys : [],
  );
  const duplicateKeys = criticalKeys.filter(
    (key, index) => criticalKeys.indexOf(key) !== index,
  );
  for (const key of new Set(duplicateKeys)) {
    hardErrors.push({ type: "critical_duplicate_key", key });
  }

  const invariantEntries = Array.isArray(invariantsFile.invariants)
    ? invariantsFile.invariants
    : [];
  const invariantKeys = new Set();
  for (const invariant of invariantEntries) {
    if (
      !invariant ||
      typeof invariant.key !== "string" ||
      !Array.isArray(invariant.locales) ||
      typeof invariant.reason !== "string" ||
      invariant.reason.trim().length < 8
    ) {
      hardErrors.push({ type: "critical_invariant_invalid", invariant });
      continue;
    }
    for (const locale of invariant.locales) {
      invariantKeys.add(`${locale}:${invariant.key}`);
    }
  }

  const catalogs = {};
  for (const locale of locales) {
    const file = path.join(dir, `${locale}.json`);
    try {
      catalogs[locale] = flatten(loadJson(file), "", {});
    } catch (error) {
      hardErrors.push({
        locale,
        type: "critical_catalog_invalid",
        error: error.message,
      });
      catalogs[locale] = {};
    }
  }
  const source = catalogs.en || {};

  for (const key of criticalKeys) {
    const sourceValue = source[key];
    if (typeof sourceValue !== "string") {
      hardErrors.push({ locale: "en", type: "critical_missing_key", key });
      continue;
    }
    const expectedPlaceholders = placeholderCounts(sourceValue);
    for (const locale of locales.filter((candidate) => candidate !== "en")) {
      const value = catalogs[locale][key];
      if (typeof value !== "string") {
        hardErrors.push({
          locale,
          type:
            locale === "es-AR"
              ? "critical_inherited_unreviewed"
              : "critical_missing_key",
          key,
        });
        continue;
      }
      if (value === sourceValue && !invariantKeys.has(`${locale}:${key}`)) {
        hardErrors.push({ locale, type: "critical_same_as_en", key });
      }
      const actualPlaceholders = placeholderCounts(value);
      if (
        JSON.stringify(actualPlaceholders) !==
        JSON.stringify(expectedPlaceholders)
      ) {
        hardErrors.push({
          locale,
          type: "critical_placeholder_mismatch",
          key,
          expected: expectedPlaceholders,
          actual: actualPlaceholders,
        });
      }
    }
  }

  const configuredErrorCodes = config.publicErrorCodes || {};
  const repoRoot = opts.repoRoot || path.resolve(__dirname, "..", "..");
  const publicErrorCodes = Array.isArray(opts.publicErrorCodes)
    ? opts.publicErrorCodes
    : discoverPublicErrorCodes(repoRoot);
  for (const code of publicErrorCodes) {
    const key = configuredErrorCodes[code];
    if (typeof key !== "string") {
      hardErrors.push({ type: "public_error_code_unmapped", code });
      continue;
    }
    if (!criticalKeys.includes(key)) {
      hardErrors.push({ type: "public_error_mapping_not_critical", code, key });
    }
    if (!guestErrorCodeIsConsumed(repoRoot, code)) {
      hardErrors.push({ type: "public_error_code_not_consumed", code, key });
    }
  }

  return { exitCode: hardErrors.length > 0 ? 1 : 0, hardErrors };
}

const BASELINE_FILENAME = "guest-same-as-en-baseline.json";
const DEFAULT_BASELINE_PATH = path.resolve(__dirname, BASELINE_FILENAME);

// Computes the raw per-locale same-as-en count: the number of string keys whose
// value is byte-identical to en.json, BEFORE any allowlist adjustment. This is
// the metric the ratchet locks — it counts untranslated leaves regardless of
// whether they're allowlisted, so the backlog of real English-in-place can only
// shrink. (Allowlisting an invariant key, e.g. a currency symbol, does not lower
// this number; it only silences the advisory warning. That's intentional: the
// ratchet protects against NEW untranslated leaves, not against allowlist churn.)
function computeSameAsEnCounts(opts = {}) {
  const dir = opts.dir || DEFAULT_DIR;
  const en = loadJson(path.join(dir, "en.json"));
  const enFlat = flatten(en, "", {});
  const enKeys = Object.keys(enFlat);
  const files = fs
    .readdirSync(dir)
    .filter(
      (f) => f.endsWith(".json") && f !== "en.json" && !f.startsWith("."),
    );
  const counts = {};
  for (const file of files) {
    const locale = path.basename(file, ".json");
    let flat;
    try {
      flat = flatten(loadJson(path.join(dir, file)), "", {});
    } catch {
      continue;
    }
    let sameAsEn = 0;
    for (const key of enKeys) {
      const enValue = enFlat[key];
      const localeValue = flat[key];
      if (
        typeof enValue === "string" &&
        typeof localeValue === "string" &&
        localeValue === enValue
      ) {
        sameAsEn++;
      }
    }
    counts[locale] = sameAsEn;
  }
  return counts;
}

// Loads the committed ratchet baseline ({ locale: maxSameAsEnCount }). A missing
// file disables the ratchet (returns null) so the gate stays usable before the
// baseline is first generated.
function loadBaseline(baselinePath = DEFAULT_BASELINE_PATH) {
  if (!fs.existsSync(baselinePath)) return null;
  const raw = loadJson(baselinePath);
  // Tolerate an optional `_comment` string sibling to the per-locale counts.
  const out = {};
  for (const [k, v] of Object.entries(raw)) {
    if (k === "_comment") continue;
    if (typeof v === "number") out[k] = v;
  }
  return out;
}

// Regenerates and writes the ratchet baseline from the current tree.
function writeBaseline(opts = {}) {
  const dir = opts.dir || DEFAULT_DIR;
  const baselinePath = opts.baselinePath || DEFAULT_BASELINE_PATH;
  const counts = computeSameAsEnCounts({ dir });
  const ordered = {};
  ordered._comment =
    "GUEST-3 ratchet baseline: max allowed same-as-en (untranslated-leaf) count per locale. " +
    "The validator (--strict / --ratchet) FAILS if any locale's current count EXCEEDS its baseline, " +
    "so the backlog can only shrink, never grow. After translating keys, regenerate with " +
    "`node scripts/check-guest-locales.js --write-baseline`. A locale missing here is treated as 0.";
  for (const loc of Object.keys(counts).sort()) {
    ordered[loc] = counts[loc];
  }
  fs.writeFileSync(baselinePath, JSON.stringify(ordered, null, 2) + "\n");
  return counts;
}

function runValidator(opts) {
  const dir = opts.dir || DEFAULT_DIR;
  const strict = Boolean(opts.strict);
  const coverageThreshold =
    typeof opts.coverageThreshold === "number" ? opts.coverageThreshold : 95;

  const hardErrors = [];
  const softWarnings = [];
  const coverage = {};

  const enPath = path.join(dir, "en.json");
  if (!fs.existsSync(enPath)) {
    hardErrors.push({ locale: "en", type: "missing_source_file" });
    return { exitCode: 1, hardErrors, softWarnings, coverage };
  }

  let enFlat;
  try {
    const en = loadJson(enPath);
    enFlat = flatten(en, "", {});
  } catch (err) {
    hardErrors.push({ locale: "en", type: "parse_error", error: err.message });
    return { exitCode: 1, hardErrors, softWarnings, coverage };
  }
  const enKeys = Object.keys(enFlat);

  const allowlist = loadAllowlist(dir);

  const files = fs
    .readdirSync(dir)
    .filter(
      (f) => f.endsWith(".json") && f !== "en.json" && !f.startsWith("."),
    );

  for (const file of files) {
    const locale = path.basename(file, ".json");
    const filePath = path.join(dir, file);
    let flat;
    try {
      flat = flatten(loadJson(filePath), "", {});
    } catch (err) {
      hardErrors.push({ locale, type: "parse_error", error: err.message });
      continue;
    }
    const localeKeys = new Set(Object.keys(flat));
    const allowed = new Set([
      ...allowlist.all,
      ...(allowlist.perLocale[locale] || []),
    ]);

    let translated = 0;
    let sameAsEn = 0;
    let sameAsEnAllowed = 0;

    for (const key of enKeys) {
      if (!localeKeys.has(key)) {
        hardErrors.push({ locale, type: "missing_key", key });
        continue;
      }
      const enValue = enFlat[key];
      const localeValue = flat[key];
      if (typeof enValue === "string" && typeof localeValue === "string") {
        const enPlaceholders = placeholdersIn(enValue);
        const localePlaceholders = new Set(placeholdersIn(localeValue));
        const missing = enPlaceholders.filter(
          (p) => !localePlaceholders.has(p),
        );
        if (missing.length > 0) {
          hardErrors.push({
            locale,
            type: "placeholder_mismatch",
            key,
            missing,
          });
        }
        if (localeValue === enValue) {
          sameAsEn++;
          if (allowed.has(key)) {
            sameAsEnAllowed++;
          } else {
            // An English leaf in a non-English bundle is untranslated copy a
            // diner sees: a hard error. Genuinely identical words (cognates,
            // brand names, legal terms) go in .same-as-en-allowlist.json.
            hardErrors.push({ locale, type: "same_as_en", key });
          }
        } else {
          translated++;
        }
      }
    }

    for (const key of localeKeys) {
      if (!(key in enFlat)) {
        hardErrors.push({ locale, type: "extra_key", key });
      }
    }

    // "Healthy" = translated + same-as-en values that are explicitly allowlisted.
    // The denominator is the full key set so the metric stays comparable across
    // locales and cannot exceed 100%.
    const healthy = translated + sameAsEnAllowed;
    const coveragePct =
      enKeys.length > 0 ? (healthy / enKeys.length) * 100 : 100;
    coverage[locale] = {
      total: enKeys.length,
      translated,
      sameAsEn,
      allowed: allowed.size,
      coverage: Number(coveragePct.toFixed(1)),
    };

    if (strict && coveragePct < coverageThreshold) {
      softWarnings.push({
        locale,
        type: "coverage_below_threshold",
        coverage: Number(coveragePct.toFixed(1)),
        threshold: coverageThreshold,
      });
    }
  }

  // GUEST-3 ratchet: when a baseline is supplied, each locale's RAW same-as-en
  // count must not EXCEED its baseline. A locale absent from the baseline is
  // treated as 0, so a brand-new bundle can't smuggle in a fresh backlog. This
  // is a HARD error (fatal) — unlike the advisory same-as-en warning — because
  // it's a true regression: untranslated leaves grew. Counts can only go down;
  // after translating keys, regenerate the baseline with --write-baseline.
  if (opts.ratchetBaseline && typeof opts.ratchetBaseline === "object") {
    for (const [locale, data] of Object.entries(coverage)) {
      const baseline =
        typeof opts.ratchetBaseline[locale] === "number"
          ? opts.ratchetBaseline[locale]
          : 0;
      if (data.sameAsEn > baseline) {
        hardErrors.push({
          locale,
          type: "ratchet_exceeded",
          count: data.sameAsEn,
          baseline,
        });
      }
    }
  }

  // The coverage threshold is a metric, not a correctness failure, so it
  // stays advisory under --strict. Missing/extra keys, placeholder mismatches
  // and non-allowlisted same-as-en leaves are hard errors (exit 1) in every
  // mode.
  const STRICT_NONFATAL_WARNING_TYPES = new Set(["coverage_below_threshold"]);

  let exitCode = 0;
  if (hardErrors.length > 0) {
    exitCode = 1;
  } else if (strict) {
    const fatalSoft = softWarnings.filter(
      (w) => !STRICT_NONFATAL_WARNING_TYPES.has(w.type),
    );
    if (fatalSoft.length > 0) exitCode = 2;
  }

  return { exitCode, hardErrors, softWarnings, coverage };
}

function formatReport(result, mode) {
  const lines = [];
  lines.push("Guest-locale validator report");
  lines.push("");
  lines.push("Coverage by locale:");
  for (const [locale, data] of Object.entries(result.coverage)) {
    lines.push(
      `  ${locale.padEnd(4)} ${String(data.coverage).padStart(6)}%  translated=${data.translated} same-as-en=${data.sameAsEn} allowed=${data.allowed}`,
    );
  }
  if (result.hardErrors.length > 0) {
    lines.push("");
    lines.push(`Hard errors (${result.hardErrors.length}):`);
    for (const e of result.hardErrors.slice(0, 50)) {
      if (e.type === "ratchet_exceeded") {
        lines.push(
          `  [${e.locale}] ratchet_exceeded same-as-en=${e.count} > baseline=${e.baseline} (translate keys or this can't merge; regenerate baseline only when count DROPS)`,
        );
      } else {
        lines.push(
          `  [${e.locale}] ${e.type} ${e.key || ""} ${e.missing ? JSON.stringify(e.missing) : ""}`,
        );
      }
    }
    if (result.hardErrors.length > 50) {
      lines.push(`  ... and ${result.hardErrors.length - 50} more`);
    }
  }
  if (
    mode === "report" ||
    (result.softWarnings.length > 0 && mode === "strict")
  ) {
    lines.push("");
    lines.push(`Soft warnings (${result.softWarnings.length}):`);
    for (const w of result.softWarnings.slice(0, 100)) {
      if (w.type === "same_as_en") {
        lines.push(`  [${w.locale}] same-as-en  ${w.key}`);
      } else if (w.type === "coverage_below_threshold") {
        lines.push(
          `  [${w.locale}] coverage ${w.coverage}% < threshold ${w.threshold}%`,
        );
      } else {
        lines.push(`  [${w.locale}] ${w.type} ${w.key || ""}`);
      }
    }
    if (result.softWarnings.length > 100) {
      lines.push(`  ... and ${result.softWarnings.length - 100} more`);
    }
  }
  return lines.join("\n");
}

// Guest-facing source dirs whose literal t("...") keys must exist in en.json.
// Limitation: wrapper-composed keys (tr(), reservationT(), prefix-built keys)
// are not resolvable statically and are not checked here.
const GUEST_SOURCE_DIRS = [
  "src/components/guest",
  "src/components/business-page",
  "src/components/payment",
  "src/components/splitting",
  "src/components/navigation",
  "src/components/delivery",
  "src/components/customer",
  "src/components/menu",
  "src/components/receipt",
  "src/app/t",
  "src/app/b",
  "src/app/delivery",
  "src/app/reservations",
];

const T_CALL_RE = /\bt\(\s*["']([A-Za-z0-9_.-]+)["']/g;

// Detects files that define a local `t` wrapper which composes a prefix before
// delegating to the real translation function (e.g. `const t = (key) => guestT(`crm.${key}`)`).
// T_CALL_RE would match the bare suffix keys (e.g. "joinFailed") in these files,
// but those bare keys don't exist in en.json — only the fully-qualified
// "crm.joinFailed" does. Skipping t("...") call-sites in such files prevents
// permanent false positives while honouring the documented limitation that
// "wrapper-composed keys are not resolvable statically and are not checked".
//
// Detection: the file text contains a local `const t` binding whose body uses a
// template literal of the form `guestT(`<prefix>.\${` — i.e. it prepends a static
// prefix to the key argument.
const LOCAL_T_WRAPPER_RE =
  /\bconst\s+t\b[\s\S]{0,300}guestT\(`[A-Za-z][A-Za-z0-9_.]*\.\$\{/;

/**
 * Returns true if the file content defines a local `t` function that is a
 * prefix-composing wrapper around the real guest translation function.
 * Call-sites of such a local `t` pass only the bare suffix, which cannot be
 * looked up directly in en.json — they must be skipped by the key checker.
 */
function hasLocalTWrapper(content) {
  return LOCAL_T_WRAPPER_RE.test(content);
}

function collectGuestKeys(frontendRoot) {
  const found = []; // { key, file, line }
  for (const dir of GUEST_SOURCE_DIRS) {
    const abs = path.join(frontendRoot, dir);
    if (!fs.existsSync(abs)) continue;
    const stack = [abs];
    while (stack.length) {
      const cur = stack.pop();
      for (const entry of fs.readdirSync(cur, { withFileTypes: true })) {
        const p = path.join(cur, entry.name);
        if (entry.isDirectory()) stack.push(p);
        else if (
          /\.(ts|tsx)$/.test(entry.name) &&
          !/\.test\./.test(entry.name)
        ) {
          const content = fs.readFileSync(p, "utf8");
          // Skip files where t is a local prefix-composing wrapper: the bare
          // keys captured by T_CALL_RE would be false positives because the
          // real en.json key includes the prepended prefix (e.g. "crm.joinFailed").
          if (hasLocalTWrapper(content)) continue;
          const lines = content.split("\n");
          lines.forEach((line, i) => {
            for (const m of line.matchAll(T_CALL_RE)) {
              found.push({
                key: m[1],
                file: path.relative(frontendRoot, p),
                line: i + 1,
              });
            }
          });
        }
      }
    }
  }
  return found;
}

function runKeysCheck(opts = {}) {
  const dir = opts.dir || DEFAULT_DIR;
  const frontendRoot = path.join(__dirname, "..");
  const enFlat = flatten(loadJson(path.join(dir, "en.json")), "", {});
  // Filter out prefix-built keys (e.g. `t("menu." + allergen.name)`): the regex
  // captures only the literal prefix portion ending with ".", which can never
  // exist in en.json. Skip them rather than reporting permanent false positives.
  const used = collectGuestKeys(frontendRoot).filter(
    ({ key }) => !key.endsWith("."),
  );
  const missing = used.filter(({ key }) => !(key in enFlat));
  return { missing, usedCount: used.length };
}

// NEW-3 / MIN-7: cookie banner + menu.viewMode.label must exist in every
// guest-messages locale (not only en). Hard-fail if any file is missing them.
const COOKIE_AND_VIEWMODE_KEYS = [
  "cookies.banner.title",
  "cookies.banner.description",
  "cookies.banner.acceptAll",
  "cookies.banner.declineAll",
  "cookies.banner.customize",
  "cookies.banner.essentialLabel",
  "cookies.banner.essentialDescription",
  "cookies.banner.analyticsLabel",
  "cookies.banner.analyticsDescription",
  "cookies.banner.marketingLabel",
  "cookies.banner.marketingDescription",
  "cookies.banner.savePreferences",
  "cookies.banner.privacyLink",
  "cookies.banner.closeLabel",
  "cookies.footer.preferences",
  "menu.viewMode.label",
];

function runCookieAndViewModeCheck(opts = {}) {
  const dir = opts.dir || DEFAULT_DIR;
  const files = fs
    .readdirSync(dir)
    .filter((f) => f.endsWith(".json") && !f.startsWith("."));
  const hardErrors = [];
  for (const file of files) {
    const locale = path.basename(file, ".json");
    let flat;
    try {
      flat = flatten(loadJson(path.join(dir, file)), "", {});
    } catch (error) {
      hardErrors.push({ locale, type: "parse_error", error: error.message });
      continue;
    }
    for (const key of COOKIE_AND_VIEWMODE_KEYS) {
      if (typeof flat[key] !== "string" || flat[key].trim() === "") {
        hardErrors.push({ locale, type: "missing_cookie_or_viewmode_key", key });
      }
    }
    // MIN-7: non-en locales must not leave menu.viewMode.label in English.
    if (
      locale !== "en" &&
      flat["menu.viewMode.label"] === "Menu view"
    ) {
      hardErrors.push({
        locale,
        type: "untranslated_viewmode_label",
        key: "menu.viewMode.label",
      });
    }
  }
  return { exitCode: hardErrors.length > 0 ? 1 : 0, hardErrors };
}

if (require.main === module) {
  const args = process.argv.slice(2);

  if (args.includes("--keys")) {
    const { missing, usedCount } = runKeysCheck();
    if (missing.length > 0) {
      console.error(
        `Key-existence check: ${missing.length}/${usedCount} literal t() keys missing from en.json:`,
      );
      for (const m of missing) console.error(`  ${m.file}:${m.line}  ${m.key}`);
      process.exit(1);
    }
    console.log(
      `Key-existence check: all ${usedCount} literal t() keys exist in en.json.`,
    );
    process.exit(0);
  }

  if (args.includes("--cookies") || args.includes("--viewmode")) {
    const result = runCookieAndViewModeCheck();
    if (result.exitCode !== 0) {
      console.error(
        `Cookie/viewMode guest-locale gate failed (${result.hardErrors.length}):`,
      );
      for (const error of result.hardErrors)
        console.error(`  ${JSON.stringify(error)}`);
      process.exit(1);
    }
    console.log(
      "Cookie/viewMode guest-locale gate: all 21 locales define cookies.* and translated menu.viewMode.label.",
    );
    process.exit(0);
  }

  if (args.includes("--critical")) {
    const result = runCriticalValidator();
    if (result.hardErrors.length > 0) {
      console.error(
        `Critical guest-locale gate failed (${result.hardErrors.length}):`,
      );
      for (const error of result.hardErrors)
        console.error(`  ${JSON.stringify(error)}`);
      process.exit(1);
    }
    console.log(
      "Critical guest-locale gate: reviewed en/es/es-AR keys and public error mappings pass.",
    );
    process.exit(0);
  }

  // GUEST-3: (re)generate the committed ratchet baseline from the current tree.
  // Run after translating keys so the baseline captures the new (lower) counts.
  if (args.includes("--write-baseline")) {
    const counts = writeBaseline();
    console.log(`Wrote ${BASELINE_FILENAME}:`);
    for (const loc of Object.keys(counts).sort()) {
      console.log(`  ${loc.padEnd(6)} same-as-en=${counts[loc]}`);
    }
    process.exit(0);
  }

  const strict = args.includes("--strict");
  const reportMode = args.includes("--report");
  // GUEST-3: --ratchet runs the same-as-en backlog gate. It's also folded into
  // --strict so CI's existing strict invocation enforces the ratchet for free.
  // --no-ratchet opts a strict run out (e.g. while bulk-editing locally).
  const ratchet =
    (args.includes("--ratchet") || strict) && !args.includes("--no-ratchet");
  const thresholdArg = args.find((a) => a.startsWith("--threshold="));
  const coverageThreshold = thresholdArg
    ? Number(thresholdArg.split("=")[1])
    : 95;
  const ratchetBaseline = ratchet ? loadBaseline() : null;
  if (ratchet && ratchetBaseline === null) {
    console.warn(
      `Ratchet requested but ${BASELINE_FILENAME} is missing — skipping ratchet. Run --write-baseline to create it.`,
    );
  }
  const result = runValidator({
    strict: strict || reportMode || ratchet,
    coverageThreshold,
    ratchetBaseline,
  });
  console.log(
    formatReport(result, reportMode ? "report" : strict ? "strict" : "default"),
  );
  process.exit(result.exitCode);
}

module.exports = {
  runValidator,
  runCriticalValidator,
  runCookieAndViewModeCheck,
  discoverPublicErrorCodes,
  flatten,
  placeholdersIn,
  runKeysCheck,
  computeSameAsEnCounts,
  loadBaseline,
  writeBaseline,
  BASELINE_FILENAME,
};
