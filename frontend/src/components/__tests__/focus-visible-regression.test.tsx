/**
 * Regression test for WCAG 2.4.7 (Focus Visible).
 *
 * Components that remove the default focus ring with
 * `focus:outline-none` or `focus-visible:outline-none` MUST supply a
 * replacement focus indicator. The approved pattern across the Payverge
 * frontend is a `focus-visible:ring-*` class in the same className string.
 *
 * This test reads the source of each tracked component, isolates every
 * className string literal, and verifies that any class list containing
 * `focus:outline-none` (or `focus-visible:outline-none`) also contains a
 * matching `focus-visible:ring-` class in the same literal.
 */
import * as fs from "fs";
import * as path from "path";

const TRACKED_FILES = [
  path.resolve(__dirname, "../auth/AuthModal.tsx"),
  path.resolve(__dirname, "../business/BillFilters.tsx"),
];

/**
 * Extract every className="..." string literal from a source file.
 * Intentionally limited to the simple double-quoted form because every
 * focus-outline-none occurrence in the tracked files uses that form.
 */
function extractClassNameLiterals(source: string): string[] {
  const out: string[] = [];
  const re = /className\s*=\s*"([^"]*)"/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(source)) !== null) {
    out.push(m[1]);
  }
  return out;
}

describe("focus-visible regression", () => {
  it.each(TRACKED_FILES)(
    "every outline-none class list in %s also defines a focus-visible:ring- indicator",
    (file) => {
      const src = fs.readFileSync(file, "utf8");
      const offenders: string[] = [];
      for (const classList of extractClassNameLiterals(src)) {
        const classes = classList.split(/\s+/).filter(Boolean);
        const hasOutlineNone = classes.some(
          (c) =>
            c === "focus:outline-none" || c === "focus-visible:outline-none",
        );
        if (!hasOutlineNone) continue;
        const hasRingIndicator = classes.some((c) =>
          c.startsWith("focus-visible:ring-"),
        );
        if (!hasRingIndicator) {
          offenders.push(classList);
        }
      }
      expect(offenders).toEqual([]);
    },
  );
});
