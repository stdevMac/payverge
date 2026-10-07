/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../page.tsx"),
  "utf-8",
);

describe("dashboard bills feed uses the extractable live-feed throw path (#647)", () => {
  it("reconciles active bills through fetchActiveBillsFeed, not a bare getAllActiveBills", () => {
    expect(SOURCE).toMatch(/fetchActiveBillsFeed\(numericBusinessId\)/);
    expect(SOURCE).not.toMatch(/getAllActiveBills\(/);
    expect(SOURCE).toMatch(/setGlobalBillsFailed\(true\)/);
    expect(SOURCE).toMatch(/setGlobalBillsFailed\(false\)/);
    expect(SOURCE).toMatch(/globalBillsFailed=\{globalBillsFailed\}/);
  });

  it("still unblocks the skeleton after the first bills attempt (#96)", () => {
    expect(SOURCE).toMatch(/setGlobalBillsLoaded\(true\)/);
  });
});
