/**
 * Delivery guest happy-path spec.
 *
 * End-to-end: visit a public business page, request a delivery quote
 * for a serviceable address, browse the menu with delivery context,
 * add an item to the cart, check out as a guest, and land on the
 * tracking page for the freshly-created delivery_order.
 *
 * Selector strategy: prefer existing data-testids (add-to-cart-btn,
 * open-cart-btn, checkout-btn, place-order-btn) and stable role+name
 * pairs (role="tab" name="Delivery"). NextUI Inputs use label="…" not
 * placeholder, so getByLabel(...) is the right matcher for the address
 * modal fields.
 *
 * SEED DEPENDENCY: backend/scripts/demo_seed.sql provisions
 *   - business with custom_url = "demo-core-kitchen"
 *   - delivery_settings with delivery_enabled=true,
 *     in_house_delivery_enabled=true
 *   - a zone covering postal_code "10001"
 *   - menu items in stock
 *
 * Run: PLAYWRIGHT_RUN_DELIVERY_E2E=1 npx playwright test delivery-guest-happy-path
 */

import { test, expect, request as apiRequestFactory } from "@playwright/test";
import { ensureDeliveryCapacity } from "./helpers/delivery-test-setup";

const BUSINESS_SLUG = process.env.PLAYWRIGHT_DELIVERY_SLUG || "demo-core-kitchen";
const API_BASE = process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1";

async function resolveBusinessId(): Promise<number> {
  // Query the public business endpoint so we don't need to hardcode the
  // numeric id — the seed re-runs and shifts ids each time it deletes the
  // demo block before re-inserting.
  const ctx = await apiRequestFactory.newContext();
  try {
    const resp = await ctx.get(`${API_BASE}/business/${BUSINESS_SLUG}`);
    if (!resp.ok()) {
      throw new Error(`failed to resolve business id for ${BUSINESS_SLUG}: ${resp.status()}`);
    }
    const body = await resp.json();
    return Number(body.id);
  } finally {
    await ctx.dispose();
  }
}

const SERVICEABLE_ADDRESS = {
  street: "350 W 34th St",
  city: "New York",
  postal: "10001",
  country: "United States",
};

const GUEST = {
  name: "Test Guest",
  phone: "+1 555-000-0001",
};

