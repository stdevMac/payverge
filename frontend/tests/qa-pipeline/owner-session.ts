import {
  request as apiRequestFactory,
  test,
  type APIRequestContext,
  type BrowserContext,
  type Page,
} from "@playwright/test";

export const API_BASE =
  process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1";

export const OWNER_EMAIL =
  process.env.QA_OWNER_EMAIL ||
  process.env.LOCAL_ADMIN_EMAIL ||
  "admin@local.test";

// No baked-in fallback: the password comes from the environment only (set it
// to the owner account of the local stack under test). Specs that need an
// owner session skip, rather than fail, when it is unset.
export const OWNER_PASSWORD =
  process.env.QA_OWNER_PASSWORD || process.env.LOCAL_ADMIN_PASSWORD || "";

export const OWNER_PASSWORD_MISSING_REASON =
  "Set QA_OWNER_PASSWORD (or LOCAL_ADMIN_PASSWORD) to run owner-session QA specs";

/** Skip the current test/hook when no owner password is configured. */
function skipWithoutOwnerPassword(): void {
  test.skip(!OWNER_PASSWORD, OWNER_PASSWORD_MISSING_REASON);
}

export interface QaBusiness {
  id: number;
  name: string;
  custom_url: string;
}

export interface QaTable {
  id: number;
  name: string;
  table_code: string;
}

/**
 * Login as owner via API. Uses storageState cookies (session_token +
 * refresh_token) so the browser context can hit authenticated routes.
 *
 * Prefer hostname `localhost` (not 127.0.0.1) so Domain=localhost cookies
 * issued by the backend match the frontend origin.
 */
export async function loginOwnerApi(): Promise<{
  api: APIRequestContext;
  token: string;
  cookies: Array<{
    name: string;
    value: string;
    domain: string;
    path: string;
    httpOnly?: boolean;
    secure?: boolean;
    sameSite?: "Strict" | "Lax" | "None";
  }>;
}> {
  skipWithoutOwnerPassword();
  // Use origin-only baseURL. Playwright joins leading-slash paths against the
  // ORIGIN and drops path segments on the base (so baseURL .../api/v1 +
  // "/auth/login" becomes host/auth/login). Always call absolute API_BASE URLs.
  const apiOrigin = new URL(API_BASE).origin;
  const api = await apiRequestFactory.newContext({
    baseURL: apiOrigin,
    extraHTTPHeaders: {
      Origin: process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000",
    },
  });

  const resp = await api.post(`${API_BASE}/auth/login`, {
    data: { email: OWNER_EMAIL, password: OWNER_PASSWORD },
  });
  if (!resp.ok()) {
    const body = await resp.text();
    await api.dispose();
    throw new Error(`owner login failed: ${resp.status()} ${body}`);
  }
  const body = (await resp.json()) as { token?: string };
  if (!body.token) {
    await api.dispose();
    throw new Error("owner login returned no token");
  }

  const state = await api.storageState();
  const cookies = state.cookies.map((c) => ({
    name: c.name,
    value: c.value,
    domain: c.domain.replace(/^\./, ""),
    path: c.path || "/",
    httpOnly: c.httpOnly,
    secure: c.secure,
    sameSite: (c.sameSite as "Strict" | "Lax" | "None") || "Lax",
  }));

  // Ensure session_token is present even if cookie parsing was partial.
  if (!cookies.some((c) => c.name === "session_token")) {
    const host = new URL(API_BASE).hostname;
    cookies.push({
      name: "session_token",
      value: body.token,
      domain: host,
      path: "/",
      httpOnly: true,
      secure: false,
      sameSite: "Lax",
    });
  }

  return { api, token: body.token, cookies };
}

export async function seedOwnerSession(
  context: BrowserContext,
  cookies: Awaited<ReturnType<typeof loginOwnerApi>>["cookies"],
): Promise<void> {
  // Also seed for the frontend hostname if different from API host.
  const feHost = new URL(
    process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000",
  ).hostname;
  const expanded = [...cookies];
  for (const c of cookies) {
    if (c.domain !== feHost) {
      expanded.push({ ...c, domain: feHost });
    }
  }
  await context.addCookies(expanded);
}

export async function listOwnerBusinesses(
  api: APIRequestContext,
  token: string,
): Promise<QaBusiness[]> {
  const resp = await api.get(`${API_BASE}/inside/businesses`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!resp.ok()) {
    throw new Error(`list businesses: ${resp.status()} ${await resp.text()}`);
  }
  const data = await resp.json();
  const items: unknown[] = Array.isArray(data)
    ? data
    : (data?.businesses ?? data?.data ?? data?.items ?? []);
  return items.map((raw) => {
    const b = raw as Record<string, unknown>;
    return {
      id: Number(b.id),
      name: String(b.name || `business-${b.id}`),
      custom_url: String(b.custom_url || b.slug || ""),
    };
  });
}

export async function listBusinessTables(
  api: APIRequestContext,
  token: string,
  businessId: number,
): Promise<QaTable[]> {
  const resp = await api.get(
    `${API_BASE}/inside/businesses/${businessId}/tables`,
    {
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  if (!resp.ok()) {
    throw new Error(
      `list tables ${businessId}: ${resp.status()} ${await resp.text()}`,
    );
  }
  const data = await resp.json();
  const items: unknown[] = Array.isArray(data)
    ? data
    : (data?.tables ?? data?.data ?? []);
  return items
    .map((raw) => {
      const t = raw as Record<string, unknown>;
      return {
        id: Number(t.id),
        name: String(t.name || `table-${t.id}`),
        table_code: String(t.table_code || t.code || ""),
      };
    })
    .filter((t) => !!t.table_code);
}

/**
 * Light interaction pass: click non-destructive inner tab/filter controls.
 * Never clicks buttons whose accessible name looks destructive/payment-related.
 */
export async function safeClickAround(page: Page): Promise<void> {
  const dangerous =
    /\b(delete|remove|void|refund|pay|charge|close bill|destroy|wipe|logout|sign out|disable|revoke)\b/i;

  // Prefer role=tab first (accounting/staff/crm sub-navs). Cap tightly — this
  // runs on every surface and must not dominate the crawl budget.
  const tabs = page.getByRole("tab");
  const tabCount = Math.min(await tabs.count().catch(() => 0), 4);
  for (let i = 0; i < tabCount; i++) {
    const tab = tabs.nth(i);
    if (!(await tab.isVisible().catch(() => false))) continue;
    const name = ((await tab.innerText().catch(() => "")) || "").trim();
    if (dangerous.test(name)) continue;
    await tab.click({ timeout: 1_500 }).catch(() => {});
    await page.waitForTimeout(100);
  }
}
