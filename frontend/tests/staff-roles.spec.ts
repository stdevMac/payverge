import { test, expect, type BrowserContext } from "@playwright/test";
import { loginStaff } from "./helpers/staff-login";
import { prepareGuestPage } from "./helpers/guest-page";
import {
  findBookableReservationSlot,
  resolveBusinessId,
} from "./helpers/journeys";

const STAFF_EMAILS = {
  manager: "manager@core-demo.payverge.example",
  server: "server1@core-demo.payverge.example",
  host: "host@core-demo.payverge.example",
  kitchen: "kitchen@core-demo.payverge.example",
} as const;

const ROLES: Array<{ email: string; role: string; allowedTabs: string[] }> = [
  {
    email: STAFF_EMAILS.manager,
    role: "manager",
    allowedTabs: [
      "overview",
      "analytics",
      "accounting",
      "menu",
      "inventory",
      "tables",
      "bills",
      "staff",
      "settings",
      "plugins",
      "counter",
      "kitchen",
    ],
  },
  {
    email: STAFF_EMAILS.server,
    role: "server",
    allowedTabs: [
      "overview",
      "menu",
      "tables",
      "bills",
      "reservations",
      "counter",
      "kitchen",
    ],
  },
  {
    email: STAFF_EMAILS.host,
    role: "host",
    allowedTabs: [
      "overview",
      "menu",
      "tables",
      "reservations",
      "bills",
      "kitchen",
    ],
  },
  {
    email: STAFF_EMAILS.kitchen,
    role: "kitchen",
    allowedTabs: ["overview", "kitchen"],
  },
];

let BUSINESS_ID = 0;
const API_BASE =
  process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1";

test.beforeAll(async () => {
  BUSINESS_ID = await resolveBusinessId();
});

for (const r of ROLES) {
  // Run each role's tab tests serially in a single browser context so we only
  // hit /staff/verify-login-code once per role (production defaults to a
  // strict 5 req/minute with burst 2; the isolated CI stack raises that budget).
  test.describe.serial(`${r.role}`, () => {
    let context: BrowserContext;

    test.beforeAll(async ({ browser, playwright }) => {
      context = await browser.newContext();
      const apiRequest = await playwright.request.newContext({
        baseURL:
          process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1",
      });
      try {
        await loginStaff(apiRequest, r.email);
        const state = await apiRequest.storageState();
        // Carry the staff_token cookie into the browser context.
        await context.addCookies(state.cookies);
      } finally {
        await apiRequest.dispose();
      }
    });

    test.afterAll(async () => {
      await context.close();
    });

    for (const tab of r.allowedTabs) {
      test(`can open ${tab} without permission-denied copy`, async () => {
        const page = await context.newPage();
        try {
          await page.goto(`/business/${BUSINESS_ID}/dashboard?tab=${tab}`);
          // Wait for the dashboard shell to render — sidebar marker is reliable.
          await expect(
            page
              .getByText(
                /Welcome to|Bill Management|Kitchen Management|Accounting|Plugins|Settings|Staff/,
              )
              .first(),
          ).toBeVisible({ timeout: 10_000 });
          // Critical assertion: no permission-denied copy.
          await expect(
            page.getByText("permission to access this page"),
          ).toHaveCount(0);
        } finally {
          await page.close();
        }
      });
    }
  });
}

