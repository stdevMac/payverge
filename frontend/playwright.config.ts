import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  testMatch: "**/*.spec.ts",
  timeout: 30_000,
  retries: 0,
  workers: 1,
  fullyParallel: false,
  reporter: process.env.CI
    ? [
        ["list"],
        ["./tests/helpers/no-skipped-tests-reporter.ts"],
      ]
    : [["list"]],
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    launchOptions:
      process.env.PLAYWRIGHT_FORCE_IPV4_LOCALHOST === "1"
        ? { args: ["--host-resolver-rules=MAP localhost 127.0.0.1"] }
        : undefined,
  },
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    { name: "firefox", use: { ...devices["Desktop Firefox"] } },
    { name: "webkit", use: { ...devices["Desktop Safari"] } },
    {
      name: "mobile-chrome",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 390, height: 844 },
        hasTouch: true,
        isMobile: true,
      },
    },
    {
      name: "iphone-webkit",
      use: { ...devices["iPhone 15"] },
    },
  ],
});
