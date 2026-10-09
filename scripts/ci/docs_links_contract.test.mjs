#!/usr/bin/env node
// Contract for the public reader docs: every relative link resolves, every
// heading anchor exists, and every promptfoo command uses the release that
// evals/promptfoo/runner.sh pins.
//
// Run from the repository root:
//   node --test scripts/ci/docs_links_contract.test.mjs
//
// On a branch where a linked page is still being written elsewhere, list the
// repo-relative paths that may be missing (comma-separated) in
// DOCS_LINKS_ALLOW_MISSING. The integrated tree runs with that variable unset.

import assert from "node:assert/strict";
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

const SCOPE = [
  "docs/README.md",
  "docs/code-tour.md",
  "docs/telegram-plugin-runbook.md",
  "docs/ai",
  "docs/architecture",
  "docs/adr",
  "docs/self-hosting",
  "evals/promptfoo/README.md",
];

const allowMissing = new Set(
  (process.env.DOCS_LINKS_ALLOW_MISSING ?? "")
    .split(",")
    .map((item) => item.trim().replace(/\/$/, ""))
    .filter(Boolean),
);

function scopedFiles() {
  return SCOPE.flatMap((rel) => {
    const abs = path.join(ROOT, rel);
    if (!existsSync(abs)) return [];
    if (statSync(abs).isDirectory()) {
      return readdirSync(abs)
        .filter((name) => name.endsWith(".md"))
        .sort()
        .map((name) => path.join(abs, name));
    }
    return [abs];
  });
}

// GitHub's heading slug: lower-case, drop punctuation, spaces become hyphens,
// repeated headings get -1, -2, ...
function slugify(text) {
  return text
    .replace(/\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/<[^>]+>/g, "")
    .trim()
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\p{M} _-]/gu, "")
    .replace(/ /g, "-");
}

function anchorsOf(markdown) {
  const seen = new Map();
  const anchors = new Set();
  let fenced = false;
  for (const line of markdown.split("\n")) {
    if (/^\s*(```|~~~)/.test(line)) {
      fenced = !fenced;
      continue;
    }
    if (fenced) continue;
    const match = /^(#{1,6})\s+(.*?)\s*#*\s*$/.exec(line);
    if (!match) continue;
    const base = slugify(match[2]);
    const count = seen.get(base) ?? 0;
    seen.set(base, count + 1);
    anchors.add(count === 0 ? base : `${base}-${count}`);
  }
  return anchors;
}

function* relativeLinks(markdown) {
  let fenced = false;
  const lines = markdown.split("\n");
  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i];
    if (/^\s*(```|~~~)/.test(line)) {
      fenced = !fenced;
      continue;
    }
    if (fenced) continue;
    const prose = line.replace(/`[^`]*`/g, "");
    for (const match of prose.matchAll(/\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)/g)) {
      const raw = match[1];
      if (/^(https?:|mailto:|tel:)/.test(raw)) continue;
      yield { raw, line: i + 1 };
    }
  }
}

test("public docs: relative links and anchors resolve", () => {
  const files = scopedFiles();
  assert.ok(files.length > 20, `expected the reader docs in scope, found ${files.length}`);
  const broken = [];
  for (const file of files) {
    const markdown = readFileSync(file, "utf8");
    const where = path.relative(ROOT, file);
    for (const { raw, line } of relativeLinks(markdown)) {
      const [target, anchor] = raw.split("#");
      const abs = target ? path.resolve(path.dirname(file), decodeURIComponent(target)) : file;
      const rel = path.relative(ROOT, abs);
      if (rel.startsWith("..")) {
        broken.push(`${where}:${line} -> ${raw} (outside the repository)`);
        continue;
      }
      if (!existsSync(abs)) {
        if (!allowMissing.has(rel)) broken.push(`${where}:${line} -> ${raw} (missing)`);
        continue;
      }
      if (!anchor) continue;
      if (/^L\d+(-L\d+)?$/.test(anchor)) continue;
      if (!rel.endsWith(".md") || statSync(abs).isDirectory()) {
        broken.push(`${where}:${line} -> ${raw} (anchor on a non-Markdown target)`);
        continue;
      }
      if (!anchorsOf(readFileSync(abs, "utf8")).has(anchor)) {
        broken.push(`${where}:${line} -> ${raw} (no such heading)`);
      }
    }
  }
  assert.deepEqual(broken, [], `broken links:\n${broken.join("\n")}`);
});

test("promptfoo commands in docs and configs use the pinned release", () => {
  const runner = readFileSync(path.join(ROOT, "evals/promptfoo/runner.sh"), "utf8");
  const pin = /promptfoo@(\d+\.\d+\.\d+)/.exec(runner);
  assert.ok(pin, "runner.sh must pin a promptfoo release");
  const pinned = `promptfoo@${pin[1]}`;

  const configDir = path.join(ROOT, "evals/promptfoo/configs");
  const configs = readdirSync(configDir)
    .filter((name) => name.endsWith(".yaml"))
    .map((name) => path.join(configDir, name));
  const offenders = [];
  for (const file of [...scopedFiles(), ...configs]) {
    const lines = readFileSync(file, "utf8").split("\n");
    lines.forEach((line, i) => {
      if (!/\bpromptfoo(@\S+)?\s+(eval|validate|view)\b/.test(line)) return;
      if (/PROMPTFOO_CMD|runner\.sh/.test(line)) return;
      if (!line.includes(pinned)) offenders.push(`${path.relative(ROOT, file)}:${i + 1}: ${line.trim()}`);
    });
  }
  assert.deepEqual(offenders, [], `use ${pinned}:\n${offenders.join("\n")}`);
});
