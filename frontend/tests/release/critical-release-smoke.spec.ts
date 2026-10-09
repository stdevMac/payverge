/**
 * Critical release smoke — runs against a full stack built from the release images.
 *
 * Covers the minimal production-critical path:
 *   1. Frontend boots
 *   2. Backend readiness (GET /api/v1/health/ready == 200)
 *   3. Register + login + token refresh
 *   4. Public business/menu load
 *   5. Table / bill token load
 *   6. Atomic guest checkout, same-key replay, and unavailable-item rollback
 *   7. Browser payment provider redirect/cancel return and storage cleanup
 *   8. Arabic RTL on the server response and hydrated table bill
 *   9. Authenticated operator dashboard load
 *
 * Seeds isolated data via API (+ SQL for email verification), then DELETEs the
 * business (and relies on cascade) after the run.
 *
 * Run:
 *   npx playwright test tests/release/critical-release-smoke.spec.ts
 *
 * Env:
 *   PLAYWRIGHT_BASE_URL  — frontend origin (Caddy edge or :3000)
 *   PLAYWRIGHT_API_BASE  — API origin including /api/v1
 */
import {
  test,
  expect,
  type APIRequestContext,
  type Page,
} from "@playwright/test";
import { execFileSync } from "node:child_process";
import { createServer, type Server } from "node:http";
import { connect } from "node:net";
import path from "node:path";

const API_BASE =
  process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1";

const EMAIL_RE = /^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$/;
const COMPOSE_FILE = path.resolve(
  __dirname,
  "..",
  "..",
  "..",
  "docker-compose.yml",
);
const POSTGRES_USER = "payverge";
const POSTGRES_DB = "payverge";

interface LocalAPIProxy {
  server: string;
  close: () => Promise<void>;
}

/**
 * Playwright's APIRequestContext can prefer ::1 even when Compose publishes
 * the API only on 127.0.0.1. Route local HTTP CONNECTs through an ephemeral,
 * IPv4-only tunnel while preserving `localhost` as the request hostname so
 * production-mode Secure cookies keep their valid localhost scope.
 */
async function startLocalAPIProxy(
  apiBase: string,
): Promise<LocalAPIProxy | null> {
  const apiURL = new URL(apiBase);
  if (apiURL.protocol !== "http:" || apiURL.hostname !== "localhost") {
    return null;
  }

  const apiPort = Number(apiURL.port || "80");
  const proxy: Server = createServer((_request, response) => {
    response.writeHead(405);
    response.end();
  });

  proxy.on("connect", (request, clientSocket, head) => {
    const requested = new URL(`http://${request.url}`);
    const requestedPort = Number(requested.port || "80");
    if (requested.hostname !== "localhost" || requestedPort !== apiPort) {
      clientSocket.end("HTTP/1.1 403 Forbidden\r\n\r\n");
      return;
    }

    const upstream = connect(apiPort, "127.0.0.1", () => {
      clientSocket.write("HTTP/1.1 200 Connection Established\r\n\r\n");
      if (head.length > 0) {
        upstream.write(head);
      }
      upstream.pipe(clientSocket);
      clientSocket.pipe(upstream);
    });
    upstream.on("error", () => {
      clientSocket.end("HTTP/1.1 502 Bad Gateway\r\n\r\n");
    });
  });

  await new Promise<void>((resolve, reject) => {
    const onError = (error: Error) => reject(error);
    proxy.once("error", onError);
    proxy.listen(0, "127.0.0.1", () => {
      proxy.off("error", onError);
      resolve();
    });
  });

  const address = proxy.address();
  if (address == null || typeof address === "string") {
    await new Promise<void>((resolve) => proxy.close(() => resolve()));
    throw new Error("local API proxy did not bind an IPv4 port");
  }

  return {
    server: `http://127.0.0.1:${address.port}`,
    close: () =>
      new Promise<void>((resolve, reject) => {
        proxy.close((error) => (error == null ? resolve() : reject(error)));
      }),
  };
}

