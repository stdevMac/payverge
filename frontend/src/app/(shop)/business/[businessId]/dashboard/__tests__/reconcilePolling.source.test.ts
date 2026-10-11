/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../page.tsx"),
  "utf-8",
);

describe("dashboard 60s reconcile uses usePolling", () => {
  it("does not arm a bare setInterval for loadGlobalBills", () => {
    expect(SOURCE).toMatch(/usePolling\s*\(/);
    expect(SOURCE).toMatch(/from\s+["']@\/hooks\/usePolling["']/);
    expect(SOURCE).not.toMatch(/setInterval\(\s*\(\)\s*=>\s*\{\s*loadGlobalBills/);
  });
});
