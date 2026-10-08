#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const rules = [
  {
    name: "private-key",
    pattern:
      /-----BEGIN ((?:RSA |EC |OPENSSH )?PRIVATE KEY)-----[\s\S]{1,16384}?-----END \1-----/g,
  },
  { name: "aws-access-key", pattern: /\bAKIA[0-9A-Z]{16}\b/g },
  { name: "github-token", pattern: /\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{30,}\b/g },
  { name: "slack-token", pattern: /\bxox[baprs]-[A-Za-z0-9-]{20,}\b/g },
  { name: "stripe-live-secret", pattern: /\bsk_live_[A-Za-z0-9]{16,}\b/g },
];

// Exact fingerprints of deliberate synthetic fixtures already tracked by the
// repository. Path + fingerprint means a different value introduced in the
// same test still fails. The scanner never prints matched material.
export const fixtureAllowlist = new Set([
  "stripe-live-secret:backend/internal/database/sanitize_clone_test.go:160a759929934e74445662eea1fe492245448930d42bd37f7624251ac0627812",
  "private-key:backend/internal/fiscal/credentials_test.go:f0367119ea3d2fb3f132c911e269d3e7679d11bc82bd262cbb238b0c93ceade5",
  "stripe-live-secret:backend/internal/llm/diagnostics_test.go:4e11569af566341d2dd68adc677dc13c68116d4b60c535deaa0d6560aa6b28ff",
  "private-key:backend/internal/observability/scrub_test.go:ea8696da0734af5173edae257b040d52281cb29f422cb1a2192e269cdc58b9ed",
  "stripe-live-secret:backend/internal/security/config_secrets_test.go:fe709a5f81decd30cc2c07e5aa7c3a0e8027e127da9c0dfabd80f38ea8aa50d8",
  "stripe-live-secret:backend/internal/services/menu_ai_diagnostics_test.go:c71dc50c6d0af42b23a5ff24d7885193ce30ab962a1e412441ff90058c9383da",
  "stripe-live-secret:docs/superpowers/plans/2026-07-18-production-qa-lane2-ai-media-reliability.md:c71dc50c6d0af42b23a5ff24d7885193ce30ab962a1e412441ff90058c9383da",
  "aws-access-key:frontend/src/components/business/Marketing/templates/renderPost.test.ts:1a5d44a2dca19669d72edf4c4f1c27c4c1ca4b4408fbb17f6ce4ad452d78ddb3",
  "stripe-live-secret:frontend/src/components/business/plugins/__tests__/StripeConfig.live-key.test.tsx:fb2dd2a4e0ea901871b91990a8ae6da8b3c72dc07a3316829d79a0649e268cb4",
]);

const fixtureFingerprints = new Set(
  [...fixtureAllowlist].map((entry) => entry.slice(entry.lastIndexOf(":") + 1)),
);

export function scanText(source, file, options = {}) {
  const findings = [];
  if (options.includeTestCanary) {
    const canary = /PAYVERGE_FAKE_SECRET_CANARY_DO_NOT_USE=\S+/g;
    for (const match of source.matchAll(canary)) {
      findings.push({
        rule: "payverge-test-canary",
        file,
        line: lineNumber(source, match.index),
        fingerprint: fingerprint(match[0]),
      });
    }
  }
  for (const rule of rules) {
    for (const match of source.matchAll(rule.pattern)) {
      findings.push({
        rule: rule.name,
        file,
        line: lineNumber(source, match.index),
        fingerprint: fingerprint(match[0]),
      });
    }
  }
  return findings;
}

function fingerprint(value) {
  return createHash("sha256").update(value).digest("hex");
}

function lineNumber(source, index = 0) {
  return source.slice(0, index).split("\n").length;
}

function git(args, options = {}) {
  return execFileSync("git", args, { maxBuffer: 512 * 1024 * 1024, ...options });
}

function gitFiles(args) {
  return git(["ls-files", "-z", ...args], { encoding: "utf8" })
    .split("\0")
    .filter(Boolean);
}

const zeroOid = /^0+$/;
const gitlinkMode = "160000";
// Bounds one `git cat-file --batch` round trip; the largest tracked blob is a
// few MiB, so a chunk stays far below maxBuffer.
const blobChunkSize = 500;

// Decoded text of a blob, or null for binary content. Matches the original
// scanner contract: anything containing NUL is binary and skipped.
function textOf(bytes) {
  return bytes.includes(0) ? null : bytes.toString("utf8");
}

