#!/usr/bin/env node
// Licence gate and report for the frontend's production dependency tree.
//
//   node scripts/licenses/npm-licenses.mjs check    # exit 1 on a denied licence
//   node scripts/licenses/npm-licenses.mjs report   # Markdown section on stdout
//
// Options: --frontend <dir> (default: frontend/), --overrides <tsv>
// (default: scripts/licenses/npm-overrides.tsv).
//
// The dependency set comes from frontend/package-lock.json (lockfileVersion 3).
// Every package entry that is not dev/devOptional counts as a production
// dependency. That includes optional platform binaries that never install on
// the machine running the check, such as the linux-musl sharp/libvips builds the
// alpine image pulls. A package's licence comes from the lockfile, then from the
// installed package.json (same version only), then from a `license` row in the
// overrides file. A package with no licence from any of these sources fails the
// check; it is never treated as fine.
//
// Policy, evaluated on the SPDX expression (OR takes the best branch, AND the
// worst):
//   permissive  allowed (MIT, BSD, Apache-2.0, ISC, ...)
//   review      allowed only with an `allow` row naming the package and the
//               exact expression (weak copyleft, attribution-required data,
//               source-available, LicenseRef-*, any identifier not listed here)
//   forbidden   always fails, whatever the overrides say (GPL, AGPL, SSPL,
//               BUSL, CC *-NC / *-ND, Commons-Clause, UNLICENSED)

import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(HERE, "..", "..");

export const PERMISSIVE = new Set([
  "0BSD",
  "Apache-2.0",
  "BlueOak-1.0.0",
  "BSD-2-Clause",
  "BSD-3-Clause",
  "BSD-3-Clause-Clear",
  "BSL-1.0", // Boost Software License (not BUSL)
  "CC0-1.0",
  "ISC",
  "MIT",
  "MIT-0",
  "Python-2.0",
  "Unicode-3.0",
  "Unicode-DFS-2016",
  "Unlicense",
  "W3C",
  "WTFPL",
  "X11",
  "Zlib",
]);

const FORBIDDEN = [
  /^A?GPL(-|\+|$)/i, // GPL-*, AGPL-* (LGPL is weak copyleft: review)
  /^SSPL(-|$)/i,
  /^BUSL(-|$)/i,
  /^CC-.*-(NC|ND)(-|$)/i,
  /^Commons-Clause$/i,
  /^UNLICENSED$/i,
];

// Linking exceptions that turn a GPL-family base into something that can ship
// with a permissive project after review.
const LINKING_EXCEPTIONS = new Set([
  "Classpath-exception-2.0",
  "GCC-exception-3.1",
  "LLVM-exception",
]);

const RANK = { permissive: 0, review: 1, forbidden: 2 };

// SPDX identifiers are case-insensitive.
const PERMISSIVE_LOWER = new Set([...PERMISSIVE].map((id) => id.toLowerCase()));

export function classifyId(id) {
  if (PERMISSIVE_LOWER.has(id.toLowerCase())) return "permissive";
  if (FORBIDDEN.some((pattern) => pattern.test(id))) return "forbidden";
  return "review";
}

// Normalise the many shapes package.json authors use into one SPDX-ish string.
export function normaliseLicense(value) {
  if (value == null) return null;
  if (Array.isArray(value)) {
    const parts = value.map(normaliseLicense).filter(Boolean);
    if (parts.length === 0) return null;
    return parts.length === 1 ? parts[0] : `(${parts.join(" OR ")})`;
  }
  if (typeof value === "object") return normaliseLicense(value.type ?? null);
  const text = String(value).trim();
  if (!text) return null;
  return text.replace(/SEE LICEN[CS]E IN\s+\S+/gi, "LicenseRef-see-file");
}

function tokenize(expression) {
  const tokens = [];
  const re = /\s*(\(|\)|[^\s()]+)/g;
  let match;
  while ((match = re.exec(expression)) !== null) {
    const raw = match[1];
    const upper = raw.toUpperCase();
    tokens.push(upper === "AND" || upper === "OR" || upper === "WITH" ? upper : raw);
  }
  return tokens;
}

