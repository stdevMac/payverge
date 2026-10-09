// frontend/tests/printer-flow.spec.ts
//
// Wave 4 PRINT e2e — browser queue leadership + confirm flow.
// Requires a seeded business with operator access. Specs skip cleanly when
// PLAYWRIGHT_RUN_PRINT_E2E is not set so CI without a live stack does not flake.
import { test, expect } from "@playwright/test";

import { installPrintDialogStub } from "./helpers/print-dialog-fixture";
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

const assignPrintStation = (
  context: import("@playwright/test").BrowserContext,
  printerId: number,
) =>
  context.addInitScript(
    ({ businessId: stationBusinessId, printerId: selectedPrinterId }) => {
      localStorage.setItem(
        `payverge_print_station:${stationBusinessId}`,
        String(selectedPrinterId),
      );
    },
    { businessId, printerId },
  );

async function enqueueTestJob(
  api: import("@playwright/test").APIRequestContext,
  printerId: number,
): Promise<number> {
  const resp = await api.post(
    `${API_BASE}/inside/businesses/${businessId}/printers/${printerId}/test`,
  );
  expect(resp.ok(), await resp.text()).toBeTruthy();
  const job = await resp.json();
  expect(job.id).toBeTruthy();
  return Number(job.id);
}

test.describe("printer flow (Wave 4 browser agent)", () => {
  test.skip(
    !runLive,
    "Set PLAYWRIGHT_RUN_PRINT_E2E=1 (requires full stack + demo seed)",
  );

  test("exactly one of two tabs claims a job; confirm reaches printed", async ({
    browser,
    playwright,
  }) => {
    test.setTimeout(120_000);

    const ctx1 = await browser.newContext();
    const ctx2 = await browser.newContext();
    const api = await authenticatedPrintAPI(
      playwright,
      ctx1,
      API_BASE,
      STAFF_EMAIL,
    );
    await ctx2.addCookies((await api.storageState()).cookies);

    try {
      const printerId = await ensureBrowserPrinter(api, businessId, API_BASE);
      await assignPrintStation(ctx1, printerId);
      await assignPrintStation(ctx2, printerId);
      const jobId = await enqueueTestJob(api, printerId);

      const page1 = await ctx1.newPage();
      const page2 = await ctx2.newPage();
      const sawPrint1 = await installPrintDialogStub(page1);
      const sawPrint2 = await installPrintDialogStub(page2);

      await page1.goto(`/business/${businessId}/dashboard`);
      await page2.goto(`/business/${businessId}/dashboard`);

      const dialog = page1.getByRole("dialog").or(page2.getByRole("dialog"));
      await expect(dialog.first()).toBeVisible({ timeout: 10_000 });

      const p1 = await sawPrint1();
      const p2 = await sawPrint2();
      expect(p1 || p2).toBeTruthy();
      // Leadership failure if both tabs printed the same job.
      expect(Number(p1) + Number(p2)).toBeLessThanOrEqual(1);

      await dialog
        .first()
        .getByRole("button", { name: /printed/i })
        .click();

      await expect
        .poll(
          async () => {
            const r = await api.get(
              `${API_BASE}/inside/businesses/${businessId}/print/jobs?status=printed`,
            );
            if (!r.ok()) return false;
            const body = await r.json();
            const items: Array<{ id: number; status: string }> =
              body.items ?? [];
            return items.some((j) => j.id === jobId && j.status === "printed");
          },
          { timeout: 15_000 },
        )
        .toBeTruthy();
    } finally {
      await api.dispose();
      await ctx1.close();
      await ctx2.close();
    }
  });

  test("lease recovers after leader tab crash", async ({
    browser,
    playwright,
  }) => {
    test.setTimeout(120_000);
    const ctxLeader = await browser.newContext();
    const ctxFollower = await browser.newContext();
    const api = await authenticatedPrintAPI(
      playwright,
      ctxLeader,
      API_BASE,
      STAFF_EMAIL,
    );
    await ctxFollower.addCookies((await api.storageState()).cookies);

    try {
      const printerId = await ensureBrowserPrinter(api, businessId, API_BASE);
      await assignPrintStation(ctxLeader, printerId);
      await assignPrintStation(ctxFollower, printerId);
      const jobId = await enqueueTestJob(api, printerId);

      const leader = await ctxLeader.newPage();
      await installPrintDialogStub(leader);
      await leader.goto(`/business/${businessId}/dashboard`);

      await expect(
        leader.getByTestId("print-station-tray").or(leader.getByRole("dialog")),
      ).toBeVisible({ timeout: 30_000 });

      await ctxLeader.close();

      const follower = await ctxFollower.newPage();
      await installPrintDialogStub(follower);
      await follower.goto(`/business/${businessId}/dashboard`);

      // Reclaim runs on next claim once the lease expires (DefaultBrowserLease).
      await expect(follower.getByRole("dialog")).toBeVisible({
        timeout: 90_000,
      });
      await follower.getByRole("button", { name: /printed/i }).click();

      await expect
        .poll(
          async () => {
            const r = await api.get(
              `${API_BASE}/inside/businesses/${businessId}/print/jobs?status=printed`,
            );
            if (!r.ok()) return false;
            const body = await r.json();
            const items: Array<{ id: number }> = body.items ?? [];
            return items.some((j) => j.id === jobId);
          },
          { timeout: 20_000 },
        )
        .toBeTruthy();
    } finally {
      await api.dispose();
      await ctxFollower.close().catch(() => undefined);
    }
  });

  test("permission loss stops the agent tray", async ({
    page,
    playwright,
    context,
  }) => {
    const api = await authenticatedPrintAPI(
      playwright,
      context,
      API_BASE,
      STAFF_EMAIL,
    );
    try {
      const printerId = await ensureBrowserPrinter(api, businessId, API_BASE);
      await assignPrintStation(context, printerId);
      await page.goto(`/business/${businessId}/dashboard`);
      await expect(page.getByTestId("print-station-tray")).toBeVisible({
        timeout: 20_000,
      });

      await context.clearCookies();
      await page.reload();
      await expect(page.getByTestId("print-station-tray")).toHaveCount(0, {
        timeout: 15_000,
      });
    } finally {
      await api.dispose();
    }
  });
});

test.describe("printer settings (Sprint 1)", () => {
  test.skip(!runLive, "Set PLAYWRIGHT_RUN_PRINT_E2E=1");

  test("operator can add a browser printer", async ({
    page,
    playwright,
    context,
  }) => {
    const api = await authenticatedPrintAPI(
      playwright,
      context,
      API_BASE,
      STAFF_EMAIL,
    );
    try {
      await page.goto(`/business/${businessId}/settings/printers`);
      await page
        .getByRole("button", { name: /add a printer|add printer/i })
        .first()
        .click();
      await page.getByLabel(/printer name/i).fill("E2E Front Settings");
      await page.getByRole("button", { name: /^save$/i }).click();
      await expect(page.getByText("E2E Front Settings")).toBeVisible({
        timeout: 5_000,
      });
    } finally {
      await api.dispose();
    }
  });
});
