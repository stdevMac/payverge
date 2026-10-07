/**
 * Regression test for Round-3 Task 12.
 *
 * CLAUDE.md design principles explicitly prohibit gradient text on
 * headings. After Round 1/2 cleanup, three gradient-text H1/H2/H3
 * headings remained (two were pointless no-op single-color gradients).
 * This test asserts none of those files use the `bg-clip-text` +
 * `text-transparent` class combination any more.
 *
 * The `bg-clip-text` utility exists in this codebase exclusively for
 * gradient-text effects, so we can assert against that single token
 * without false positives.
 */

import fs from "fs";
import path from "path";

const FRONTEND_ROOT = path.resolve(__dirname, "..", "..");

function read(relative: string): string {
  return fs.readFileSync(path.join(FRONTEND_ROOT, relative), "utf8");
}

describe("gradient-text headings are removed per design principles", () => {
  const targets = [
    "src/app/(shop)/dashboard/page.tsx",
    "src/components/business/MenuBuilder/AIMenuOnboarding/AIWizard.tsx",
  ] as const;

  for (const file of targets) {
    describe(file, () => {
      let src: string;

      beforeAll(() => {
        src = read(file);
      });

      it("does not use bg-clip-text anywhere (gradient-text only token)", () => {
        expect(src).not.toMatch(/bg-clip-text/);
      });

      it("does not combine bg-clip-text with text-transparent", () => {
        expect(src).not.toMatch(/bg-clip-text[^"']*text-transparent/);
        expect(src).not.toMatch(/text-transparent[^"']*bg-clip-text/);
      });
    });
  }
});
