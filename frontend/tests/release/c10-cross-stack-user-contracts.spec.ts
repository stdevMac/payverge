/**
 * C10 cross-stack release contracts.
 *
 * Every test below drives a shipped browser surface and then checks the API or
 * database state that surface changed. Provider-dependent boundaries are
 * intercepted at the browser edge; Payverge's own handlers and persistence
 * remain real.
 */
import {
  expect,
  test,
  type APIRequestContext,
  type Page,
} from "@playwright/test";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import path from "node:path";

const API_BASE =
  process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1";
const ORIGIN = process.env.PLAYWRIGHT_API_ORIGIN || "http://localhost:3000";
const BASE_URL = process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000";
const COMPOSE_FILE = path.resolve(__dirname, "../../..", "docker-compose.yml");
const EMAIL_RE = /^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$/;

let api: APIRequestContext;
let ownerToken = "";
let businessId = 0;
let customUrl = "";
let fixtureOwnerEmail = "";
let fixtureOwnerPassword = "";
let fixtureInviteDigest = "";
const createdBusinessIds: number[] = [];
const suspendedBusinessIds = new Set<number>();

const ownerHeaders = () => ({
  Authorization: `Bearer ${ownerToken}`,
  Origin: ORIGIN,
});
const guestHeaders = () => ({ Origin: ORIGIN });

async function renewOwnerToken(): Promise<void> {
  if (!fixtureOwnerEmail || !fixtureOwnerPassword) {
    throw new Error("owner credentials are not initialized");
  }
  // Multi-project serial runs (5×11) re-login every journey. Local auth rate
  // limits can return 429 after Chromium; backoff and retry instead of failing
  // the suite on a transient harness throttle.
  const maxAttempts = 8;
  let lastStatus = 0;
  let lastBody = "";
  for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
    const login = await api.post(`${API_BASE}/auth/login`, {
      headers: guestHeaders(),
      data: {
        email: fixtureOwnerEmail,
        password: fixtureOwnerPassword,
      },
    });
    lastStatus = login.status();
    if (lastStatus === 200) {
      const token = String((await login.json()).token || "");
      if (!token) {
        throw new Error("owner login returned no token");
      }
      ownerToken = token;
      return;
    }
    lastBody = await login.text();
    if (lastStatus === 429 && attempt < maxAttempts) {
      await new Promise((resolve) => setTimeout(resolve, 1500 * attempt));
      continue;
    }
    break;
  }
  throw new Error(`owner login failed: ${lastStatus} ${lastBody}`);
}

