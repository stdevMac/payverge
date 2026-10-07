import AxeBuilder from "@axe-core/playwright";
import {
  expect,
  test,
  type APIRequestContext,
  type APIResponse,
  type Page,
} from "@playwright/test";

const API_BASE =
  process.env.PLAYWRIGHT_API_BASE || "http://localhost:8080/api/v1";
const ORIGIN = process.env.PLAYWRIGHT_API_ORIGIN || "http://localhost:3000";
const AUTH_RATE_LIMIT_TIMEOUT_MS = 120_000;

test.setTimeout(AUTH_RATE_LIMIT_TIMEOUT_MS);

let api: APIRequestContext;
let token = "";
let businessId = 0;
let businessSlug = "";

const authHeaders = () => ({
  Authorization: `Bearer ${token}`,
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

/** Mirrors operator-accessibility Root A named-rule tier (Session O). */
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

  const namedWaveO = result.violations.filter((violation) =>
    (WAVE_O_NAMED_RULE_IDS as readonly string[]).includes(violation.id),
  );
  expect(namedWaveO).toEqual([]);
}

async function openPublicMenu(page: Page) {
  const menuTab = page.getByRole("tab", { name: "Menu", exact: true });
  if (await menuTab.isVisible()) {
    await menuTab.click();
    return;
  }

  await page
    .getByRole("button", { name: "View Menu", exact: true })
    .first()
    .click();
}

test.describe.serial("public business accessibility release gate", () => {
  test.beforeAll(async ({ playwright }) => {
    api = await playwright.request.newContext({ ignoreHTTPSErrors: true });
    const stamp = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const email = `public-a11y-${stamp}@payverge.test`;
    const register = await postWithRateLimitRetry(`${API_BASE}/auth/register`, {
      headers: { Origin: ORIGIN },
      data: {
        email,
        password: "E2e!Public-A11y-42",
        name: "Public Accessibility Owner",
      },
    });
    expect(register.status(), await register.text()).toBe(201);
    token = String((await register.json()).token);

    businessSlug = `public-a11y-${stamp}`;
    const business = await api.post(`${API_BASE}/inside/businesses`, {
      headers: authHeaders(),
      data: {
        name: "Public Accessibility Bistro",
        owner_name: "Public Accessibility Owner",
        email,
        business_type: "restaurant",
        address: { country: "US" },
        custom_url: businessSlug,
        business_page_enabled: true,
      },
    });
    expect(business.status(), await business.text()).toBe(201);
    businessId = Number((await business.json()).id);

    const category = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/menu/categories`,
      {
        headers: authHeaders(),
        data: { name: "Mains" },
      },
    );
    expect(category.status(), await category.text()).toBe(201);

    const item = await api.post(
      `${API_BASE}/inside/businesses/${businessId}/menu/items`,
      {
        headers: authHeaders(),
        data: {
          category_index: 0,
          item: {
            name: "Keyboard Risotto",
            description: "A release-gate fixture",
            price: 18,
            is_available: true,
          },
        },
      },
    );
    expect(item.status(), await item.text()).toBe(201);
  });

  test.afterAll(async () => {
    if (businessId && token) {
      await api.delete(`${API_BASE}/inside/businesses/${businessId}`, {
        headers: authHeaders(),
      });
    }
    await api?.dispose();
  });

  test("public menu is axe-clean and its item dialog restores keyboard focus", async ({
    page,
  }) => {
    await page.goto(`/b/${businessSlug}`);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await expect(
      page.getByRole("link", { name: "Skip to main content" }),
    ).toHaveCount(1);

    await openPublicMenu(page);
    const details = page.locator('button[aria-label^="Open "]').first();
    await expect(details).toBeVisible();
    await expectNoSeriousViolations(page);

    await details.focus();
    await page.keyboard.press("Enter");
    const dialog = page.locator('[role="dialog"][aria-modal="true"]:visible');
    await expect(dialog).toHaveCount(1);
    await expectNoSeriousViolations(page);

    await page.keyboard.press("Escape");
    await expect(dialog).toBeHidden();
    await expect(details).toBeFocused();
  });
});
