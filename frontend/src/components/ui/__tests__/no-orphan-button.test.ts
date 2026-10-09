/**
 * Q-6 regression lock: ensures the orphaned ui/Button.tsx component has been
 * deleted and no code re-introduces an import from it.
 */
import * as fs from "fs";
import * as path from "path";
import { globSync } from "glob";

const rootDir = path.resolve(__dirname, "../../../../");

describe("Q-6: ui/Button orphan guard", () => {
  it("ui/Button.tsx must not exist (it is replaced by NextUI Button)", () => {
    const orphanPath = path.join(rootDir, "src/components/ui/Button.tsx");
    expect(fs.existsSync(orphanPath)).toBe(false);
  });

  it("no source file imports from @/components/ui/Button", () => {
    const files = globSync("src/**/*.{ts,tsx}", { cwd: rootDir, ignore: ["**/__tests__/**", "**/*.test.ts", "**/*.test.tsx"] });
    const offenders: string[] = [];
    for (const file of files) {
      const abs = path.join(rootDir, file);
      const content = fs.readFileSync(abs, "utf8");
      if (content.includes('from "@/components/ui/Button"') || content.includes("from '@/components/ui/Button'")) {
        offenders.push(file);
      }
    }
    expect(offenders).toEqual([]);
  });
});
