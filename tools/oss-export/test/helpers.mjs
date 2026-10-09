// Shared fixtures for the oss-export tests: a throwaway git repository, stub
// secret scanners, and a runner for export.sh. Everything lives under one
// temporary directory per test file; nothing touches the real repository.
//
// Fake secrets are assembled at runtime from a marker plus random characters,
// so this file never contains anything a real scanner would flag.

import { spawnSync } from "node:child_process";
import { createHash, randomBytes } from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

export const TOOL_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
export const EXPORT_SH = path.join(TOOL_DIR, "export.sh");

// The real Apache-2.0 text: gate (h) compares the whole of it.
export const APACHE_LICENSE = fs.readFileSync(path.join(TOOL_DIR, "lib", "apache-2.0.txt"), "utf8");

export const MARKERS = {
  gitleaks: "STUBSECRET_",
  trufflehog: "STUBTRUFFLE_",
  trufflehogLive: "STUBLIVE_",
};

export function fakeSecret(marker) {
  return marker + randomBytes(12).toString("hex").toUpperCase();
}

export function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

export function makeTempRoot(label) {
  const base = fs.realpathSync(os.tmpdir());
  return fs.mkdtempSync(path.join(base, `oss-export-${label}-`));
}

export function isolatedEnv(extra = {}) {
  const env = {
    ...process.env,
    GIT_CONFIG_GLOBAL: "/dev/null",
    GIT_CONFIG_NOSYSTEM: "1",
    GIT_AUTHOR_NAME: "Fixture Author",
    GIT_AUTHOR_EMAIL: "fixture@example.com",
    GIT_COMMITTER_NAME: "Fixture Author",
    GIT_COMMITTER_EMAIL: "fixture@example.com",
    ...extra,
  };
  for (const key of ["GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GITLEAKS_BIN", "TRUFFLEHOG_BIN"]) {
    if (!(key in extra)) delete env[key];
  }
  return env;
}

function git(cwd, args, options = {}) {
  const result = spawnSync("git", args, { cwd, env: isolatedEnv(), encoding: "utf8", ...options });
  if (result.status !== 0) {
    throw new Error(`git ${args.join(" ")} failed: ${result.stderr}`);
  }
  return result.stdout;
}

// The clean baseline: every gate passes and every config file comes from the
// ref's docs/superpowers/oss-export/.
export function baseFiles() {
  return {
    "LICENSE": APACHE_LICENSE,
    "NOTICE": "Fixture\nCopyright 2026 Fixture Author\n",
    "README.md": "# fixture\n",
    ".gitignore": "node_modules/\n*.log\n!keep.log\n",
    ".env.example": "API_URL=http://localhost:8080\n",
    "src/app.js": "export const answer = 42;\n",
    "src/paths.txt": "built on /home/alice/project/src\n",
    "private/notes.md": "internal notes\n",
    "docs/superpowers/plans/plan.md": "# private plan\n",
    "docs/superpowers/oss-export/forbidden.txt": "acme-private-corp ;; name=private-org\n",
    "docs/superpowers/oss-export/drop.txt": "private\n",
    "docs/superpowers/oss-export/scrub.rules": "s#/home/alice/#/path/to/#g ;; name=home-path\n",
  };
}

// makeRepo writes files (null deletes a base file), symlinks and forced
// (ignored) files, commits them, and returns { root, repo }.
export function makeRepo(root, { files = {}, symlinks = {}, forced = {}, base = true } = {}) {
  const repo = path.join(root, "repo");
  fs.mkdirSync(repo, { recursive: true });
  git(repo, ["-c", "init.defaultBranch=main", "init", "-q", "--template="]);
  const all = { ...(base ? baseFiles() : {}), ...files };
  const tracked = [];
  for (const [rel, content] of Object.entries(all)) {
    if (content === null) continue;
    const abs = path.join(repo, rel);
    fs.mkdirSync(path.dirname(abs), { recursive: true });
    fs.writeFileSync(abs, content);
    tracked.push(rel);
  }
  for (const [rel, target] of Object.entries(symlinks)) {
    const abs = path.join(repo, rel);
    fs.mkdirSync(path.dirname(abs), { recursive: true });
    fs.symlinkSync(target, abs);
    tracked.push(rel);
  }
  for (const [rel, content] of Object.entries(forced)) {
    const abs = path.join(repo, rel);
    fs.mkdirSync(path.dirname(abs), { recursive: true });
    fs.writeFileSync(abs, content);
  }
  if (tracked.length) git(repo, ["add", "--", ...tracked]);
  const forcedPaths = Object.keys(forced);
  if (forcedPaths.length) git(repo, ["add", "-f", "--", ...forcedPaths]);
  git(repo, ["commit", "-q", "--no-verify", "-m", "fixture"]);
  return { root, repo };
}

