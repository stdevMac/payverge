/** @jest-environment jsdom */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../PublicMenuDisplay.tsx"),
  "utf-8",
);

describe("PublicMenuDisplay shell calibration", () => {
  test("imports SectionHeader for the menu intro", () => {
    expect(SOURCE).toMatch(/from\s+["']\.\/SectionHeader["']/);
  });

  test("does not render the bordered pulsing eyebrow card", () => {
    expect(SOURCE).not.toMatch(/motion-safe:animate-pulse/);
    expect(SOURCE).not.toMatch(/bg-white\/50 backdrop-blur-sm/);
  });

  test("does not render the centered intro card", () => {
    expect(SOURCE).not.toMatch(
      /text-center bg-white px-8 py-8 rounded-3xl shadow-sm border border-gray-200 max-w-3xl/,
    );
  });

  test("search input wrapper does not paint shadow-xl", () => {
    const inputBlock = SOURCE.match(/inputWrapper:\s*`[^`]+`/)?.[0] ?? "";
    expect(inputBlock).not.toMatch(/shadow-xl/);
  });

  test("offers section does not paint amber-50 background card", () => {
    expect(SOURCE).not.toMatch(/border border-amber-200 bg-amber-50/);
  });

  test("bundles section does not paint brand/10 envelope", () => {
    expect(SOURCE).not.toMatch(/border border-brand\/20 bg-brand\/10/);
  });

  test("nested offer/bundle items do not stack rounded-xl on top of card chrome", () => {
    expect(SOURCE).not.toMatch(/bg-white rounded-xl border border-amber-100/);
    expect(SOURCE).not.toMatch(/bg-white rounded-xl border border-brand\/10/);
  });

  test("fulfillment strip does not paint emerald accent boxes", () => {
    expect(SOURCE).not.toMatch(/rounded-2xl border border-emerald-100 bg-emerald-50/);
  });

  test("hardcoded English fulfillment-strip copy is replaced by i18n keys", () => {
    expect(SOURCE).not.toMatch(/Delivery context active/);
    expect(SOURCE).not.toMatch(/Menu browsing stays tied to/);
    expect(SOURCE).not.toMatch(/What happens next/);
    expect(SOURCE).not.toMatch(/Browse the menu now, then confirm/);
  });

  test("floating cart pill copy uses i18n key", () => {
    expect(SOURCE).not.toMatch(/View cart · \$/);
  });

  test("bundle thumb is gated by bundle.image truthy check", () => {
    // Find every <NextUIImage> inside the bundles map; each must be inside a
    // ternary or && guarded by bundle.image.
    const bundleSection = SOURCE.match(/{bundles\.length > 0[\s\S]*?<\/section>/g)?.[0] ?? "";
    const thumbMatches = bundleSection.match(/<NextUIImage[^/]*\/>/g) ?? [];
    for (const thumb of thumbMatches) {
      const idx = bundleSection.indexOf(thumb);
      const window = bundleSection.slice(Math.max(0, idx - 200), idx);
      expect(window).toMatch(/bundle\.image\s*&&|bundle\.image\s*\?/);
    }
  });

  test("offer thumb is gated by offer.image truthy check", () => {
    const offerSection = SOURCE.match(/{offers\.length > 0[\s\S]*?<\/section>/g)?.[0] ?? "";
    const thumbMatches = offerSection.match(/<NextUIImage[^/]*\/>/g) ?? [];
    for (const thumb of thumbMatches) {
      const idx = offerSection.indexOf(thumb);
      const window = offerSection.slice(Math.max(0, idx - 200), idx);
      expect(window).toMatch(/offer\.image\s*&&|offer\.image\s*\?/);
    }
  });
});