// Parse an SPDX expression into a tree of {op, args} / {id, exception}.
export function parseExpression(expression) {
  const tokens = tokenize(expression);
  let pos = 0;
  const peek = () => tokens[pos];
  const next = () => tokens[pos++];

  function parseOr() {
    const args = [parseAnd()];
    while (peek() === "OR") {
      next();
      args.push(parseAnd());
    }
    return args.length === 1 ? args[0] : { op: "OR", args };
  }
  function parseAnd() {
    const args = [parseWith()];
    while (peek() === "AND") {
      next();
      args.push(parseWith());
    }
    return args.length === 1 ? args[0] : { op: "AND", args };
  }
  function parseWith() {
    const node = parsePrimary();
    if (peek() === "WITH") {
      next();
      const exception = next();
      if (!exception || exception === "(" || exception === ")") {
        throw new Error(`bad WITH clause in "${expression}"`);
      }
      return { ...node, exception };
    }
    return node;
  }
  function parsePrimary() {
    const token = next();
    if (token === "(") {
      const node = parseOr();
      if (next() !== ")") throw new Error(`unbalanced parentheses in "${expression}"`);
      return node;
    }
    if (!token || token === ")" || token === "AND" || token === "OR" || token === "WITH") {
      throw new Error(`unexpected token ${token ?? "<end>"} in "${expression}"`);
    }
    return { id: token };
  }

  const tree = parseOr();
  if (pos !== tokens.length) throw new Error(`trailing tokens in "${expression}"`);
  return tree;
}

// Evaluate to { category, ids } where ids are the identifiers the chosen
// branch actually binds us to.
export function evaluate(expression) {
  let tree;
  try {
    tree = parseExpression(expression);
  } catch {
    return { category: "review", ids: [expression] };
  }
  return evaluateNode(tree);
}

function evaluateNode(node) {
  if (node.id) {
    const base = node.id.replace(/\+$/, "");
    let category = classifyId(base);
    if (node.exception && category === "forbidden" && LINKING_EXCEPTIONS.has(node.exception)) {
      category = "review";
    }
    const label = node.exception ? `${node.id} WITH ${node.exception}` : node.id;
    return { category, ids: [label] };
  }
  const results = node.args.map(evaluateNode);
  if (node.op === "OR") {
    return results.reduce((best, r) => (RANK[r.category] < RANK[best.category] ? r : best));
  }
  const worst = results.reduce((w, r) => (RANK[r.category] > RANK[w.category] ? r : w));
  return { category: worst.category, ids: results.flatMap((r) => r.ids) };
}

function globToRegExp(glob) {
  const escaped = glob.replace(/[.+?^${}()|[\]\\]/g, "\\$&").replace(/\*/g, ".*");
  return new RegExp(`^${escaped}$`);
}

// Overrides TSV: kind<TAB>package-glob<TAB>license<TAB>reason. `#` comments.
export function parseOverrides(text) {
  const rows = [];
  text.split(/\r?\n/).forEach((line, index) => {
    if (!line.trim() || line.trimStart().startsWith("#")) return;
    const [kind, pattern, license, reason] = line.split("\t");
    if (!["license", "allow"].includes(kind) || !pattern || !license || !reason?.trim()) {
      throw new Error(`npm overrides line ${index + 1}: expected kind, package, license, reason`);
    }
    rows.push({ kind, pattern, license: license.trim(), reason: reason.trim(), re: globToRegExp(pattern), used: 0 });
  });
  return rows;
}

export function collectPackages(lock, { frontendDir = null } = {}) {
  if (!lock || lock.lockfileVersion < 2 || !lock.packages) {
    throw new Error("package-lock.json must be lockfileVersion 2 or 3 (needs the packages map)");
  }
  const packages = [];
  for (const [key, entry] of Object.entries(lock.packages)) {
    if (!key || entry.link || entry.dev || entry.devOptional) continue;
    const name = entry.name ?? key.slice(key.lastIndexOf("node_modules/") + "node_modules/".length);
    let license = normaliseLicense(entry.license);
    let source = license ? "lockfile" : null;
    const notice = frontendDir ? findNoticeFile(path.join(frontendDir, key)) : null;
    if (!license && frontendDir) {
      const manifest = path.join(frontendDir, key, "package.json");
      if (existsSync(manifest)) {
        try {
          const pkg = JSON.parse(readFileSync(manifest, "utf8"));
          if (pkg.version === entry.version) {
            license = normaliseLicense(pkg.license ?? pkg.licenses ?? null);
            if (license) source = "package.json";
          }
        } catch {
          // unreadable manifest: fall through to overrides
        }
      }
    }
    packages.push({ key, name, version: entry.version, license, source, notice, optional: Boolean(entry.optional) });
  }
  return packages;
}

