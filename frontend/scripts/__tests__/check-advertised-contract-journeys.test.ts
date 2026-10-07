import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { discoverAdvertisedJourneys } from "../check-advertised-contract-journeys";

function fixture(files: Record<string, string>): string {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "journey-discovery-"));
  for (const [name, body] of Object.entries(files)) {
    const target = path.join(root, name);
    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.writeFileSync(target, body);
  }
  return root;
}

describe("advertised journey discovery", () => {
  test("resolves exact Playwright declarations selected by release config", () => {
    const root = fixture({
      "tests/release/capabilities.spec.ts": `import { test } from "@playwright/test"; test("catalog exposes Stripe checkout setup", async () => {});`,
      "playwright.config.ts": `export default { testDir: "./tests", testMatch: ["release/**/*.spec.ts"] };`,
    });
    const result = discoverAdvertisedJourneys({
      testsDir: path.join(root, "tests"),
      releaseConfigPath: path.join(root, "playwright.config.ts"),
      references: [
        "capabilities.spec.ts::catalog exposes Stripe checkout setup",
      ],
    });
    expect(result.errors).toEqual([]);
  });

  test("does not accept a title substring or comment as a journey", () => {
    const root = fixture({
      "tests/release/capabilities.spec.ts": `// catalog exposes Stripe checkout setup\nimport { test } from "@playwright/test"; test("different test", async () => {});`,
      "playwright.config.ts": `export default { testDir: "./tests", testMatch: ["release/**/*.spec.ts"] };`,
    });
    const result = discoverAdvertisedJourneys({
      testsDir: path.join(root, "tests"),
      releaseConfigPath: path.join(root, "playwright.config.ts"),
      references: [
        "capabilities.spec.ts::catalog exposes Stripe checkout setup",
      ],
    });
    expect(result.errors.join("\n")).toContain("no exact test declaration");
  });

  test("rejects referenced specs with suite or runtime skips", () => {
    const root = fixture({
      "tests/release/capabilities.spec.ts": `import { test } from "@playwright/test"; test.describe("x", () => { test.skip(!process.env.RUN, "conditional"); test("real outcome", async () => {}); });`,
      "playwright.config.ts": `export default { testDir: "./tests", testMatch: ["release/**/*.spec.ts"] };`,
    });
    const result = discoverAdvertisedJourneys({
      testsDir: path.join(root, "tests"),
      releaseConfigPath: path.join(root, "playwright.config.ts"),
      references: ["capabilities.spec.ts::real outcome"],
    });
    expect(result.errors.join("\n")).toContain("skip/fixme");
  });

  test("rejects tests excluded from the release candidate config", () => {
    const root = fixture({
      "tests/nightly/capabilities.spec.ts": `import { test } from "@playwright/test"; test("real outcome", async () => {});`,
      "playwright.config.ts": `export default { testDir: "./tests", testMatch: ["release/**/*.spec.ts"] };`,
    });
    const result = discoverAdvertisedJourneys({
      testsDir: path.join(root, "tests"),
      releaseConfigPath: path.join(root, "playwright.config.ts"),
      references: ["capabilities.spec.ts::real outcome"],
    });
    expect(result.errors.join("\n")).toContain(
      "not selected by release config",
    );
  });
});
