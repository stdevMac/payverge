import fs from "node:fs";
import path from "node:path";

const readFrontendFile = (relativePath: string): string =>
  fs.readFileSync(path.resolve(process.cwd(), relativePath), "utf8");

describe("production remediation release-gate collection", () => {
  it("selects atomic checkout identity/count and unavailable rollback acceptance", () => {
    const config = readFrontendFile("playwright.config.ts");
    const smoke = readFrontendFile(
      "tests/release/critical-release-smoke.spec.ts",
    );

    expect(config).toContain('testDir: "./tests"');
    expect(config).toContain('testMatch: "**/*.spec.ts"');
    expect(smoke).toContain("PV-PROD-001 atomic checkout identity and counts");
    expect(smoke).toContain("PV-PROD-001 unavailable checkout rollback");
    expect(smoke).toContain('reason: "manual_disabled"');
    expect(smoke).not.toContain("Idempotent checkout SETUP (create guest bill");
  });

  it("selects guest realtime, reservation occupancy, and checkout axe acceptance", () => {
    const config = readFrontendFile("playwright.config.ts");
    const accessibility = readFrontendFile(
      "tests/release/operator-accessibility.spec.ts",
    );

    expect(config).toContain('testMatch: "**/*.spec.ts"');
    for (const marker of [
      "PV-PROD-013 waiter chooser on table and menu routes",
      "PV-PROD-014 open guest bill closes live",
      "PV-PROD-015 business time and occupancy warnings",
      "PV-PROD-016 guest checkout axe",
    ]) {
      expect(accessibility).toContain(marker);
    }
  });

  it("selects business-time offer schedules and cent-exact quote parity", () => {
    const config = readFrontendFile("playwright.config.ts");
    const commerce = readFrontendFile(
      "tests/release/commerce-correctness.spec.ts",
    );

    expect(config).toContain('testMatch: "**/*.spec.ts"');
    expect(commerce).toContain("PV-PROD-003 schedules in business time");
    expect(commerce).toContain("PV-PROD-011 quotes match at cent boundaries");
  });
});
