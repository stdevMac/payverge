"use strict";

const fs = require("fs");
const os = require("os");
const path = require("path");
const { runGrayGuard } = require("../check-gray-tokens");

function tmpTree(files) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "gray-guard-"));
  for (const [rel, content] of Object.entries(files)) {
    const p = path.join(dir, rel);
    fs.mkdirSync(path.dirname(p), { recursive: true });
    fs.writeFileSync(p, content);
  }
  return dir;
}

describe("check-gray-tokens guard", () => {
  it("flags a new text-gray-* class in a non-allowlisted file", () => {
    const dir = tmpTree({ "a.tsx": 'const c = "text-gray-500";' });
    const { violations } = runGrayGuard({ dir, allowlist: {} });
    expect(violations.length).toBe(1);
    expect(violations[0]).toMatch(/a\.tsx/);
  });

  it("does not flag ink/warm text classes", () => {
    const dir = tmpTree({ "a.tsx": 'const c = "text-ink-500 text-warm-600";' });
    const { violations } = runGrayGuard({ dir, allowlist: {} });
    expect(violations).toEqual([]);
  });

  it("ignores files present in the allowlist (pre-existing debt)", () => {
    const dir = tmpTree({ "legacy.tsx": 'const c = "text-gray-500";' });
    const { violations } = runGrayGuard({ dir, allowlist: { "legacy.tsx": "pre-existing" } });
    expect(violations).toEqual([]);
  });

  it("flags bg-gray-* classes too", () => {
    const dir = tmpTree({ "Surface.tsx": 'const c = "bg-gray-100";' });
    const { violations } = runGrayGuard({ dir, allowlist: {} });
    expect(violations.length).toBe(1);
  });

  it("skips .test.tsx files", () => {
    const dir = tmpTree({ "Component.test.tsx": 'const c = "text-gray-500";' });
    const { violations } = runGrayGuard({ dir, allowlist: {} });
    expect(violations).toEqual([]);
  });
});
