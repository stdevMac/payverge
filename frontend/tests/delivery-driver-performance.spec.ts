/**
 * Driver Performance API — integration coverage.
 *
 * Verifies the backend endpoints introduced in d9eac46:
 *   GET /businesses/:id/drivers/performance
 *   GET /businesses/:id/drivers/:driver_id/performance
 *
 * Both are gated by the new RBAC permission `delivery:drivers:read`. This
 * spec authenticates as a manager and asserts the JSON shape the FE
 * scorecard relies on (Jest covers the render itself).
 *
 * Run: PLAYWRIGHT_RUN_DELIVERY_E2E=1 npx playwright test delivery-driver-performance
 */

import { test, expect } from "@playwright/test";
import { loginStaff } from "./helpers/staff-login";

const BUSINESS_ID = process.env.PLAYWRIGHT_BUSINESS_ID || "1";
const MANAGER_EMAIL = process.env.PLAYWRIGHT_MANAGER_EMAIL || "manager@core-demo.payverge.test";

test.describe.serial("Driver Performance API", () => {
  test.skip(
    !process.env.PLAYWRIGHT_RUN_DELIVERY_E2E,
    "Set PLAYWRIGHT_RUN_DELIVERY_E2E=1 to run (requires full stack + demo seed + manager role)",
  );

  // Auth uses a 5-req/window rate-limit; pause between distinct logins to
  // avoid 429s when this spec runs alongside others that also call /staff.
  test.beforeEach(async () => {
    await new Promise((r) => setTimeout(r, 12_000));
  });

  test("manager can list driver performance and the payload carries the new KPI fields", async ({
    playwright,
  }) => {
    const apiRequest = await playwright.request.newContext({
      baseURL: process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1",
    });
    try {
      await loginStaff(apiRequest, MANAGER_EMAIL);

      const listResp = await apiRequest.get(`${process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1"}/inside/businesses/${BUSINESS_ID}/drivers/performance`);
      expect(listResp.status()).toBe(200);
      const listJson = await listResp.json();
      expect(Array.isArray(listJson.drivers)).toBe(true);
      expect(listJson.drivers.length).toBeGreaterThan(0);

      const first = listJson.drivers[0];
      // Every KPI field the FE reads must be present, including the
      // `failed_count` and `gross_*` fields added by the review-fix pass.
      for (const key of [
        "driver_id",
        "driver_name",
        "completed_today",
        "completed_week",
        "completed_all_time",
        "cancelled_count",
        "failed_count",
        "in_progress_count",
        "avg_pickup_minutes",
        "avg_delivery_minutes",
        "on_time_rate",
        "average_rating",
        "gross_fees_collected",
        "gross_tips_collected",
        "active_queue",
      ]) {
        expect(first).toHaveProperty(key);
      }
      expect(Array.isArray(first.active_queue)).toBe(true);

      const detailResp = await apiRequest.get(
        `${process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1"}/inside/businesses/${BUSINESS_ID}/drivers/${first.driver_id}/performance`,
      );
      expect(detailResp.status()).toBe(200);
      const detailJson = await detailResp.json();
      expect(detailJson.driver_id).toBe(first.driver_id);
      expect(detailJson.driver_name).toBe(first.driver_name);
    } finally {
      await apiRequest.dispose();
    }
  });

  test("non-manager (server) cannot read driver performance", async ({ playwright }) => {
    const apiRequest = await playwright.request.newContext({
      baseURL: process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1",
    });
    try {
      const serverEmail =
        process.env.PLAYWRIGHT_SERVER_EMAIL || "server1@core-demo.payverge.test";
      await loginStaff(apiRequest, serverEmail);

      const resp = await apiRequest.get(`${process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1"}/inside/businesses/${BUSINESS_ID}/drivers/performance`);
      // RBAC denies — exact code may be 403 (forbidden) or 401, both reject.
      expect([401, 403]).toContain(resp.status());
    } finally {
      await apiRequest.dispose();
    }
  });
});
