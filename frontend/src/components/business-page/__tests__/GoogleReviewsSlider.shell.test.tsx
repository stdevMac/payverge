/** @jest-environment jsdom */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../GoogleReviewsSlider.tsx"),
  "utf-8",
);

describe("GoogleReviewsSlider shell calibration", () => {
  test("does not redeclare local design helpers", () => {
    expect(SOURCE).not.toMatch(/const\s+getRadiusClass\s*=/);
    expect(SOURCE).not.toMatch(/const\s+getShadowClass\s*=/);
  });

  test("imports shared helpers from designClasses", () => {
    expect(SOURCE).toMatch(
      /from\s+["']\.\/designClasses["']/,
    );
  });

  test("does not paint gradient envelope on review surfaces", () => {
    expect(SOURCE).not.toMatch(/bg-gradient-to-br from-white to-gray-50\/50/);
  });

  test("does not render the rotated MessageCircle watermark", () => {
    expect(SOURCE).not.toMatch(/transform rotate-12/);
    expect(SOURCE).not.toMatch(/opacity-5/);
  });

  test("does not hardcode rounded-3xl shell radius", () => {
    expect(SOURCE).not.toMatch(/rounded-3xl/);
  });

  test("does not paint linear-gradient CTAs", () => {
    expect(SOURCE).not.toMatch(/linear-gradient\(135deg/);
  });

  test("does not animate hover -translate-y on review CTA", () => {
    expect(SOURCE).not.toMatch(/hover:-translate-y-0\.5/);
  });

  test("header heading uses design-taste typography (no tracking-wide)", () => {
    expect(SOURCE).not.toMatch(/tracking-wide/);
  });

  test("header icon container does not hardcode bg-brand/10", () => {
    expect(SOURCE).not.toMatch(/bg-brand\/10/);
  });

  test("review card does not animate transition-all duration-300 on hover", () => {
    expect(SOURCE).not.toMatch(/hover:shadow-md transition-all duration-300/);
  });

  test("review card hover uses border tone, not shadow lift", () => {
    expect(SOURCE).toMatch(/hover:border-gray-300/);
  });

  test("review card has no fixed minHeight", () => {
    expect(SOURCE).not.toMatch(/minHeight:\s*["']280px["']/);
  });

  test("empty state delegates to ReviewsEmptyState component", () => {
    expect(SOURCE).toMatch(/from\s+["']\.\/ReviewsEmptyState["']/);
    expect(SOURCE).toMatch(/<ReviewsEmptyState/);
  });
});
