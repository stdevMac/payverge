/**
 * Journey 4 — Staff shift: OTP login (helpers/staff-login.ts, fixed OTP
 * 424242) → /staff/home (StaffDashboardShell → StaffTodayHome → TodayCard)
 * → clock in → clock out. Timeclock routes (backend/cmd/app/main.go, Slice 4):
 *   POST /inside/businesses/:id/me/clock-in   (timeclock:punch)
 *   POST /inside/businesses/:id/me/clock-out  (timeclock:punch)
 *   GET  /inside/businesses/:id/me/timesheet  (timeclock:punch) — own entries
 * (The wave-5 plan draft referenced GET /me/time-entries; the real self-read
 * route is /me/timesheet. POST /time-entries is the manager manual-entry
 * route, timeclock:manage — not used here.)
 *
 * SEED: backend/scripts/demo_seed.sql — staff of demo-core-kitchen are
 * @core-demo.payverge.example (server1/server2/host/kitchen/manager). Every
 * staff role carries timeclock:punch (internal/server/rbac.go), and punching
 * does not require an active scheduled shift (shift_id is optional on
 * clock-in) — but the spec still probes /me/timesheet first and skips with a
 * clear message if the seeded staffer can't reach the timeclock (403).
 *
 * UI copy is the English bundle src/i18n/messages/en/staffTimeclock.json:
 * "Clock in" / "Clock out" buttons, "On the clock" while open,
 * "You're not clocked in yet." + "Worked today" after closing.
 *
 * Run: PLAYWRIGHT_RUN_JOURNEYS_E2E=1 npx playwright test staff-shift-journey
 */
import { test, expect } from "@playwright/test";
import { loginStaff } from "./helpers/staff-login";
import { journeysEnabled, API_BASE, resolveBusinessId } from "./helpers/journeys";

/** Seeded core-demo server (demo_seed.sql, business demo-core-kitchen). */
const STAFF_EMAIL =
  process.env.PLAYWRIGHT_STAFF_EMAIL || "server1@core-demo.payverge.example";

test.describe("Staff shift journey", () => {
  test.skip(!journeysEnabled, "Set PLAYWRIGHT_RUN_JOURNEYS_E2E=1 (requires full stack + demo seed)");

  test("server logs in, clocks in, clocks out", async ({ browser, playwright }) => {
    test.setTimeout(90_000);
    const businessId = await resolveBusinessId();
    const context = await browser.newContext();
    const apiRequest = await playwright.request.newContext({ baseURL: API_BASE });
    try {
      // 1. OTP login via the seeded-code seam (same approach as
      //    staff-roles.spec.ts); carry the staff_token cookie into the browser.
      const token = await loginStaff(apiRequest, STAFF_EMAIL);
      const auth = { Authorization: `Bearer ${token}` };
      await context.addCookies((await apiRequest.storageState()).cookies);
      await context.addInitScript(() => {
        window.localStorage.setItem("payverge_had_session", "1");
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

      // 2. Preflight: can this staffer reach their own timesheet at all?
      //    403 → missing timeclock:punch, or the business is suspended
      //    (RequireOperationalBusiness wraps every timeclock route). Either
      //    means the environment can't exercise the journey — skip, don't fail.
      const probe = await apiRequest.get(`${API_BASE}/inside/businesses/${businessId}/me/timesheet`, {
        headers: auth,
      });
      if (probe.status() === 403) {
        test.skip(
          true,
          `Seeded staff ${STAFF_EMAIL} cannot use the timeclock on business ${businessId} ` +
            `(GET /me/timesheet → ${probe.status()}). Reseed demo data or grant timeclock:punch.`,
        );
      }
      expect(probe.ok(), `timesheet preflight failed: ${probe.status()}`).toBeTruthy();

      // 3. Clean slate: a previous aborted run may have left the shift open.
      //    Non-2xx here is fine (no open entry) — Playwright doesn't throw on status.
      await apiRequest.post(`${API_BASE}/inside/businesses/${businessId}/me/clock-out`, { headers: auth });

      // 4. Staff front door. The cookie is canonical; the local session hint
      //    tells HybridAuthProvider to hydrate it on this otherwise-public route.
      const page = await context.newPage();
      await page.goto(`/staff/home`);

      // TodayCard idle state (en/staffTimeclock.json): "Clock in" button.
      const clockInButton = page.getByRole("button", { name: /clock in/i }).first();
      await expect(clockInButton).toBeVisible({ timeout: 20_000 });

      // 5. Clock in via UI → running-timer state shows "On the clock".
      await clockInButton.click();
      await expect(page.getByText(/on the clock/i).first()).toBeVisible({ timeout: 10_000 });

      // 6. Clock out via UI → back to idle copy plus the worked-today line.
      await page.getByRole("button", { name: /clock out/i }).first().click();
      await expect(
        page.getByText(/you're not clocked in yet/i).first(),
      ).toBeVisible({ timeout: 10_000 });
      await expect(page.getByText(/worked today/i).first()).toBeVisible({ timeout: 5_000 });

      // 7. Backend agrees: my timesheet has a CLOSED entry from this run
      //    (clock_out_at set; status leaves "open" — Slice 4 closes into
      //    pending_review).
      const entriesResp = await apiRequest.get(
        `${API_BASE}/inside/businesses/${businessId}/me/timesheet`,
        { headers: auth },
      );
      expect(entriesResp.ok()).toBeTruthy();
      // GetMyTimesheet responds with a {success, data} envelope on both the
      // open-entries and date-range paths (backend/internal/handlers/timeclock.go:188)
      // — entries live under `data`.
      const entriesBody = (await entriesResp.json()) as {
        data?: Array<{ status: string; clock_in_at: string; clock_out_at: string | null }>;
      };
      const entries = entriesBody.data ?? [];
      const closedToday = entries.find(
        (e) =>
          e.status !== "open" &&
          e.clock_out_at !== null &&
          new Date(e.clock_in_at).toDateString() === new Date().toDateString(),
      );
      expect(closedToday, "expected a closed time entry for today").toBeTruthy();
    } finally {
      await context.close();
      await apiRequest.dispose();
    }
  });
});
