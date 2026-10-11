/**
 * Journey 3 — Reservation: public booking via API → confirmation page →
 * public cancellation. Email is only the delivery channel for the code; the
 * code itself is in the create response (reservationPublicView embeds
 * TableReservation, whose `confirmation_code` is serialized — models.go:575),
 * so no mail interception is needed.
 *
 * SEED: backend/scripts/demo_seed.sql — demo-core-kitchen reservation
 * settings: enabled, approval_mode "manual", auto_assign_tables,
 * allow_cancellation with a 24h
 * cancellation_deadline, min_advance 30min, max_advance 45 days.
 *
 * ADAPTATIONS vs the wave-5 plan draft:
 * - A hardcoded now+26h reservation_time can violate operating hours
 *   (validateReservationWindow rejects closed days / out-of-window slots).
 *   Instead findBookableReservationSlot asks the public availability endpoint
 *   (GET /business/:customUrl/reservations/availability) for a real slot on a
 *   day ≥2 days out — slot times are RFC3339 UTC and feed straight into
 *   `reservation_time`; ≥2 days also clears the 24h cancellation deadline.
 * - Online bookings may not set `duration` (400 duration_not_allowed —
 *   ErrGuestDurationNotAllowed); the server applies settings.default_duration,
 *   the same duration availability used to validate the slot.
 * - The details page renders its NextUI navigation CTA with button semantics;
 *   it opens /reservations/[code]/cancel, where a second danger button performs
 *   the cancel (guard against strict-mode double-fire requires the click).
 * - Success copy: "Reservation Cancelled" title (guest catalog
 *   reservationConfirmation.action.cancelledTitle).
 * - Backend agreement is asserted on GET /reservations/:code →
 *   { reservation: { status: "cancelled" }, can_cancel: false }.
 *
 * Run: PLAYWRIGHT_RUN_JOURNEYS_E2E=1 npx playwright test reservation-public-journey
 */
import { test, expect, request as apiRequestFactory } from "@playwright/test";
import {
  journeysEnabled,
  API_BASE,
  BUSINESS_SLUG,
  findBookableReservationSlot,
} from "./helpers/journeys";
import { prepareGuestPage } from "./helpers/guest-page";

test.describe("Public reservation journey", () => {
  test.skip(
    !journeysEnabled,
    "Set PLAYWRIGHT_RUN_JOURNEYS_E2E=1 (requires full stack + demo seed)",
  );

  test("guest books, views confirmation, and cancels", async ({ page }) => {
    test.setTimeout(90_000);
    await prepareGuestPage(page);
    const customerName = `E2E Reservation ${Date.now()}`;

    // 1. Create the reservation through the public API (the /b/<slug> booking
    //    form drives this same endpoint — POST /business/:customUrl/reservations,
    //    server.CreatePublicReservation; API-first keeps the spec stable
    //    against form-layout churn).
    const api = await apiRequestFactory.newContext();
    let confirmationCode: string;
    try {
      const slotTime = await findBookableReservationSlot(api, {
        partySize: 2,
        minDaysAhead: 2,
      });
      const resp = await api.post(
        `${API_BASE}/business/${BUSINESS_SLUG}/reservations`,
        {
          data: {
            customer_name: customerName,
            customer_phone: "+1 555-000-0042",
            customer_email: "e2e-reservation@payverge.test",
            party_size: 2,
            reservation_time: slotTime,
            language: "en",
          },
        },
      );
      if (resp.status() !== 201) {
        expect(
          resp.status(),
          `public reservation create failed with ${resp.status()}: ${await resp.text()}`,
        ).toBe(201);
      }
      const body = await resp.json();
      confirmationCode = body.confirmation_code;
      expect(
        confirmationCode,
        "confirmation_code missing from create response",
      ).toBeTruthy();
      // The demo venue deliberately requires operator approval.
      expect(body.status).toBe("pending");
    } finally {
      await api.dispose();
    }

    // 2. Confirmation page renders the reservation
    //    (src/app/reservations/[confirmationCode]/page.tsx).
    await page.goto(`/reservations/${confirmationCode}?lang=en`);
    await expect(
      page.getByRole("heading", { name: /your reservation/i }),
    ).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText(customerName)).toBeVisible();
    await expect(page.getByText(confirmationCode)).toBeVisible();
    await expect(page.getByText(/^pending$/i).first()).toBeVisible();

    // 3. Cancel: the details page's NextUI CTA has button semantics and opens
    //    /reservations/[code]/cancel; the action page requires an explicit
    //    button press (danger button, same "Cancel Reservation" label) before
    //    calling POST /reservations/:code/cancel.
    await page
      .getByRole("button", { name: /cancel reservation/i })
      .first()
      .click();
    await expect(page).toHaveURL(
      new RegExp(`/reservations/${confirmationCode}/cancel`),
      {
        timeout: 15_000,
      },
    );
    await expect(
      page.getByText(/cancel your reservation/i).first(),
    ).toBeVisible({
      timeout: 15_000,
    });
    await page
      .getByRole("button", { name: /cancel reservation/i })
      .first()
      .click();
    await expect(
      page.getByRole("heading", { name: /reservation cancelled/i }),
    ).toBeVisible({ timeout: 15_000 });

    // 4. Backend agrees: public GET reflects the cancelled status and the
    //    cancel affordance is gone (PublicReservationDetailsDTO.can_cancel).
    const verify = await apiRequestFactory.newContext();
    try {
      const getResp = await verify.get(
        `${API_BASE}/reservations/${confirmationCode}`,
      );
      expect(getResp.ok()).toBeTruthy();
      const details = await getResp.json();
      expect(details.reservation?.status).toBe("cancelled");
      expect(details.can_cancel).toBe(false);
    } finally {
      await verify.dispose();
    }
  });
});
