/**
 * L6-2 wiring smoke (Dashboard → PaymentHistory prop).
 * Primary D1 proof is PaymentHistory.l6-2.period.test.tsx (network body).
 * This file only guards that Dashboard still threads period={…} and does not
 * reintroduce the yesterday→undefined rewrite.
 */
import fs from "fs";
import path from "path";

const dash = fs.readFileSync(path.join(__dirname, "Dashboard.tsx"), "utf8");

describe("L6-2 Dashboard period prop wiring (secondary)", () => {
  it("passes period through to PaymentHistory without yesterday→undefined rewrite", () => {
    expect(dash).not.toMatch(
      /effectivePeriod\s*===\s*["']yesterday["']\s*\?\s*undefined/,
    );
    expect(dash).toMatch(/<PaymentHistory[\s\S]*?period=\{/);
  });
});
