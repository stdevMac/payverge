/** @jest-environment jsdom */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../BusinessAboutTab.tsx"),
  "utf-8",
);

describe("BusinessAboutTab — Why Choose Us flat grid", () => {
  test("does not apply zig-zag offset to alternating items", () => {
    expect(SOURCE).not.toMatch(/md:mt-12/);
  });

  test("uses auto-rows-fr for equal-height rows", () => {
    expect(SOURCE).toMatch(/auto-rows-fr/);
  });

  test("alternates icon-container tint by row", () => {
    // Implementation uses Math.floor(idx / 2) % 2 to derive an even/odd row tint.
    expect(SOURCE).toMatch(/Math\.floor\(index\s*\/\s*2\)\s*%\s*2/);
  });
});
