import { expect, test } from "@playwright/test";

import {
  authenticatedPrintAPI,
  ensureBrowserPrinter,
} from "./helpers/print-station";
import {
  API_BASE,
  MANAGER_EMAIL,
  resolveBusinessId,
} from "./helpers/journeys";

const STAFF_EMAIL =
  process.env.PLAYWRIGHT_OPERATOR_EMAIL || MANAGER_EMAIL;
const runLive = process.env.PLAYWRIGHT_RUN_PRINT_E2E === "1";

let businessId = 0;

test.beforeAll(async () => {
  if (!runLive) return;
  businessId = process.env.PLAYWRIGHT_BUSINESS_ID
    ? Number(process.env.PLAYWRIGHT_BUSINESS_ID)
    : await resolveBusinessId();
});

test.describe("recent alerts with browser print station", () => {
  test.skip(
    !runLive,
    "Set PLAYWRIGHT_RUN_PRINT_E2E=1 (requires full stack + demo seed)",
  );

  test("Recent Alerts opens by pointer with printer error tray visible", async ({
    page,
    context,
    playwright,
  }) => {
    const api = await authenticatedPrintAPI(
      playwright,
      context,
      API_BASE,
      STAFF_EMAIL,
    );
    try {
      const printerId = await ensureBrowserPrinter(api, businessId, API_BASE);
      await page.addInitScript(
        ({ businessId: stationBusinessId, printerId: selectedPrinterId }) => {
          localStorage.setItem(
            `payverge_print_station:${stationBusinessId}`,
            String(selectedPrinterId),
          );
        },
        { businessId, printerId },
      );
      await page.route("**/inside/businesses/*/print/jobs/claim", (route) =>
        route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "synthetic print outage" }),
        }),
      );
      await page.goto(`/business/${businessId}/dashboard`);
      await expect(page.getByTestId("print-station-tray")).toBeVisible();

      // DashboardLayout renders desktop sidebar and mobile header controls;
      // exercise whichever trigger is actually pointer-visible at this viewport.
      const trigger = page
        .getByTestId("recent-alerts-trigger")
        .filter({ visible: true });
      await trigger.click({ position: { x: 8, y: 8 } });
      await expect(trigger).toHaveAttribute("aria-expanded", "true");
      await expect(page.locator("#recent-alerts-panel")).toBeVisible();

      await page.mouse.click(500, 120);
      await expect(trigger).toHaveAttribute("aria-expanded", "false");
    } finally {
      await api.dispose();
    }
  });
});