const NOTICE_FILE = /^NOTICE(\.(txt|md))?$/i;

function findNoticeFile(dir) {
  try {
    return readdirSync(dir).find((file) => NOTICE_FILE.test(file)) ?? null;
  } catch {
    return null; // not installed on this machine
  }
}

// Characters that can be part of an npm package name or a Go module path.
// Keep in sync with lic_notice_names in lib.sh.
const NAME_CHARS = "A-Za-z0-9._~@/-";

// True when `name` appears in `text` as a whole name, not inside a longer one:
// "yaml" does not match "yaml.v3", "sharp" does not match "sharp-libvips", and
// "@scope/pkg" does not match "@scope/pkg-extra". A URL ("https://host/name")
// and a sentence-ending period right after the name still count.
export function mentionsName(text, name) {
  const escaped = name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(`(?:^|[^${NAME_CHARS}]|://)${escaped}(?:\\.?(?:[^${NAME_CHARS}]|$))`, "m").test(text);
}

// Apache-2.0 section 4(d): a dependency's NOTICE must be reproduced when we
// redistribute it. Each installed production package that ships a NOTICE file
// must be named in the root NOTICE.
export function checkNotices(packages, rootNotice) {
  const missing = new Map();
  for (const pkg of packages) {
    if (pkg.notice && !mentionsName(rootNotice, pkg.name)) missing.set(pkg.name, `${pkg.key}/${pkg.notice}`);
  }
  return [...missing.entries()].map(
    ([name, file]) => `${name}: ships ${file}; reproduce its notice in the root NOTICE file`,
  );
}

export function checkPackages(packages, overrides) {
  const errors = [];
  const results = [];
  for (const pkg of packages) {
    let { license, source } = pkg;
    if (!license) {
      const row = overrides.find((o) => o.kind === "license" && o.re.test(pkg.name));
      if (row) {
        row.used += 1;
        license = row.license;
        source = "override";
      }
    }
    if (!license) {
      errors.push(`${pkg.name}@${pkg.version}: no licence declared (add a \`license\` row after checking the registry)`);
      results.push({ ...pkg, license: "UNKNOWN", source: "none", category: "forbidden" });
      continue;
    }
    const { category, ids } = evaluate(license);
    let allow = null;
    if (category === "review") {
      allow = overrides.find((o) => o.kind === "allow" && o.re.test(pkg.name) && o.license === license);
      if (allow) allow.used += 1;
      else errors.push(`${pkg.name}@${pkg.version}: "${license}" needs review (add an \`allow\` row with the reason)`);
    } else if (category === "forbidden") {
      errors.push(`${pkg.name}@${pkg.version}: "${license}" is not allowed in production dependencies (${ids.join(", ")})`);
    }
    results.push({ ...pkg, license, source, category, allow });
  }
  const stale = overrides.filter((o) => o.used === 0).map((o) => `${o.kind}\t${o.pattern}\t${o.license}`);
  return { errors, results, stale };
}

function loadLock(frontendDir) {
  const lockPath = path.join(frontendDir, "package-lock.json");
  return JSON.parse(readFileSync(lockPath, "utf8"));
}

function escapeCell(text) {
  return String(text).replace(/\\/g, "\\\\").replace(/\|/g, "\\|");
}