/** Mark email as verified so POST /auth/login succeeds after register. */
function markEmailVerified(email: string): void {
  if (!EMAIL_RE.test(email)) {
    throw new Error(`refusing to verify non-email value: ${email}`);
  }
  const sql = [
    `UPDATE user_auths`,
    `SET email_verified = true, updated_at = NOW()`,
    `WHERE provider = 'email' AND provider_user_id = $$${email}$$;`,
  ].join(" ");

  if (process.env.PLAYWRIGHT_DIRECT_PSQL === "1") {
    const dbHost = process.env.PLAYWRIGHT_DB_HOST || "localhost";
    const dbPort = process.env.PLAYWRIGHT_DB_PORT || "5432";
    const dbPassword =
      process.env.PLAYWRIGHT_DB_PASSWORD || "payverge_password";
    execFileSync(
      "psql",
      [
        "-h",
        dbHost,
        "-p",
        dbPort,
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
        env: { ...process.env, PGPASSWORD: dbPassword },
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

type ConsoleFailure = { type: string; text: string };
type ResponseFailure = { url: string; status: number };

/**
 * Fail the test on browser console errors or failed same-origin API responses.
 * External third-party noise (analytics, fonts) is ignored.
 */
function attachStrictObservers(page: Page): {
  consoleErrors: ConsoleFailure[];
  apiFailures: ResponseFailure[];
  assertClean: () => void;
} {
  const consoleErrors: ConsoleFailure[] = [];
  const apiFailures: ResponseFailure[] = [];

  page.on("console", (msg) => {
    if (msg.type() === "error") {
      consoleErrors.push({ type: msg.type(), text: msg.text() });
    }
  });

  page.on("pageerror", (err) => {
    consoleErrors.push({ type: "pageerror", text: err.message });
  });

  page.on("response", (response) => {
    const url = response.url();
    const status = response.status();
    if (status < 400) return;

    // Only same-origin API / app failures — not third-party 4xx/5xx.
    let isSameOrigin = false;
    try {
      const base = new URL(
        page.url() ||
          process.env.PLAYWRIGHT_BASE_URL ||
          "http://localhost:3000",
      );
      const resUrl = new URL(url);
      isSameOrigin = resUrl.origin === base.origin;
    } catch {
      isSameOrigin = false;
    }

    const isApi =
      url.includes("/api/v1/") ||
      url.includes("api.payverge.io") ||
      (process.env.PLAYWRIGHT_API_BASE != null &&
        url.startsWith(process.env.PLAYWRIGHT_API_BASE));

    if (isSameOrigin || isApi) {
      apiFailures.push({ url, status });
    }
  });

  return {
    consoleErrors,
    apiFailures,
    assertClean: () => {
      expect(
        consoleErrors,
        `browser console errors:\n${consoleErrors.map((e) => `  [${e.type}] ${e.text}`).join("\n")}`,
      ).toEqual([]);
      expect(
        apiFailures,
        `failed same-origin/API responses:\n${apiFailures.map((f) => `  ${f.status} ${f.url}`).join("\n")}`,
      ).toEqual([]);
    },
  };
}

async function registerOwner(
  api: APIRequestContext,
  email: string,
  password: string,
  name: string,
): Promise<{ token: string; userId: number }> {
  const resp = await api.post(`${API_BASE}/auth/register`, {
    data: { email, password, name },
  });
  expect(resp.status(), await resp.text()).toBe(201);
  const body = await resp.json();
  expect(body.token, "register must issue a session token").toBeTruthy();
  return { token: body.token as string, userId: Number(body.user_id) };
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
  expect(body.token, "login must issue a session token").toBeTruthy();
  return body.token as string;
}

async function loadCheckoutRows(
  api: APIRequestContext,
  businessId: number,
  authHeaders: { Authorization: string },
): Promise<{
  bills: Array<{ id: number; table_id?: number }>;
  billTotal: number;
  orders: Array<{ id: number; bill_id: number }>;
  orderTotal: number;
}> {
  const [billResponse, orderResponse] = await Promise.all([
    api.get(
      `${API_BASE}/inside/businesses/${businessId}/bills?page=1&page_size=100`,
      { headers: authHeaders },
    ),
    api.get(
      `${API_BASE}/inside/businesses/${businessId}/orders?page=1&page_size=100`,
      { headers: authHeaders },
    ),
  ]);
  expect(billResponse.status(), await billResponse.text()).toBe(200);
  expect(orderResponse.status(), await orderResponse.text()).toBe(200);
  const billBody = await billResponse.json();
  const orderBody = await orderResponse.json();
  return {
    bills: billBody.bills ?? [],
    billTotal: Number(billBody.total ?? 0),
    orders: orderBody.orders ?? [],
    orderTotal: Number(orderBody.total ?? 0),
  };
}

test.describe("Critical release smoke", () => {
  test("covers auth, checkout, provider return, RTL, and the operator dashboard", async ({
    page,
    playwright,
  }) => {
    test.setTimeout(180_000);
    const stamp = Date.now();
    const email = `release-smoke-${stamp}@payverge.test`;
    const password = "E2e!Release-Smoke-42";
    const ownerName = "Release Smoke Owner";
    const customUrl = `rsmoke-${stamp}`;
    const tableName = `RS-${stamp % 10000}`;

    const observers = attachStrictObservers(page);
    const localAPIProxy = await startLocalAPIProxy(API_BASE);
    const api = await playwright.request.newContext({
      baseURL: API_BASE,
      // Release smoke terminates production hostnames through an ephemeral
      // self-signed Caddy certificate generated by the workflow.
      ignoreHTTPSErrors: true,
      // Capture Set-Cookie from register/login for /auth/refresh.
      extraHTTPHeaders: { Accept: "application/json" },
      ...(localAPIProxy
        ? { proxy: { server: localAPIProxy.server } }
        : undefined),
    });

    let businessNumericId: number | undefined;
    let token = "";

    try {
      // ── 1. Frontend boots ───────────────────────────────────────────────
      const home = await page.goto("/", { waitUntil: "domcontentloaded" });
      expect(home, "frontend root must respond").toBeTruthy();
      expect(home!.status(), "frontend root HTTP status").toBeLessThan(500);
      await expect(page.locator("body")).toBeVisible({ timeout: 30_000 });

      // ── 2. Backend readiness ────────────────────────────────────────────
      // Prefer absolute ready URL (works whether Playwright base is Caddy edge
      // or direct frontend port). API_BASE ends with /api/v1.
      const readyUrl = `${API_BASE.replace(/\/api\/v1\/?$/, "")}/api/v1/health/ready`;
      const ready = await api.get(readyUrl);
      expect(ready.status(), await ready.text()).toBe(200);

      // ── 3. Register → verify email (SQL) → login → token refresh ───────
      const registered = await registerOwner(api, email, password, ownerName);
      token = registered.token;
      markEmailVerified(email);
      token = await loginOwner(api, email, password);

      const loginState = await api.storageState();
      const persistedRefresh = loginState.cookies.find(
        (cookie) => cookie.name === "refresh_token",
      )?.value;
      const persistedSession = loginState.cookies.find(
        (cookie) => cookie.name === "session_token",
      )?.value;
      expect(
        persistedRefresh,
        `login must persist refresh_token cookie for ${new URL(API_BASE).hostname}`,
      ).toBeTruthy();
      expect(
        persistedSession,
        `login must persist session_token cookie for ${new URL(API_BASE).hostname}`,
      ).toBeTruthy();

      const refresh = await api.post(`${API_BASE}/auth/refresh`);
      expect(refresh.status(), await refresh.text()).toBe(200);

      // Refresh rotates both the refresh token and the server-side hash of the
      // access token. The endpoint intentionally returns only cookies, so keep
      // using the newly issued session_token rather than the now-revoked login
      // bearer token.
      const refreshedState = await api.storageState();
      const refreshedSession = refreshedState.cookies.find(
        (cookie) => cookie.name === "session_token",
      )?.value;
      const refreshedRefresh = refreshedState.cookies.find(
        (cookie) => cookie.name === "refresh_token",
      )?.value;
      expect(
        refreshedSession,
        "refresh must rotate session_token cookie",
      ).toBeTruthy();
      expect(
        refreshedRefresh,
        "refresh must rotate refresh_token cookie",
      ).toBeTruthy();
      expect(refreshedRefresh).not.toBe(persistedRefresh);
      token = refreshedSession!;

      const authHeaders = { Authorization: `Bearer ${token}` };

      // ── Seed: business + publish + menu + table + kitchen/orders ───────
      const bizName = `Release Smoke Bistro ${stamp}`;
      const createResp = await api.post(`${API_BASE}/inside/businesses`, {
        headers: authHeaders,
        data: {
          name: bizName,
          owner_name: ownerName,
          email,
          business_type: "restaurant",
          address: { country: "US" },
          custom_url: customUrl,
          business_page_enabled: true,
        },
      });
      expect(createResp.status(), await createResp.text()).toBe(201);
      const business = await createResp.json();
      businessNumericId = Number(business.id);
      expect(businessNumericId, "created business id").toBeTruthy();

      // Ensure public storefront is published and guest ordering is on.
      const updateResp = await api.put(
        `${API_BASE}/inside/businesses/${businessNumericId}`,
        {
          headers: authHeaders,
          data: {
            custom_url: customUrl,
            business_page_enabled: true,
          },
        },
      );
      expect(updateResp.ok(), await updateResp.text()).toBeTruthy();

      const kitchenResp = await api.post(
        `${API_BASE}/inside/businesses/${businessNumericId}/toggle-kitchen-orders`,
        {
          headers: authHeaders,
          data: { enabled: true },
        },
      );
      expect(kitchenResp.ok(), await kitchenResp.text()).toBeTruthy();

      const catResp = await api.post(
        `${API_BASE}/inside/businesses/${businessNumericId}/menu/categories`,
        {
          headers: authHeaders,
          data: { name: "Smoke Mains", description: "" },
        },
      );
      expect(catResp.status(), await catResp.text()).toBe(201);

      const itemResp = await api.post(
        `${API_BASE}/inside/businesses/${businessNumericId}/menu/items`,
        {
          headers: authHeaders,
          data: {
            category_index: 0,
            item: {
              name: "Smoke Empanada",
              description: "Release smoke item",
              price: 9.5,
              currency: "USD",
              is_available: true,
            },
          },
        },
      );
      expect(itemResp.status(), await itemResp.text()).toBe(201);

      const unavailableItemResp = await api.post(
        `${API_BASE}/inside/businesses/${businessNumericId}/menu/items`,
        {
          headers: authHeaders,
          data: {
            category_index: 0,
            item: {
              name: "Smoke Sold Out",
              description: "Release rollback fixture",
              price: 11,
              currency: "USD",
              is_available: false,
            },
          },
        },
      );
      expect(
        unavailableItemResp.status(),
        await unavailableItemResp.text(),
      ).toBe(201);

      const tableResp = await api.post(
        `${API_BASE}/inside/businesses/${businessNumericId}/tables`,
        {
          headers: authHeaders,
          data: { name: tableName, capacity: 2 },
        },
      );
      expect(tableResp.status(), await tableResp.text()).toBe(201);
      const tableBody = await tableResp.json();
      // CreateTableWithQR embeds *database.Table + qr_url at the top level.
      const tableCode: string = String(tableBody.table_code ?? "");
      expect(tableCode, "table code from create").toBeTruthy();

      const rollbackTableResp = await api.post(
        `${API_BASE}/inside/businesses/${businessNumericId}/tables`,
        {
          headers: authHeaders,
          data: { name: `${tableName}-rollback`, capacity: 2 },
        },
      );
      expect(rollbackTableResp.status(), await rollbackTableResp.text()).toBe(
        201,
      );
      const rollbackTableCode = String(
        (await rollbackTableResp.json()).table_code ?? "",
      );
      expect(rollbackTableCode, "rollback table code from create").toBeTruthy();

      // ── 4. Public business / menu load ─────────────────────────────────
      const pubBiz = await api.get(`${API_BASE}/business/${customUrl}`);
      expect(pubBiz.status(), await pubBiz.text()).toBe(200);
      const pubBizBody = await pubBiz.json();
      expect(pubBizBody.custom_url ?? pubBizBody.id).toBeTruthy();

      const pubMenu = await api.get(`${API_BASE}/business/${customUrl}/menu`);
      expect(pubMenu.status(), await pubMenu.text()).toBe(200);
      const menuText = JSON.stringify(await pubMenu.json());
      expect(menuText).toContain("Smoke Empanada");

      // ── 5. Table / bill token load ─────────────────────────────────────
      const guestTable = await api.get(`${API_BASE}/guest/table/${tableCode}`);
      expect(guestTable.status(), await guestTable.text()).toBe(200);

      const guestMenu = await api.get(
        `${API_BASE}/guest/table/${tableCode}/menu`,
      );
      expect(guestMenu.status(), await guestMenu.text()).toBe(200);
      const guestMenuBody = await guestMenu.json();
      const smokeMenuItems = (
        (guestMenuBody.categories ?? []) as Array<{
          items?: Array<{ id?: string; name?: string }>;
        }>
      ).flatMap((category) => category.items ?? []);
      const availableItemId = String(
        smokeMenuItems.find((item) => item.name === "Smoke Empanada")?.id ?? "",
      );
      const unavailableItemId = String(
        smokeMenuItems.find((item) => item.name === "Smoke Sold Out")?.id ?? "",
      );
      expect(availableItemId, "available menu item id").toBeTruthy();
      expect(unavailableItemId, "unavailable menu item id").toBeTruthy();

      // The checkout acceptance begins with no prior bill.
      const guestBillProbe = await api.get(
        `${API_BASE}/guest/table/${tableCode}/bill`,
      );
      expect(guestBillProbe.status(), await guestBillProbe.text()).toBe(200);
      expect((await guestBillProbe.json()).bill).toBeNull();
      expect(
        await loadCheckoutRows(api, businessNumericId, authHeaders),
      ).toMatchObject({
        bills: [],
        billTotal: 0,
        orders: [],
        orderTotal: 0,
      });

      // ── 6a. PV-PROD-001 atomic checkout identity and counts ────────────
      // Submit without bill_id. The endpoint must create one bill and one order
      // in the same transaction, then return those exact rows for a same-key retry.
      const requestId = `release-smoke-order-${stamp}`;
      const orderRequest = {
        headers: { "X-Request-Id": requestId },
        data: {
          items: [
            {
              menu_item_name: "Smoke Empanada",
              menu_item_id: availableItemId,
              quantity: 1,
              price: 9.5,
            },
          ],
          notes: "Release smoke retry-safe checkout setup",
        },
      };
      const firstOrder = await api.post(
        `${API_BASE}/guest/table/${tableCode}/order`,
        orderRequest,
      );
      expect(firstOrder.status(), await firstOrder.text()).toBe(201);
      const firstOrderBody = await firstOrder.json();
      const firstOrderId = Number(firstOrderBody.order?.id);
      const billId = Number(firstOrderBody.bill?.id);
      const publicToken = String(firstOrderBody.bill?.public_token ?? "");
      expect(firstOrderId, "created guest order id").toBeTruthy();
      expect(billId, "atomically created guest bill id").toBeTruthy();
      expect(firstOrderBody.replay).toBe(false);
      expect(publicToken, "opaque guest public token").toMatch(
        /^[a-f0-9]{32}$/i,
      );

      const afterFirst = await loadCheckoutRows(
        api,
        businessNumericId,
        authHeaders,
      );
      expect(afterFirst.billTotal).toBe(1);
      expect(afterFirst.orderTotal).toBe(1);
      expect(afterFirst.bills.map((bill) => Number(bill.id))).toEqual([billId]);
      expect(afterFirst.orders.map((order) => Number(order.id))).toEqual([
        firstOrderId,
      ]);
      expect(Number(afterFirst.orders[0]?.bill_id)).toBe(billId);

      const replayOrder = await api.post(
        `${API_BASE}/guest/table/${tableCode}/order`,
        orderRequest,
      );
      expect(replayOrder.status(), await replayOrder.text()).toBe(200);
      const replayOrderBody = await replayOrder.json();
      expect(replayOrderBody.duplicate).toBe(true);
      expect(replayOrderBody.replay).toBe(true);
      expect(Number(replayOrderBody.order?.id)).toBe(firstOrderId);
      expect(Number(replayOrderBody.bill?.id)).toBe(billId);

      const afterReplay = await loadCheckoutRows(
        api,
        businessNumericId,
        authHeaders,
      );
      expect(afterReplay).toEqual(afterFirst);

      // The public capability returned by the atomic endpoint resolves the same bill.
      const byToken = await api.get(`${API_BASE}/guest/bill/${publicToken}`);
      expect(byToken.ok(), await byToken.text()).toBeTruthy();
      expect(Number((await byToken.json()).bill?.id)).toBe(billId);

      // ── 7. Provider redirect / return in the real guest bill UI ──────
      // Exercise the application-owned browser seam without contacting a live
      // provider: the payment catalog and initiation responses are deterministic,
      // while the real UI still builds its return/cancel URLs, stores only the
      // opaque resume reference, navigates cross-origin, and consumes the return.
      const appOrigin = new URL(
        process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000",
      ).origin;
      const providerOrigin = "https://provider.payverge.test";
      const providerHandoff = `${providerOrigin}/provider-handoff`;
      let capturedCancelURL = "";

      await page.route(
        "**/api/v1/businesses/**/payment-plugins**",
        async (route) => {
          await route.fulfill({
            status: 200,
            contentType: "application/json",
            body: JSON.stringify({
              plugins: [
                {
                  plugin_id: 2,
                  name: "paypal",
                  display_name: "PayPal",
                  description: "Release smoke provider",
                  image: "",
                  category: "payment",
                  version: "test",
                  is_enabled: true,
                },
              ],
            }),
          });
        },
      );
      await page.route(
        "**/api/v1/guest/bill/**/plugin-payment",
        async (route) => {
          expect(route.request().method()).toBe("POST");
          const payload = route.request().postDataJSON() as {
            plugin_id?: string;
            return_url?: string;
            cancel_url?: string;
          };
          expect(payload.plugin_id).toBe("paypal");
          expect(payload.return_url).toContain(
            `/t/${tableCode}/bill?payment=success&method=paypal&bill_token=${publicToken}`,
          );
          expect(payload.cancel_url).toContain(
            `/t/${tableCode}/bill?payment=cancelled&method=paypal&bill_token=${publicToken}`,
          );
          expect(new URL(payload.return_url!).origin).toBe(appOrigin);
          expect(new URL(payload.cancel_url!).origin).toBe(appOrigin);
          capturedCancelURL = payload.cancel_url!;
          await route.fulfill({
            status: 200,
            contentType: "application/json",
            body: JSON.stringify({
              payment_id: `release-smoke-payment-${stamp}`,
              status: "pending",
              payment_url: providerHandoff,
            }),
          });
        },
      );
      await page.route(`${providerHandoff}**`, async (route) => {
        const returnHref = capturedCancelURL.replaceAll("&", "&amp;");
        await route.fulfill({
          status: 200,
          contentType: "text/html; charset=utf-8",
          body: `<!doctype html><html lang="en"><body><h1>Provider handoff</h1><a href="${returnHref}">Return to Payverge</a></body></html>`,
        });
      });

      await page.goto(`/t/${tableCode}/bill`, {
        waitUntil: "domcontentloaded",
      });
      await expect(page.getByRole("button", { name: /PayPal/i })).toBeVisible({
        timeout: 30_000,
      });
      await page.getByRole("button", { name: /PayPal/i }).click();
      await Promise.all([
        page.waitForURL(`${providerHandoff}**`, { timeout: 30_000 }),
        page.getByRole("button", { name: /Pay Now/i }).click(),
      ]);
      await expect(
        page.getByRole("heading", { name: "Provider handoff" }),
      ).toBeVisible();
      await page.getByRole("link", { name: "Return to Payverge" }).click();
      await expect(page).toHaveURL(new RegExp(`/t/${tableCode}/bill$`), {
        timeout: 30_000,
      });
      await expect(
        page.getByText(/PayPal payment was cancelled/i),
      ).toBeVisible();
      expect(
        await page.evaluate(() => sessionStorage.getItem("payverge_payment")),
      ).toBeNull();

      // ── 8. Arabic RTL is present before hydration and remains stable ──
      const rtlResponse = await page.goto(`/t/${tableCode}/bill?lang=ar`, {
        waitUntil: "domcontentloaded",
      });
      expect(rtlResponse, "Arabic bill response must exist").toBeTruthy();
      expect(rtlResponse!.status()).toBe(200);
      const rtlHTML = await rtlResponse!.text();
      expect(rtlHTML).toMatch(/<html[^>]*lang="ar"[^>]*dir="rtl"/i);
      await expect(page.locator("html")).toHaveAttribute("lang", "ar");
      await expect(page.locator("html")).toHaveAttribute("dir", "rtl");
      await expect(page.locator("body")).toBeVisible();

      // ── 6b. PV-PROD-001 unavailable checkout rollback ────────────────
      // Use a fresh table so a rejected item can prove that neither aggregate
      // is created. The unavailable item's exact identity must survive the envelope.
      const rejected = await api.post(
        `${API_BASE}/guest/table/${rollbackTableCode}/order`,
        {
          headers: { "X-Request-Id": `release-smoke-unavailable-${stamp}` },
          data: {
            items: [
              {
                menu_item_name: "Smoke Sold Out",
                menu_item_id: unavailableItemId,
                quantity: 1,
                price: 11,
              },
            ],
          },
        },
      );
      expect(rejected.status(), await rejected.text()).toBe(409);
      const rejectedBody = await rejected.json();
      expect(rejectedBody.code).toBe("item_not_orderable");
      expect(rejectedBody.details?.items).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            menu_item_id: unavailableItemId,
            reason: "manual_disabled",
          }),
        ]),
      );
      expect(
        await loadCheckoutRows(api, businessNumericId, authHeaders),
      ).toEqual(afterFirst);
      const rejectedTableBill = await api.get(
        `${API_BASE}/guest/table/${rollbackTableCode}/bill`,
      );
      expect(rejectedTableBill.status(), await rejectedTableBill.text()).toBe(
        200,
      );
      expect((await rejectedTableBill.json()).bill).toBeNull();

      // ── 7. Authenticated operator dashboard load ───────────────────────
      const base = new URL(
        process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000",
      );
      await page.context().addCookies([
        {
          name: "session_token",
          value: token,
          domain: base.hostname,
          path: "/",
          httpOnly: false,
          secure: base.protocol === "https:",
          sameSite: "Lax",
        },
      ]);
      await page.goto("/dashboard", { waitUntil: "domcontentloaded" });
      await expect(page).not.toHaveURL(/stripe\.com/);
      // Owner session should land on dashboard or a business console route —
      // never bounce to the public login funnel.
      await expect(page).not.toHaveURL(/\/login(\?|$)/, { timeout: 30_000 });
      const dashUrl = page.url();
      expect(
        /dashboard|business|onboarding|home/i.test(dashUrl),
        `expected operator surface, got ${dashUrl}`,
      ).toBeTruthy();

      observers.assertClean();
    } finally {
      // ── Cleanup: delete isolated business (best-effort cascade) ────────
      if (businessNumericId && token) {
        try {
          await api.delete(
            `${API_BASE}/inside/businesses/${businessNumericId}`,
            {
              headers: { Authorization: `Bearer ${token}` },
            },
          );
        } catch {
          // Do not mask the test failure if cleanup fails.
        }
      }
      await api.dispose();
      await localAPIProxy?.close();
    }
  });
});
