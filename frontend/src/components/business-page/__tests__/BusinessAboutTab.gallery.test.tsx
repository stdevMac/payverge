/** @jest-environment jsdom */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../BusinessAboutTab.tsx"),
  "utf-8",
);

describe("BusinessAboutTab — Gallery layout", () => {
  test("does not use the fixed md:grid-cols-3 layout", () => {
    // The old 3-column grid left an empty bottom-right cell when the
    // image count didn't divide evenly. Auto-fill grid replaces it.
    expect(SOURCE).not.toMatch(/md:grid-cols-3/);
  });

  test("uses an auto-fill grid that reflows symmetrically", () => {
    // The new gallery layout uses `auto-fill` + `minmax` so any number
    // of images packs cleanly without orphan cells.
    expect(SOURCE).toMatch(
      /grid-cols-\[repeat\(auto-fill,minmax\(\d+px,1fr\)\)\]/,
    );
  });

  test("does not slice gallery images at 6", () => {
    // Grid is count-agnostic, no orphan cell to worry about.
    expect(SOURCE).not.toMatch(/galleryImages\.slice\(0,\s*6\)/);
  });

  test("retains the mobile horizontal slider", () => {
    // Mobile still uses the touch-friendly horizontal scroller; that's
    // a separate code path from the desktop grid.
    expect(SOURCE).toMatch(/md:hidden overflow-x-auto/);
  });
});
