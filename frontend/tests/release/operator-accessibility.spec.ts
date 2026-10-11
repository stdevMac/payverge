import AxeBuilder from "@axe-core/playwright";
import { execFileSync } from "node:child_process";
import path from "node:path";
import {
  expect,
  test,
  type APIRequestContext,
  type APIResponse,
  type BrowserContext,
  type Locator,
  type Page,
  type Route,
} from "@playwright/test";

const APP_BASE_URL = process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000";
const API_BASE =
  process.env.PLAYWRIGHT_API_BASE || "http://127.0.0.1:8080/api/v1";
const ORIGIN = process.env.PLAYWRIGHT_API_ORIGIN || "http://localhost:3000";
const COMPOSE_FILE = path.resolve(
  __dirname,
  "..",
  "..",
  "..",
  "docker-compose.yml",
);
const POSTGRES_USER = "payverge";
const POSTGRES_DB = "payverge";
const BUSINESS_TIMEZONE = "America/New_York";
const DEVICE_TIMEZONE = "America/Buenos_Aires";
const AUTH_RATE_LIMIT_TIMEOUT_MS = 120_000;

test.setTimeout(AUTH_RATE_LIMIT_TIMEOUT_MS);

let api: APIRequestContext;
let ownerToken = "";
let ownerEmail = "";
const ownerPassword = "E2e!Operator-A11y-42";
let staffToken = "";
let businessID = 0;
let tableCode = "";
let billID = 0;

const ownerHeaders = () => ({
  Authorization: `Bearer ${ownerToken}`,
  Origin: ORIGIN,
});

const wait = (ms: number) =>
  new Promise<void>((resolve) => setTimeout(resolve, ms));

async function postWithRateLimitRetry(
  url: string,
  options: Parameters<APIRequestContext["post"]>[1],
): Promise<APIResponse> {
  const response = await api.post(url, options);
  if (response.status() !== 429) return response;

  let retryAfterSeconds = 60;
  try {
    const body = (await response.json()) as { retry_after?: unknown };
    const parsed = Number(body.retry_after);
    if (Number.isFinite(parsed) && parsed > 0) {
      retryAfterSeconds = Math.min(parsed, 60);
    }
  } catch {
    // Keep the conservative default.
  }

  await wait(retryAfterSeconds * 1000 + 1000);
  return api.post(url, options);
}

function executeDatabaseSQL(sql: string): void {
  if (process.env.PLAYWRIGHT_DIRECT_PSQL === "1") {
    execFileSync(
      "psql",
      [
        "-h",
        process.env.PLAYWRIGHT_DB_HOST || "localhost",
        "-p",
        process.env.PLAYWRIGHT_DB_PORT || "5432",
        "-U",
        POSTGRES_USER,
        "-d",
        POSTGRES_DB,
        "-v",
        "ON_ERROR_STOP=1",
        "-q",
      ],
      {
        input: sql,
        stdio: ["pipe", "pipe", "pipe"],
        env: {
          ...process.env,
          PGPASSWORD: process.env.PLAYWRIGHT_DB_PASSWORD || "payverge_password",
        },
      },
    );
    return;
  }

  execFileSync(
    "docker",
    [
      "compose",
      "-f",
      COMPOSE_FILE,
      "exec",
      "-T",
      "postgres",
      "psql",
      "-U",
      POSTGRES_USER,
      "-d",
      POSTGRES_DB,
      "-v",
      "ON_ERROR_STOP=1",
      "-q",
    ],
    { input: sql, stdio: ["pipe", "pipe", "pipe"] },
  );
}

function backdateBill(id: number): void {
  if (!Number.isSafeInteger(id) || id <= 0) {
    throw new Error(`refusing to backdate invalid bill id: ${id}`);
  }
  executeDatabaseSQL(
    `UPDATE bills SET created_at = NOW() - INTERVAL '13 hours', updated_at = NOW() - INTERVAL '13 hours' WHERE id = ${id};`,
  );
}