export function renderReport(results) {
  const seen = new Map();
  for (const r of results) {
    const id = `${r.name}@${r.version}`;
    if (!seen.has(id)) seen.set(id, r);
  }
  // Code-point order, not localeCompare, so the output is identical everywhere.
  const cmp = (x, y) => (x < y ? -1 : x > y ? 1 : 0);
  const rows = [...seen.values()].sort((a, b) =>
    a.name === b.name ? cmp(String(a.version), String(b.version)) : cmp(a.name, b.name),
  );
  const lines = [];
  lines.push("| Package | Version | License |");
  lines.push("| --- | --- | --- |");
  for (const r of rows) {
    const url = `https://www.npmjs.com/package/${r.name}/v/${r.version}`;
    lines.push(`| [${escapeCell(r.name)}](${url}) | ${escapeCell(r.version)} | ${escapeCell(r.license)} |`);
  }
  const reviewed = new Map();
  for (const r of rows) {
    if (!r.allow) continue;
    const key = `${r.allow.pattern}\t${r.allow.license}`;
    if (!reviewed.has(key)) reviewed.set(key, { ...r.allow, packages: [] });
    reviewed.get(key).packages.push(`${r.name}@${r.version}`);
  }
  const notes = [];
  if (reviewed.size > 0) {
    notes.push("| Packages | License | Why it is acceptable |");
    notes.push("| --- | --- | --- |");
    for (const entry of reviewed.values()) {
      notes.push(`| ${escapeCell(entry.packages.join(", "))} | ${escapeCell(entry.license)} | ${escapeCell(entry.reason)} |`);
    }
  }
  return { table: lines.join("\n"), reviewed: notes.join("\n"), count: rows.length };
}

function parseArgs(argv) {
  const opts = {
    mode: argv[0],
    frontendDir: path.join(ROOT, "frontend"),
    overridesPath: path.join(HERE, "npm-overrides.tsv"),
  };
  for (let i = 1; i < argv.length; i += 1) {
    if (argv[i] === "--frontend") opts.frontendDir = path.resolve(argv[++i]);
    else if (argv[i] === "--overrides") opts.overridesPath = path.resolve(argv[++i]);
    else throw new Error(`unknown argument ${argv[i]}`);
  }
  if (!["check", "report"].includes(opts.mode)) {
    throw new Error("usage: npm-licenses.mjs check|report [--frontend DIR] [--overrides TSV]");
  }
  return opts;
}

function main(argv) {
  const opts = parseArgs(argv);
  const overrides = parseOverrides(readFileSync(opts.overridesPath, "utf8"));
  const packages = collectPackages(loadLock(opts.frontendDir), { frontendDir: opts.frontendDir });
  const { errors, results, stale } = checkPackages(packages, overrides);

  if (opts.mode === "report") {
    if (errors.length > 0) {
      for (const e of errors) console.error(`npm-licenses: ${e}`);
      return 1;
    }
    const { table, reviewed, count } = renderReport(results);
    process.stdout.write(`${count} distinct packages (name@version).\n\n${table}\n`);
    if (reviewed) {
      process.stdout.write(`\n#### Reviewed non-permissive licences\n\n${reviewed}\n`);
    }
    return 0;
  }

  for (const s of stale) console.error(`npm-licenses: warning: override row matched nothing: ${s}`);
  const rootNotice = path.join(ROOT, "NOTICE");
  if (existsSync(rootNotice)) {
    errors.push(...checkNotices(packages, readFileSync(rootNotice, "utf8")));
  } else {
    errors.push("root NOTICE file is missing");
  }
  if (errors.length > 0) {
    for (const e of errors) console.error(`npm-licenses: ${e}`);
    console.error(`npm-licenses: FAIL (${errors.length} problem(s) in ${packages.length} production packages)`);
    return 1;
  }
  const bySource = results.reduce((acc, r) => ({ ...acc, [r.source]: (acc[r.source] ?? 0) + 1 }), {});
  const sources = Object.entries(bySource)
    .map(([k, v]) => `${k}=${v}`)
    .join(" ");
  console.log(`npm-licenses: ok (${packages.length} production packages; licence source ${sources})`);
  return 0;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    process.exitCode = main(process.argv.slice(2));
  } catch (error) {
    console.error(`npm-licenses: ${error.message}`);
    process.exitCode = 2;
  }
}
