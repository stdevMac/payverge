/**
 * L4-10 — AI insights TrendChart must pass operator locale (no English default).
 * Structural test over the shipped call site.
 */
import fs from "fs";
import path from "path";

describe("AiWaiterDashboard TrendChart locale (L4-10)", () => {
  it("passes locale={locale} into TrendChart at the insights call site", () => {
    const src = fs.readFileSync(
      path.join(__dirname, "../AiWaiterDashboard.tsx"),
      "utf8",
    );
    // Capture the insights TrendChart block (conversation_trends_7d).
    const match = src.match(
      /<TrendChart[\s\S]*?points=\{insights\.conversation_trends_7d\}[\s\S]*?\/>/,
    );
    expect(match).not.toBeNull();
    expect(match![0]).toMatch(/locale=\{locale\}/);
  });
});
