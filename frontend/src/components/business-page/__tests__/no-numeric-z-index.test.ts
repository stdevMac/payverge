/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(__dirname, "..");

function walk(dir: string): string[] {
  const out: string[] = [];
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.name.startsWith("__tests__") || entry.name.startsWith("_")) continue;
    const p = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...walk(p));
    else if (/\.(ts|tsx)$/.test(entry.name)) out.push(p);
  }
  return out;
}

describe("business-page surface — no ad-hoc numeric z-index", () => {
  test("no z-[NNNN] arbitrary values and no bare z-{20-70} except via designLayers", () => {
    const files = walk(ROOT);
    const offenders: string[] = [];
    for (const f of files) {
      const src = fs.readFileSync(f, "utf-8");
      // Permitted: z-0, z-10 (default Tailwind scale, not floating/modal).
      if (/z-\[\d+\]/.test(src)) offenders.push(`${f}: arbitrary z-[NNNN]`);
      if (/className=[^>]*\bz-(20|30|40|50|60|70)\b/.test(src)) offenders.push(`${f}: bare z-{20-70}`);
    }
    expect(offenders).toEqual([]);
  });
});
