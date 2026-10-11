/**
 * Journey 2 — Delivery: storefront → delivery checkout (steps reused verbatim
 * from delivery-guest-happy-path.spec.ts) → operator accepts via
 * PUT /api/v1/inside/businesses/:id/deliveries/:delivery_id/status
 * (main.go:1938, deliveryHandler.UpdateDeliveryStatus) → tracking page
 * reflects acceptance and DOES NOT crash when the ETA countdown expires
 * (Wave 4 TDZ fix regression guard).
 *
 * ADAPTATIONS vs the wave-5 plan draft (verified against live code):
 *   - There is NO "accepted" delivery status. The enum
 *     (backend/internal/database/delivery_models.go) is pending → confirmed →
 *     preparing → ready → assigned → picked_up → in_transit → nearby →
 *     delivered / cancelled / failed. Operator "acceptance" of a fresh guest
 *     checkout (created at "pending", delivery_v1.go:585) is the
 *     pending → "confirmed" edge, which validDeliveryTransitions allows
 *     (delivery_status_transition.go:39).
 *   - Manager routes live under /api/v1/inside (main.go:1595). We pass
 *     ABSOLUTE URLs to the manager context: Playwright resolves a
 *     leading-slash path against the baseURL ORIGIN (new URL(url, base)),
 *     which would silently drop the /api/v1/inside prefix.
 *   - GET .../deliveries returns { deliveries, total, has_more }
 *     (services.DeliveryListResult), not a bare array.
 *   - Wave 3 removed the live driver map from the tracking page. The honest
 *     surfaces are the 5-stage ladder (data-testid="status-timeline",
 *     StageLadder in track/page.tsx) and the ETADisplay card ("Estimated Arrival").
 *     "confirmed" maps to the "Accepted" ladder stage (STAGE_BY_STATUS).
 *
 * SEED DEPENDENCY: backend/scripts/demo_seed.sql — delivery_enabled=true with
 * a zone covering postal code 10001 on demo-core-kitchen. If the seed lacks
 * delivery config the spec SKIPS (public delivery-settings probe) rather than
 * failing.
 *
 * Run: PLAYWRIGHT_RUN_JOURNEYS_E2E=1 npx playwright test delivery-operator-journey
 */

import { test, expect, request as apiRequestFactory } from "@playwright/test";
import {
  ensureBusinessOpenForJourney,
  ensureDeliveryCapacity,
} from "./helpers/delivery-test-setup";
import { prepareGuestPage } from "./helpers/guest-page";
import {
  API_BASE,
  BUSINESS_SLUG,
  journeysEnabled,
  newManagerApi,
  resolveBusinessId,
} from "./helpers/journeys";

const SERVICEABLE_ADDRESS = {
  street: "350 W 34th St",
  city: "New York",
  postal: "10001",
  country: "United States",
};

const GUEST = {
  name: "Journey Two Guest",
  phone: "+1 555-000-0002",
  email: "journey-two@payverge.test",
};

