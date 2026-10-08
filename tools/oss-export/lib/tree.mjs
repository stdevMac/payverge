// Filesystem helpers for the exported tree. Nothing here follows symlinks.

import fs from "node:fs";
import path from "node:path";

// walk returns every non-directory entry below root as
// { rel, abs, kind: "file" | "symlink" | "other", size }, sorted by rel.
// `.git` at the root is skipped (the export repo is created after the gates).
export function walk(root) {
  const out = [];
  const stack = [""];
  while (stack.length) {
    const relDir = stack.pop();
    const absDir = relDir ? path.join(root, relDir) : root;
    for (const dirent of fs.readdirSync(absDir, { withFileTypes: true })) {
      const rel = relDir ? `${relDir}/${dirent.name}` : dirent.name;
      if (!relDir && dirent.name === ".git") continue;
      const abs = path.join(root, rel);
      if (dirent.isDirectory()) {
        stack.push(rel);
      } else if (dirent.isSymbolicLink()) {
        out.push({ rel, abs, kind: "symlink", size: fs.lstatSync(abs).size });
      } else if (dirent.isFile()) {
        out.push({ rel, abs, kind: "file", size: fs.statSync(abs).size });
      } else {
        out.push({ rel, abs, kind: "other", size: 0 });
      }
    }
  }
  out.sort((a, b) => (a.rel < b.rel ? -1 : a.rel > b.rel ? 1 : 0));
  return out;
}

export function listDirs(root) {
  const dirs = [];
  const stack = [""];
  while (stack.length) {
    const relDir = stack.pop();
    const absDir = relDir ? path.join(root, relDir) : root;
    for (const dirent of fs.readdirSync(absDir, { withFileTypes: true })) {
      if (!dirent.isDirectory()) continue;
      if (!relDir && dirent.name === ".git") continue;
      const rel = relDir ? `${relDir}/${dirent.name}` : dirent.name;
      dirs.push(rel);
      stack.push(rel);
    }
  }
  return dirs;
}

// removeEmptyDirs deletes directories that no longer hold any file, deepest
// first. git archive never emits empty directories, so any empty directory
// left after the drop step was emptied by it.
export function removeEmptyDirs(root) {
  const dirs = listDirs(root).sort((a, b) => b.split("/").length - a.split("/").length);
  let removed = 0;
  for (const rel of dirs) {
    const abs = path.join(root, rel);
    if (fs.readdirSync(abs).length === 0) {
      fs.rmdirSync(abs);
      removed += 1;
    }
  }
  return removed;
}

const SNIFF_BYTES = 8000;

export function isBinaryBuffer(buffer) {
  const end = Math.min(buffer.length, SNIFF_BYTES);
  for (let i = 0; i < end; i += 1) {
    if (buffer[i] === 0) return true;
  }
  return false;
}

// lineIndex builds a sorted array of line-start offsets for offset -> line.
export function lineIndex(text) {
  const starts = [0];
  for (let i = text.indexOf("\n"); i !== -1; i = text.indexOf("\n", i + 1)) starts.push(i + 1);
  return starts;
}

export function lineAt(starts, offset) {
  let lo = 0;
  let hi = starts.length - 1;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (starts[mid] <= offset) lo = mid;
    else hi = mid - 1;
  }
  return lo + 1;
}

export function formatBytes(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
