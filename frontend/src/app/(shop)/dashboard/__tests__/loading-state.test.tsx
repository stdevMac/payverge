/**
 * Source-level regression test for the Dashboard loading state.
 *
 * Issue: `frontend/src/app/(shop)/dashboard/page.tsx` previously rendered
 * the literal word `await ` as a JSX text node in the loading branch, so
 * signed-in users briefly saw the word "await" on-screen before data loaded.
 *
 * Full rendering of the Dashboard page requires a DOM-capable Jest env plus
 * mocking wagmi, the user store, the hybrid auth provider, the analytics
 * layer, the i18n provider, and the business/analytics APIs. The underlying
 * bug is a pure source-level mistake (stray text node in JSX), so a
 * source-level assertion is both sufficient and faster to run reliably.
 */

import fs from "fs";
import path from "path";

describe("dashboard loading state source", () => {
  const pagePath = path.resolve(
    __dirname,
    "..",
    "page.tsx",
  );
  const source = fs.readFileSync(pagePath, "utf8");

  it("does not contain a stray 'await' text node in JSX", () => {
    // Match an opening `>` (closing of some tag) followed by the literal
    // word `await` (possibly with surrounding whitespace / newlines) and
    // then a `<` (opening of the next tag) — i.e. a JSX text node of
    // "await". This is the exact shape of the bug we're guarding against.
    const strayAwaitInJsx = /(>)\s*await\s+(<)/;
    expect(source).not.toMatch(strayAwaitInJsx);
  });

  it("keeps a loading indicator in the loading branch", () => {
    // Basic sanity check: when loading is true the render should still
    // provide some spinner-like element. We don't pin the exact markup;
    // we just require *some* animate-spin class or a translated loading
    // string to be present in the source.
    const hasSpinner = /animate-spin/.test(source);
    const hasLoadingLabel = /tString\("loading"\)/.test(source);
    expect(hasSpinner || hasLoadingLabel).toBe(true);
  });
});
