/**
 * Regression guard (UI/UX remediation — box M31 / G3).
 *
 * The Tailwind config enables `darkMode: "class"`, but no runtime theme toggle
 * ships (see CLAUDE.md "No runtime dark mode yet"). Every `dark:`-prefixed
 * utility is therefore dead code: it renders nothing today and would light up an
 * unaudited, half-built dark theme the moment a toggle lands — a real hazard on
 * money-adjacent trust surfaces like the auth modal. M31 stripped the last nine
 * files that still carried these variants; this guard proves the strip is
 * complete and blocks any re-introduction.
 *
 * It is a source-grep guard in the same style as
 * `no-gradient-text-headings.test.tsx` and `icon-only-buttons-have-aria-label.test.tsx`:
 * it recursively walks every `.ts`/`.tsx` under `src/**` (excluding tests and
 * this guard file itself) and asserts none of them contains a Tailwind `dark:`
 * class token.
 *
 * The matcher is tuned to fire ONLY on Tailwind class tokens. A `dark:` variant
 * always sits at a class-string boundary (start-of-string, a quote/backtick, a
 * space, or a preceding variant colon) and is immediately followed by the utility
 * body — a lowercase letter, a digit, or `[` for an arbitrary value. That shape
 * deliberately does NOT match:
 *   - the JS object key `dark: foregroundColor` (space after the colon — QRCode
 *     library color config, not a class), or
 *   - prose words like "darkness"/"dark-mode" (no `:` after `dark`).
 */

import fs from "fs";
import path from "path";

const SRC_ROOT = path.resolve(__dirname, "..");

// A Tailwind `dark:` class token: preceded by a class-string boundary
// (start, quote/backtick, whitespace, or a variant `:`) and immediately
// followed by a utility body character (lowercase letter, digit, or `[`).
const DARK_VARIANT = /(?:^|["'`\s:])dark:[a-z0-9[]/;

function collectSourceFiles(dir: string, out: string[]): void {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      // Test directories are documentation/fixtures, not shipped UI.
      if (entry.name === "__tests__") continue;
      collectSourceFiles(full, out);
    } else if (
      (entry.name.endsWith(".ts") || entry.name.endsWith(".tsx")) &&
      !/\.(test|spec)\.[tj]sx?$/.test(entry.name)
    ) {
      out.push(full);
    }
  }
}

describe("no dead Tailwind dark: variants ship (no runtime dark mode yet)", () => {
  const files: string[] = [];
  collectSourceFiles(SRC_ROOT, files);

  const violations: string[] = [];

  for (const file of files) {
    // Skip this guard itself so its own documentation of the pattern (in
    // regexes/comments) does not trip the scan. Compare by basename so it
    // holds regardless of how the runner resolves __filename.
    if (path.basename(file) === "no-dark-variants.test.tsx") continue;

    const src = fs.readFileSync(file, "utf8");
    if (DARK_VARIANT.test(src)) {
      const rel = path.relative(SRC_ROOT, file);
      const line =
        src.split("\n").find((l) => DARK_VARIANT.test(l))?.trim().slice(0, 160) ??
        "";
      violations.push(`${rel}: ${line}`);
    }
  }

  it("has no source file containing a Tailwind dark: class token", () => {
    expect(violations).toEqual([]);
  });

  // Sanity checks on the matcher itself: it must catch a real dark: class and
  // must ignore the legitimate look-alikes that live in the codebase.
  it("matches a synthetic dark: class token", () => {
    expect(DARK_VARIANT.test('className="bg-white dark:bg-black"')).toBe(true);
    expect(DARK_VARIANT.test('"hover:bg-gray-100 dark:hover:bg-gray-800"')).toBe(
      true,
    );
    expect(DARK_VARIANT.test("dark:[color:red]")).toBe(true);
  });

  it("ignores non-class look-alikes (object key, prose)", () => {
    expect(DARK_VARIANT.test("          dark: foregroundColor,")).toBe(false);
    expect(DARK_VARIANT.test("no dark-mode variants, per the design system")).toBe(
      false,
    );
    expect(DARK_VARIANT.test("the darkness of the room")).toBe(false);
  });
});
