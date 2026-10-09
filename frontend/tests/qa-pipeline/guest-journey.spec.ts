/**
 * Critical guest journey against local demo tables:
 *   table landing → menu (browser) → atomic order (API) → bill page (browser)
 *
 * Uses real guest endpoints (POST /guest/table/:code/order) and the browser for
 * visual/UX surfaces. Gated by PLAYWRIGHT_RUN_QA_PIPELINE=1.
 */
import { test, expect, request as apiRequestFactory } from "@playwright/test";
import path from "node:path";
import {
  buildReport,
  type Finding,
  type SurfaceResult,
  writeReport,
} from "./collectors";
import {
  API_BASE,
  listBusinessTables,
  listOwnerBusinesses,
  loginOwnerApi,
} from "./owner-session";

const ENABLED = process.env.PLAYWRIGHT_RUN_QA_PIPELINE === "1";

test.describe("QA pipeline — guest order journey", () => {
  test.skip(!ENABLED, "Set PLAYWRIGHT_RUN_QA_PIPELINE=1");
  test.describe.configure({ mode: "serial", timeout: 120_000 });

  test("table → menu → atomic order → bill for first core demo table", async ({
    page,
    baseURL,
  }) => {
    const startedAt = new Date().toISOString();
    const findings: Finding[] = [];
    const results: SurfaceResult[] = [];
    const shotDir =
      process.env.QA_SCREENSHOT_DIR ||
      path.resolve(__dirname, "../../test-results/qa-pipeline/screenshots");

    // Resolve a live table from the owner businesses (demo table codes).
    const session = await loginOwnerApi();
    const businesses = await listOwnerBusinesses(session.api, session.token);
    const core = businesses[0];
    expect(core, "need at least one owner business").toBeTruthy();
    const tables = await listBusinessTables(
      session.api,
      session.token,
      core.id,
    );
    await session.api.dispose();
    expect(tables.length, "business must have tables").toBeGreaterThan(0);
    const tableCode = tables[0].table_code;

    const guestApi = await apiRequestFactory.newContext({
      baseURL: new URL(API_BASE).origin,
    });

    try {
      // Consent banner out of the way.
      await page.addInitScript(() => {
        window.localStorage.setItem(
          "payverge_cookie_consent",
          JSON.stringify({
            version: 1,
            analytics: false,
            marketing: false,
            decidedAt: new Date().toISOString(),
          }),
        );
      });

      // ── 1. Table landing (browser) ────────────────────────────────────
      const t0 = Date.now();
      await page.goto(`/t/${tableCode}`, {
        waitUntil: "domcontentloaded",
        timeout: 30_000,
      });
      await page.waitForTimeout(600);
      const landingBody = (
        (await page.locator("body").innerText().catch(() => "")) || ""
      ).toLowerCase();
      if (/something went wrong|application error/i.test(landingBody)) {
        findings.push({
          severity: "P0",
          surfaceId: "journey.guest.table-landing",
          surfaceLabel: "Guest journey · table landing",
          url: `/t/${tableCode}`,
          kind: "crash",
          message: "Table landing showed crash/error shell",
          timestamp: new Date().toISOString(),
        });
      }
      await page
        .screenshot({
          path: path.join(shotDir, `journey-${tableCode}-landing.png`),
          fullPage: true,
        })
        .catch(() => {});
      results.push({
        surfaceId: "journey.guest.table-landing",
        surfaceLabel: "Guest journey · table landing",
        url: `/t/${tableCode}`,
        businessId: core.id,
        businessName: core.name,
        ok: findings.every((f) => f.surfaceId !== "journey.guest.table-landing"),
        durationMs: Date.now() - t0,
        findings: findings.filter(
          (f) => f.surfaceId === "journey.guest.table-landing",
        ),
        finalUrl: page.url(),
        title: await page.title().catch(() => ""),
      });

      // ── 2. Menu (browser) ─────────────────────────────────────────────
      const t1 = Date.now();
      // Prefer CTA if present, else deep-link menu.
      const browse = page.getByRole("link", { name: /browse menu|view menu|menu/i }).first();
      if (await browse.isVisible().catch(() => false)) {
        await browse.click().catch(() => {});
      } else {
        await page.goto(`/t/${tableCode}/menu`, {
          waitUntil: "domcontentloaded",
          timeout: 30_000,
        });
      }
      await page.waitForTimeout(800);
      await expect
        .poll(() => page.url(), { timeout: 15_000 })
        .toMatch(new RegExp(`/t/${tableCode}/menu`));

      // Visible add affordance or menu item name from live catalog.
      const menuOk =
        (await page
          .getByRole("button", { name: /add(\s|$)|add to (cart|order)/i })
          .first()
          .isVisible()
          .catch(() => false)) ||
        (await page.getByText(/harvest bowl|menu|\$/i).first().isVisible().catch(() => false));
      if (!menuOk) {
        findings.push({
          severity: "P1",
          surfaceId: "journey.guest.menu",
          surfaceLabel: "Guest journey · menu",
          url: page.url(),
          kind: "assert",
          message: "Menu page rendered without add/item affordances",
          timestamp: new Date().toISOString(),
        });
      }
      await page
        .screenshot({
          path: path.join(shotDir, `journey-${tableCode}-menu.png`),
          fullPage: true,
        })
        .catch(() => {});
      results.push({
        surfaceId: "journey.guest.menu",
        surfaceLabel: "Guest journey · menu",
        url: page.url(),
        businessId: core.id,
        businessName: core.name,
        ok: !findings.some(
          (f) => f.surfaceId === "journey.guest.menu" && f.severity !== "P3",
        ),
        durationMs: Date.now() - t1,
        findings: findings.filter((f) => f.surfaceId === "journey.guest.menu"),
        finalUrl: page.url(),
        title: await page.title().catch(() => ""),
      });

      // ── 3. Atomic order via real public API ───────────────────────────
      const t2 = Date.now();
      const menuResp = await guestApi.get(
        `${API_BASE}/guest/table/${tableCode}/menu`,
      );
      expect(menuResp.ok(), await menuResp.text()).toBeTruthy();
      const menuBody = await menuResp.json();
      const items = (
        (menuBody.categories ?? []) as Array<{
          items?: Array<{
            id?: string | number;
            name?: string;
            price?: number;
            is_available?: boolean;
          }>;
        }>
      ).flatMap((c) => c.items ?? []);
      const available = items.find(
        (i) =>
          i?.id &&
          i?.name &&
          i.is_available !== false &&
          typeof i.price === "number",
      );
      expect(available, "need an orderable menu item").toBeTruthy();

      const requestId = `qa-pipeline-guest-${Date.now()}`;
      const orderResp = await guestApi.post(
        `${API_BASE}/guest/table/${tableCode}/order`,
        {
          headers: { "X-Request-Id": requestId },
          data: {
            items: [
              {
                menu_item_name: available!.name,
                menu_item_id: String(available!.id),
                quantity: 1,
                price: available!.price,
              },
            ],
            notes: "qa-pipeline guest journey",
          },
        },
      );
      const orderStatus = orderResp.status();
      const orderText = await orderResp.text();
      if (orderStatus !== 201 && orderStatus !== 200) {
        findings.push({
          severity: "P0",
          surfaceId: "journey.guest.atomic-order",
          surfaceLabel: "Guest journey · atomic order",
          url: `${API_BASE}/guest/table/${tableCode}/order`,
          kind: "network",
          message: `POST order failed ${orderStatus}: ${orderText.slice(0, 200)}`,
          timestamp: new Date().toISOString(),
        });
      }
      let billId = 0;
      let publicToken = "";
      let orderId = 0;
      try {
        const orderBody = JSON.parse(orderText) as {
          bill?: { id?: number; public_token?: string };
          order?: { id?: number };
        };
        billId = Number(orderBody.bill?.id || 0);
        publicToken = String(orderBody.bill?.public_token || "");
        orderId = Number(orderBody.order?.id || 0);
        if (!billId) {
          findings.push({
            severity: "P0",
            surfaceId: "journey.guest.atomic-order",
            surfaceLabel: "Guest journey · atomic order",
            url: `${API_BASE}/guest/table/${tableCode}/order`,
            kind: "assert",
            message: "Order response missing bill.id",
            timestamp: new Date().toISOString(),
          });
        }
      } catch {
        findings.push({
          severity: "P0",
          surfaceId: "journey.guest.atomic-order",
          surfaceLabel: "Guest journey · atomic order",
          url: `${API_BASE}/guest/table/${tableCode}/order`,
          kind: "assert",
          message: "Order response was not JSON",
          timestamp: new Date().toISOString(),
        });
      }
      results.push({
        surfaceId: "journey.guest.atomic-order",
        surfaceLabel: "Guest journey · atomic order",
        url: `${API_BASE}/guest/table/${tableCode}/order`,
        businessId: core.id,
        businessName: core.name,
        ok: !findings.some(
          (f) =>
            f.surfaceId === "journey.guest.atomic-order" &&
            (f.severity === "P0" || f.severity === "P1"),
        ),
        durationMs: Date.now() - t2,
        findings: findings.filter(
          (f) => f.surfaceId === "journey.guest.atomic-order",
        ),
        finalUrl: publicToken || String(billId),
        title: `bill=${billId}`,
      });

      // ── 4. Bill page (browser) ────────────────────────────────────────
      const t3 = Date.now();
      await page.goto(`/t/${tableCode}/bill`, {
        waitUntil: "domcontentloaded",
        timeout: 30_000,
      });
      await page.waitForTimeout(900);
      const billText = (
        (await page.locator("body").innerText().catch(() => "")) || ""
      ).toLowerCase();
      if (/something went wrong|application error/i.test(billText)) {
        findings.push({
          severity: "P0",
          surfaceId: "journey.guest.bill",
          surfaceLabel: "Guest journey · bill",
          url: `/t/${tableCode}/bill`,
          kind: "crash",
          message: "Bill page showed crash/error shell",
          timestamp: new Date().toISOString(),
        });
      }
      // Should show total / bill context after order.
      const hasTotal = /total|bill|pay|\$|usd/i.test(billText);
      if (!hasTotal) {
        findings.push({
          severity: "P1",
          surfaceId: "journey.guest.bill",
          surfaceLabel: "Guest journey · bill",
          url: `/t/${tableCode}/bill`,
          kind: "assert",
          message: "Bill page missing total/pay copy after order",
          timestamp: new Date().toISOString(),
        });
      }
      await page
        .screenshot({
          path: path.join(shotDir, `journey-${tableCode}-bill.png`),
          fullPage: true,
        })
        .catch(() => {});
      results.push({
        surfaceId: "journey.guest.bill",
        surfaceLabel: "Guest journey · bill",
        url: `/t/${tableCode}/bill`,
        businessId: core.id,
        businessName: core.name,
        ok: !findings.some(
          (f) =>
            f.surfaceId === "journey.guest.bill" &&
            (f.severity === "P0" || f.severity === "P1"),
        ),
        durationMs: Date.now() - t3,
        findings: findings.filter((f) => f.surfaceId === "journey.guest.bill"),
        finalUrl: page.url(),
        title: await page.title().catch(() => ""),
      });

      // H-03: cancel the journey's pending order so demo bills don't accumulate
      // duplicate Harvest Bowl lines / kitchen noise across QA runs.
      if (orderId > 0) {
        const cancelResp = await guestApi.post(
          `${API_BASE}/guest/table/${tableCode}/orders/${orderId}/cancel`,
          {
            headers: { "X-Request-Id": `qa-pipeline-guest-cancel-${Date.now()}` },
            data: { reason: "qa-pipeline cleanup" },
          },
        );
        if (!cancelResp.ok()) {
          findings.push({
            severity: "P2",
            surfaceId: "journey.guest.cleanup",
            surfaceLabel: "Guest journey · order cleanup",
            url: `${API_BASE}/guest/table/${tableCode}/orders/${orderId}/cancel`,
            kind: "network",
            message: `POST cancel after journey failed ${cancelResp.status()}`,
            timestamp: new Date().toISOString(),
          });
        }
      }
    } finally {
      await guestApi.dispose();
    }

    const outDir =
      process.env.QA_REPORT_DIR ||
      path.resolve(__dirname, "../../test-results/qa-pipeline");
    const report = buildReport(
      startedAt,
      results,
      findings,
      baseURL || "http://localhost:3000",
      API_BASE,
    );
    writeReport(report, path.join(outDir, "partial-guest-journey"));

    const p0 = report.summary.bySeverity.P0;
    expect
      .soft(
        p0,
        `P0 on guest journey:\n${findings
          .filter((f) => f.severity === "P0")
          .map((f) => f.message)
          .join("\n")}`,
      )
      .toBe(0);
    expect(results.length).toBeGreaterThanOrEqual(3);
  });
});