test.describe("Delivery journey with operator acceptance", () => {
  test.skip(
    !journeysEnabled,
    "Set PLAYWRIGHT_RUN_JOURNEYS_E2E=1 (requires full stack + demo seed)",
  );

  let deliveryConfigured = false;

  test.beforeAll(async () => {
    const businessId = await resolveBusinessId();

    // Detect whether the seed actually has delivery configured — public
    // endpoint GET /businesses/:business_id/delivery-settings (main.go:1929).
    // If not, the test skips with a clear message instead of failing on a
    // missing Delivery tab.
    const probe = await apiRequestFactory.newContext();
    try {
      const resp = await probe.get(
        `${API_BASE}/businesses/${businessId}/delivery-settings`,
      );
      if (resp.ok()) {
        const settings = await resp.json();
        deliveryConfigured = settings.delivery_enabled === true;
      }
    } finally {
      await probe.dispose();
    }

    // Demo seed runs at capacity-full on purpose (operator "capacity reached"
    // UI); bump the cap so a fresh quote can be placed.
    ensureDeliveryCapacity(businessId, 50);
    ensureBusinessOpenForJourney(businessId);
  });

  test("guest orders delivery, operator accepts, tracking survives countdown expiry", async ({
    page,
    playwright,
  }) => {
    test.skip(
      !deliveryConfigured,
      `Demo seed has no enabled delivery settings for ${BUSINESS_SLUG} — re-run backend/scripts/demo_seed.sql`,
    );
    test.setTimeout(120_000);
    await prepareGuestPage(page);

    // ── Steps 1–9: guest delivery checkout ─────────────────────────────────
    // Copied verbatim from delivery-guest-happy-path.spec.ts (single source
    // of truth for the storefront delivery selectors).

    // Step 1: public business page (/b/<custom_url>; legacy /business/… 404s).
    await page.goto(`/b/${BUSINESS_SLUG}`);
    await expect(page.getByRole("tab", { name: /menu/i })).toBeVisible({
      timeout: 15_000,
    });

    // Step 2: Delivery tab → "Check address and start order" CTA.
    await page
      .getByRole("tab", { name: /delivery/i })
      .first()
      .click();
    const startOrderBtn = page.getByRole("button", {
      name: /check address and start order/i,
    });
    await expect(startOrderBtn).toBeVisible({ timeout: 10_000 });
    await startOrderBtn.click();

    // Step 3: address modal (NextUI Inputs use label=…, so getByLabel).
    await page.getByLabel(/street address/i).fill(SERVICEABLE_ADDRESS.street);
    await page.getByLabel(/^city$/i).fill(SERVICEABLE_ADDRESS.city);
    await page.getByLabel(/postal code/i).fill(SERVICEABLE_ADDRESS.postal);
    const countryField = page.getByLabel(/^country$/i);
    if ((await countryField.inputValue()) === "") {
      await countryField.fill(SERVICEABLE_ADDRESS.country);
    }

    // Step 4: quote — CTA enables when the address is serviceable.
    await page.getByRole("button", { name: /check address/i }).click();
    const browseMenuBtn = page.getByRole("button", {
      name: /browse menu with delivery/i,
    });
    await expect(browseMenuBtn).toBeEnabled({ timeout: 15_000 });
    await browseMenuBtn.click();

    // Step 5: menu with delivery context — open a standalone item above the
    // $15 delivery minimum (Seared Salmon Bowl = $22 in the seed).
    await expect(page.getByText(/explore our offerings/i)).toBeVisible({
      timeout: 10_000,
    });
    const targetItemName =
      process.env.PLAYWRIGHT_DELIVERY_ITEM || "Seared Salmon Bowl";
    const itemHeading = page
      .getByRole("heading", {
        level: 3,
        name: new RegExp(`^${targetItemName}$`, "i"),
      })
      .first();
    await expect(itemHeading).toBeVisible({ timeout: 10_000 });
    await itemHeading.scrollIntoViewIfNeeded();
    await itemHeading.click();

    const addToCart = page.getByTestId("add-to-cart-btn");
    await expect(addToCart).toBeVisible({ timeout: 8_000 });
    await addToCart.click();

    // Step 6: open the cart.
    await page.getByTestId("open-cart-btn").click();
    await expect(page.getByTestId("cart-panel")).toBeVisible({
      timeout: 5_000,
    });

    // Step 7: checkout.
    await page.getByTestId("checkout-btn").click();
    const placeOrder = page.getByTestId("place-order-btn");
    await expect(placeOrder).toBeVisible({ timeout: 10_000 });

    // Step 8: guest details.
    await page
      .getByLabel(/full name|name/i)
      .first()
      .fill(GUEST.name);
    await page.getByLabel(/phone/i).first().fill(GUEST.phone);
    await page.getByLabel(/email/i).first().fill(GUEST.email);

    // Step 9: place the order → redirect to the tracking page.
    await placeOrder.click();
    await page.waitForURL(/\/delivery\/[A-Z0-9-]+\/track/i, {
      timeout: 30_000,
    });

    // ── Step 10: capture the delivery number ───────────────────────────────
    const match = page.url().match(/\/delivery\/([^/]+)\/track/i);
    expect(match).not.toBeNull();
    const deliveryNumber = match![1];

    // ── Step 11: operator accepts via API (pending → confirmed) ────────────
    const businessId = await resolveBusinessId();
    const managerApi = await newManagerApi(playwright);
    try {
      // Absolute URL on purpose — see ADAPTATIONS header.
      const listResp = await managerApi.get(
        `${API_BASE}/inside/businesses/${businessId}/deliveries`,
      );
      expect(listResp.ok()).toBeTruthy();
      const list = await listResp.json();
      const deliveries: Array<{
        id: number;
        order_id?: number;
        delivery_number: string;
        status: string;
      }> = Array.isArray(list) ? list : (list.deliveries ?? []);
      const order = deliveries.find(
        (d) => d.delivery_number === deliveryNumber,
      );
      expect(
        order,
        `delivery ${deliveryNumber} visible to the operator`,
      ).toBeTruthy();
      expect(
        order!.order_id,
        "delivery must expose its linked order",
      ).toBeTruthy();

      const claimResp = await managerApi.post(
        `${API_BASE}/inside/businesses/${businessId}/deliveries/${order!.id}/claim`,
        { data: { steal: false } },
      );
      expect(
        claimResp.ok(),
        `claim failed: ${claimResp.status()} ${await claimResp.text()}`,
      ).toBeTruthy();

      const acceptResp = await managerApi.put(
        `${API_BASE}/inside/businesses/${businessId}/orders/${order!.order_id}/status`,
        { data: { status: "approved" } },
      );
      expect(
        acceptResp.ok(),
        `accept linked order failed: ${acceptResp.status()} ${await acceptResp.text()}`,
      ).toBeTruthy();
    } finally {
      await managerApi.dispose();
    }

    // Backend truth: public track endpoint now serves the confirmed row.
    const trackResp = await page.request.get(
      `${API_BASE}/delivery/${deliveryNumber}/track`,
    );
    expect(trackResp.status()).toBe(200);
    const trackJson = await trackResp.json();
    expect(trackJson.status).toBe("confirmed");

    // ── Step 12: tracking page reflects acceptance ─────────────────────────
    // Install the fake clock BEFORE the reload so page timers (15s polling,
    // ETA math) run under it. Then verify the 5-stage ladder renders with
    // "Accepted" as the CURRENT stage. The ladder exposes the current stage
    // semantically (aria-current="step" on its listitem); the label colour is
    // a contrast detail pinned by page.contrast.test.tsx, not asserted here.
    await page.clock.install();
    await page.reload();

    const timeline = page.getByTestId("status-timeline");
    await expect(timeline).toBeVisible({ timeout: 15_000 });
    await expect(timeline.getByText(/^accepted$/i)).toBeVisible();
    const currentStage = timeline.locator(
      '[role="listitem"][aria-current="step"]',
    );
    await expect(currentStage).toHaveCount(1);
    await expect(currentStage).toHaveText(/^\s*accepted\s*$/i);

    // ── Step 13: countdown-expiry regression guard (Wave 4 TDZ fix) ────────
    // Fast-forward an hour so any ETA/payment countdown runs past zero and the
    // polling interval fires repeatedly; the page must not crash or blank out.
    await page.clock.fastForward("01:00:00");
    await expect(
      page.getByText(/application error|something went wrong/i),
    ).toHaveCount(0);
    await expect(page.getByTestId("error-message")).toHaveCount(0);
    await expect(page.locator("main").first()).toBeVisible();
    await expect(timeline).toBeVisible();
  });
});