// Stub scanners: small node scripts that mimic the JSON output of gitleaks and
// trufflehog for marker strings, and log their argv for flag assertions.
const STUB_GITLEAKS = String.raw`#!/usr/bin/env node
const fs = require("fs");
const path = require("path");
const args = process.argv.slice(2);
if (process.env.STUB_LOG) fs.appendFileSync(process.env.STUB_LOG, "gitleaks " + JSON.stringify(args) + "\n");
if (args.includes("--help")) process.exit(0);
const report = args[args.indexOf("--report-path") + 1];
const root = args[args.length - 1];
const findings = [];
const walk = (dir) => {
  for (const d of fs.readdirSync(dir, { withFileTypes: true })) {
    const abs = path.join(dir, d.name);
    if (d.isDirectory()) { if (d.name !== ".git") walk(abs); continue; }
    if (!d.isFile()) continue;
    fs.readFileSync(abs, "utf8").split("\n").forEach((line, i) => {
      for (const m of line.matchAll(/STUBSECRET_[A-F0-9]{8,}/g)) {
        findings.push({ RuleID: "stub-rule", File: abs, StartLine: i + 1, Secret: m[0], Match: "secret=" + m[0] });
      }
    });
  }
};
walk(root);
fs.writeFileSync(report, JSON.stringify(findings));
process.exit(0);
`;

const STUB_TRUFFLEHOG = String.raw`#!/usr/bin/env node
const fs = require("fs");
const path = require("path");
const args = process.argv.slice(2);
if (process.env.STUB_LOG) fs.appendFileSync(process.env.STUB_LOG, "trufflehog " + JSON.stringify(args) + "\n");
// Mimic trufflehog 3.82: "--only-verified=false" still means only-verified.
const onlyVerified = args.some((a) => a === "--only-verified" || a.startsWith("--only-verified="));
const root = args[1];
const walk = (dir) => {
  for (const d of fs.readdirSync(dir, { withFileTypes: true })) {
    const abs = path.join(dir, d.name);
    if (d.isDirectory()) { if (d.name !== ".git") walk(abs); continue; }
    if (!d.isFile()) continue;
    fs.readFileSync(abs, "utf8").split("\n").forEach((line, i) => {
      for (const m of line.matchAll(/STUB(TRUFFLE|LIVE)_[A-F0-9]{8,}/g)) {
        const verified = m[1] === "LIVE" && !args.includes("--no-verification");
        if (onlyVerified && !verified) continue;
        const finding = {
          SourceMetadata: { Data: { Filesystem: { file: abs, line: i + 1 } } },
          DetectorName: "StubDetector",
          // Like the real tool, nothing verifies under --no-verification.
          Verified: verified,
          Raw: m[0],
          Redacted: "",
        };
        process.stdout.write(JSON.stringify(finding) + "\n");
      }
    });
  }
};
walk(root);
process.exit(0);
`;

export function installStubScanners(root) {
  const bin = path.join(root, "stub-bin");
  fs.mkdirSync(bin, { recursive: true });
  const gitleaks = path.join(bin, "gitleaks");
  const trufflehog = path.join(bin, "trufflehog");
  fs.writeFileSync(gitleaks, STUB_GITLEAKS, { mode: 0o755 });
  fs.writeFileSync(trufflehog, STUB_TRUFFLEHOG, { mode: 0o755 });
  return { gitleaks, trufflehog };
}

// runExport runs export.sh from inside the fixture repo and returns the
// process result plus parsed reports.
export function runExport(fixture, args, { env = {}, scanners, outName = "out" } = {}) {
  const out = path.join(fixture.root, outName);
  const reportDir = path.join(fixture.root, `${outName}.report`);
  const fullArgs = [...args.map((a) => (a === "$OUT" ? out : a)), "--report-dir", reportDir];
  // OSS_EXPORT_BASH=/bin/bash exercises the macOS bash 3.2 path.
  const result = spawnSync(process.env.OSS_EXPORT_BASH || "bash", [EXPORT_SH, ...fullArgs], {
    cwd: fixture.repo,
    env: isolatedEnv({
      TMPDIR: fixture.root,
      ...(scanners ? { GITLEAKS_BIN: scanners.gitleaks, TRUFFLEHOG_BIN: scanners.trufflehog } : {}),
      ...env,
    }),
    encoding: "utf8",
  });
  let gates = null;
  try {
    gates = JSON.parse(fs.readFileSync(path.join(reportDir, "gates.json"), "utf8"));
  } catch {
    // no gate report (usage error before the gates ran)
  }
  return {
    status: result.status,
    stdout: result.stdout,
    stderr: result.stderr,
    output: `${result.stdout}\n${result.stderr}`,
    out,
    reportDir,
    gates,
    gate(id) {
      return gates?.gates.find((g) => g.id === id) ?? null;
    },
  };
}

export function gitIn(dir, args) {
  return git(dir, args).trim();
}
