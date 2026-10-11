/**
 * Seeded Assistant V2 journeys.
 *
 * The product shell, guest catalog, authentication, RBAC and dashboard routes
 * are real. Only model-facing endpoints are intercepted so the browser checks
 * deterministic UI behavior instead of depending on a live LLM.
 *
 * Run:
 *   PLAYWRIGHT_RUN_ASSISTANT_V2_E2E=1 npx playwright test assistant-v2.spec.ts \
 *     --project=chromium --project=mobile-chrome
 */
import {
  expect,
  test,
  type BrowserContext,
  type Page,
  type PlaywrightWorkerArgs,
} from "@playwright/test";
import type { AssistantResponse } from "@/types/assistant";
import { loginStaff } from "./helpers/staff-login";
import {
  API_BASE,
  MARKETING_BUSINESS_SLUG,
  MARKETING_MANAGER_EMAIL,
  resolveBusinessId,
  seededTableCode,
} from "./helpers/journeys";
import { prepareGuestPage } from "./helpers/guest-page";
import { ensureBusinessOpenForJourney } from "./helpers/delivery-test-setup";

const ENABLED = process.env.PLAYWRIGHT_RUN_ASSISTANT_V2_E2E === "1";
const APP_BASE_URL =
  process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3001";
// "Main Room 2" on demo-ai-lounge — the table verify_demo_seed.sql asserts.
const WAITER_TABLE_CODE =
  process.env.PLAYWRIGHT_AI_TABLE_CODE || seededTableCode("ai-pro", 2);
const RESTRICTED_STAFF_EMAIL =
  process.env.PLAYWRIGHT_AI_RESTRICTED_EMAIL || "host@ai-demo.payverge.example";
const UNAVAILABLE_MENU_ITEM_ID = "ffffffff-ffff-4fff-8fff-ffffffffffff";

type SeededGuestTable = {
  business?: { id?: number };
  bundles?: Array<{
    id?: number;
    name?: string;
    is_active?: boolean;
  }>;
};

type SeededBundle = {
  id: number;
  name: string;
};

function assistantResponse(
  responseID: string,
  answer: string,
  overrides: Partial<AssistantResponse> = {},
): AssistantResponse {
  return {
    version: 2,
    response_id: responseID,
    answer: { format: "markdown", content: answer },
    sections: [],
    steps: [],
    actions: [],
    sources: [],
    entities: [],
    follow_ups: [],
    workflow: null,
    notices: [],
    status: "complete",
    ...overrides,
  };
}

function compatibilityProjection(response: AssistantResponse) {
  return {
    answer: response.answer.content,
    steps: response.steps,
    actions: response.actions.flatMap((action) =>
      action.type === "navigate"
        ? [
            {
              label: action.label,
              href: action.target.href,
              kind: "navigate" as const,
            },
          ]
        : [],
    ),
    follow_ups: response.follow_ups.map(({ prompt }) => prompt),
  };
}

async function authenticateStaffContext(
  context: BrowserContext,
  playwright: PlaywrightWorkerArgs["playwright"],
  email: string,
): Promise<void> {
  const request = await playwright.request.newContext({ baseURL: API_BASE });
  try {
    const token = await loginStaff(request, email);
    // Install the short-lived access cookie against the frontend origin
    // explicitly. Copying APIRequestContext cookies leaves WebKit with a
    // host-only cookie that is not sent to the app origin (the trace then
    // shows an empty Cookie header and the dashboard redirects to login).
    await context.addCookies([
      {
        name: "staff_token",
        value: token,
        url: new URL(APP_BASE_URL).origin,
        httpOnly: true,
        secure: new URL(APP_BASE_URL).protocol === "https:",
        sameSite: "Lax",
      },
    ]);
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
  } finally {
    await request.dispose();
  }
}

async function interceptOpsAsk(
  page: Page,
  businessID: number,
  response: AssistantResponse,
): Promise<void> {
  await page.route(
    `**/api/v1/inside/businesses/${businessID}/assistant/ask`,
    async (route) => {
      if (route.request().method() !== "POST") {
        await route.continue();
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          thread: { id: 91, title: "Assistant V2 Playwright" },
          assistant_message: {
            id: 92,
            content: response.answer.content,
          },
          response: compatibilityProjection(response),
          response_v2: response,
          usage: { model: "intercepted", latency_ms: 4 },
          contract_version: "v2",
        }),
      });
    },
  );
}

/**
 * The dashboard hides the Ops Assistant FAB (DashboardLayout `hideFab`); its
 * entry point is "Ask assistant" inside the sidebar Help menu.
 */
async function openOpsAssistant(page: Page) {
  await page.getByRole("button", { name: "Help", exact: true }).click();
  await page.getByRole("menuitem", { name: "Ask assistant" }).click();
}

async function sendInChat(page: Page, dialogName: RegExp, text: string) {
  const dialog = page.getByRole("dialog", { name: dialogName });
  const composer = dialog.getByRole("textbox");
  await composer.fill(text);
  await dialog.getByRole("button", { name: /send/i }).click();
  return dialog;
}

