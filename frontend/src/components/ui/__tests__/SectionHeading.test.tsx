/** @jest-environment jsdom */
import fs from "fs";
import path from "path";

it("FiscalDashboard panel headings are sans (serif budget: page title + activation panels only)", () => {
  const src = fs.readFileSync(path.resolve(__dirname, "../../business/fiscal/FiscalDashboard.tsx"), "utf8");
  expect(src).not.toMatch(/font-title/);
  expect(src).toMatch(/text-heading-sm font-semibold text-ink-950/);
});
