import fs from "fs";
import path from "path";

const SRC = path.resolve(__dirname, "..");
const SKIP_DIRS = new Set(["node_modules", ".next", "__tests__", "test", "tests"]);
const ALPHA_RING = /focus-visible:ring-brand\/(30|40)\b/;

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (SKIP_DIRS.has(entry.name)) continue;
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walk(full, out);
    } else if (/\.(tsx?|jsx?)$/.test(entry.name) && !entry.name.includes(".test.")) {
      out.push(full);
    }
  }
  return out;
}

describe("focus ring contrast", () => {
  it("does not use 30/40 alpha brand focus rings (below 3:1 vs white)", () => {
    const hits: string[] = [];
    for (const file of walk(SRC)) {
      const src = fs.readFileSync(file, "utf8");
      if (ALPHA_RING.test(src)) {
        hits.push(path.relative(SRC, file));
      }
    }
    expect(hits).toEqual([]);
  });
});
