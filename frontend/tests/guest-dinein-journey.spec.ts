/**
 * Journey 1 — QR dine-in: guest scans table QR → menu → add items →
 * place order → bill visible. Payment is exercised at the ALTERNATIVE
 * PAYMENT seam (manager records a cash payment via
 * POST /api/v1/inside/bills/:bill_id/alternative-payment) — no card/crypto provider is
 * touched. Demo crypto would be Base Sepolia; deliberately out of scope.
 *
 * SEED: backend/scripts/demo_seed.sql (table "Indoor 1" on demo-core-kitchen;
 * its high-entropy code is derived in helpers/journeys.ts TABLE_CODE).
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

    // 3. Place the order from the cart. The order response names the bill the
    //    guest's items landed on — "Indoor 1" already carries a seeded open
    //    bill (CORE-DEMO-061), so the order appends to it rather than opening
    //    a new one, and the business has a dozen other open bills.
    const orderResponsePromise = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new RegExp(`/guest/table/${TABLE_CODE}/order$`, "i").test(
          new URL(response.url()).pathname,
        ),
    );
    await page
      .getByRole("button", { name: /place order/i })
      .first()
      .click();
    const orderResponse = await orderResponsePromise;
    expect(orderResponse.status(), await orderResponse.text()).toBe(201);
    const placed = (await orderResponse.json()) as {
      bill?: { id?: number };
      order?: { bill_id?: number };
    };
    const guestBillId = placed.bill?.id ?? placed.order?.bill_id;
    expect(
      typeof guestBillId,
      `order response missing bill id: ${JSON.stringify(placed)}`,
    ).toBe("number");

    // 4. Order confirmation (OrderSuccessModal → "Order Submitted!").
    await expect(page.getByText(/order submitted/i).first()).toBeVisible({
      timeout: 15_000,
    });

    // 5. Guest bill page shows the running total.
    await page.goto(`/t/${TABLE_CODE}/bill`);
    await expect(page.getByText(/total/i).first()).toBeVisible({
      timeout: 15_000,
    });

    // 6. Payment seam: the manager finds the guest's bill among the open
    //    bills and records an alternative (cash) payment against it. Route +
    //    body match paymentHandler.MarkAlternativePayment
    //    (backend/internal/handlers/payments.go): POST
    //    /api/v1/inside/bills/:bill_id/alternative-payment with a string
    //    `amount` in dollars and a `payment_method`. Cash is refused (409
    //    cash_session_required) unless a cash-register drawer is open, so the
    //    manager opens one first — exactly what staff do before taking cash —
    //    and closes it afterwards with the tender counted. URLs are ABSOLUTE —
    //    a leading-slash path would be joined against the baseURL origin and
    //    silently drop /api/v1 (see helpers/journeys.ts).
    const businessId = await resolveBusinessId();
    const managerApi = await newManagerApi(playwright);
    const cashRegisterBase = `${API_BASE}/inside/businesses/${businessId}/cash-register`;
    let openedSessionId: number | undefined;
    try {
      // GET /api/v1/inside/businesses/:id/bills/open → server.GetOpenBusinessBills.
      const billsResp = await managerApi.get(
        `${API_BASE}/inside/businesses/${businessId}/bills/open?page=1&page_size=100`,
      );
      expect(billsResp.status(), await billsResp.text()).toBe(200);
      const body = await billsResp.json();
      const bills: Array<{ id: number; status: string; remaining: number }> =
        Array.isArray(body) ? body : (body.bills ?? body.data ?? []);
      const bill = bills.find((b) => b.id === guestBillId);
      expect(
        bill,
        `guest bill ${guestBillId} missing from open bills: ${bills.map((b) => b.id).join(",")}`,
      ).toBeTruthy();
      const amount = bill!.remaining.toFixed(2);

      const currentResp = await managerApi.get(`${cashRegisterBase}/current`);
      expect(currentResp.status(), await currentResp.text()).toBe(200);
      if (!(await currentResp.json()).session) {
        const openResp = await managerApi.post(`${cashRegisterBase}/sessions`, {
          data: {
            opening_float: "0.00",
            opening_note: "guest-dinein-journey drawer",
          },
        });
        expect(openResp.status(), await openResp.text()).toBe(201);
        openedSessionId = Number((await openResp.json()).id);
      }

      const payResp = await managerApi.post(
        `${API_BASE}/inside/bills/${bill!.id}/alternative-payment`,
        {
          headers: { "Idempotency-Key": `guest-dinein-${Date.now()}` },
          data: {
            payment_method: "cash",
            amount,
            business_confirmation: true,
          },
        },
      );
      expect(payResp.status(), await payResp.text()).toBe(200);
      expect((await payResp.json()).success).toBe(true);

      if (openedSessionId !== undefined) {
        // The drawer this journey opened expects exactly the cash tendered.
        const closeResp = await managerApi.post(
          `${cashRegisterBase}/sessions/${openedSessionId}/close`,
          {
            data: {
              counted_cash: amount,
              closing_note: "guest-dinein-journey",
            },
          },
        );
        expect(closeResp.status(), await closeResp.text()).toBe(200);
        openedSessionId = undefined;
        const closed = await closeResp.json();
        expect(Number(closed.expected_cash)).toBeCloseTo(Number(amount), 2);
        expect(Number(closed.variance)).toBeCloseTo(0, 2);
      }
    } finally {
      if (openedSessionId !== undefined) {
        // A failed step above must not leave the venue's drawer open for
        // later specs; close it best-effort without masking the failure.
        await managerApi
          .post(`${cashRegisterBase}/sessions/${openedSessionId}/close`, {
            data: { counted_cash: "0.00", closing_note: "guest-dinein cleanup" },
          })
          .catch(() => undefined);
      }
      await managerApi.dispose();
    }
  });
});