function moveReservationToUiToday(id: number): void {
  if (!Number.isSafeInteger(id) || id <= 0) {
    throw new Error(`refusing to move invalid reservation id: ${id}`);
  }
  // "UI today" is whatever day the dashboard asks the API for, and
  // ReservationManager derives that from the BROWSER clock
  // (localDateKey(new Date())) — which this suite pins to DEVICE_TIMEZONE via
  // test.use({ timezoneId }). Deriving it from the runner clock instead (UTC on
  // CI) writes tomorrow's reservation whenever the two dates disagree, i.e.
  // every run between 00:00 and 03:00 UTC, and the "Today" filter then returns
  // nothing. Must stay DEVICE_TIMEZONE, not BUSINESS_TIMEZONE: the date key is
  // device-derived, and only the 19:30 wall time below is business-anchored.
  const uiTodayKey = new Intl.DateTimeFormat("en-CA", {
    timeZone: DEVICE_TIMEZONE,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(new Date());
  executeDatabaseSQL(
    `UPDATE table_reservations SET reservation_time = (DATE '${uiTodayKey}' + TIME '19:30') AT TIME ZONE '${BUSINESS_TIMEZONE}', updated_at = NOW() WHERE id = ${id};`,
  );
}

const soon = (days: number): string => {
  const date = new Date(Date.now() + days * 86_400_000);
  date.setUTCHours(19, 30, 0, 0);
  return date.toISOString();
};

/**
 * Round-3 Session O (Root A): named axe rule ids that previously landed at
 * moderate/minor and were silently dropped by the serious/critical floor.
 * Assert these regardless of impact so decorative-SVG, name, and label
 * defects cannot walk through again.
 */
const WAVE_O_NAMED_RULE_IDS = [
  "button-name",
  "link-name",
  "image-alt",
  "svg-img-alt",
  "label",
  "aria-allowed-attr",
  "aria-required-attr",
  "aria-valid-attr-value",
] as const;

async function expectNoSeriousViolations(page: Page) {
  // NextUI modals fade/slide in; axe samples blended colors if we analyze
  // during that transition.
  await page.waitForTimeout(350);
  const result = await new AxeBuilder({ page })
    .withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"])
    .analyze();
  const seriousOrCritical = result.violations.filter(
    (violation) =>
      violation.impact === "serious" || violation.impact === "critical",
  );
  expect(seriousOrCritical).toEqual([]);

  // Second assertion tier (Root A hole 2): fail on the named rule ids this
  // wave fixes even when axe rates them moderate/minor. Full moderate floor
  // is too noisy on the existing tree; named ids keep the gate precise.
  const namedWaveO = result.violations.filter((violation) =>
    (WAVE_O_NAMED_RULE_IDS as readonly string[]).includes(violation.id),
  );
  expect(namedWaveO).toEqual([]);
}

async function authenticateOperator(page: Page, context: BrowserContext) {
  const login = await postWithRateLimitRetry(`${API_BASE}/auth/login`, {
    headers: { Origin: ORIGIN },
    data: { email: ownerEmail, password: ownerPassword },
  });
  expect(login.status(), await login.text()).toBe(200);
  ownerToken = String((await login.json()).token);

  const app = new URL(APP_BASE_URL);
  const apiBase = new URL(API_BASE);
  const cookieHosts = Array.from(new Set([app.hostname, apiBase.hostname]));
  await context.addCookies(
    cookieHosts.map((hostname) => ({
      name: "session_token",
      value: ownerToken,
      domain: hostname,
      path: "/",
      httpOnly: false,
      secure: app.protocol === "https:" || apiBase.protocol === "https:",
      sameSite: "Lax",
    })),
  );
  await page.addInitScript(() => {
    localStorage.setItem("payverge_had_session", "1");
  });
}

async function recentAlertsTrigger(page: Page): Promise<Locator> {
  const triggers = page.getByTestId("recent-alerts-trigger");
  await expect(triggers.filter({ visible: true }).first()).toBeVisible();

  const viewportIndex = await triggers.evaluateAll((elements) => {
    const visibleIndexes: number[] = [];
    const viewportIndexes: number[] = [];

    elements.forEach((element, index) => {
      const rect = element.getBoundingClientRect();
      const style = window.getComputedStyle(element);
      const visible =
        style.display !== "none" &&
        style.visibility !== "hidden" &&
        rect.width > 0 &&
        rect.height > 0;

      if (!visible) return;
      visibleIndexes.push(index);

      if (
        rect.bottom > 0 &&
        rect.right > 0 &&
        rect.top < window.innerHeight &&
        rect.left < window.innerWidth
      ) {
        viewportIndexes.push(index);
      }
    });

    return viewportIndexes[0] ?? visibleIndexes[0] ?? 0;
  });

  const trigger = triggers.nth(viewportIndex);
  await expect(trigger).toBeVisible();
  return trigger;
}

test.describe.serial("operator accessibility release gate", () => {
  test.use({ timezoneId: DEVICE_TIMEZONE });

  test.beforeAll(async ({ playwright }) => {
    api = await playwright.request.newContext({ ignoreHTTPSErrors: true });
    const stamp = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    ownerEmail = `operator-a11y-owner-${stamp}@payverge.test`;
    const register = await postWithRateLimitRetry(`${API_BASE}/auth/register`, {
      headers: { Origin: ORIGIN },
      data: {
        email: ownerEmail,
        password: ownerPassword,
        name: "Operator Accessibility Owner",
      },
    });
    expect(register.status(), await register.text()).toBe(201);
    ownerToken = String((await register.json()).token);
    executeDatabaseSQL(
      `UPDATE user_auths SET email_verified = true, updated_at = NOW() WHERE provider = 'email' AND provider_user_id = $$${ownerEmail}$$;`,
    );

    const business = await api.post(`${API_BASE}/inside/businesses`, {
      headers: ownerHeaders(),
      data: {
        name: "Operator Accessibility Bistro",
        owner_name: "Operator Accessibility Owner",
        email: ownerEmail,
        business_type: "restaurant",
        address: { country: "US" },
        custom_url: `operator-a11y-${stamp}`,
        business_page_enabled: true,
        timezone: BUSINESS_TIMEZONE,
      },
    });
    expect(business.status(), await business.text()).toBe(201);
    businessID = Number((await business.json()).id);

    const ordering = await api.post(
      `${API_BASE}/inside/businesses/${businessID}/toggle-kitchen-orders`,
      { headers: ownerHeaders(), data: { enabled: true } },
    );
    expect(ordering.status(), await ordering.text()).toBe(200);

    const reservationSettings = await api.put(
      `${API_BASE}/inside/businesses/${businessID}/reservations/settings`,
      { headers: ownerHeaders(), data: { enabled: true } },
    );
    expect(reservationSettings.status(), await reservationSettings.text()).toBe(
      200,
    );

    const category = await api.post(
      `${API_BASE}/inside/businesses/${businessID}/menu/categories`,
      { headers: ownerHeaders(), data: { name: "Mains" } },
    );
    expect(category.status(), await category.text()).toBe(201);
    const item = await api.post(
      `${API_BASE}/inside/businesses/${businessID}/menu/items`,
      {
        headers: ownerHeaders(),
        data: {
          category_index: 0,
          item: {
            name: "Accessible Burger",
            description: "A release-gate fixture",
            price: 12.5,
            is_available: true,
          },
        },
      },
    );
    expect(item.status(), await item.text()).toBe(201);

    const table = await api.post(
      `${API_BASE}/inside/businesses/${businessID}/tables`,
      {
        headers: ownerHeaders(),
        data: { name: "Accessibility-1", capacity: 4 },
      },
    );
    expect(table.status(), await table.text()).toBe(201);
    const tablePayload = (await table.json()) as {
      id: number;
      table_code: string;
    };
    tableCode = String(tablePayload.table_code);
    const bill = await api.post(`${API_BASE}/guest/table/${tableCode}/bill`, {
      headers: { Origin: ORIGIN },
      data: {},
    });
    expect(bill.status(), await bill.text()).toBe(201);
    billID = Number((await bill.json()).bill.id);
    const order = await api.post(`${API_BASE}/guest/table/${tableCode}/order`, {
      headers: {
        Origin: ORIGIN,
        "X-Request-Id": `operator-a11y-order-${stamp}`,
      },
      data: {
        bill_id: billID,
        items: [{ menu_item_name: "Accessible Burger", quantity: 1 }],
      },
    });
    expect(order.status(), await order.text()).toBe(201);

    const staleTable = await api.post(
      `${API_BASE}/inside/businesses/${businessID}/tables`,
      {
        headers: ownerHeaders(),
        data: { name: "Accessibility-Stale", capacity: 4 },
      },
    );
    expect(staleTable.status(), await staleTable.text()).toBe(201);
    const staleTableCode = String((await staleTable.json()).table_code);
    const staleBill = await api.post(
      `${API_BASE}/guest/table/${staleTableCode}/bill`,
      { headers: { Origin: ORIGIN }, data: {} },
    );
    expect(staleBill.status(), await staleBill.text()).toBe(201);
    backdateBill(Number((await staleBill.json()).bill.id));

    const conflictTable = await api.post(
      `${API_BASE}/inside/businesses/${businessID}/tables`,
      {
        headers: ownerHeaders(),
        data: { name: "Accessibility-Conflict", capacity: 4 },
      },
    );
    expect(conflictTable.status(), await conflictTable.text()).toBe(201);
    const conflictTablePayload = (await conflictTable.json()) as {
      id: number;
    };

    const reservation = await api.post(
      `${API_BASE}/inside/businesses/${businessID}/reservations`,
      {
        headers: ownerHeaders(),
        data: {
          table_id: conflictTablePayload.id,
          customer_name: "Keyboard Guest",
          party_size: 2,
          reservation_time: soon(7),
          phone: "+15551230000",
        },
      },
    );
    expect(reservation.status(), await reservation.text()).toBe(201);
    moveReservationToUiToday(Number((await reservation.json()).id));

    const staffEmail = `operator-a11y-manager-${stamp}@payverge.test`;
    const invite = await api.post(
      `${API_BASE}/inside/businesses/${businessID}/staff/invite`,
      {
        headers: ownerHeaders(),
        data: {
          email: staffEmail,
          name: "Accessibility Manager",
          role: "manager",
        },
      },
    );
    expect(invite.status(), await invite.text()).toBe(201);
    // Invitation tokens are never listed on GET staff (staff:read). Use the
    // invitation_url returned on invite (or staff:invite-gated /link).
    const inviteBody = await invite.json();
    const inviteToken = new URL(String(inviteBody.invitation_url)).searchParams.get(
      "token",
    );
    expect(inviteToken, "invite invitation_url must include token").toBeTruthy();
    const accept = await api.post(`${API_BASE}/staff/accept-invitation`, {
      headers: { Origin: ORIGIN },
      data: { token: inviteToken, name: "Accessibility Manager" },
    });
    expect(accept.status(), await accept.text()).toBe(201);
    staffToken = String((await accept.json()).token);
    expect(staffToken, "accepted staff invitation token").toBeTruthy();
  });

  test.afterAll(async () => {
    if (businessID && ownerToken) {
      await api.delete(`${API_BASE}/inside/businesses/${businessID}`, {
        headers: ownerHeaders(),
      });
    }
    await api?.dispose();
  });

  test.beforeEach(async ({ page, context }) => {
    await authenticateOperator(page, context);
  });

  // Root A hole 1: Overview is where Metric, ProactiveInsights, the more-
  // metrics disclosure, and quick-action target-size work live — scanning only
  // menu/bills/reservations let those defects walk through the axe gate.
  for (const tab of ["overview", "menu", "bills", "reservations"] as const) {
    test(`${tab} has no serious/critical (or wave-O named-rule) axe violations`, async ({
      page,
    }) => {
      await page.goto(`/business/${businessID}/dashboard?tab=${tab}`);
      await expect(page).toHaveURL(
        new RegExp(`/business/${businessID}/dashboard`),
      );
      await expect(page.getByTestId("dashboard-shell")).toBeVisible({
        timeout: 30_000,
      });
      await expectNoSeriousViolations(page);
    });
  }

  test("onboarding setup status recovers from a failed first request and stays axe-clean", async ({
    page,
  }) => {
    let setupStatusAttempts = 0;
    let releaseFirstRequest: (() => void) | undefined;
    const firstRequestHeld = new Promise<void>((resolve) => {
      releaseFirstRequest = resolve;
    });
    const setupStatusPattern = new RegExp(
      `/api/v1/inside/businesses/${businessID}/setup-status(?:\\?.*)?$`,
    );

    await page.route(setupStatusPattern, async (route: Route) => {
      setupStatusAttempts += 1;
      if (setupStatusAttempts === 1) {
        await firstRequestHeld;
        await route.fulfill({
          status: 503,
          contentType: "application/json",
          body: JSON.stringify({ error: "temporary setup-status failure" }),
        });
        return;
      }
      await route.continue();
    });

    await page.goto(`/business/${businessID}/dashboard`);
    const loading = page.getByRole("status", {
      name: "Loading your setup checklist",
    });
    await expect(loading).toBeVisible({ timeout: 30_000 });
    await expectNoSeriousViolations(page);

    releaseFirstRequest?.();
    const retry = page.getByRole("button", { name: "Try again" });
    await expect(retry).toBeVisible();
    await expectNoSeriousViolations(page);
    await retry.click();

    await expect
      .poll(() => setupStatusAttempts, { timeout: 30_000 })
      .toBeGreaterThanOrEqual(2);
    await expect(retry).toBeHidden();
    await expectNoSeriousViolations(page);
  });

  test("Recent Alerts opens by keyboard, closes with Escape, and restores focus", async ({
    page,
  }) => {
    await page.goto(`/business/${businessID}/dashboard`);
    const trigger = await recentAlertsTrigger(page);
    await trigger.focus();
    await page.keyboard.press("Space");
    await expect(page.locator("#recent-alerts-panel")).toBeVisible();
    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    const dialog = page.locator('[role="dialog"]:visible');
    await expect(dialog).toHaveCount(1);
    await expect
      .poll(() =>
        dialog.evaluate((element) => element.contains(document.activeElement)),
      )
      .toBe(true);
    await page.keyboard.press("Escape");
    await expect(page.locator("#recent-alerts-panel")).toBeHidden();
    await expect(trigger).toHaveAttribute("aria-expanded", "false");
    await expect(trigger).toBeFocused();
  });

  test("Recent Alerts opens through the project's pointer or touchscreen input", async ({
    page,
  }, testInfo) => {
    await page.goto(`/business/${businessID}/dashboard`);
    const trigger = await recentAlertsTrigger(page);

    await expect(trigger).toHaveAttribute("aria-expanded", "false");
    if (testInfo.project.use.hasTouch) {
      await trigger.tap();
    } else {
      await trigger.click();
    }

    await expect(trigger).toHaveAttribute("aria-expanded", "true");
    await expect(page.locator("#recent-alerts-panel")).toBeVisible();
    await expect(page.locator('[role="dialog"]:visible')).toHaveCount(1);

    await page.keyboard.press("Escape");
    await expect(trigger).toHaveAttribute("aria-expanded", "false");
  });

  test("bill creation modal is keyboard reachable and axe-clean", async ({
    page,
  }) => {
    await page.goto(`/business/${businessID}/dashboard?tab=bills`);
    const create = page
      .getByRole("button", { name: /create bill|new bill/i })
      .first();
    await create.focus();
    await page.keyboard.press("Enter");
    const dialog = page.locator('[role="dialog"]:visible');
    await expect(dialog).toHaveCount(1);
    await expectNoSeriousViolations(page);
    await page.keyboard.press("Escape");
    await expect(create).toBeFocused();
  });

  test("menu edit and kitchen bill actions activate from the keyboard", async ({
    page,
  }) => {
    await page.goto(`/business/${businessID}/dashboard?tab=menu`);
    const editItem = page.getByRole("button", {
      name: "Edit Accessible Burger",
      exact: true,
    });
    await editItem.focus();
    await page.keyboard.press("Enter");
    await expect(page.locator('[role="dialog"]:visible')).toHaveCount(1);
    await page.keyboard.press("Escape");

    await page.goto(`/business/${businessID}/dashboard?tab=kitchen`);
    const needsApproval = page.getByRole("region", {
      name: "Needs approval",
      exact: true,
    });
    const approve = needsApproval.getByRole("button", {
      name: "Approve",
      exact: true,
    });
    await expect(approve).toBeVisible();
    await approve.focus();
    await expect(approve).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(needsApproval).toBeHidden();

    const approvedTab = page.getByRole("tab", { name: /^Approved/ });
    await expect(approvedTab).toHaveAttribute("aria-selected", "true");
    const viewBill = page
      .getByRole("button", {
        name: `View bill #${billID}`,
        exact: true,
      })
      .filter({ visible: true });
    await expect(viewBill).toBeVisible({ timeout: 15_000 });
    await viewBill.focus();
    await expect(viewBill).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(
      new RegExp(`tab=bills&billId=${billID}(?:&|$)`),
    );
  });

  test("menu grid item editor opens from a pointer click", async ({ page }) => {
    await page.goto(`/business/${businessID}/dashboard?tab=menu`);
    await page.getByRole("button", { name: "Grid view" }).click();

    const editItem = page.getByRole("button", {
      name: "Edit Accessible Burger",
      exact: true,
    });
    await expect(editItem).toBeVisible();
    await editItem.click();

    const dialog = page.locator('[role="dialog"]:visible');
    await expect(dialog).toHaveCount(1);
    await expect(dialog).toContainText("Edit Item");
    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
  });

  test("menu import dropdown closes with Escape and restores trigger focus", async ({
    page,
  }) => {
    await page.goto(`/business/${businessID}/dashboard?tab=menu`);
    const trigger = page.getByRole("button", {
      name: "Import a menu from PDF or AI",
      exact: true,
    });

    await trigger.focus();
    await trigger.click();
    const menu = page.getByRole("menu", {
      name: "Import a menu from PDF or AI",
      exact: true,
    });
    await expect(menu).toBeVisible();

    await page.keyboard.press("Escape");
    await expect(menu).toBeHidden();
    await expect(trigger).toBeFocused();
  });

  test("reservation details expose one dialog and return focus on Escape", async ({
    page,
  }) => {
    await page.goto(`/business/${businessID}/dashboard?tab=reservations`);
    await page
      .getByRole("textbox", {
        name: "Search by customer name, phone, or table...",
        exact: true,
      })
      .fill("Keyboard Guest");
    await page
      .getByRole("button", { name: "All in window", exact: true })
      .click();
    const details = page
      .getByRole("button", {
        name: /^View reservation Keyboard Guest,/i,
      })
      .filter({ visible: true });
    await expect(details).toHaveCount(1, { timeout: 10_000 });
    await expect(details).toBeVisible();
    await details.focus();
    await page.keyboard.press("Enter");
    await expect(page.locator('[role="dialog"]:visible')).toHaveCount(1);
    await page.keyboard.press("Escape");
    await expect(details).toBeFocused();
  });

  test("PV-PROD-013 waiter chooser on table and menu routes", async ({
    page,
  }, testInfo) => {
    const routes = [`/t/${tableCode}`, `/t/${tableCode}/menu`];

    for (const [index, route] of routes.entries()) {
      await page.goto(route);
      const trigger = page
        .getByRole("button", { name: "Call waiter", exact: true })
        .filter({ visible: true });
      await expect(trigger).toHaveCount(1);

      if (index === 0 && testInfo.project.use.hasTouch) {
        await trigger.tap();
      } else {
        await trigger.focus();
        await page.keyboard.press(index === 0 ? "Enter" : "Space");
      }

      const chooser = page.getByRole("group", {
        name: "Call waiter",
        exact: true,
      });
      await expect(chooser).toBeVisible();
      await expect(
        chooser.getByRole("button", { name: "Water", exact: true }),
      ).toBeFocused();
      await page.keyboard.press("Escape");
      await expect(chooser).toBeHidden();
      await expect(trigger).toBeFocused();
    }
  });

  test("PV-PROD-015 business time and occupancy warnings", async ({ page }) => {
    await page.goto(`/business/${businessID}/dashboard?tab=reservations`);
    await page.getByTestId("reservation-create-button").click();

    const dialog = page.locator('[role="dialog"]:visible');
    await expect(dialog).toHaveCount(1);
    await expect(
      dialog.getByRole("radio", { name: "Business time", exact: true }),
    ).toBeChecked();

    const conversion = dialog.locator(
      'dl[aria-label="Time conversion before save"]',
    );
    await expect(conversion).toContainText(BUSINESS_TIMEZONE);
    await expect(conversion).toContainText(DEVICE_TIMEZONE);

    const occupied = dialog.getByRole("button", {
      name: "Accessibility-1, Occupied — active bill",
      exact: true,
    });
    const staleOccupied = dialog.getByRole("button", {
      name: "Accessibility-Stale, Stale occupied — old active bill",
      exact: true,
    });
    await expect(occupied).toBeVisible();
    await expect(staleOccupied).toBeVisible();

    await occupied.click();
    const override = dialog.getByRole("checkbox", {
      name: "Override active bill conflict for this reservation",
      exact: true,
    });
    await expect(override).toBeVisible();
    await expect(
      dialog.getByRole("button", { name: "Create Reservation", exact: true }),
    ).toBeDisabled();
    await override.check();
    await expect(
      dialog.getByRole("button", { name: "Create Reservation", exact: true }),
    ).toBeEnabled();
    await expectNoSeriousViolations(page);
  });

  test("PV-PROD-016 guest checkout axe", async ({ page }) => {
    await page.goto(`/t/${tableCode}/menu`);
    await expectNoSeriousViolations(page);

    const add = page
      .getByRole("button", { name: /^(Add|Add to order|Add to Cart)$/ })
      .filter({ visible: true })
      .first();
    await expect(add).toBeVisible();
    await add.click();

    const viewCart = page
      .getByRole("button", { name: /View cart/i })
      .filter({ visible: true })
      .first();
    await expect(viewCart).toBeVisible();
    await viewCart.click();
    await expect(page.locator('[role="dialog"]:visible')).toHaveCount(1);
    await expectNoSeriousViolations(page);
  });

  test("PV-PROD-014 open guest bill closes live", async ({ page }) => {
    await page.goto(`/t/${tableCode}`);
    await expect(page.getByText("Current Bill", { exact: true })).toBeVisible();

    const close = await api.post(`${API_BASE}/inside/bills/${billID}/close`, {
      headers: {
        ...ownerHeaders(),
        "Idempotency-Key": `release-close-bill-${billID}`,
      },
      data: {},
    });
    expect(close.status(), await close.text()).toBe(200);

    // The SSE event should update immediately; the 15-second recovery poll is
    // still inside this timeout so a dropped event cannot leave stale bill UI.
    await expect(page.getByText("No Active Bill", { exact: true })).toBeVisible(
      {
        timeout: 20_000,
      },
    );
    await expect(page.getByText("Current Bill", { exact: true })).toBeHidden();
  });
});