// Git object id of raw worktree bytes, computed in-process so an unchanged
// file never needs its index blob read back. Hash length follows the
// repository object format (40 hex = sha1, 64 hex = sha256).
function blobOid(bytes, oidLength) {
  return createHash(oidLength === 64 ? "sha256" : "sha1")
    .update(`blob ${bytes.length}\0`)
    .update(bytes)
    .digest("hex");
}

// Reads many blobs through one `git cat-file --batch` process per chunk
// instead of spawning `git show` once per file (the old ~2 minute cost on a
// full checkout). Returns oid -> decoded text, or null when binary.
function readBlobs(oids) {
  const blobs = new Map();
  const unique = [...new Set(oids)];
  for (let start = 0; start < unique.length; start += blobChunkSize) {
    const chunk = unique.slice(start, start + blobChunkSize);
    const out = git(["cat-file", "--batch"], { input: `${chunk.join("\n")}\n` });
    let pos = 0;
    while (pos < out.length) {
      const headerEnd = out.indexOf(0x0a, pos);
      if (headerEnd === -1) break;
      const [oid, type, size] = out.toString("utf8", pos, headerEnd).split(" ");
      pos = headerEnd + 1;
      if (size === undefined) continue; // "<oid> missing" / "ambiguous"
      const body = out.subarray(pos, pos + Number(size));
      pos += Number(size) + 1; // contents are followed by one LF
      if (type === "blob") blobs.set(oid, textOf(body));
    }
  }
  return blobs;
}

function makeCollector() {
  const findings = [];
  const add = (source, displayFile, allowlistFile) => {
    if (source == null || source.includes("\0")) return;
    for (const finding of scanText(source, displayFile)) {
      const key = `${finding.rule}:${allowlistFile}:${finding.fingerprint}`;
      if (!fixtureAllowlist.has(key)) findings.push(finding);
    }
  };
  return { findings, add };
}

// Full scan (`--all`, also the no-argument default and the base of
// `--history`): every tracked and non-ignored untracked worktree file, plus
// any index blob that differs from its worktree file.
function scanRepository() {
  const { findings, add } = makeCollector();
  const worktreeOids = new Map();
  let oidLength = 40;

  // `git ls-files -s` lists the index blob id for each tracked path:
  // "<mode> <oid> <stage>\t<path>".
  const indexEntries = [];
  for (const record of gitFiles(["--stage"])) {
    const tab = record.indexOf("\t");
    const [mode, oid] = record.slice(0, tab).split(" ");
    indexEntries.push({ mode, oid, file: record.slice(tab + 1) });
    oidLength = oid.length;
  }

  // Scan both tracked worktree files and non-ignored untracked files. Ignored
  // local runtime material remains outside repository scope, while a newly
  // created source/config file cannot bypass the gate merely by being untracked.
  for (const file of gitFiles(["--cached", "--others", "--exclude-standard"])) {
    let bytes;
    try {
      bytes = fs.readFileSync(file);
    } catch {
      continue;
    }
    worktreeOids.set(file, blobOid(bytes, oidLength));
    add(textOf(bytes), file, file);
  }

  // The index can contain material that is no longer present in the worktree.
  // Scan its blob whenever it differs, and identify findings as index-owned.
  const differing = indexEntries.filter(
    ({ mode, oid, file }) =>
      mode !== gitlinkMode && !zeroOid.test(oid) && worktreeOids.get(file) !== oid,
  );
  const blobs = readBlobs(differing.map(({ oid }) => oid));
  for (const { oid, file } of differing) {
    add(blobs.get(oid), `git-index:${file}`, file);
  }
  return findings;
}

// Staged scan (`--staged`, used by the pre-commit hook): only the index
// blobs of paths added, copied, modified or type-changed relative to HEAD,
// i.e. exactly the content the commit being made will introduce. Worktree
// edits and untracked files are out of scope here; the full scan in CI still
// covers them. Honors GIT_INDEX_FILE, so `git commit -a` / `git commit
// <paths>` scan the temporary index git builds for the commit.
function scanStaged() {
  const { findings, add } = makeCollector();
  const raw = git(
    ["diff", "--cached", "--raw", "-z", "--no-abbrev", "--no-renames", "--diff-filter=d"],
    { encoding: "utf8" },
  ).split("\0");

  // -z raw records are ":<old mode> <new mode> <old oid> <new oid> <status>"
  // followed by the path, each NUL-terminated (--no-renames: one path each).
  const staged = [];
  for (let i = 0; i + 1 < raw.length; i += 2) {
    const meta = raw[i].replace(/^:/, "").split(" ");
    const file = raw[i + 1];
    const [, mode, , oid] = meta;
    if (!file || !oid || mode === gitlinkMode || zeroOid.test(oid)) continue;
    staged.push({ oid, file });
  }

  const blobs = readBlobs(staged.map(({ oid }) => oid));
  for (const { oid, file } of staged) {
    add(blobs.get(oid), file, file);
  }
  return findings;
}

