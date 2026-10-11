/** @jest-environment jsdom */
import fs from "node:fs";
import path from "node:path";

const HERO = fs.readFileSync(
  path.resolve(__dirname, "../BusinessHeroSection.tsx"),
  "utf-8",
);

describe("BusinessHeroSection — meta-strip extracted", () => {
  test("does not render the highlights pill block inside the hero", () => {
    // The block was at lines 329-383, identifiable by deliveryHighlights iteration
    expect(HERO).not.toMatch(/deliveryHighlights\s*\.slice\(0, 3\)/);
    expect(HERO).not.toMatch(/reservationHighlights\s*\.slice\(0, 3\)/);
  });

  test("centered hero hides standalone logo above H1 on mobile", () => {
    // The logo above H1 in centered layout should be wrapped in `hidden md:` so
    // mobile centered hero doesn't render the duplicate logo.
    const centeredLogoBlock = HERO.match(/!isSplit[\s\S]*?business\.logo[\s\S]*?<\/div>/);
    if (centeredLogoBlock) {
      // If a centered-layout logo block exists, it must be hidden on mobile
      expect(centeredLogoBlock[0]).toMatch(/hidden md:/);
    }
  });
});
