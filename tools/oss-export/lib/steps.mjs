// Drop, scrub and stats steps. Each mutates or reads the exported tree only.

import fs from "node:fs";

import { compileGlob } from "./config.mjs";
import { isBinaryBuffer, removeEmptyDirs, walk } from "./tree.mjs";

// The private planning tree never ships, whatever drop.txt says. It is matched
// at any depth: a nested copy (e.g. frontend/docs/superpowers/) is just as
// private as the root one.
export const BUILTIN_DROPS = ["**/docs/superpowers"];

export function withBuiltinDrops(entries) {
  const builtins = BUILTIN_DROPS.map((pattern) => ({
    pattern,
    lineNo: 0,
    builtin: true,
    match: compileGlob(pattern),
  }));
  return [...builtins, ...entries];
}

export function applyDrop(root, entries) {
  const all = withBuiltinDrops(entries);
  const results = all.map((entry) => ({
    pattern: entry.pattern,
    lineNo: entry.lineNo,
    builtin: Boolean(entry.builtin),
    files: 0,
    bytes: 0,
  }));
  for (const file of walk(root)) {
    const index = all.findIndex((entry) => entry.match(file.rel));
    if (index === -1) continue;
    fs.unlinkSync(file.abs);
    results[index].files += 1;
    results[index].bytes += file.size;
  }
  const emptyDirsRemoved = removeEmptyDirs(root);
  return {
    entries: results,
    droppedFiles: results.reduce((sum, r) => sum + r.files, 0),
    droppedBytes: results.reduce((sum, r) => sum + r.bytes, 0),
    unmatched: results.filter((r) => r.files === 0).map((r) => r.pattern),
    emptyDirsRemoved,
  };
}

const utf8 = new TextDecoder("utf-8", { fatal: true });

export function applyScrub(root, rules) {
  const results = rules.map((rule) => ({ name: rule.name, lineNo: rule.lineNo, replacements: 0, files: [] }));
  const skippedNonUtf8 = [];
  if (rules.length === 0) return { rules: results, filesChanged: 0, skippedNonUtf8 };
  let filesChanged = 0;
  for (const file of walk(root)) {
    if (file.kind !== "file") continue;
    const applicable = [];
    rules.forEach((rule, index) => {
      if (rule.only && !rule.only(file.rel)) return;
      if (rule.exclude && rule.exclude(file.rel)) return;
      applicable.push(index);
    });
    if (applicable.length === 0) continue;
    const buffer = fs.readFileSync(file.abs);
    if (isBinaryBuffer(buffer)) continue;
    let text;
    try {
      text = utf8.decode(buffer);
    } catch {
      skippedNonUtf8.push(file.rel);
      continue;
    }
    let changed = false;
    for (const index of applicable) {
      const rule = rules[index];
      rule.regex.lastIndex = 0;
      let count = 0;
      const next = text.replace(rule.regex, (...args) => {
        count += 1;
        return rule.replacer(...args);
      });
      rule.regex.lastIndex = 0;
      if (count > 0 && next !== text) {
        text = next;
        changed = true;
        results[index].replacements += count;
        results[index].files.push(file.rel);
      }
    }
    if (changed) {
      fs.writeFileSync(file.abs, text);
      filesChanged += 1;
    }
  }
  return { rules: results, filesChanged, skippedNonUtf8 };
}

export function treeStats(root, top = 10) {
  const entries = walk(root);
  const byTop = new Map();
  let files = 0;
  let symlinks = 0;
  let bytes = 0;
  for (const entry of entries) {
    if (entry.kind === "symlink") symlinks += 1;
    else files += 1;
    bytes += entry.size;
    const head = entry.rel.includes("/") ? `${entry.rel.split("/")[0]}/` : entry.rel;
    const bucket = byTop.get(head) || { path: head, files: 0, bytes: 0 };
    bucket.files += 1;
    bucket.bytes += entry.size;
    byTop.set(head, bucket);
  }
  const largest = entries
    .filter((entry) => entry.kind === "file")
    .sort((a, b) => b.size - a.size)
    .slice(0, top)
    .map(({ rel, size }) => ({ path: rel, bytes: size }));
  return {
    files,
    symlinks,
    bytes,
    topLevel: [...byTop.values()].sort((a, b) => b.bytes - a.bytes),
    largest,
  };
}
