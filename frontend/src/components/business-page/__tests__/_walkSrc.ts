import fs from "fs";
import path from "path";

const FRONTEND_ROOT = path.resolve(__dirname, "../../../..");
const SRC_ROOT = path.join(FRONTEND_ROOT, "src");

const SKIP_DIRS = new Set([
  "node_modules",
  ".next",
  "__tests__",
  "test",
  "tests",
]);

export function walkSrcFiles(): string[] {
  const out: string[] = [];
  const stack: string[] = [SRC_ROOT];
  while (stack.length) {
    const dir = stack.pop()!;
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true });
    } catch {
      continue;
    }
    for (const entry of entries) {
      if (SKIP_DIRS.has(entry.name)) continue;
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        stack.push(full);
      } else if (/\.(tsx?|jsx?)$/.test(entry.name)) {
        out.push(full);
      }
    }
  }
  return out;
}

export function fileContains(filePath: string, needle: string | RegExp): boolean {
  const src = fs.readFileSync(filePath, "utf8");
  return needle instanceof RegExp ? needle.test(src) : src.includes(needle);
}

export { FRONTEND_ROOT, SRC_ROOT };