function runSQL(sql: string): void {
  const commonArgs = [
    "-U",
    "payverge",
    "-d",
    "payverge",
    "-v",
    "ON_ERROR_STOP=1",
    "-q",
  ];
  if (process.env.PLAYWRIGHT_DIRECT_PSQL === "1") {
    execFileSync(
      "psql",
      [
        "-h",
        process.env.PLAYWRIGHT_DB_HOST || "localhost",
        "-p",
        process.env.PLAYWRIGHT_DB_PORT || "5432",
        ...commonArgs,
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
      ...commonArgs,
    ],
    { input: sql, stdio: ["pipe", "pipe", "pipe"] },
  );
}

function markEmailVerified(email: string): void {
  if (!EMAIL_RE.test(email)) {
    throw new Error(`refusing to verify non-email value: ${email}`);
  }
  runSQL(
    `UPDATE user_auths SET email_verified = true, updated_at = NOW() ` +
      `WHERE provider = 'email' AND provider_user_id = $$${email}$$; ` +
      `UPDATE users SET email_verified = true, updated_at = NOW() ` +
      `WHERE LOWER(email) = LOWER($$${email}$$);`,
  );
}

function setFixtureBusinessSuspended(id: number, suspended: boolean): void {
  if (!Number.isSafeInteger(id) || id <= 0) {
    throw new Error(`refusing to mutate invalid business id: ${id}`);
  }
  runSQL(
    `UPDATE businesses SET is_active = ${suspended ? "false" : "true"}, updated_at = NOW() ` +
      `WHERE id = ${id};`,
  );
  if (suspended) suspendedBusinessIds.add(id);
  else suspendedBusinessIds.delete(id);
}

async function authenticateOwner(page: Page): Promise<void> {
  // Mirror operator-accessibility: seed session_token on both the app and API
  // hostnames (cookies are host-scoped; ports share a host for localhost), and
  // set payverge_had_session so HybridAuthProvider bootstraps from the cookie
  // instead of treating the visit as anonymous.
  const app = new URL(BASE_URL);
  const api = new URL(API_BASE);
  const cookieHosts = Array.from(new Set([app.hostname, api.hostname]));
  await page.context().addCookies(
    cookieHosts.map((hostname) => ({
      name: "session_token",
      value: ownerToken,
      domain: hostname,
      path: "/",
      httpOnly: false,
      secure: app.protocol === "https:" || api.protocol === "https:",
      sameSite: "Lax" as const,
    })),
  );
  await page.addInitScript(() => {
    localStorage.setItem("payverge_had_session", "1");
  });
}

async function openRecentAlerts(page: Page): Promise<void> {
  await page
    .getByTestId("recent-alerts-trigger")
    .first()
    .evaluate((element: HTMLElement) => element.click());
}

async function createBusiness(
  stamp: string,
  name: string,
  email: string,
): Promise<{ id: number; customUrl: string }> {
  const url = `c10-${stamp}`.toLowerCase().replace(/[^a-z0-9-]/g, "-");
  const response = await api.post(`${API_BASE}/inside/businesses`, {
    headers: ownerHeaders(),
    data: {
      name,
      owner_name: "C10 Owner",
      email,
      business_type: "restaurant",
      address: { country: "US" },
      custom_url: url,
      business_page_enabled: true,
      settlement_address: "0x3333333333333333333333333333333333333333",
    },
  });
  expect(response.status(), await response.text()).toBe(201);
  const id = Number((await response.json()).id);
  expect(id).toBeGreaterThan(0);
  createdBusinessIds.push(id);
  return { id, customUrl: url };
}

async function newTableCode(label: string): Promise<string> {
  const response = await api.post(
    `${API_BASE}/inside/businesses/${businessId}/tables`,
    {
      headers: ownerHeaders(),
      data: { name: label, capacity: 4 },
    },
  );
  expect(response.status(), await response.text()).toBe(201);
  return String((await response.json()).table_code);
}

type OrderedBill = {
  billId: number;
  token: string;
  number: string;
  orderId: number;
};

async function seatOrderedBill(
  code: string,
  quantity = 2,
): Promise<OrderedBill> {
  const billResponse = await api.post(`${API_BASE}/guest/table/${code}/bill`, {
    headers: guestHeaders(),
    data: {},
  });
  expect(billResponse.status(), await billResponse.text()).toBe(201);
  const bill = (await billResponse.json()).bill;
  const orderResponse = await api.post(
    `${API_BASE}/guest/table/${code}/order`,
    {
      headers: {
        ...guestHeaders(),
        "X-Request-Id": `c10-order-${code}-${Date.now()}`,
      },
      data: {
        bill_id: Number(bill.id),
        items: [{ menu_item_name: "Burger", quantity }],
      },
    },
  );
  expect(orderResponse.status(), await orderResponse.text()).toBe(201);
  const orderId = Number((await orderResponse.json()).order.id);
  const approve = await api.put(
    `${API_BASE}/inside/businesses/${businessId}/orders/${orderId}/status`,
    { headers: ownerHeaders(), data: { status: "approved" } },
  );
  expect(approve.status(), await approve.text()).toBe(200);
  return {
    billId: Number(bill.id),
    token: String(bill.public_token),
    number: String(bill.bill_number),
    orderId,
  };
}

async function settleOrderedBillWithCash(
  bill: OrderedBill,
  idempotencyScope: string,
): Promise<void> {
  const response = await api.post(
    `${API_BASE}/inside/bills/${bill.billId}/alternative-payment`,
    {
      headers: {
        ...ownerHeaders(),
        "Idempotency-Key": `c10-${idempotencyScope}-${bill.billId}`,
      },
      data: {
        amount: "25.00",
        payment_method: "cash",
        business_confirmation: true,
      },
    },
  );
  expect(response.status(), await response.text()).toBe(200);
}

async function inviteAndAcceptStaff(
  targetBusinessId: number,
  email: string,
  name: string,
): Promise<number> {
  const invite = await api.post(
    `${API_BASE}/inside/businesses/${targetBusinessId}/staff/invite`,
    {
      headers: ownerHeaders(),
      data: { email, name, role: "server" },
    },
  );
  expect(invite.status(), await invite.text()).toBe(201);
  const inviteBody = await invite.json();
  const inviteToken = String(
    new URL(String(inviteBody.invitation_url)).searchParams.get("token") || "",
  );
  expect(inviteToken.length).toBeGreaterThan(10);
  const accept = await api.post(`${API_BASE}/staff/accept-invitation`, {
    headers: guestHeaders(),
    data: { token: inviteToken, name },
  });
  expect(accept.status(), await accept.text()).toBe(201);
  return Number((await accept.json()).staff.id);
}

function isoDay(offset: number): string {
  const date = new Date(Date.now() + offset * 86_400_000);
  date.setUTCHours(0, 0, 0, 0);
  return date.toISOString();
}

test.describe.serial("C10 cross-stack user contracts", () => {
  // These are full browser-to-API-to-PostgreSQL journeys. Keep the timeout in
  // source so the protected release workflow has the same contract as focused
  // local runs; a CLI-only override would leave the official gate at 120s.
  test.describe.configure({ timeout: 300_000 });

  test.beforeAll(async ({ playwright }) => {
    api = await playwright.request.newContext({ ignoreHTTPSErrors: true });
    const stamp = `${Date.now()}-${process.pid}`;
    const email = `c10-owner-${stamp}@payverge.test`;
    fixtureOwnerEmail = email;
    const password = "C10!Release-42";
    fixtureOwnerPassword = password;
    const inviteCode = `c10-release-${stamp}`;
    const inviteDigest = createHash("sha256").update(inviteCode).digest("hex");
    fixtureInviteDigest = inviteDigest;
    runSQL(
      `INSERT INTO runtime_invite_batches ` +
        `(name, code_digest, cohort_cap, claimed_count, active, owner, reason, expires_at, created_by, created_at, updated_at) ` +
        `VALUES ('C10 release ${stamp}', '${inviteDigest}', 10, 0, true, ` +
        `'release-test', 'C10 isolated fixture', NOW() + INTERVAL '1 hour', 'playwright', NOW(), NOW());`,
    );
    const register = await api.post(`${API_BASE}/auth/register`, {
      headers: guestHeaders(),
      data: {
        email,
        password,
        name: "C10 Owner",
        invite_code: inviteCode,
      },
    });
    expect(register.status(), await register.text()).toBe(201);
    markEmailVerified(email);
    await renewOwnerToken();

    const business = await createBusiness(stamp, "C10 Contract Bistro", email);
    businessId = business.id;
    customUrl = business.customUrl;

    const enableOrdering = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/toggle-kitchen-orders`,
      { headers: ownerHeaders(), data: { enabled: true } },
    );
    expect(enableOrdering.status(), await enableOrdering.text()).toBe(200);
    const category = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/menu/categories`,
      { headers: ownerHeaders(), data: { name: "Mains" } },
    );
    expect(category.status(), await category.text()).toBe(201);
    const item = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/menu/items`,
      {
        headers: ownerHeaders(),
        data: {
          category_index: 0,
          item: {
            name: "Burger",
            description: "C10 fixture burger",
            price: 12.5,
            is_available: true,
          },
        },
      },
    );
    expect(item.status(), await item.text()).toBe(201);
  });

  test.beforeEach(async () => {
    // Access JWTs are intentionally short-lived. This serial behavioral suite
    // runs longer than one token lifetime, so renew through the real login
    // endpoint before each journey instead of weakening auth or leaking stale
    // credentials into later tests.
    await renewOwnerToken();
  });

  test.afterAll(async () => {
    const cleanupFailures: string[] = [];
    let businessesCleaned = true;
    try {
      await renewOwnerToken();
    } catch (error) {
      cleanupFailures.push(
        `owner token renewal: ${error instanceof Error ? error.message : String(error)}`,
      );
    }
    for (const id of suspendedBusinessIds) {
      try {
        setFixtureBusinessSuspended(id, false);
      } catch (error) {
        cleanupFailures.push(
          `suspension restore ${id}: ${error instanceof Error ? error.message : String(error)}`,
        );
      }
    }
    for (const id of [...createdBusinessIds].reverse()) {
      const response = await api?.delete(
        `${API_BASE}/inside/businesses/${id}`,
        { headers: ownerHeaders() },
      );
      if (!response) {
        businessesCleaned = false;
        cleanupFailures.push(`business ${id}: API context unavailable`);
      } else if (!response.ok()) {
        businessesCleaned = false;
        cleanupFailures.push(
          `business ${id}: ${response.status()} ${await response.text()}`,
        );
      }
    }
    try {
      if (fixtureOwnerEmail && fixtureInviteDigest && businessesCleaned) {
        runSQL(
          `BEGIN; ` +
            `DELETE FROM user_sessions WHERE user_id IN ` +
            `(SELECT id FROM users WHERE LOWER(email) = LOWER($$${fixtureOwnerEmail}$$)); ` +
            `DELETE FROM user_auths WHERE user_id IN ` +
            `(SELECT id FROM users WHERE LOWER(email) = LOWER($$${fixtureOwnerEmail}$$)); ` +
            `DELETE FROM runtime_invite_claims WHERE normalized_email = LOWER($$${fixtureOwnerEmail}$$); ` +
            `DELETE FROM runtime_invite_batches WHERE code_digest = '${fixtureInviteDigest}'; ` +
            `DELETE FROM users WHERE LOWER(email) = LOWER($$${fixtureOwnerEmail}$$); ` +
            `COMMIT;`,
        );
      } else if (!businessesCleaned) {
        cleanupFailures.push(
          "identity cleanup skipped because at least one fixture business remains active",
        );
      }
    } catch (error) {
      cleanupFailures.push(
        `identity cleanup: ${error instanceof Error ? error.message : String(error)}`,
      );
    }
    await api?.dispose();
    expect(cleanupFailures, cleanupFailures.join("\n")).toEqual([]);
  });

  test("pending payment request is not presented as payment received", async ({
    page,
  }) => {
    const bill = await seatOrderedBill(await newTableCode("C10-Pending"));
    const request = await api.post(
      `${API_BASE}/guest/bill/${bill.token}/request-alternative-payment`,
      {
        headers: {
          ...guestHeaders(),
          "Idempotency-Key": `c10-pending-${bill.billId}`,
        },
        data: {
          amount: "25.00",
          payment_method: "cash",
          participant_name: "C10 Guest",
        },
      },
    );
    expect(request.status(), await request.text()).toBe(200);
    const requestId = Number((await request.json()).request_id);

    await authenticateOwner(page);
    await page.goto(`/business/${businessId}/dashboard`);
    await openRecentAlerts(page);
    await expect(
      page.getByText("Payment requested", { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByText("Payment received", { exact: true }),
    ).toHaveCount(0);

    const confirm = await api.post(
      `${API_BASE}/inside/bills/${bill.billId}/alternative-payment`,
      {
        headers: {
          ...ownerHeaders(),
          "Idempotency-Key": `c10-confirm-${bill.billId}`,
        },
        data: {
          request_id: requestId,
          amount: "25.00",
          payment_method: "cash",
          business_confirmation: true,
        },
      },
    );
    expect(confirm.status(), await confirm.text()).toBe(200);
    await page.reload();
    await openRecentAlerts(page);
    await expect(
      page.getByText("Payment received", { exact: true }),
    ).toBeVisible();
  });

  test("alternative-payment limits and retry idempotency survive the browser boundary", async ({
    page,
  }) => {
    const operatorBill = await seatOrderedBill(
      await newTableCode("C10-Operator-Limit"),
    );
    await authenticateOwner(page);
    await page.goto(
      `/business/${businessId}/bills/${operatorBill.billId}/alternative-payments`,
    );
    await page.getByRole("button", { name: /Add Payment/i }).click();
    await page.getByLabel("Amount (USD)").fill("25.01");
    let operatorPosts = 0;
    page.on("request", (request) => {
      if (
        request.method() === "POST" &&
        request
          .url()
          .includes(`/inside/bills/${operatorBill.billId}/alternative-payment`)
      ) {
        operatorPosts += 1;
      }
    });
    await page.getByRole("button", { name: /Mark Payment Received/i }).click();
    await expect(
      page.getByText("Amount cannot exceed bill total"),
    ).toBeVisible();
    expect(operatorPosts).toBe(0);

    const guestCode = await newTableCode("C10-Guest-Retry");
    const guestBill = await seatOrderedBill(guestCode);
    const keys: string[] = [];
    let attempts = 0;
    await page.route(
      `**/guest/bill/${guestBill.token}/request-alternative-payment`,
      async (route) => {
        attempts += 1;
        keys.push(route.request().headers()["idempotency-key"] || "");
        // Always hit the real handler so idempotency and persistence stay real.
        // Firefox/WebKit route.continue() after a prior fulfill is flaky; use
        // fetch + fulfill for every attempt and only rewrite the first status.
        const upstream = await route.fetch();
        const body = await upstream.text();
        if (attempts === 1) {
          expect(upstream.status(), body).toBe(200);
          // Model an edge/gateway losing the successful upstream response: the
          // real handler has committed, while the browser receives only a
          // retryable gateway failure.
          await route.fulfill({
            status: 504,
            contentType: "application/json",
            body: JSON.stringify({
              code: "gateway_timeout",
              error: "The upstream response was lost",
            }),
          });
          return;
        }
        await route.fulfill({
          status: upstream.status(),
          contentType:
            upstream.headers()["content-type"] || "application/json",
          body,
        });
      },
    );
    await page.goto(`/t/${guestCode}/bill`);
    const cashierPay = page.getByRole("button", {
      name: "Pay with Cashier (Cash/Card)",
    });
    await cashierPay.click();
    await expect(
      page.getByText(
        /Something went wrong\. Please try again\.|Could not (reach the cashier|notify staff)/i,
      ),
    ).toBeVisible({ timeout: 30_000 });
    // After the synthetic 504 the button re-enables; force-click so a sticky
    // toast overlay on Firefox/WebKit cannot swallow the retry. The guest
    // client keeps the same Idempotency-Key for this fingerprint.
    await expect(cashierPay).toBeEnabled({ timeout: 30_000 });
    await expect(cashierPay).not.toHaveAttribute("data-loading", "true");
    const retryRequest = page.waitForRequest(
      (request) =>
        request.url().includes("/request-alternative-payment") &&
        request.method() === "POST",
      { timeout: 30_000 },
    );
    await cashierPay.click({ force: true });
    await retryRequest;
    await expect(
      page.getByText(
        /Pay at the counter|Your table has been notified/i,
      ),
    ).toBeVisible({ timeout: 30_000 });
    expect(keys).toHaveLength(2);
    expect(keys[0]).toBeTruthy();
    expect(keys[1]).toBe(keys[0]);
    const pending = await api.get(
      `${API_BASE}/inside/bills/${guestBill.billId}/pending-alternative-payments`,
      { headers: ownerHeaders() },
    );
    expect(pending.status(), await pending.text()).toBe(200);
    const pendingBody = await pending.json();
    expect(
      pendingBody.requests ?? pendingBody.pending_payments ?? [],
    ).toHaveLength(1);
  });

  test("an occupied counter opens the existing bill instead of creating a duplicate", async ({
    page,
  }) => {
    const settings = await api.put(
      `${API_BASE}/inside/businesses/${businessId}/counters/settings`,
      {
        headers: ownerHeaders(),
        data: { counter_enabled: true, counter_count: 2, counter_prefix: "C" },
      },
    );
    expect(settings.status(), await settings.text()).toBe(200);
    const counters = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/counters`,
      { headers: ownerHeaders() },
    );
    const counter = (await counters.json()).counters[0] as {
      id: number;
      name: string;
    };
    await authenticateOwner(page);
    await page.goto(`/business/${businessId}/dashboard?tab=bills`);
    await page
      .getByRole("button", { name: /Create Bill/i })
      .first()
      .click();
    const counterTrigger = page.getByRole("button", {
      name: "Choose a counter for takeaway",
    });
    await counterTrigger.click();
    // Prefer option click over keyboard navigation: NextUI listboxes do not
    // always move selection on ArrowDown under headless Chromium, and an
    // unselected trigger never shows the counter name.
    await page.getByRole("option", { name: counter.name, exact: true }).click();
    await expect(
      page.locator('button[aria-haspopup="listbox"]', {
        hasText: counter.name,
      }),
    ).toBeVisible();
    // NextUI menu rows are pressable cards (role=button). Click the row, not
    // the bare text node, so onPress adds the item to the draft.
    const burgerRow = page
      .getByRole("button")
      .filter({ hasText: /^Burger/ })
      .first();
    await expect(burgerRow).toBeVisible();
    await burgerRow.click();
    await expect(page.getByText(/1 items? selected/i).first()).toBeVisible();
    const submit = page.getByRole("button", { name: /Create Bill/i }).last();
    await expect(submit).toBeEnabled();

    // Race another operator into the same counter after this draft selected it
    // but before it submits. This is the real 409 recovery path; occupied
    // counters are intentionally absent when the modal first loads.
    const create = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/bills`,
      {
        headers: ownerHeaders(),
        data: {
          counter_id: counter.id,
          notes: "occupied fixture",
          items: [
            { menu_item_id: "", name: "Burger", price: 12.5, quantity: 1 },
          ],
        },
      },
    );
    expect(create.status(), await create.text()).toBe(201);
    const existingBill = (await create.json()).bill;
    await submit.click();
    await expect(
      page.getByText("That location already has an open bill. Opening it now."),
    ).toBeVisible();
    // Toast and bill header can both include the number; pin exact text so
    // Playwright's strict mode does not fail on the dual match.
    await expect(
      page.getByText(`Bill #${existingBill.bill_number}`, { exact: true }),
    ).toBeVisible();

    const open = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/bills/open`,
      { headers: ownerHeaders() },
    );
    const matches = (
      (await open.json()).bills as Array<{ counter_id: number }>
    ).filter((bill) => Number(bill.counter_id) === Number(counter.id));
    expect(matches).toHaveLength(1);
  });

  test("staff can be reactivated and a multi-business login requires a venue choice", async ({
    page,
  }) => {
    const stamp = `${Date.now()}-${process.pid}`;
    const staffEmail = `c10-staff-${stamp}@payverge.test`;
    const staffId = await inviteAndAcceptStaff(
      businessId,
      staffEmail,
      "Casey C10",
    );
    const deactivate = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/staff/${staffId}/deactivate`,
      {
        headers: ownerHeaders(),
        data: { reason: "C10 inactive fixture" },
      },
    );
    expect(deactivate.status(), await deactivate.text()).toBe(200);

    await authenticateOwner(page);
    await page.goto(`/business/${businessId}/dashboard?tab=staff`);
    const staffRow = page.locator("tr", {
      has: page.getByText(staffEmail, { exact: true }),
    });
    await expect(staffRow.getByText("Inactive", { exact: true })).toBeVisible();
    await staffRow.getByRole("button", { name: "Manage staff" }).click();
    await expect(
      page.getByRole("button", { name: "Reactivate Staff" }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Reactivate Staff" }).click();
    await expect(
      page.getByText("Staff reactivated successfully"),
    ).toBeVisible();
    const staffList = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/staff`,
      { headers: ownerHeaders() },
    );
    const reactivated = ((await staffList.json()).staff ?? []).find(
      (member: { id: number }) => Number(member.id) === staffId,
    );
    expect(reactivated?.is_active).toBe(true);

    const second = await createBusiness(
      `${stamp}-second`,
      "C10 Second Venue",
      `c10-second-${stamp}@payverge.test`,
    );
    await inviteAndAcceptStaff(second.id, staffEmail, "Casey C10");
    const loginCode = String((Date.now() % 900_000) + 100_000);
    runSQL(
      `INSERT INTO staff_login_codes (staff_id, code, expires_at, used, created_at) ` +
        `VALUES (${staffId}, '${loginCode}', NOW() + INTERVAL '10 minutes', false, NOW());`,
    );

    await page.context().clearCookies();
    await page.route("**/staff/request-login-code", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: "{}",
      });
    });
    const verifyBodies: Array<Record<string, unknown>> = [];
    page.on("request", (request) => {
      if (
        request.method() === "POST" &&
        request.url().endsWith("/staff/verify-login-code")
      ) {
        verifyBodies.push(request.postDataJSON() as Record<string, unknown>);
      }
    });
    await page.goto("/staff/login");
    await page.getByLabel("Enter your email").fill(staffEmail.toUpperCase());
    await page.getByRole("button", { name: "Send Code" }).click();
    await page.getByLabel("Enter verification code").fill(loginCode);
    const membershipChoiceResponsePromise = page.waitForResponse(
      (response) =>
        response.url().endsWith("/staff/verify-login-code") &&
        response.request().method() === "POST" &&
        !Boolean(
          (response.request().postDataJSON() as Record<string, unknown>)
            .selection_token,
        ),
    );
    // Header nav uses "Sign in"; the staff form submit is "Sign In" (exact).
    await page.getByRole("button", { name: "Sign In", exact: true }).click();
    const membershipChoiceResponse = await membershipChoiceResponsePromise;
    expect(
      membershipChoiceResponse.status(),
      await membershipChoiceResponse.text(),
    ).toBe(200);
    const membershipChoice = await membershipChoiceResponse.json();
    expect(membershipChoice.membership_selection_required).toBe(true);
    expect(membershipChoice.token).toBeUndefined();
    expect(membershipChoice.staff_token).toBeUndefined();
    expect(
      (await page.context().cookies()).some(
        (cookie) => cookie.name === "staff_token",
      ),
    ).toBe(false);
    await expect(
      page.getByRole("heading", { name: "Choose a business" }),
    ).toBeVisible();
    await expect(
      page.getByText("C10 Contract Bistro", { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByText("C10 Second Venue", { exact: true }),
    ).toBeVisible();
    const selectedMembershipResponsePromise = page.waitForResponse(
      (response) =>
        response.url().endsWith("/staff/verify-login-code") &&
        response.request().method() === "POST" &&
        Boolean(
          (response.request().postDataJSON() as Record<string, unknown>)
            .selection_token,
        ),
    );
    await page.getByText("C10 Second Venue", { exact: true }).click();
    const selectedMembershipResponse = await selectedMembershipResponsePromise;
    expect(
      selectedMembershipResponse.status(),
      await selectedMembershipResponse.text(),
    ).toBe(200);
    const selectedMembership = await selectedMembershipResponse.json();
    expect(Number(selectedMembership.staff?.business_id)).toBe(second.id);
    const issuedToken = String(selectedMembership.token || "");
    expect(issuedToken.length).toBeGreaterThan(20);
    // Venue selection must not put the access token in document.cookie (HttpOnly
    // staff_token only). WebKit on localhost often will not surface host-only
    // cookies for a different port via credentials:include even though Chromium
    // and Firefox do; production uses shared parent Domain=.payverge.io.
    const staffTokenJsVisible = await page.evaluate(() =>
      document.cookie
        .split(";")
        .some((part) => part.trim().startsWith("staff_token=")),
    );
    expect(staffTokenJsVisible).toBe(false);
    const profile = await page.request.get(`${API_BASE}/staff/profile`, {
      headers: {
        ...guestHeaders(),
        Authorization: `Bearer ${issuedToken}`,
      },
    });
    expect(profile.status(), await profile.text()).toBe(200);
    expect(Number((await profile.json()).staff?.business_id)).toBe(second.id);
    expect(verifyBodies).toHaveLength(2);
    expect(verifyBodies[1].selection_token).toBeTruthy();
    expect(Number(verifyBodies[1].business_id)).toBe(second.id);
  });

  test("suspended business settings are read-only with the administrator notice", async ({
    page,
  }) => {
    await authenticateOwner(page);
    setFixtureBusinessSuspended(businessId, true);
    try {
      await page.goto(`/business/${businessId}/dashboard?tab=settings`);
      await expect(page.getByTestId("admin-lock-notice").first()).toBeVisible();
      await expect(page.getByLabel("Business Name")).toHaveCount(0);
      await expect(page.getByRole("button", { name: /^Save$/ })).toHaveCount(0);
    } finally {
      setFixtureBusinessSuspended(businessId, false);
    }
  });

  test("crypto is stopped at an unavailable quote before any wallet transfer", async ({
    page,
  }) => {
    const code = await newTableCode("C10-Crypto");
    const bill = await seatOrderedBill(code);
    let quoteRequests = 0;
    let paymentPosts = 0;
    await page.addInitScript(() => {
      const state = window as typeof window & { __c10WalletWrites?: number };
      state.__c10WalletWrites = 0;
      Object.defineProperty(window, "ethereum", {
        configurable: true,
        value: {
          on: () => undefined,
          removeListener: () => undefined,
          request: async ({ method }: { method: string }) => {
            if (method === "eth_requestAccounts" || method === "eth_accounts") {
              return ["0x4444444444444444444444444444444444444444"];
            }
            if (method === "eth_chainId") return "0x14a34";
            if (method === "wallet_switchEthereumChain") return null;
            if (method.toLowerCase().includes("sendtransaction")) {
              state.__c10WalletWrites = (state.__c10WalletWrites || 0) + 1;
              return "0xdeadbeef";
            }
            return null;
          },
        },
      });
    });
    await page.route(
      `**/businesses/${businessId}/payment-plugins**`,
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            plugins: [
              {
                plugin_id: 901,
                name: "usdc_payment",
                display_name: "USDC",
                description: "Pay with USDC",
                category: "payment",
                version: "test",
                is_enabled: true,
              },
            ],
          }),
        });
      },
    );
    page.on("request", (request) => {
      if (
        request.method() === "POST" &&
        request.url().endsWith(`/guest/bill/${bill.token}/crypto-quote`)
      ) {
        quoteRequests += 1;
      }
      if (
        request.method() === "POST" &&
        request.url().endsWith(`/guest/bill/${bill.token}/payment`)
      ) {
        paymentPosts += 1;
      }
    });
    await page.goto(`/t/${code}/bill`);
    await page.getByRole("button", { name: /USDC/i }).click();
    await page.getByRole("button", { name: /Pay Now/i }).click();
    await page.getByRole("button", { name: /Browser Wallet/i }).click();
    setFixtureBusinessSuspended(businessId, true);
    try {
      const quoteResponsePromise = page.waitForResponse(
        (response) =>
          response.url().endsWith(`/guest/bill/${bill.token}/crypto-quote`) &&
          response.request().method() === "POST",
      );
      await page.getByRole("button", { name: /^Pay \$/ }).click();
      const quoteResponse = await quoteResponsePromise;
      expect(quoteResponse.status(), await quoteResponse.text()).toBe(403);
      await expect(quoteResponse.json()).resolves.toMatchObject({
        code: "business_unavailable",
      });
      await expect(
        page
          .getByRole("dialog")
          .getByText("Ordering is not available for this table right now.")
          .first(),
      ).toBeVisible();
    } finally {
      setFixtureBusinessSuspended(businessId, false);
    }
    expect(quoteRequests).toBe(1);
    expect(paymentPosts).toBe(0);
    expect(
      await page.evaluate(
        () =>
          (window as typeof window & { __c10WalletWrites?: number })
            .__c10WalletWrites || 0,
      ),
    ).toBe(0);
  });

  test("receipt history distinguishes queued, permanently failed, and reprinted jobs", async ({
    page,
  }) => {
    const printer = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/printers`,
      {
        headers: ownerHeaders(),
        data: {
          name: "C10 Receipt Station",
          role: "bill",
          transport: "browser",
          paper_width_mm: 80,
          enabled: true,
        },
      },
    );
    expect(printer.status(), await printer.text()).toBe(201);
    const printerId = Number((await printer.json()).id);
    const bill = await seatOrderedBill(await newTableCode("C10-Receipt"));

    // A receipt represents collected money: the operator API must reject an
    // open bill, then the real payment transition must enqueue it automatically.
    const unpaidReceipt = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs`,
      {
        headers: ownerHeaders(),
        data: { kind: "receipt", source_type: "bill", source_id: bill.billId },
      },
    );
    expect(unpaidReceipt.status(), await unpaidReceipt.text()).toBe(409);
    await settleOrderedBillWithCash(bill, "receipt-paid");
    const firstJobsResponse = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs`,
      { headers: ownerHeaders() },
    );
    expect(firstJobsResponse.status(), await firstJobsResponse.text()).toBe(
      200,
    );
    const firstJob = (
      (await firstJobsResponse.json()).items as Array<{
        id: number;
        kind: string;
        source_type: string;
        source_id: number;
      }>
    ).find(
      (job) =>
        job.kind === "receipt" &&
        job.source_type === "bill" &&
        Number(job.source_id) === bill.billId,
    );
    expect(firstJob).toBeTruthy();
    const firstJobId = Number(firstJob?.id);
    const clientId = "7f2864c5-8b1c-4e56-9c33-a85ab7726df1";
    const claim = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs/claim`,
      {
        headers: ownerHeaders(),
        data: { client_id: clientId, printer_id: printerId },
      },
    );
    expect(claim.status(), await claim.text()).toBe(200);
    expect(Number((await claim.json()).job.id)).toBe(firstJobId);
    const cancel = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs/${firstJobId}/agent-cancel`,
      {
        headers: ownerHeaders(),
        data: { client_id: clientId, reason: "station offline" },
      },
    );
    expect(cancel.status(), await cancel.text()).toBe(200);
    // Use a distinct paid bill for the queued control row. Its payment
    // transition, not a manual print request, must be the receipt producer.
    const queuedBill = await seatOrderedBill(
      await newTableCode("C10-Receipt-Queued"),
    );
    await settleOrderedBillWithCash(queuedBill, "receipt-queued");
    const queuedJobsResponse = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/print/jobs`,
      { headers: ownerHeaders() },
    );
    expect(queuedJobsResponse.status(), await queuedJobsResponse.text()).toBe(
      200,
    );
    const queuedJob = (
      (await queuedJobsResponse.json()).items as Array<{
        id: number;
        kind: string;
        source_type: string;
        source_id: number;
      }>
    ).find(
      (job) =>
        job.kind === "receipt" &&
        job.source_type === "bill" &&
        Number(job.source_id) === queuedBill.billId,
    );
    expect(queuedJob).toBeTruthy();
    const queuedId = Number(queuedJob?.id);

    await authenticateOwner(page);
    await page.goto(`/business/${businessId}/dashboard?tab=printers`);
    const failedRow = page.locator("li", { hasText: `Job #${firstJobId}` });
    await expect(failedRow.getByText("Failed (permanent)")).toBeVisible();
    await expect(failedRow.getByText("station offline")).toBeVisible();
    const queuedRow = page.locator("li", { hasText: `Job #${queuedId}` });
    await expect(queuedRow.getByText("Queued")).toBeVisible();
    await expect(
      queuedRow.getByRole("button", { name: "Reprint" }),
    ).toHaveCount(0);
    const reprintResponse = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        response.url().endsWith(`/print/jobs/${firstJobId}/reprint`),
    );
    await failedRow.getByRole("button", { name: "Reprint" }).click();
    const reprinted = await reprintResponse;
    expect(reprinted.status()).toBe(201);
    const newJobId = Number((await reprinted.json()).id);
    await expect(
      page.locator("li", { hasText: `Job #${newJobId}` }),
    ).toContainText("Queued");
  });

  test("scheduled-report terminal failure uses actionable localized alert copy", async ({
    page,
  }) => {
    const resourceId = Date.now();
    runSQL(
      `INSERT INTO operational_alerts ` +
        `(business_id, alert_type, resource_type, resource_id, status, priority, title, body, metadata, created_at, updated_at) ` +
        `VALUES (${businessId}, 'report_delivery_failed', 'report_delivery', '${resourceId}', ` +
        `'open', 'high', 'WRONG BACKEND TITLE', 'WRONG BACKEND BODY', ` +
        `'${JSON.stringify({ state: "failed_permanent", attempts: 6 })}'::jsonb, NOW(), NOW());`,
    );
    await authenticateOwner(page);
    await page.goto(`/business/${businessId}/dashboard`);
    await openRecentAlerts(page);
    const reportAlert = page.locator("#recent-alerts-panel li", {
      hasText: "Scheduled report delivery failed",
    });
    await expect(
      reportAlert.getByText("Scheduled report delivery failed", {
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      reportAlert.getByText("WRONG BACKEND TITLE", { exact: true }),
    ).toHaveCount(0);
    await expect(reportAlert.getByText("Open", { exact: true })).toBeVisible();
  });

  test("failed payroll deletion restores confirmation and can be retried", async ({
    page,
  }) => {
    const create = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/accounting/payroll-runs`,
      {
        headers: ownerHeaders(),
        data: {
          period_start: isoDay(-2),
          period_end: isoDay(0),
          notes: "C10 retry deletion",
          line_items: [
            {
              payee_type: "contractor",
              payee_name: "C10 Contractor",
              gross_amount: 123.45,
              bonus_amount: 0,
              deduction_amount: 0,
            },
          ],
        },
      },
    );
    expect(create.status(), await create.text()).toBe(201);
    const runId = Number((await create.json()).data.id);
    let deletes = 0;
    await page.route(
      `**/inside/businesses/${businessId}/accounting/payroll-runs/${runId}`,
      async (route) => {
        if (route.request().method() !== "DELETE") {
          await route.continue();
          return;
        }
        deletes += 1;
        if (deletes === 1) {
          await route.fulfill({
            status: 500,
            contentType: "application/json",
            body: JSON.stringify({ error: "temporary delete failure" }),
          });
          return;
        }
        await route.continue();
      },
    );
    await authenticateOwner(page);
    await page.goto(
      `/business/${businessId}/dashboard?tab=accounting&sub=payroll&status=draft`,
    );
    const payrollRow = page.locator("tr", { hasText: "$123.45" });
    await payrollRow.click();
    await expect(
      page.getByRole("heading", { name: "Payroll run" }),
    ).toBeVisible();
    await page.getByRole("button", { name: "Delete" }).click();
    let confirmation = page.getByRole("dialog", {
      name: "Delete this draft payroll run?",
    });
    await confirmation.getByRole("button", { name: "Delete" }).click();
    confirmation = page.getByRole("dialog", {
      name: "Delete this draft payroll run?",
    });
    await expect(confirmation).toBeVisible();
    await expect(
      confirmation.getByRole("button", { name: "Delete" }),
    ).toBeEnabled();
    await confirmation.getByRole("button", { name: "Delete" }).click();
    await expect(confirmation).toBeHidden();
    await expect(payrollRow).toHaveCount(0);
    expect(deletes).toBe(2);
    const missing = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/accounting/payroll-runs/${runId}`,
      { headers: ownerHeaders() },
    );
    expect(missing.status()).toBe(404);
  });

  test("menu import previews dropped fields and persists only the sanitized menu", async ({
    page,
  }) => {
    const importedName = `C10 Taco ${Date.now()}`;
    await page.route(
      `**/inside/businesses/${businessId}/ai/wizard/start`,
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            session_id: 901,
            response: {
              message: "Tell me about the menu.",
              suggested_options: [],
              is_complete: false,
            },
          }),
        });
      },
    );
    await page.route(
      `**/inside/businesses/${businessId}/ai/wizard/*/message`,
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            session_id: 901,
            response: {
              message: "Ready to generate.",
              suggested_options: [],
              is_complete: true,
            },
          }),
        });
      },
    );
    await page.route(
      `**/inside/businesses/${businessId}/ai/wizard/*/generate`,
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            session_id: 901,
            menu: {
              currency: "USD",
              categories: [
                {
                  name: "AI Imports",
                  items: [
                    {
                      name: importedName,
                      description: "Sanitization fixture",
                      price: 14,
                      allergens: ["peanuts", "unicorn_dust"],
                      dietary_tags: ["moon_diet"],
                    },
                    { name: "Free sample", description: "drop me", price: 0 },
                  ],
                },
              ],
            },
          }),
        });
      },
    );
    const importBodies: Array<Record<string, unknown>> = [];
    page.on("request", (request) => {
      if (
        request.method() === "POST" &&
        request.url().includes("/ai/import-extracted-menu")
      ) {
        importBodies.push(request.postDataJSON() as Record<string, unknown>);
      }
    });
    await authenticateOwner(page);
    await page.goto(`/business/${businessId}/dashboard?tab=menu`);
    await page
      .getByRole("button", { name: "Import a menu from PDF or AI" })
      .click();
    await page.getByRole("menuitem", { name: /Generate with AI/i }).click();
    await page.getByRole("button", { name: /Start Conversation/i }).click();
    await page.getByPlaceholder("Type your response...").fill("A taco menu");
    await page.getByRole("button", { name: /Send message/i }).click();
    await page.getByRole("button", { name: /^Generate/i }).click();
    await page.getByRole("button", { name: /Import to Menu/i }).click();

    const review = page.getByRole("alertdialog", {
      name: "Review menu changes",
    });
    await expect(review).toBeVisible();
    await expect(review).toContainText("unicorn_dust");
    await expect(review).toContainText("moon_diet");
    await expect(review).toContainText("Free sample");
    await expect(review).toContainText(/peanuts.*peanut|peanut.*peanuts/i);
    expect(importBodies[0]?.confirm_sanitization).toBe(false);

    const before = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/menu`,
      { headers: ownerHeaders() },
    );
    expect(await before.text()).not.toContain(importedName);
    await review.getByRole("button", { name: /Confirm and import/i }).click();
    await expect(review).toBeHidden();
    expect(importBodies[1]?.confirm_sanitization).toBe(true);
    const after = await api.get(
      `${API_BASE}/inside/businesses/${businessId}/menu`,
      {
        headers: ownerHeaders(),
      },
    );
    const persisted = await after.text();
    expect(persisted).toContain(importedName);
    expect(persisted).toContain("peanut");
    expect(persisted).not.toContain("unicorn_dust");
    expect(persisted).not.toContain("moon_diet");
    expect(persisted).not.toContain("Free sample");
  });
});
