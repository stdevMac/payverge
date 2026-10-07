/** @jest-environment node */
/**
 * Static gate for GAP-1 config: projects must include phone + tablet viewports
 * in addition to Desktop Chrome (no live stack required).
 */
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(__dirname, "../..");
const CONFIG = fs.readFileSync(
  path.join(ROOT, "playwright.qa-pipeline.config.ts"),
  "utf-8",
);

describe("playwright.qa-pipeline.config (GAP-1)", () => {
  it("declares desktop, mobile-390 (390×844), and tablet-834 (834×1112) projects", () => {
    expect(CONFIG).toMatch(/name:\s*["']chromium["']/);
    expect(CONFIG).toMatch(/name:\s*["']mobile-390["']/);
    expect(CONFIG).toMatch(/name:\s*["']tablet-834["']/);
    expect(CONFIG).toMatch(/width:\s*390/);
    expect(CONFIG).toMatch(/height:\s*844/);
    expect(CONFIG).toMatch(/width:\s*834/);
    expect(CONFIG).toMatch(/height:\s*1112/);
  });

  it("responsive-owner-tabs.spec reuses OWNER_TABS and owner-session", () => {
    const spec = fs.readFileSync(
      path.join(ROOT, "tests/qa-pipeline/responsive-owner-tabs.spec.ts"),
      "utf-8",
    );
    expect(spec).toMatch(/OWNER_TABS/);
    expect(spec).toMatch(/loginOwnerApi/);
    expect(spec).toMatch(/tab\.kitchen/);
    expect(spec).toMatch(/tab\.counter/);
    expect(spec).toMatch(/tab\.cash-register/);
    expect(spec).toMatch(/tab\.bills/);
  });
});
