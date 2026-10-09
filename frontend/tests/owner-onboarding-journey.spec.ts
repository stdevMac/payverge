/**
 * Journey 5 — Owner: email/password registration → first business →
 * first menu item. No payment provider is touched at any point.
 *
 * ─────────────────────────────────────────────────────────────────────────
 * WHY THIS SPEC DIVERGES FROM THE WAVE-5 PLAN DRAFT (all findings verified
 * against live code):
 *
 * 1. Registration itself happens inside an `AuthModal`, not on the
 *    `/business/register` page, so the journey drives the API directly.
 *
 * 2. The path is the invite-gated direct-API business route:
 *      POST /auth/register            → consumes a short-lived launch-cohort
 *                                       invitation and creates a pending identity
 *                                       without issuing a session.
 *      email verification + /auth/login → the isolated database advances the
 *                                       otherwise non-recoverable email-link step,
 *                                       then the normal login issues the session.
 *      POST /businesses               → server.CreateBusiness. Returns 201 with
 *                                       the full business row (has .id + .business_id).
 *      POST /businesses/:id/menu/categories, then
 *      POST /businesses/:id/menu/items → server.AddMenuCategory / AddMenuItem,
 *                                       both behind RequireOperationalBusiness; a
 *                                       new business is active, so they pass.
 *    These are exercised via Playwright's `request` context with a Bearer JWT —
 *    the same seam convention as tests/guest-dinein-journey.spec.ts. The auth
 *    middleware accepts `Authorization: Bearer` with a session_token cookie
 *    fallback.
 *
 * 3. NO DB VERIFICATION-TOKEN SEAM EXISTS. The plan sketch read a plaintext
 *    token out of an `email_verification_tokens` table. That table does not
 *    exist and the plaintext token never touches the DB: the auth record stores
 *    only HashToken(plaintext) (backend/internal/auth/token_hashing_at_rest_test.go).
 *    The plaintext is emailed and never persisted, so it cannot be recovered
 *    from Postgres — the psql seam the draft sketched is impossible AND
 *    unnecessary. This spec instead updates only the isolated identity's
 *    verified flags, then proves the public login path before continuing.
 *
 * 4. Money: MenuItem.Price is float64 DOLLARS (database/models.go:487, per the
 *    money wire contract), so "6.50" is a dollar amount, not cents.
 *
 * The browser is still used for a real UI assertion: after the API creates the
 * business, we log the session cookie into the browser context and confirm the
 * owner dashboard for that business is reachable (not bounced to login).
 *
 * Run: PLAYWRIGHT_RUN_JOURNEYS_E2E=1 npx playwright test owner-onboarding-journey
 * (requires the full local stack: backend :8080, frontend :3001, postgres).
 * ─────────────────────────────────────────────────────────────────────────
 */
import { test, expect, type APIRequestContext } from "@playwright/test";
import { API_BASE, journeysEnabled } from "./helpers/journeys";
import {
  seedOwnerLaunchInvite,
  verifyOwnerEmailForJourney,
} from "./helpers/owner-onboarding";

/** POST /auth/register creates a pending, invite-admitted identity. */
async function registerOwner(
  api: APIRequestContext,
  email: string,
  password: string,
  name: string,
  inviteCode: string,
): Promise<{ userId: number }> {
  const resp = await api.post(`${API_BASE}/auth/register`, {
    data: { email, password, name, invite_code: inviteCode },
  });
  expect(resp.status(), await resp.text()).toBe(201);
  const body = await resp.json();
  expect(body.verification_required).toBe(true);
  expect(body.token, "pending registration must not issue a session").toBe("");
  return { userId: body.user_id };
}

async function loginOwner(
  api: APIRequestContext,
  email: string,
  password: string,
): Promise<string> {
  const resp = await api.post(`${API_BASE}/auth/login`, {
    data: { email, password },
  });
  expect(resp.status(), await resp.text()).toBe(200);
  const body = await resp.json();
  expect(body.token, "verified owner login must issue a session").toBeTruthy();
  return body.token;
}