function historyPath(value, prefix) {
  if (!value || value === "/dev/null") return "";
  return value.startsWith(prefix) ? value.slice(prefix.length) : value;
}

export function scanHistoryPatch(patch) {
  const findings = [];
  const seen = new Set();
  let commit = "unknown";
  let oldPath = "";
  let newPath = "";
  let reconstructed = [];
  let inHunk = false;

  const flush = () => {
    const file = newPath || oldPath;
    if (!file || reconstructed.length === 0) {
      reconstructed = [];
      return;
    }

    const location = `git-history:${commit}:${file}`;
    for (const finding of scanText(reconstructed.join("\n"), location)) {
      const key = `${finding.rule}:${location}:${finding.fingerprint}`;
      if (!seen.has(key)) {
        seen.add(key);
        findings.push(finding);
      }
    }
    reconstructed = [];
  };

  for (const line of patch.split("\n")) {
    if (line.startsWith("commit:")) {
      flush();
      commit = line.slice("commit:".length).trim() || "unknown";
      oldPath = "";
      newPath = "";
      inHunk = false;
      continue;
    }
    if (line.startsWith("diff --git ")) {
      flush();
      oldPath = "";
      newPath = "";
      inHunk = false;
      continue;
    }
    if (!inHunk && line.startsWith("--- ")) {
      oldPath = historyPath(line.slice(4), "a/");
      continue;
    }
    if (!inHunk && line.startsWith("+++ ")) {
      newPath = historyPath(line.slice(4), "b/");
      continue;
    }
    if (line.startsWith("@@")) {
      inHunk = true;
      continue;
    }
    if (inHunk && /^[+ -]/.test(line)) {
      reconstructed.push(line.slice(1));
    }
  }
  flush();
  return findings;
}

function scanHistory() {
  const historyRef = process.env.SCAN_SECRETS_HISTORY_REF;
  const historyArgs = ["log", "--format=commit:%H", "-p"];
  if (historyRef) {
    historyArgs.push(historyRef);
  } else {
    historyArgs.push("--all");
  }
  const patch = execFileSync(
    "git",
    [...historyArgs, "--", "."],
    { encoding: "utf8", maxBuffer: 512 * 1024 * 1024 },
  );
  return scanHistoryPatch(patch).filter(
    (finding) => !fixtureFingerprints.has(finding.fingerprint),
  );
}

const usage = `usage: node scripts/scan-secrets.mjs [--all | --staged] [--history]
  --all      tracked + untracked worktree files and differing index blobs (default)
  --staged   only the staged blobs the next commit introduces (pre-commit hook)
  --history  also scan every commit patch (SCAN_SECRETS_HISTORY_REF narrows it)
`;

function runCLI() {
  const args = process.argv.slice(2);
  const known = new Set(["--all", "--staged", "--history"]);
  const unknown = args.filter((arg) => !known.has(arg));
  if (unknown.length > 0 || (args.includes("--all") && args.includes("--staged"))) {
    process.stderr.write(usage);
    process.exitCode = 2;
    return;
  }
  const stagedOnly = args.includes("--staged");
  const includeHistory = args.includes("--history");
  const findings = [
    ...(stagedOnly ? scanStaged() : scanRepository()),
    ...(includeHistory ? scanHistory() : []),
  ];
  if (findings.length > 0) {
    for (const finding of findings) {
      process.stderr.write(`${finding.rule}: ${finding.file}:${finding.line}\n`);
    }
    process.exitCode = 1;
  } else {
    const scope = stagedOnly ? "staged" : "repository";
    process.stdout.write(
      includeHistory ? `${scope} and history secret scan passed\n` : `${scope} secret scan passed\n`,
    );
  }
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  runCLI();
}
