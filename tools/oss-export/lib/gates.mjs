// Fatal gates over the exported tree. A gate never reports a matched value:
// hits carry a path, a line (when known) and a rule name only.

import { spawn, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";

import { compileGlobs, escapeRegExp, secretKey } from "./config.mjs";
import { withBuiltinDrops } from "./steps.mjs";
import { DECODED_LABELS, contentViews, decodedViews } from "./content.mjs";
import { formatBytes, lineAt, lineIndex, walk } from "./tree.mjs";

export const GATE_ORDER = [
  ["gitleaks", "a", "gitleaks secret scan"],
  ["trufflehog", "b", "trufflehog secret scan"],
  ["forbidden-strings", "c", "forbidden-strings list"],
  ["file-size", "d", "no file over the size limit"],
  ["symlinks", "e", "no symlink leaves the tree"],
  ["env-files", "f", "no .env files except *.example"],
  ["gitignore-clean", "g", "nothing in the tree is git-ignored"],
  ["license", "h", "LICENSE is Apache-2.0"],
  ["private-paths", "i", "no docs/superpowers (any depth) or drop entry"],
  ["doc-links", "j", "entry-point docs link only to paths in the tree"],
];

export const DEFAULT_SIZE_ALLOW = ["frontend/public/fonts/print/noto-*-cjk-*"];

function gate(id, extra = {}) {
  const meta = GATE_ORDER.find(([gateId]) => gateId === id);
  return { id, letter: meta[1], title: meta[2], status: "pass", hits: [], notes: [], ...extra };
}

function finish(result) {
  if (result.hits.length > 0) result.status = "fail";
  return result;
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

// ---------------------------------------------------------------------------
// Scanner plumbing

function findOnPath(command) {
  for (const dir of (process.env.PATH || "").split(path.delimiter)) {
    if (!dir) continue;
    const candidate = path.join(dir, command);
    try {
      fs.accessSync(candidate, fs.constants.X_OK);
      if (fs.statSync(candidate).isFile()) return candidate;
    } catch {
      // keep looking
    }
  }
  return null;
}

export function resolveScanner(envName, command) {
  const explicit = process.env[envName];
  if (explicit) {
    try {
      fs.accessSync(explicit, fs.constants.X_OK);
      return { bin: explicit, source: envName };
    } catch {
      return { bin: null, source: `${envName}=${explicit} (not executable)` };
    }
  }
  const bin = findOnPath(command);
  return { bin, source: bin ? "PATH" : "not on PATH" };
}

function runCommand(bin, args, options = {}) {
  return new Promise((resolve) => {
    const child = spawn(bin, args, { ...options, stdio: ["ignore", "pipe", "pipe"] });
    const stdout = [];
    const stderr = [];
    child.stdout.on("data", (chunk) => stdout.push(chunk));
    child.stderr.on("data", (chunk) => stderr.push(chunk));
    child.on("error", (error) => resolve({ status: -1, stdout: "", stderr: String(error) }));
    child.on("close", (status) =>
      resolve({
        status,
        stdout: Buffer.concat(stdout).toString("utf8"),
        stderr: Buffer.concat(stderr).toString("utf8"),
      }),
    );
  });
}

function stderrTail(text, lines = 5) {
  return text
    .split("\n")
    .filter(Boolean)
    .slice(-lines)
    .map((line) => line.slice(0, 300));
}

function relativize(root, file) {
  if (!file) return "(unknown)";
  const rel = path.isAbsolute(file) ? path.relative(root, file) : path.normalize(file);
  return rel.split(path.sep).join("/").replace(/^\.\//, "");
}

function missingScanner(result, name, resolved, allowMissing) {
  result.scanner = { name, found: false, source: resolved.source };
  if (allowMissing) {
    result.status = "warn";
    result.notes.push(
      `${name} is NOT installed (${resolved.source}); secret scanning by ${name} was SKIPPED because --allow-missing-scanner was passed. Do not publish an export produced this way.`,
    );
  } else {
    result.status = "fail";
    result.error = `${name} not installed (${resolved.source}); install it or pass --allow-missing-scanner for a dry run`;
  }
  return result;
}

function classifySecret(result, { scanner, rule, file, line, value, verified, allowlist, candidates }) {
  const digest = sha256(value || "");
  const allowed = allowlist.has(secretKey(scanner, rule, file, digest));
  if (verified) {
    result.hits.push({ file, line, rule: `${rule} (VERIFIED LIVE)` });
    return;
  }
  if (allowed) {
    result.allowlisted += 1;
    return;
  }
  result.hits.push({ file, line, rule });
  candidates.push(`${scanner} ${rule} ${file} sha256:${digest}`);
}

export async function gitleaksGate({ root, tmpDir, allowlist, allowMissingScanner, candidates }) {
  const result = gate("gitleaks", { allowlisted: 0 });
  const resolved = resolveScanner("GITLEAKS_BIN", "gitleaks");
  if (!resolved.bin) return missingScanner(result, "gitleaks", resolved, allowMissingScanner);
  result.scanner = { name: "gitleaks", found: true, source: resolved.source };

  // gitleaks 8.x always honours <source>/.gitleaksignore, even with
  // --gitleaks-ignore-path pointed elsewhere (verified on 8.21.2), so a root
  // ignore file in the export could silence the gate. Fail closed: reviewed
  // fixtures belong in the private secrets-allow.txt instead.
  if (fs.existsSync(path.join(root, ".gitleaksignore"))) {
    result.hits.push({ file: ".gitleaksignore", rule: "scanner-suppression-file" });
    result.notes.push(
      "a root .gitleaksignore would suppress gitleaks findings; drop it (drop.txt) and pin reviewed fixtures in secrets-allow.txt",
    );
  }

  // Pin the config so a .gitleaks.toml inside the export cannot weaken the
  // gate, point the ignore-file lookup at an empty directory, and ignore
  // inline gitleaks:allow comments.
  const workDir = fs.mkdtempSync(path.join(tmpDir, "gitleaks-"));
  const configPath = path.join(workDir, "gitleaks.toml");
  fs.writeFileSync(configPath, '[extend]\nuseDefault = true\n');
  const ignoreDir = path.join(workDir, "ignore");
  fs.mkdirSync(ignoreDir);
  const reportPath = path.join(workDir, "report.json");
  const common = [
    "--no-banner",
    "--log-level",
    "error",
    "--config",
    configPath,
    "--gitleaks-ignore-path",
    ignoreDir,
    "--ignore-gitleaks-allow",
    "--report-format",
    "json",
    "--report-path",
    reportPath,
    "--exit-code",
    "0",
  ];
  const hasDir = spawnSync(resolved.bin, ["dir", "--help"], { stdio: "ignore" }).status === 0;
  const args = hasDir ? ["dir", ...common, root] : ["detect", "--no-git", "--source", root, ...common];
  result.scanner.command = hasDir ? "gitleaks dir" : "gitleaks detect --no-git";
  const run = await runCommand(resolved.bin, args, { cwd: workDir });
  try {
    if (run.status !== 0) {
      result.status = "fail";
      result.error = `gitleaks exited ${run.status}`;
      result.notes.push(...stderrTail(run.stderr));
      return result;
    }
    let findings;
    try {
      findings = JSON.parse(fs.readFileSync(reportPath, "utf8") || "[]");
    } catch (error) {
      result.status = "fail";
      result.error = `could not read the gitleaks report: ${error.message}`;
      return result;
    }
    for (const finding of findings) {
      classifySecret(result, {
        scanner: "gitleaks",
        rule: finding.RuleID || "unknown",
        file: relativize(root, finding.File),
        line: finding.StartLine,
        value: finding.Secret || finding.Match,
        verified: false,
        allowlist,
        candidates,
      });
    }
    result.scanned = findings.length;
    return finish(result);
  } finally {
    fs.rmSync(workDir, { recursive: true, force: true });
  }
}

export async function trufflehogGate({ root, tmpDir, allowlist, allowMissingScanner, verify, candidates }) {
  const result = gate("trufflehog", { allowlisted: 0 });
  const resolved = resolveScanner("TRUFFLEHOG_BIN", "trufflehog");
  if (!resolved.bin) return missingScanner(result, "trufflehog", resolved, allowMissingScanner);
  result.scanner = { name: "trufflehog", found: true, source: resolved.source, verification: verify };

  const workDir = fs.mkdtempSync(path.join(tmpDir, "trufflehog-"));
  // `--only-verified=false` is parsed as `--only-verified` by trufflehog's
  // flag library (3.82.x), which silently hides every unverified result. The
  // negated form is the one that actually reports unverified findings.
  const args = ["filesystem", root, "--json", "--no-update", "--no-only-verified"];
  if (!verify) args.push("--no-verification");
  const run = await runCommand(resolved.bin, args, { cwd: workDir });
  try {
    if (run.status !== 0) {
      result.status = "fail";
      result.error = `trufflehog exited ${run.status}`;
      result.notes.push(...stderrTail(run.stderr));
      return result;
    }
    const seen = new Set();
    let scanned = 0;
    for (const line of run.stdout.split("\n")) {
      if (!line.trim().startsWith("{")) continue;
      let finding;
      try {
        finding = JSON.parse(line);
      } catch {
        continue;
      }
      if (!finding.DetectorName && !finding.SourceMetadata) continue;
      const meta = finding.SourceMetadata?.Data?.Filesystem || {};
      const file = relativize(root, meta.file);
      const value = finding.RawV2 || finding.Raw || finding.Redacted || "";
      const dedupe = `${finding.DetectorName}\t${file}\t${meta.line}\t${sha256(value)}`;
      if (seen.has(dedupe)) continue;
      seen.add(dedupe);
      scanned += 1;
      classifySecret(result, {
        scanner: "trufflehog",
        rule: finding.DetectorName || "unknown",
        file,
        line: meta.line,
        value,
        verified: finding.Verified === true,
        allowlist,
        candidates,
      });
    }
    result.scanned = scanned;
    return finish(result);
  } finally {
    fs.rmSync(workDir, { recursive: true, force: true });
  }
}

// ---------------------------------------------------------------------------
// (c) forbidden strings

// The pseudo-rule for files the gate cannot read (see content.mjs).
export const OPAQUE_RULE = "opaque-archive";

export function forbiddenGate({ root, entries, rules, author, message = null, opaqueAllow = [] }) {
  const result = gate("forbidden-strings", { perRule: {} });
  const seen = new Set();
  const add = (file, line, ruleName) => {
    const key = `${file}\t${line}\t${ruleName}`;
    if (seen.has(key)) return;
    seen.add(key);
    result.hits.push({ file, line, rule: ruleName });
    const bucket = result.perRule[ruleName] || { lines: 0, files: new Set() };
    bucket.lines += 1;
    bucket.files.add(file);
    result.perRule[ruleName] = bucket;
  };
  const textRules = rules.filter((rule) => rule.regex);
  const blobRules = rules.filter((rule) => rule.digests);
  const opaqueAllowed = compileGlobs(opaqueAllow);
  // scanText runs the text rules over one string. `label` names a derived
  // view (null for the file itself); `lines` says whether line numbers mean
  // anything in it, and `lineMap` (a decoded view, which keeps only the lines
  // it changed) maps them back to the file's own lines. `skip` holds rules
  // that already hit this file in another byte-level view, so one binary file
  // is not reported once per view; `lineHits` collects "view\tline\trule"
  // for the views that are not decoded, so a decoded line that still shows
  // the same hit as the line it came from is not reported twice.
  const scanText = (
    file,
    text,
    { lines = true, label = null, path: isPath = false, skip = null, lineMap = null, lineHits = null } = {},
  ) => {
    const starts = lines ? lineIndex(text) : null;
    const hitRules = new Set();
    for (const rule of textRules) {
      if (rule.only && !rule.only(file)) continue;
      if (rule.allow && rule.allow(file)) continue;
      if (skip && skip.has(rule.name)) continue;
      rule.regex.lastIndex = 0;
      for (const match of text.matchAll(rule.regex)) {
        let line;
        if (isPath) {
          line = "path";
        } else if (starts) {
          const viewLine = lineAt(starts, match.index);
          const sourceLine = lineMap ? lineMap[viewLine - 1] : viewLine;
          const key = `${lineMap ? decodedFrom(label) : label}\t${sourceLine}\t${rule.name}`;
          if (!lineMap) {
            if (lineHits) lineHits.add(key);
          } else if (lineHits && lineHits.has(key)) {
            continue;
          }
          line = label ? `${label}:${sourceLine}` : sourceLine;
        } else {
          line = label || "binary";
        }
        add(file, line, rule.name);
        hitRules.add(rule.name);
      }
      rule.regex.lastIndex = 0;
    }
    return hitRules;
  };
  // decodedFrom names the view a decoded view was decoded from ("gzip" for
  // "gzip/unescaped", null for "unescaped").
  const decodedFrom = (label) => {
    for (const name of DECODED_LABELS) {
      if (label === name) return null;
      if (label.endsWith(`/${name}`)) return label.slice(0, -name.length - 1);
    }
    return label;
  };
  // scanViews scans every view of one file (or of the commit message) and
  // reports what could not be read.
  const scanViews = (file, { views, opaque }, { allowOpaque = false } = {}) => {
    const byteLevelHits = new Set();
    const lineHits = new Set();
    for (const view of views) {
      const hits = scanText(file, view.text, {
        lines: view.lines,
        label: view.label,
        skip: view.lines ? null : byteLevelHits,
        lineMap: view.lineMap || null,
        lineHits,
      });
      if (!view.lines) for (const name of hits) byteLevelHits.add(name);
    }
    for (const reason of opaque) {
      if (allowOpaque) {
        result.notes.push(`not scanned (${reason}), allowed by --allow-opaque: ${file}`);
      } else {
        add(file, reason, OPAQUE_RULE);
      }
    }
  };

  for (const entry of entries) {
    // The path itself can leak (e.g. a file named after a private client).
    scanText(entry.rel, entry.rel, { lines: false, path: true });
    if (entry.kind === "symlink") {
      scanText(entry.rel, fs.readlinkSync(entry.abs), { lines: false, path: true });
      continue;
    }
    if (entry.kind !== "file") continue;
    const buffer = fs.readFileSync(entry.abs);
    if (blobRules.length) {
      // Content identity: catches a known-bad blob (e.g. a screenshot whose
      // pixels leak data no text rule can see) under any name.
      const digest = sha256(buffer);
      for (const rule of blobRules) if (rule.digests.has(digest)) add(entry.rel, "blob", rule.name);
    }
    scanViews(entry.rel, contentViews(buffer, entry.rel), { allowOpaque: opaqueAllowed(entry.rel) });
  }
  if (author) scanText("<commit author>", author, { lines: false, path: true });
  // The commit message ships in the public history just like the tree, and
  // is read the way a text file is (decoded views included).
  if (message) {
    const decoded = decodedViews(message);
    scanViews("<commit message>", { views: [{ label: null, text: message, lines: true }, ...decoded.views], opaque: decoded.opaque });
  }

  for (const [name, bucket] of Object.entries(result.perRule)) {
    result.perRule[name] = { lines: bucket.lines, files: bucket.files.size };
  }
  result.rules = rules.length;
  return finish(result);
}

// ---------------------------------------------------------------------------
// (d) file size

export function sizeGate({ entries, maxBytes, sizeAllow }) {
  const result = gate("file-size", { maxBytes });
  const allowed = compileGlobs(sizeAllow);
  for (const entry of entries) {
    if (entry.kind !== "file" || entry.size <= maxBytes) continue;
    if (allowed(entry.rel)) {
      result.notes.push(`allowlisted large file: ${entry.rel} (${formatBytes(entry.size)})`);
      continue;
    }
    result.hits.push({ file: entry.rel, rule: `over ${formatBytes(maxBytes)}`, detail: formatBytes(entry.size) });
  }
  return finish(result);
}

// ---------------------------------------------------------------------------
// (e) symlinks

const MAX_LINK_HOPS = 40;

function isAbsoluteTarget(target) {
  return target.startsWith("/") || target.startsWith("\\") || /^[A-Za-z]:[\\/]/.test(target);
}

// resolveLink resolves the symlink at linkRel the way the kernel would, one
// component at a time, following every symlink it meets on the way (so a
// chain such as `up -> ..` plus `esc -> up/../..` is judged by where it
// really lands, not by its text). Backslashes count as separators, which can
// only make the check stricter. Components that do not exist are taken
// lexically. Returns { verdict: "ok" | "absolute-target" | "escapes-tree" |
// "symlink-loop", dangling }.
export function resolveLink(root, linkRel) {
  let dir = linkRel.split("/").slice(0, -1);
  let pending = [linkRel.split("/").pop()];
  let hops = 0;
  let dangling = false;
  while (pending.length) {
    const part = pending.shift();
    if (part === "" || part === ".") continue;
    if (part === "..") {
      if (dir.length === 0) return { verdict: "escapes-tree", dangling };
      dir = dir.slice(0, -1);
      continue;
    }
    const candidate = [...dir, part];
    let stat = null;
    try {
      stat = fs.lstatSync(path.join(root, ...candidate));
    } catch {
      dangling = true;
    }
    if (stat?.isSymbolicLink()) {
      hops += 1;
      if (hops > MAX_LINK_HOPS) return { verdict: "symlink-loop", dangling };
      const target = fs.readlinkSync(path.join(root, ...candidate));
      if (isAbsoluteTarget(target)) return { verdict: "absolute-target", dangling };
      pending = [...target.split(/[\\/]/), ...pending];
      continue;
    }
    dir = candidate;
  }
  return { verdict: "ok", dangling };
}

export function symlinkGate({ root, entries }) {
  const result = gate("symlinks");
  const realRoot = fs.realpathSync(root);
  for (const entry of entries) {
    if (entry.kind === "other") {
      result.hits.push({ file: entry.rel, rule: "special-file" });
      continue;
    }
    if (entry.kind !== "symlink") continue;
    const own = fs.readlinkSync(entry.abs);
    if (isAbsoluteTarget(own)) {
      result.hits.push({ file: entry.rel, rule: "absolute-target" });
      continue;
    }
    const { verdict, dangling } = resolveLink(realRoot, entry.rel);
    if (verdict !== "ok") {
      // An absolute target reached through another link is an escape of
      // this link; the other link reports its own absolute-target hit.
      result.hits.push({ file: entry.rel, rule: verdict === "absolute-target" ? "escapes-tree" : verdict });
      continue;
    }
    // Belt and braces: ask the kernel where the link lands.
    let real = null;
    try {
      real = fs.realpathSync.native(entry.abs);
    } catch {
      // dangling or a loop the walk above did not reach
    }
    if (real && real !== realRoot && !real.startsWith(`${realRoot}${path.sep}`)) {
      result.hits.push({ file: entry.rel, rule: "escapes-tree" });
      continue;
    }
    if (dangling || !real) result.notes.push(`dangling in-tree symlink: ${entry.rel}`);
  }
  return finish(result);
}

// ---------------------------------------------------------------------------
// (f) env files

// Env files go by many names: .env, .env.local, .env-production, .env_local,
// .envrc, prod.env, and the cookiecutter layout .envs/.production/.django.
// The check is case-insensitive and fails closed; only names ending in
// .example are exempt.
export function isEnvFile(relPath) {
  const segments = relPath.split("/");
  const base = segments[segments.length - 1];
  if (/\.example$/i.test(base)) return false;
  if (segments.slice(0, -1).some((segment) => /^\.envs$/i.test(segment))) return true;
  return /^\.env(?:[._-].*)?$/i.test(base) || /^\.envrc(?:\..*)?$/i.test(base) || /.\.env$/i.test(base);
}

export function envGate({ entries, envAllow }) {
  const result = gate("env-files");
  const allowed = compileGlobs(envAllow);
  for (const entry of entries) {
    if (!isEnvFile(entry.rel)) continue;
    if (allowed(entry.rel)) {
      result.notes.push(`allowed by --allow-env-file: ${entry.rel}`);
      continue;
    }
    result.hits.push({ file: entry.rel, rule: "env-file" });
  }
  return finish(result);
}

// ---------------------------------------------------------------------------
// (g) git check-ignore --no-index against the exported .gitignore files

export function isolatedGitEnv(extra = {}) {
  const env = { ...process.env, GIT_CONFIG_GLOBAL: "/dev/null", GIT_CONFIG_NOSYSTEM: "1", ...extra };
  for (const key of [
    "GIT_DIR",
    "GIT_WORK_TREE",
    "GIT_INDEX_FILE",
    "GIT_OBJECT_DIRECTORY",
    "GIT_ALTERNATE_OBJECT_DIRECTORIES",
    "GIT_CEILING_DIRECTORIES",
  ]) {
    delete env[key];
  }
  return env;
}

export function gitignoreGate({ root, entries, tmpDir }) {
  const result = gate("gitignore-clean");
  const probe = fs.mkdtempSync(path.join(tmpDir, "ignore-probe-"));
  const env = isolatedGitEnv();
  try {
    const init = spawnSync("git", ["init", "-q", "--template=", probe], { env, encoding: "utf8" });
    if (init.status !== 0) {
      result.status = "fail";
      result.error = `git init failed: ${init.stderr.trim()}`;
      return result;
    }
    const input = entries.map((entry) => `${entry.rel}\0`).join("");
    const check = spawnSync(
      "git",
      [
        "-c",
        "core.excludesFile=/dev/null",
        `--git-dir=${path.join(probe, ".git")}`,
        `--work-tree=${root}`,
        "check-ignore",
        "--no-index",
        "--stdin",
        "-z",
        "-v",
      ],
      { cwd: root, env, input, encoding: "utf8", maxBuffer: 256 * 1024 * 1024 },
    );
    if (check.status !== 0 && check.status !== 1) {
      result.status = "fail";
      result.error = `git check-ignore exited ${check.status}: ${String(check.stderr).trim().slice(0, 300)}`;
      return result;
    }
    const fields = check.stdout.split("\0");
    for (let i = 0; i + 3 < fields.length; i += 4) {
      const [source, lineNo, pattern, file] = fields.slice(i, i + 4);
      // -v also prints paths whose last matching pattern is a negation
      // ("!keep-me"); those are not ignored.
      if (!file || pattern.startsWith("!")) continue;
      result.hits.push({ file, rule: `${source}:${lineNo}`, detail: pattern });
    }
    return finish(result);
  } finally {
    fs.rmSync(probe, { recursive: true, force: true });
  }
}

// ---------------------------------------------------------------------------
// (h) LICENSE

// The canonical Apache-2.0 text (sha256 cfc7749b...3d30, as published at
// apache.org). A LICENSE passes only if, after whitespace is collapsed, its
// terms are exactly these and nothing follows them but the stock appendix,
// with the copyright line left as the placeholder or filled in with years
// and exactly the expected holder (--copyright-holder, which export.sh
// defaults to the author name). Marker phrases alone would also accept the
// Apache text with extra restrictions bolted on (e.g. a Commons Clause), and
// a free-text holder would let the same restriction ride in on the copyright
// line ("Copyright 2026 X. Commercial use is not permitted ...").
const APACHE_TEXT = fs.readFileSync(new URL("./apache-2.0.txt", import.meta.url), "utf8");
const TERMS_END = "END OF TERMS AND CONDITIONS";
const COPYRIGHT_PLACEHOLDER = "Copyright [yyyy] [name of copyright owner]";
const BEFORE_NOTICE = "(?= Licensed under the Apache License)";
const COPYRIGHT_PREFIX = "Copyright (?:\\([cC]\\) |\\u00a9 )?\\d{4}(?: ?[-\\u2013,] ?\\d{4})*,? ";
// Any filled-in copyright line, whatever it names: used only to tell a wrong
// holder apart from other added text, so the report says which it is.
const COPYRIGHT_ANY = new RegExp(`Copyright [^]{1,400}?${BEFORE_NOTICE}`, "u");

function normalizeHolder(holder) {
  return typeof holder === "string" ? holder.replace(/\s+/g, " ").trim() : "";
}

function copyrightFilledBy(holder) {
  return new RegExp(`${COPYRIGHT_PREFIX}${escapeRegExp(holder)}\\.?${BEFORE_NOTICE}`, "u");
}

function normalizeLicense(text) {
  return text
    .replace(/^\uFEFF/, "")
    .replace(/https:\/\/www\.apache\.org\//g, "http://www.apache.org/")
    .replace(/\s+/g, " ")
    .trim();
}

const CANONICAL = normalizeLicense(APACHE_TEXT);
const CANONICAL_TERMS = CANONICAL.slice(0, CANONICAL.indexOf(TERMS_END) + TERMS_END.length);
const CANONICAL_APPENDIX = CANONICAL.slice(CANONICAL_TERMS.length).trim();

// apacheLicenseCheck returns { rule } for a LICENSE that fails, or
// { rule: null, filledCopyright } for one that passes. A filled-in copyright
// line passes only when it names `holder` exactly; without a holder, only the
// stock placeholder passes.
export function apacheLicenseCheck(text, { holder = null } = {}) {
  const candidate = normalizeLicense(text);
  if (!candidate.startsWith(CANONICAL_TERMS)) return { rule: "not-apache-2.0" };
  const rest = candidate.slice(CANONICAL_TERMS.length).trim();
  if (rest === "" || rest === CANONICAL_APPENDIX) return { rule: null, filledCopyright: false };
  const expected = normalizeHolder(holder);
  if (expected && rest.replace(copyrightFilledBy(expected), COPYRIGHT_PLACEHOLDER) === CANONICAL_APPENDIX) {
    return { rule: null, filledCopyright: true };
  }
  if (rest.replace(COPYRIGHT_ANY, COPYRIGHT_PLACEHOLDER) === CANONICAL_APPENDIX) {
    return { rule: "copyright-holder-mismatch" };
  }
  return { rule: "text-after-apache-terms" };
}

export function licenseGate({ root, copyrightHolder = null }) {
  const result = gate("license");
  const licensePath = path.join(root, "LICENSE");
  let stat = null;
  try {
    stat = fs.lstatSync(licensePath);
  } catch {
    // missing
  }
  if (!stat) {
    result.hits.push({ file: "LICENSE", rule: "missing" });
  } else if (!stat.isFile()) {
    result.hits.push({ file: "LICENSE", rule: "not-a-regular-file" });
  } else {
    const verdict = apacheLicenseCheck(fs.readFileSync(licensePath, "utf8"), { holder: copyrightHolder });
    if (verdict.rule) result.hits.push({ file: "LICENSE", rule: verdict.rule });
    else if (verdict.filledCopyright) result.notes.push("LICENSE fills in the appendix copyright line with the expected --copyright-holder");
  }
  for (const companion of ["NOTICE"]) {
    if (!fs.existsSync(path.join(root, companion))) {
      result.notes.push(`${companion} is missing (Apache-2.0 §4(d); expected from the licence workstream)`);
    }
  }
  return finish(result);
}

// ---------------------------------------------------------------------------
// (i) private paths

// privateTreeOf returns the docs/superpowers directory that contains relPath,
// at any depth and in any letter case, or null.
export function privateTreeOf(relPath) {
  const parts = relPath.split("/");
  for (let i = 0; i + 1 < parts.length; i += 1) {
    if (parts[i].toLowerCase() === "docs" && parts[i + 1].toLowerCase() === "superpowers") {
      return parts.slice(0, i + 2).join("/");
    }
  }
  return null;
}

export function privatePathsGate({ root, entries, dropEntries }) {
  const result = gate("private-paths");
  const trees = new Set();
  if (fs.existsSync(path.join(root, "docs/superpowers"))) trees.add("docs/superpowers");
  for (const entry of entries) {
    const tree = privateTreeOf(entry.rel);
    if (tree) trees.add(tree);
  }
  for (const tree of [...trees].sort()) result.hits.push({ file: tree, rule: "private-tree-present" });
  const all = withBuiltinDrops(dropEntries);
  for (const entry of entries) {
    const match = all.find((drop) => drop.match(entry.rel));
    if (match) result.hits.push({ file: entry.rel, rule: `drop-entry-survived:${match.pattern}` });
  }
  return finish(result);
}


// ---------------------------------------------------------------------------
// (j) doc links

// The documents a newcomer opens first. A relative link in any of them that
// points at a path the export dropped is a broken promise on day one (the
// export once dropped .claude wholesale while README.md linked its skills).
export const DOC_LINK_FILES = ["README.md", "AGENTS.md", "CLAUDE.md", "CONTRIBUTING.md", "docs/agents/README.md"];

// markdownLinkTargets returns { line, target } for every inline link, image
// and reference definition outside fenced code blocks and inline code spans.
export function markdownLinkTargets(text) {
  const out = [];
  let fence = null;
  const lines = text.split(/\r?\n/);
  for (let i = 0; i < lines.length; i += 1) {
    const raw = lines[i];
    const fenceMatch = raw.match(/^\s{0,3}(`{3,}|~{3,})/);
    if (fenceMatch) {
      const marker = fenceMatch[1];
      if (!fence) fence = marker;
      else if (marker[0] === fence[0] && marker.length >= fence.length) fence = null;
      continue;
    }
    if (fence) continue;
    const line = raw.replace(/(`+)[\s\S]*?\1/g, "");
    for (const m of line.matchAll(/\]\(\s*<?([^)\s>]+)>?(?:\s+["'(][^)]*)?\)/g)) out.push({ line: i + 1, target: m[1] });
    const ref = line.match(/^\s{0,3}\[[^\]]+\]:\s*<?(\S+?)>?(?:\s|$)/);
    if (ref) out.push({ line: i + 1, target: ref[1] });
  }
  return out;
}

function isExternalTarget(target) {
  return /^[a-z][a-z0-9+.-]*:/i.test(target) || target.startsWith("//") || target.startsWith("#");
}

export function docLinksGate({ root, files = DOC_LINK_FILES }) {
  const result = gate("doc-links");
  for (const rel of files) {
    const abs = path.join(root, rel);
    let stat = null;
    try {
      stat = fs.lstatSync(abs);
    } catch {
      continue;
    }
    if (!stat.isFile()) continue;
    for (const { line, target } of markdownLinkTargets(fs.readFileSync(abs, "utf8"))) {
      if (isExternalTarget(target)) continue;
      let clean = target.split("#")[0].split("?")[0];
      try {
        clean = decodeURIComponent(clean);
      } catch {
        // keep the raw spelling
      }
      if (!clean) continue;
      const base = clean.startsWith("/") ? "" : path.posix.dirname(rel);
      const resolved = path.posix.normalize(path.posix.join(base, clean.replace(/^\/+/, "")));
      if (resolved === ".." || resolved.startsWith("../")) {
        result.hits.push({ file: rel, line, rule: "link-leaves-tree", detail: target });
        continue;
      }
      if (!fs.existsSync(path.join(root, resolved))) {
        result.hits.push({ file: rel, line, rule: "link-target-missing", detail: resolved });
      }
    }
  }
  return finish(result);
}

// ---------------------------------------------------------------------------

export async function runGates(options) {
  const {
    root: rootInput,
    tmpDir,
    forbiddenRules,
    dropEntries,
    secretsAllow,
    maxBytes,
    sizeAllow,
    envAllow,
    opaqueAllow = [],
    allowMissingScanner,
    trufflehogVerify,
    author,
    message = null,
    copyrightHolder = null,
  } = options;
  const root = fs.realpathSync(rootInput);
  const entries = walk(root);
  const candidates = [];

  const scannerRuns = Promise.all([
    gitleaksGate({ root, tmpDir, allowlist: secretsAllow, allowMissingScanner, candidates }),
    trufflehogGate({ root, tmpDir, allowlist: secretsAllow, allowMissingScanner, verify: trufflehogVerify, candidates }),
  ]);

  const local = [
    forbiddenGate({ root, entries, rules: forbiddenRules, author, message, opaqueAllow }),
    sizeGate({ entries, maxBytes, sizeAllow }),
    symlinkGate({ root, entries }),
    envGate({ entries, envAllow }),
    gitignoreGate({ root, entries, tmpDir }),
    licenseGate({ root, copyrightHolder }),
    privatePathsGate({ root, entries, dropEntries }),
    docLinksGate({ root }),
  ];
  const scanners = await scannerRuns;
  const gates = [...scanners, ...local];
  const ok = gates.every((g) => g.status !== "fail");
  return { ok, gates, candidates, entries: entries.length };
}