test.describe("Assistant V2 seeded product journeys", () => {
  // Deterministic route interception is the contract of this suite. Playwright
  // cannot intercept requests handled by a service worker, which made WebKit
  // fall through to the real (disabled in CI) AI endpoints.
  test.use({ serviceWorkers: "block" });

  test.skip(
    !ENABLED,
    "Set PLAYWRIGHT_RUN_ASSISTANT_V2_E2E=1 (requires full stack + demo seed)",
  );

  test("Ops permitted action navigates through the authenticated canonical dashboard route", async ({
    page,
    playwright,
  }) => {
    test.setTimeout(90_000);
    const businessID = await resolveBusinessId(MARKETING_BUSINESS_SLUG);
    await authenticateStaffContext(
      page.context(),
      playwright,
      MARKETING_MANAGER_EMAIL,
    );
    const href = `/business/${businessID}/dashboard?tab=menu`;
    const response = assistantResponse(
      "ops-e2e-permitted",
      "Use the verified menu workspace.",
      {
        actions: [
          {
            id: "open-menu",
            type: "navigate",
            label: "Open menu workspace",
            target: {
              kind: "dashboard_area",
              id: "menu",
              href,
            },
            state: "ready",
            confirmation: "none",
            disabled_reason: null,
            expires_at: null,
          },
        ],
      },
    );
    await interceptOpsAsk(page, businessID, response);

    await page.goto(`/business/${businessID}/dashboard?tab=overview`);
    await openOpsAssistant(page);
    const dialog = await sendInChat(
      page,
      /Ops Assistant/i,
      "Where is the menu?",
    );
    const action = dialog.getByRole("button", {
      name: "Open menu workspace",
    });
    await expect(action).toBeVisible();
    await action.click();
    await expect(page).toHaveURL(
      new RegExp(`/business/${businessID}/dashboard\\?tab=menu$`),
    );
    await expect(page.getByText(/permission to access this page/i)).toHaveCount(
      0,
    );
  });

  test("Ops restricted staff receives guidance without a forbidden action", async ({
    page,
    playwright,
  }) => {
    test.setTimeout(90_000);
    const businessID = await resolveBusinessId(MARKETING_BUSINESS_SLUG);
    await authenticateStaffContext(
      page.context(),
      playwright,
      RESTRICTED_STAFF_EMAIL,
    );
    const response = assistantResponse(
      "ops-e2e-restricted",
      "A manager can help with accounting. Your current role can continue using reservations.",
    );
    await interceptOpsAsk(page, businessID, response);

    await page.goto(`/business/${businessID}/dashboard?tab=overview`);
    await openOpsAssistant(page);
    const dialog = await sendInChat(
      page,
      /Ops Assistant/i,
      "Open accounting settings",
    );
    await expect(
      dialog.getByText(/A manager can help with accounting/i),
    ).toBeVisible();
    await expect(
      dialog.getByRole("button", { name: "Open accounting" }),
    ).toHaveCount(0);
    await expect(
      dialog.getByRole("link", { name: "Open accounting" }),
    ).toHaveCount(0);
    await expect(
      page
        .getByRole("link", { name: /^Accounting$/i })
        .or(page.getByRole("button", { name: /^Accounting$/i })),
    ).toHaveCount(0);
  });

  test("AI Waiter carries one seeded cart intent to the menu, rejects stale identity, and renders V1 fallback", async ({
    page,
    request,
  }) => {
    test.setTimeout(120_000);
    await page.emulateMedia({ reducedMotion: "reduce" });
    await prepareGuestPage(page);
    const canonicalTableCode = WAITER_TABLE_CODE.toUpperCase();

    const catalogResponse = await request.get(
      `${API_BASE}/guest/table/${WAITER_TABLE_CODE}`,
    );
    expect(
      catalogResponse.ok(),
      `seeded table ${WAITER_TABLE_CODE} was unavailable: ${catalogResponse.status()}`,
    ).toBeTruthy();
    const seeded = (await catalogResponse.json()) as SeededGuestTable;
    const seededBundle = seeded.bundles?.find(
      (bundle): bundle is SeededBundle =>
        Number.isSafeInteger(bundle.id) &&
        (bundle.id ?? 0) > 0 &&
        typeof bundle.name === "string" &&
        bundle.name.trim().length > 0 &&
        bundle.is_active === true,
    );
    expect(seeded.business?.id).toBeGreaterThan(0);
    expect(
      seededBundle,
      "expected an active bundle in the seeded AI menu",
    ).toBeTruthy();
    const businessID = seeded.business!.id!;
    const bundle = seededBundle!;
    ensureBusinessOpenForJourney(businessID);

    await page.route(
      `**/api/v1/ai-waiter/${businessID}/session`,
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            session_token: "assistant-v2-e2e-token",
            greeting: "Hello",
            expires_at: new Date(Date.now() + 60 * 60 * 1000).toISOString(),
          }),
        });
      },
    );
    await page.route(
      `**/api/v1/ai-waiter/${businessID}/messages**`,
      async (route) => {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: "[]",
        });
      },
    );
    await page.route(
      `**/api/v1/ai-waiter/${businessID}/stream`,
      async (route) => {
        await route.abort("connectionrefused");
      },
    );
    await page.route(`**/api/v1/ai-waiter/${businessID}`, async (route) => {
      if (route.request().method() !== "POST") {
        await route.continue();
        return;
      }
      const body = route.request().postDataJSON() as {
        history?: Array<{ content?: string }>;
      };
      const prompt = body.history?.at(-1)?.content ?? "";

      if (prompt.includes("legacy")) {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            id: 103,
            role: "model",
            parts: [{ text: "Legacy waiter text remains visible." }],
          }),
        });
        return;
      }

      const unavailable = prompt.includes("unavailable");
      const response = assistantResponse(
        unavailable ? "waiter-e2e-unavailable" : "waiter-e2e-seeded-bundle",
        unavailable
          ? "I checked that unavailable item."
          : `I am adding ${bundle.name}.`,
        {
          actions: [
            {
              id: unavailable ? "unavailable-item" : "seeded-bundle",
              type: "add_cart_item",
              label: unavailable
                ? "Add unavailable item"
                : `Add ${bundle.name}`,
              target: {
                kind: unavailable ? "menu_item" : "bundle",
                id: unavailable ? UNAVAILABLE_MENU_ITEM_ID : String(bundle.id),
                href: "",
                quantity: 1,
              },
              state: "ready",
              confirmation: "none",
              disabled_reason: null,
              expires_at: null,
            },
          ],
        },
      );
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          id: unavailable ? 102 : 101,
          role: "model",
          parts: [{ text: response.answer.content }],
          response_v2: response,
        }),
      });
    });

    await page.goto(`/t/${WAITER_TABLE_CODE}`);
    await expect(page).toHaveURL(new RegExp(`/t/${WAITER_TABLE_CODE}$`));
    await page.getByRole("button", { name: "Open AI assistant" }).click();
    await sendInChat(page, /Sofia|AI assistant/i, "add the seeded bundle");
    await expect(page).toHaveURL(new RegExp(`/t/${canonicalTableCode}/menu$`), {
      timeout: 20_000,
    });
    await expect(
      page.getByRole("button", { name: /View cart with 1 items/i }),
    ).toBeVisible({ timeout: 20_000 });

    const continuity = await page.evaluate((tableCode) => {
      const receipt = JSON.parse(
        sessionStorage.getItem(
          `payverge_pending_cart_v1:${tableCode}:consumed`,
        ) || '{"entries":[]}',
      ) as { entries?: unknown[] };
      const cart = JSON.parse(
        localStorage.getItem(`payverge_cart_${tableCode}`) || '{"items":[]}',
      ) as { items?: Array<{ quantity?: number }> };
      return {
        pending: sessionStorage.getItem(
          `payverge_pending_cart_v1:${tableCode}`,
        ),
        binding: sessionStorage.getItem(
          `payverge_pending_cart_session_v1:${tableCode}`,
        ),
        consumedCount: receipt.entries?.length ?? 0,
        quantity:
          cart.items?.reduce((sum, item) => sum + (item.quantity ?? 0), 0) ?? 0,
      };
    }, canonicalTableCode);
    expect(continuity.pending).toBeNull();
    expect(continuity.binding).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/,
    );
    expect(continuity.consumedCount).toBe(1);
    expect(continuity.quantity).toBe(1);

    // The assertion below is the reload contract. Waiting for DOM content is
    // sufficient and avoids Firefox waiting indefinitely for unrelated
    // long-lived guest resources before it reports the page `load` event.
    await page.reload({ waitUntil: "domcontentloaded" });
    await expect(
      page.getByRole("button", { name: /View cart with 1 items/i }),
    ).toBeVisible({ timeout: 20_000 });
    expect(
      await page.evaluate(
        (tableCode) =>
          JSON.parse(
            localStorage.getItem(`payverge_cart_${tableCode}`) ||
              '{"items":[]}',
          ).items.reduce(
            (sum: number, item: { quantity?: number }) =>
              sum + (item.quantity ?? 0),
            0,
          ),
        canonicalTableCode,
      ),
    ).toBe(1);

    await page.goto(`/t/${WAITER_TABLE_CODE}`);
    await page.getByRole("button", { name: "Open AI assistant" }).click();
    await sendInChat(page, /Sofia|AI assistant/i, "add an unavailable item");
    await expect(
      page.getByText("I checked that unavailable item."),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Add unavailable item" }),
    ).toHaveCount(0);
    await expect(page).toHaveURL(new RegExp(`/t/${WAITER_TABLE_CODE}$`));
    expect(
      await page.evaluate(
        (tableCode) =>
          JSON.parse(
            localStorage.getItem(`payverge_cart_${tableCode}`) ||
              '{"items":[]}',
          ).items.reduce(
            (sum: number, item: { quantity?: number }) =>
              sum + (item.quantity ?? 0),
            0,
          ),
        canonicalTableCode,
      ),
    ).toBe(1);

    const waiterDialog = await sendInChat(
      page,
      /Sofia|AI assistant/i,
      "show legacy text",
    );
    await expect(
      waiterDialog.getByText("Legacy waiter text remains visible."),
    ).toBeVisible();
  });
});
