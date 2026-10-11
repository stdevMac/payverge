import { defineConfig, devices } from "@playwright/test";

/**
 * Dedicated config for the catalog-driven local QA crawl.
 *
 * Requires:
 *   - a local stack with demo data and an owner admin on :3000 / :8080
 *   - PLAYWRIGHT_RUN_QA_PIPELINE=1
 *
 * Use localhost (not 127.0.0.1) so Domain=localhost session cookies match.
 */
export default defineConfig({
  testDir: "./tests/qa-pipeline",
  testMatch: "**/*.spec.ts",
  timeout: 120_000,
  globalTimeout: 30 * 60_000,
  retries: 0,
  workers: 1,
  fullyParallel: false,
  reporter: [
    ["list"],
    ["json", { outputFile: "test-results/qa-pipeline/playwright.json" }],
  ],
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "off",
    ignoreHTTPSErrors: true,
    launchOptions:
      process.env.PLAYWRIGHT_FORCE_IPV4_LOCALHOST === "1"
        ? { args: ["--host-resolver-rules=MAP localhost 127.0.0.1"] }
        : undefined,
  },
  // GAP-1: desktop + phone (390×844) + tablet (834×1112). Responsive assertions
  // live in responsive-owner-tabs.spec.ts; crawl specs still run on all projects
  // (workers:1 keeps wall clock bounded). Use Chromium for all three so the
  // harness does not require webkit/firefox browser downloads.
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    {
      name: "mobile-390",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 390, height: 844 },
        isMobile: true,
        hasTouch: true,
      },
    },
    {
      name: "tablet-834",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 834, height: 1112 },
        isMobile: true,
        hasTouch: true,
      },
    },
  ],
});