test.describe("Owner onboarding journey", () => {
  test.skip(
    !journeysEnabled,
    "Set PLAYWRIGHT_RUN_JOURNEYS_E2E=1 (requires full stack)",
  );

  test("new owner registers, creates a business, adds a menu item — no Stripe", async ({
    page,
    playwright,
  }) => {
    test.setTimeout(120_000);
    const stamp = Date.now();
    const email = `e2e-owner-${stamp}@payverge.test`;
    const password = "E2e!Owner-Journey-42";
    const ownerName = "E2E Owner";
    const inviteCode = `e2e-owner-invite-${stamp}`;

    const api = await playwright.request.newContext();
    try {
      // 1. Register through the active launch cohort gate. Registration is
      //    intentionally pending until the email link is used and issues no
      //    session. Advance only that link step through the isolated DB seam,
      //    then prove the normal password login contract.
      seedOwnerLaunchInvite(inviteCode);
      await registerOwner(api, email, password, ownerName, inviteCode);
      verifyOwnerEmailForJourney(email);
      const token = await loginOwner(api, email, password);
      const authHeaders = { Authorization: `Bearer ${token}` };

      // 2. Create the first business via the direct-API route.
      //    Country seeds currency/TZ (Wave-4 required-country parity); a
      //    settlement_address is intentionally omitted so no payment plugin
      //    auto-enables. A new business is active.
      const bizName = `E2E Bistro ${stamp}`;
      const createResp = await api.post(`${API_BASE}/inside/businesses`, {
        headers: authHeaders,
        data: {
          name: bizName,
          owner_name: ownerName,
          email,
          business_type: "restaurant",
          address: { country: "US" },
        },
      });
      expect(createResp.status(), await createResp.text()).toBe(201);
      const business = await createResp.json();
      const businessNumericId: number = business.id;
      const businessSlug: string = business.business_id;
      expect(
        businessNumericId,
        "created business must carry a numeric id",
      ).toBeTruthy();
      expect(
        businessSlug,
        "created business must carry a business_id slug",
      ).toBeTruthy();

      // 3. Add a menu category (index 0), then the first menu item under it.
      //    Both routes sit behind RequireOperationalBusiness — passing here is
      //    the proof a new business is operational with no plan step.
      const catResp = await api.post(
        `${API_BASE}/inside/businesses/${businessNumericId}/menu/categories`,
        { headers: authHeaders, data: { name: "Empanadas", description: "" } },
      );
      expect(catResp.status(), await catResp.text()).toBe(201);

      const itemResp = await api.post(
        `${API_BASE}/inside/businesses/${businessNumericId}/menu/items`,
        {
          headers: authHeaders,
          // Legacy index-based path: category_index 0 is the category we just
          // created. Price is float64 DOLLARS per the money wire contract.
          data: {
            category_index: 0,
            item: {
              name: "E2E Empanada",
              description: "Journey test item",
              price: 6.5,
              currency: "USD",
              is_available: true,
            },
          },
        },
      );
      expect(itemResp.status(), await itemResp.text()).toBe(201);

      // 4. Confirm the item is really persisted: read the owner's menu back via
      //    the authenticated GET /businesses/:id/menu route (all-staff read, no
      //    lifecycle gate). The public storefront route is deliberately NOT
      //    used — it keys on custom_url (our generated business_id slug is not a
      //    custom_url) and applies publish gating, so it would 404 on a fresh,
      //    unpublished business.
      const menuResp = await api.get(
        `${API_BASE}/inside/businesses/${businessNumericId}/menu`,
        { headers: authHeaders },
      );
      expect(menuResp.ok(), await menuResp.text()).toBeTruthy();
      const menuText = JSON.stringify(await menuResp.json());
      expect(menuText).toContain("E2E Empanada");

      // 5. UI assertion: the owner's session reaches the dashboard in a real
      //    browser — not bounced to login or Stripe. Seed the session cookie
      //    (the same cookie login set) into the browser context.
      const apiUrl = new URL(API_BASE);
      await page.context().addCookies([
        {
          name: "session_token",
          value: token,
          domain: apiUrl.hostname, // localhost — shared with the frontend origin
          path: "/",
          httpOnly: false,
          secure: false,
        },
      ]);
      await page.goto(`/dashboard`);
      // A logged-in owner lands on /dashboard (never redirected to Stripe).
      await expect(page).not.toHaveURL(/stripe\.com/);
      await expect(page).toHaveURL(/\/dashboard/, { timeout: 30_000 });

      // 6. HARD STOP: no Stripe was ever contacted.
      expect(page.url()).not.toContain("stripe.com");
    } finally {
      await api.dispose();
    }
  });
});