test.describe("Guest delivery happy path", () => {
  test.skip(
    !process.env.PLAYWRIGHT_RUN_DELIVERY_E2E,
    "Set PLAYWRIGHT_RUN_DELIVERY_E2E=1 to run (requires full stack + demo seed)",
  );

  test.beforeAll(async () => {
    // Demo seed runs at capacity-full (5 cap, 8 in-flight) to show the
    // "capacity reached" operator UI. Bump the cap so the e2e can place a
    // fresh quote without the dashboard blocking it.
    const businessId = await resolveBusinessId();
    ensureDeliveryCapacity(businessId, 50);
  });

  test("guest can place a delivery order and reach the tracking page", async ({ page }) => {
    test.setTimeout(60_000);

    // ── Step 1: Visit the public business page ──────────────────────────────
    // Public route is /b/<custom_url>; the legacy /business/... path 404s.
    await page.goto(`/b/${BUSINESS_SLUG}`);
    // Wait for the menu/business shell to mount before interacting.
    await expect(page.getByRole("tab", { name: /menu/i })).toBeVisible({ timeout: 15_000 });

    // ── Step 2: Click the Delivery tab ─────────────────────────────────────
    await page.getByRole("tab", { name: /delivery/i }).first().click();

    // The DeliveryAvailableCard CTA is the entry point.
    const startOrderBtn = page.getByRole("button", { name: /check address and start order/i });
    await expect(startOrderBtn).toBeVisible({ timeout: 10_000 });
    await startOrderBtn.click();

    // ── Step 3: Fill the address modal ──────────────────────────────────────
    await page.getByLabel(/street address/i).fill(SERVICEABLE_ADDRESS.street);
    await page.getByLabel(/^city$/i).fill(SERVICEABLE_ADDRESS.city);
    await page.getByLabel(/postal code/i).fill(SERVICEABLE_ADDRESS.postal);
    // Country defaults to "United States" but ensure it's set.
    const countryField = page.getByLabel(/^country$/i);
    if (await countryField.inputValue() === "") {
      await countryField.fill(SERVICEABLE_ADDRESS.country);
    }

    // ── Step 4: Get a quote ─────────────────────────────────────────────────
    await page.getByRole("button", { name: /check address/i }).click();

    // Wait for the "Browse menu with delivery" CTA to enable — that means
    // the quote came back successfully and the address is serviceable.
    const browseMenuBtn = page.getByRole("button", { name: /browse menu with delivery/i });
    await expect(browseMenuBtn).toBeEnabled({ timeout: 15_000 });
    await browseMenuBtn.click();

    // ── Step 5: Menu loaded with delivery context — open item then add ─────
    // PublicMenuDisplay renders item cards; clicking opens the detail modal.
    // The add-to-cart button lives in that modal's footer. Pick a known
    // standalone item from the seed (NOT a bundle — bundles use a separate
    // checkout path that the test currently doesn't exercise).
    await expect(page.getByText(/explore our offerings/i)).toBeVisible({ timeout: 10_000 });
    // Item must price above the delivery minimum ($15 in the seed). "Seared
    // Salmon Bowl" is $22.00 — comfortably over and unlikely to be discounted
    // by an offer that drops it below the minimum.
    const targetItemName = process.env.PLAYWRIGHT_DELIVERY_ITEM || "Seared Salmon Bowl";
    const itemHeading = page.getByRole("heading", { level: 3, name: new RegExp(`^${targetItemName}$`, "i") }).first();
    await expect(itemHeading).toBeVisible({ timeout: 10_000 });
    // Scroll the card into view, then click.
    await itemHeading.scrollIntoViewIfNeeded();
    await itemHeading.click();

    const addToCart = page.getByTestId("add-to-cart-btn");
    await expect(addToCart).toBeVisible({ timeout: 8_000 });
    await addToCart.click();

    // ── Step 6: Open cart ───────────────────────────────────────────────────
    await page.getByTestId("open-cart-btn").click();
    await expect(page.getByTestId("cart-panel")).toBeVisible({ timeout: 5_000 });

    // ── Step 7: Start checkout ──────────────────────────────────────────────
    await page.getByTestId("checkout-btn").click();

    // GuestDeliveryCheckout modal should appear with the place-order button.
    const placeOrder = page.getByTestId("place-order-btn");
    await expect(placeOrder).toBeVisible({ timeout: 10_000 });

    // ── Step 8: Fill name + phone ───────────────────────────────────────────
    await page.getByLabel(/full name|name/i).first().fill(GUEST.name);
    await page.getByLabel(/phone/i).first().fill(GUEST.phone);

    // ── Step 9: Submit and verify redirect to tracking page ─────────────────
    await placeOrder.click();
    await expect(page).toHaveURL(/\/delivery\/[A-Z0-9-]+\/track/i, { timeout: 20_000 });

    // Tracking page renders something — either the timeline or the loading spinner.
    await expect(page.locator('main')).toBeVisible({ timeout: 10_000 });

    // The URL alone could match even if no row was created (e.g. if the
    // redirect ran on a stub number). Confirm the tracking endpoint
    // actually serves the row before declaring victory.
    const url = page.url();
    const match = url.match(/\/delivery\/([^/]+)\/track/i);
    expect(match).not.toBeNull();
    const deliveryNumber = match![1];
    const apiBase = process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1";
    const trackResp = await page.request.get(`${apiBase}/delivery/${deliveryNumber}/track`);
    expect(trackResp.status()).toBe(200);
    const trackJson = await trackResp.json();
    expect(trackJson.delivery_number).toBe(deliveryNumber);
    expect(trackJson.business_custom_url).toBe(BUSINESS_SLUG);
  });
});
