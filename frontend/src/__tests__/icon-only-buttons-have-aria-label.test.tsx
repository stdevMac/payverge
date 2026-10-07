/**
 * Accessibility guard (UI/UX remediation — box G2).
 *
 * Icon-only buttons render no visible text, so a screen reader has nothing
 * to announce unless an explicit `aria-label` is supplied. NextUI's
 * `<Button isIconOnly>` (and equivalent icon-only controls that carry the
 * `isIconOnly` prop) MUST therefore always ship an `aria-label`.
 *
 * This is a source-grep guard (same style as
 * `no-gradient-text-headings.test.tsx`): for every `.tsx` under `src/**`, it
 * locates each JSX opening tag that contains `isIconOnly` and asserts that the
 * SAME opening tag also contains `aria-label`. Matching is scoped to the tag's
 * own boundaries (from the enclosing `<` to its matching `>`) so an
 * `aria-label` on a sibling/child element cannot mask a missing one.
 */

import fs from "fs";
import path from "path";

const SRC_ROOT = path.resolve(__dirname, "..");

function collectTsxFiles(dir: string, out: string[]): void {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      collectTsxFiles(full, out);
    } else if (entry.name.endsWith(".tsx")) {
      out.push(full);
    }
  }
}

/**
 * Given a source string and the index of an `isIconOnly` occurrence, return
 * the substring of the enclosing JSX opening tag: from the nearest preceding
 * `<` up to and including the `>` that closes that tag. String literals and
 * `{ ... }` expression containers are respected so a `>` inside e.g.
 * `onPress={() => x}` does not prematurely terminate the tag.
 */
function enclosingOpeningTag(src: string, occurrence: number): string {
  const start = src.lastIndexOf("<", occurrence);
  if (start === -1) return src.slice(0, occurrence);

  let braceDepth = 0;
  let stringDelim: string | null = null;

  for (let i = start; i < src.length; i++) {
    const ch = src[i];

    if (stringDelim) {
      if (ch === stringDelim) stringDelim = null;
      continue;
    }

    if (ch === '"' || ch === "'" || ch === "`") {
      stringDelim = ch;
      continue;
    }

    if (ch === "{") braceDepth++;
    else if (ch === "}") braceDepth--;
    else if (ch === ">" && braceDepth === 0) {
      return src.slice(start, i + 1);
    }
  }

  // Unterminated tag — return the rest so the assertion still runs.
  return src.slice(start);
}

describe("icon-only buttons declare an aria-label", () => {
  const files: string[] = [];
  collectTsxFiles(SRC_ROOT, files);

  const violations: string[] = [];

  for (const file of files) {
    // Skip this guard file itself so its documentation of the pattern does
    // not trip the scan (compare by basename so it holds regardless of how
    // the test runner resolves __filename).
    if (path.basename(file) === "icon-only-buttons-have-aria-label.test.tsx") {
      continue;
    }

    const src = fs.readFileSync(file, "utf8");
    let idx = src.indexOf("isIconOnly");

    while (idx !== -1) {
      const tag = enclosingOpeningTag(src, idx);
      if (!/aria-label/.test(tag)) {
        const rel = path.relative(SRC_ROOT, file);
        const snippet = tag.replace(/\s+/g, " ").slice(0, 200);
        violations.push(`${rel}: ${snippet}`);
      }
      idx = src.indexOf("isIconOnly", idx + "isIconOnly".length);
    }
  }

  it("has no isIconOnly control missing an aria-label", () => {
    expect(violations).toEqual([]);
  });
});