// Bug-fix coverage from STAFF-AUDIT-2026-05-02.md (#7, #8, #12).
// Scoped: per-role smoke. Not exercising bug #9/#10 here — those need a
// `kitchen_enabled=false` business state and the seed default has it on.
// A separate per-business spec can drive the toggle and assert the Bills
// tab disappears for server.
test.describe.serial("staff dashboard fixes", () => {
  test("dashboard header renders the seeded business name (not 'Business N')", async ({
    browser,
    playwright,
  }) => {
    const context = await browser.newContext();
    const apiRequest = await playwright.request.newContext({
      baseURL:
        process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1",
    });
    try {
      await loginStaff(apiRequest, STAFF_EMAILS.manager);
      const state = await apiRequest.storageState();
      await context.addCookies(state.cookies);

      const page = await context.newPage();
      await page.goto(`/business/${BUSINESS_ID}/dashboard?tab=overview`);
      await expect(
        page.locator("nav, [role='banner'], main").first(),
      ).toBeVisible({
        timeout: 15_000,
      });
      // The seeded demo business is "Demo Bistro" via demo_seed.sql defaults.
      // We only check the negative — the placeholder must NOT render.
      await expect(page.getByText(/^Business \d+$/).first()).toHaveCount(0);
    } finally {
      await context.close();
      await apiRequest.dispose();
    }
  });

  test("kitchen role sees Kitchen tab in sidebar on a quiet queue", async ({
    browser,
    playwright,
  }) => {
    const kitchenContext = await browser.newContext();
    const apiRequest = await playwright.request.newContext({
      baseURL:
        process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1",
    });
    try {
      await loginStaff(apiRequest, STAFF_EMAILS.kitchen);
      const state = await apiRequest.storageState();
      await kitchenContext.addCookies(state.cookies);

      const page = await kitchenContext.newPage();
      await page.goto(`/business/${BUSINESS_ID}/dashboard?tab=overview`);
      await expect(
        page.getByText(/Welcome to|Kitchen Management/).first(),
      ).toBeVisible({ timeout: 15_000 });
      // The Kitchen tab must be findable as a sidebar nav item regardless
      // of how many pending orders exist.
      await expect(
        page
          .getByRole("link", { name: /Kitchen/i })
          .or(page.getByRole("button", { name: /Kitchen/i }))
          .first(),
      ).toBeVisible({ timeout: 5_000 });
    } finally {
      await kitchenContext.close();
      await apiRequest.dispose();
    }
  });

  for (const role of ["host", "server"] as const) {
    test(`${role} lands on a real Reservations view (URL preserved + sidebar active)`, async ({
      browser,
      playwright,
    }) => {
      const ctx = await browser.newContext();
      const apiRequest = await playwright.request.newContext({
        baseURL:
          process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1",
      });
      try {
        await loginStaff(apiRequest, STAFF_EMAILS[role]);
        const state = await apiRequest.storageState();
        await ctx.addCookies(state.cookies);

        const page = await ctx.newPage();
        await page.goto(`/business/${BUSINESS_ID}/dashboard?tab=reservations`);

        await expect(
          page.locator("nav, [role='banner'], main").first(),
        ).toBeVisible({
          timeout: 15_000,
        });

        // URL must still carry tab=reservations — a redirect to Overview
        // would clear it and the test must fail in that case.
        await expect
          .poll(() => page.url(), { timeout: 5_000 })
          .toContain("tab=reservations");

        // Sidebar Reservations item must be the active one.
        const reservationsLink = page
          .getByRole("link", { name: /Reservations/i })
          .or(page.getByRole("button", { name: /Reservations/i }))
          .first();
        await expect(reservationsLink).toBeVisible({ timeout: 5_000 });
        const activeAttr = await reservationsLink.getAttribute("aria-current");
        const className = (await reservationsLink.getAttribute("class")) ?? "";
        expect(
          activeAttr === "page" || /active|selected|bg-/i.test(className),
        ).toBe(true);

        await expect(
          page.getByText("permission to access this page"),
        ).toHaveCount(0);
      } finally {
        await ctx.close();
        await apiRequest.dispose();
      }
    });
  }

  for (const role of ["host", "server"] as const) {
    test(`${role} sees no Settings tab on Reservations and DOES see Create button`, async ({
      browser,
      playwright,
    }) => {
      const ctx = await browser.newContext();
      const apiRequest = await playwright.request.newContext({
        baseURL:
          process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1",
      });
      try {
        await loginStaff(apiRequest, STAFF_EMAILS[role]);
        const state = await apiRequest.storageState();
        await ctx.addCookies(state.cookies);

        const page = await ctx.newPage();
        await page.goto(`/business/${BUSINESS_ID}/dashboard?tab=reservations`);
        await expect(
          page.locator("nav, [role='banner'], main").first(),
        ).toBeVisible({
          timeout: 15_000,
        });

        // Settings tab must not be in the DOM — capability gate hides it for
        // roles without reservations:settings.
        await expect(page.getByRole("tab", { name: /Settings/i })).toHaveCount(
          0,
        );
        // Header toggle button must also be absent.
        await expect(page.getByTestId("reservation-toggle-button")).toHaveCount(
          0,
        );
        // Create button must be visible (host + server both have reservations:create).
        await expect(page.getByTestId("reservation-create-button")).toBeVisible(
          {
            timeout: 5_000,
          },
        );
      } finally {
        await ctx.close();
        await apiRequest.dispose();
      }
    });
  }

  test("server Reservation detail drawer is read-only", async ({
    browser,
    playwright,
  }) => {
    // Two logins, a reservation create, and a retried Select open can
    // together outlast the 30s default on a cold CI stack.
    test.setTimeout(60_000);
    const ctx = await browser.newContext();
    const apiRequest = await playwright.request.newContext({
      baseURL:
        process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1",
    });
    const managerRequest = await playwright.request.newContext({
      baseURL:
        process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1",
    });
    const customerName = `Server Readonly ${Date.now()}`;
    let reservationId: number | undefined;
    let managerToken = "";

    try {
      managerToken = await loginStaff(managerRequest, STAFF_EMAILS.manager);
      const serverToken = await loginStaff(apiRequest, STAFF_EMAILS.server);
      const state = await apiRequest.storageState();
      await ctx.addCookies(state.cookies);

      // A fixed now+N offset lands outside the venue's operating hours at
      // some UTC run times (business hours are evaluated in the business
      // timezone, and earlier specs may widen them). Book the earliest slot
      // the public availability endpoint reports as open; it was validated
      // with settings.default_duration, so the create omits `duration` and
      // the server applies that same default.
      const reservationTime = await findBookableReservationSlot(apiRequest, {
        partySize: 2,
      });
      const createResponse = await apiRequest.post(
        `${API_BASE}/inside/businesses/${BUSINESS_ID}/reservations`,
        {
          headers: { Authorization: `Bearer ${serverToken}` },
          data: {
            customer_name: customerName,
            customer_phone: "555-0199",
            customer_email: "readonly@example.com",
            party_size: 2,
            reservation_time: reservationTime,
            source: "manual",
          },
        },
      );
      if (!createResponse.ok()) {
        const createBody = await createResponse.text();
        expect(
          createResponse.ok(),
          `create reservation failed with ${createResponse.status()}: ${createBody}`,
        ).toBeTruthy();
      }

      const createdReservation = (await createResponse.json()) as {
        id?: number;
        reservation?: { id?: number };
      };
      reservationId =
        createdReservation.id ?? createdReservation.reservation?.id;
      if (typeof reservationId !== "number") {
        throw new Error(
          `create reservation response missing id: ${JSON.stringify(createdReservation)}`,
        );
      }

      const page = await ctx.newPage();
      await prepareGuestPage(page);
      await page.goto(`/business/${BUSINESS_ID}/dashboard`);
      await expect(
        page.locator("nav, [role='banner'], main").first(),
      ).toBeVisible({
        timeout: 15_000,
      });
      await page
        .getByRole("link", { name: /^Reservations$/i })
        .or(page.getByRole("button", { name: /^Reservations$/i }))
        .first()
        .click();

      // The list defaults to "Today" in the business timezone; the earliest
      // open slot may be on a later day, so widen to "Upcoming"
      // (today → +30 days) and narrow by the unique guest name.
      //
      // The dashboard scroller is `scroll-behavior: smooth`, and React Aria
      // closes a Select popover on any scroll of an ancestor of its trigger.
      // A click that lands while the panel is still settling (first data
      // load shifts the layout, and the scroll-into-view animates after the
      // click) opens the listbox and closes it in the same frame, leaving the
      // option click to wait forever. Reopen until the option is actually
      // clickable, then assert the trigger committed the selection.
      const dateRangeTrigger = page.getByRole("button", {
        name: /Date Range/i,
      });
      const upcomingOption = page.getByRole("option", {
        name: "Upcoming",
        exact: true,
      });
      await expect(dateRangeTrigger).toBeVisible({ timeout: 15_000 });
      await expect(async () => {
        if (!(await upcomingOption.isVisible())) {
          await dateRangeTrigger.click();
        }
        await upcomingOption.click({ timeout: 2_000 });
      }).toPass({ timeout: 15_000 });
      await expect(dateRangeTrigger).toHaveText(/Upcoming/);
      await page
        .getByRole("textbox", { name: /Search by customer name/i })
        .fill(customerName);

      const reservationContainer = page
        .getByRole("row")
        .filter({ hasText: customerName });
      await expect(reservationContainer).toBeVisible({ timeout: 15_000 });
      await reservationContainer
        .getByRole("button", { name: /^View reservation/i })
        .click();

      await expect(page.getByText("Guest details")).toBeVisible({
        timeout: 5_000,
      });
      const detailDrawer = page
        .locator(".fixed")
        .filter({ hasText: "Guest details" })
        .filter({ hasText: customerName })
        .first();
      await expect(detailDrawer).toBeVisible({ timeout: 5_000 });
      await expect(page.getByText("Assign a table")).toHaveCount(0);
      await expect(page.getByText("Quick actions")).toHaveCount(0);
      await expect(page.getByRole("button", { name: /^Edit$/i })).toHaveCount(
        0,
      );
      await expect(page.getByRole("button", { name: /^Cancel$/i })).toHaveCount(
        0,
      );
      await expect(page.getByRole("button", { name: /Seat/i })).toHaveCount(0);
    } finally {
      if (reservationId) {
        try {
          const claimResponse = await managerRequest.post(
            `${API_BASE}/inside/businesses/${BUSINESS_ID}/reservations/${reservationId}/claim`,
            {
              headers: { Authorization: `Bearer ${managerToken}` },
              data: { steal: true },
            },
          );
          if (!claimResponse.ok()) {
            console.warn(
              `reservation cleanup claim failed with ${claimResponse.status()}: ${await claimResponse.text()}`,
            );
          }
          const cancelResponse = await managerRequest.delete(
            `${API_BASE}/inside/businesses/${BUSINESS_ID}/reservations/${reservationId}`,
            { headers: { Authorization: `Bearer ${managerToken}` } },
          );
          if (!cancelResponse.ok()) {
            const cleanupBody = await cancelResponse.text();
            console.warn(
              `reservation cleanup failed with ${cancelResponse.status()}: ${cleanupBody}`,
            );
          }
        } catch (cleanupError) {
          console.warn("reservation cleanup failed:", cleanupError);
        }
      }
      await ctx.close();
      await apiRequest.dispose();
      await managerRequest.dispose();
    }
  });
});
