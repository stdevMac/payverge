/**
 * Journey 1 — QR dine-in: guest scans table QR → menu → add items →
 * place order → bill visible. Payment is exercised at the ALTERNATIVE
 * PAYMENT seam (manager records a cash payment via
 * POST /api/v1/inside/bills/:bill_id/alternative-payment) — no card/crypto provider is
 * touched. Demo crypto would be Base Sepolia; deliberately out of scope.
 *
 * SEED: backend/scripts/demo_seed.sql (table CORE-T01 on demo-core-kitchen).
 * Run: PLAYWRIGHT_RUN_JOURNEYS_E2E=1 npx playwright test guest-dinein-journey
 *
 * NOTE ON CURRENT UI (differs from the wave-5 plan draft): the guest ordering
 * flow lives at /t/[tableCode]/menu (the landing /t/[tableCode] is a menu
 * gateway rendering GuestTableView). The guest menu components are driven by
 * accessible copy/roles, not test-ids; the `add-to-cart-btn` / `place-order-btn`
 * test-ids the draft referenced belong to the /business/[slug] storefront menu
 * (PublicMenuDisplay), not this dine-in flow. This spec targets the real
 * dine-in path. Copy strings are the English guest bundle
 * (src/i18n/guest-messages/en.json).
 */
import { test, expect } from "@playwright/test";
import {
  API_BASE,
  journeysEnabled,
  newManagerApi,
  resolveBusinessId,
  TABLE_CODE,
} from "./helpers/journeys";
import { prepareGuestPage } from "./helpers/guest-page";
import { ensureBusinessOpenForJourney } from "./helpers/delivery-test-setup";

test.describe("QR dine-in journey", () => {
  test.skip(
    !journeysEnabled,
    "Set PLAYWRIGHT_RUN_JOURNEYS_E2E=1 (requires full stack + demo seed)",
  );

  test.beforeAll(async () => {
    ensureBusinessOpenForJourney(await resolveBusinessId());
  });

  test("guest orders from table QR and bill reaches the manager", async ({
    page,
    playwright,
  }) => {
    test.setTimeout(90_000);
    await prepareGuestPage(page);

    // 1. Table landing (QR target) → open the menu.
    await page.goto(`/t/${TABLE_CODE}`);
    await page
      .getByRole("link", { name: /browse menu/i })
      .first()
      .click();
    await expect(page).toHaveURL(new RegExp(`/t/${TABLE_CODE}/menu`), {
      timeout: 15_000,
    });

    // 2. Add the first available menu item, then open the cart.
    const addButton = page
      .getByRole("button", { name: /add(\s|$)|add to (cart|order)/i })
      .first();
    await expect(addButton).toBeVisible({ timeout: 15_000 });
    await addButton.click();
    await page
      .getByRole("button", { name: /view cart|your order/i })
      .first()
      .click();

    // 3. Place the order from the cart.
    await page
      .getByRole("button", { name: /place order/i })
      .first()
      .click();

    // 4. Order confirmation (OrderSuccessModal → "Order Submitted!").
    await expect(page.getByText(/order submitted/i).first()).toBeVisible({
      timeout: 15_000,
    });

    // 5. Guest bill page shows the running total.
    await page.goto(`/t/${TABLE_CODE}/bill`);
    await expect(page.getByText(/total/i).first()).toBeVisible({
      timeout: 15_000,
    });

    // 6. Payment seam: manager reads the open bill via API and records an
    //    alternative (cash) payment against it. Route + body match
    //    paymentHandler.MarkAlternativePayment (backend/internal/handlers/payments.go):
    //    POST /api/v1/inside/bills/:bill_id/alternative-payment with a string
    //    `amount` in dollars and a `payment_method`. URLs are ABSOLUTE — a
    //    leading-slash path would be joined against the baseURL origin and
    //    silently drop /api/v1 (see helpers/journeys.ts).
    const businessId = await resolveBusinessId();
    const managerApi = await newManagerApi(playwright);
    try {
      // GET /api/v1/inside/businesses/:id/bills/open → server.GetOpenBusinessBills (main.go:1778).
      const billsResp = await managerApi.get(
        `${API_BASE}/inside/businesses/${businessId}/bills/open`,
      );
      expect(billsResp.ok()).toBeTruthy();
      const body = await billsResp.json();
      const bills: Array<{ id: number; status: string; total_amount: number }> =
        Array.isArray(body) ? body : (body.bills ?? body.data ?? []);
      const bill = bills.find((b) => b.status === "open") ?? bills[0];
      expect(bill).toBeTruthy();

      const payResp = await managerApi.post(
        `${API_BASE}/inside/bills/${bill.id}/alternative-payment`,
        {
          data: {
            payment_method: "cash",
            amount: String(bill.total_amount),
            business_confirmation: true,
          },
        },
      );
      expect(payResp.ok()).toBeTruthy();
    } finally {
      await managerApi.dispose();
    }
  });
});
