/**
 * Director Console journeys.
 *
 * The dashboard shell, staff login, RBAC and routing are real: the browser
 * logs in as the seeded AI-demo general manager (director:read). Only the
 * model-facing surface is replaced, so the spec never needs a live LLM:
 *
 *   - the instance probe reports `features.ai = true` (the CI stack has no
 *     provider key, which would otherwise put the console in its "AI off"
 *     state);
 *   - POST .../ai/director/ask/stream is answered by an in-page SSE stream the
 *     test drives frame by frame, so mid-stream UI is asserted
 *     deterministically;
 *   - the thread list and thread messages are fulfilled from fixtures.
 *
 * Run (with the compose stack and demo seed up):
 *   PLAYWRIGHT_RUN_JOURNEYS_E2E=1 npx playwright test director-console \
 *     --project=chromium
 */
import {
  expect,
  test,
  type BrowserContext,
  type Page,
  type PlaywrightWorkerArgs,
} from "@playwright/test";
import { loginStaff } from "./helpers/staff-login";
import {
  API_BASE,
  MARKETING_BUSINESS_SLUG,
  MARKETING_MANAGER_EMAIL,
  journeysEnabled,
  resolveBusinessId,
} from "./helpers/journeys";

const APP_BASE_URL =
  process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3001";
const THREAD_ID = 990001;
const ACTION_TITLE = "Review the Tuesday lunch line-up";

type StreamControl = {
  requests: unknown[];
  push: (event: string, data: unknown) => void;
  close: () => void;
};

declare global {
  interface Window {
    __dcStream?: StreamControl;
  }
}

async function authenticateStaffContext(
  context: BrowserContext,
  playwright: PlaywrightWorkerArgs["playwright"],
  email: string,
): Promise<void> {
  const request = await playwright.request.newContext({ baseURL: API_BASE });
  try {
    const token = await loginStaff(request, email);
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
  } finally {
    await request.dispose();
  }
}

/**
 * Runs before any page script: forces the instance seed to report AI as
 * configured and swaps the director stream fetch for a stream the test feeds
 * through `window.__dcStream`. Aborting the request errors the stream with an
 * AbortError, exactly like a real fetch.
 */
function installDirectorStubs(): void {
  const forceAI = (value: unknown): unknown => {
    if (!value || typeof value !== "object") return value;
    const info = value as { features?: Record<string, unknown> };
    return { ...info, features: { ...(info.features ?? {}), ai: true } };
  };
  let seed: unknown;
  Object.defineProperty(window, "__PAYVERGE_INSTANCE__", {
    configurable: true,
    get: () => seed,
    set: (value: unknown) => {
      seed = forceAI(value);
    },
  });

  const encoder = new TextEncoder();
  let controller: ReadableStreamDefaultController<Uint8Array> | null = null;
  const control: StreamControl = {
    requests: [],
    push: (event, data) => {
      controller?.enqueue(
        encoder.encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`),
      );
    },
    close: () => controller?.close(),
  };
  window.__dcStream = control;

  const realFetch = window.fetch.bind(window);
  window.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === "string"
        ? input
        : input instanceof URL
          ? input.href
          : input.url;
    if (!/\/ai\/director\/ask\/stream$/.test(url.split("?")[0])) {
      return realFetch(input, init);
    }
    control.requests.push(
      typeof init?.body === "string" ? JSON.parse(init.body) : null,
    );
    const body = new ReadableStream<Uint8Array>({
      start(c) {
        controller = c;
        init?.signal?.addEventListener("abort", () => {
          try {
            c.error(new DOMException("The user aborted a request.", "AbortError"));
          } catch {
            // already closed
          }
        });
      },
    });
    return Promise.resolve(
      new Response(body, {
        status: 200,
        headers: { "Content-Type": "text/event-stream" },
      }),
    );
  };
}

async function stubDirectorReads(page: Page, businessId: number): Promise<void> {
  // A client-side instance refetch must agree with the forced seed.
  await page.route("**/api/v1/instance", async (route) => {
    const response = await route.fetch();
    const body = await response.json();
    await route.fulfill({
      response,
      json: { ...body, features: { ...(body.features ?? {}), ai: true } },
    });
  });

  await page.route("**/ai/director/threads**", async (route) => {
    const url = new URL(route.request().url());
    if (route.request().method() !== "GET") {
      await route.fulfill({ status: 204, body: "" });
      return;
    }
    if (url.pathname.endsWith(`/threads/${THREAD_ID}/messages`)) {
      const now = new Date().toISOString();
      await route.fulfill({
        json: {
          messages: [
            {
              id: 1,
              thread_id: THREAD_ID,
              business_id: businessId,
              role: "user",
              locale: "en",
              content: "Top retention actions",
              created_at: now,
            },
            {
              id: 2,
              thread_id: THREAD_ID,
              business_id: businessId,
              role: "assistant",
              locale: "en",
              content: "Tuesday lunch covers dipped after the menu change.",
              structured_response: {
                summary: "Tuesday lunch covers dipped after the menu change.",
                diagnosis: "",
                evidence: [],
                actions: [
                  {
                    title: ACTION_TITLE,
                    description: "Compare the dishes guests ordered before and after.",
                    deep_link: `/business/${businessId}/dashboard?tab=analytics`,
                    priority: "high",
                  },
                ],
                expected_impact: "",
                follow_ups: [],
              },
              created_at: now,
            },
          ],
        },
      });
      return;
    }
    await route.fulfill({ json: { threads: [] } });
  });
}

async function streamFrame(page: Page, event: string, data: unknown): Promise<void> {
  await page.evaluate(
    ([name, payload]) => window.__dcStream?.push(name as string, payload),
    [event, data] as const,
  );
}

test.describe("Director Console", () => {
  test.skip(
    !journeysEnabled,
    "Set PLAYWRIGHT_RUN_JOURNEYS_E2E=1 with the seeded compose stack running.",
  );

  let businessId: number;

  test.beforeAll(async () => {
    businessId = await resolveBusinessId(MARKETING_BUSINESS_SLUG);
  });

  test.beforeEach(async ({ page, context, playwright }) => {
    await authenticateStaffContext(context, playwright, MARKETING_MANAGER_EMAIL);
    await context.addInitScript(installDirectorStubs);
    await stubDirectorReads(page, businessId);
    await page.goto(`/business/${businessId}/dashboard?tab=director-console`);
    await expect(page.getByTestId("dc-composer")).toBeVisible({ timeout: 30_000 });
  });

  function composer(page: Page) {
    return page.getByTestId("dc-composer").getByRole("textbox");
  }

  async function send(page: Page, question: string): Promise<void> {
    await composer(page).fill(question);
    await composer(page).press("ControlOrMeta+Enter");
    await expect
      .poll(() => page.evaluate(() => window.__dcStream?.requests.length ?? 0))
      .toBe(1);
  }

  test("tool-trace pills render mid-stream and the composer refocuses", async ({
    page,
  }) => {
    await send(page, "Why was Tuesday slow?");
    const placeholder = page.getByTestId("dc-assistant-placeholder");
    await expect(page.getByTestId("dc-pending-user")).toHaveText(
      "Why was Tuesday slow?",
    );
    await expect(placeholder).toBeVisible();

    await streamFrame(page, "tool.call.started", {
      name: "sales_by_day",
      human_label: "Reading daily covers",
      args: {},
    });
    await expect(placeholder.getByText("Reading daily covers")).toBeVisible();

    await streamFrame(page, "tool.call.completed", {
      name: "sales_by_day",
      summary: "7 days",
      duration_ms: 12,
      success: true,
    });
    await expect(placeholder.getByText("· 7 days")).toBeVisible();

    await streamFrame(page, "response.complete", {
      message_id: 2,
      thread_id: THREAD_ID,
      latency_ms: 40,
      model: "stub",
      proposed_actions: [],
    });
    await page.evaluate(() => window.__dcStream?.close());

    await expect(placeholder).toHaveCount(0);
    await expect(
      page.getByText("Tuesday lunch covers dipped after the menu change.").first(),
    ).toBeVisible();
    await expect(composer(page)).toBeFocused();
  });

  test("stop aborts the in-flight stream", async ({ page }) => {
    await send(page, "Plan a 7-day AOV growth campaign.");
    const placeholder = page.getByTestId("dc-assistant-placeholder");
    await expect(placeholder).toBeVisible();

    // While a stream is in flight the composer's only button is Stop.
    await page.getByTestId("dc-composer").getByRole("button").click();
    await expect(placeholder).toHaveCount(0);
    await expect(composer(page)).toBeFocused();
  });

  test("action card deep link routes to the analytics tab", async ({ page }) => {
    await send(page, "Top retention actions");
    await streamFrame(page, "response.complete", {
      message_id: 2,
      thread_id: THREAD_ID,
      latency_ms: 40,
      model: "stub",
      proposed_actions: [],
    });
    await page.evaluate(() => window.__dcStream?.close());

    const action = page.getByRole("button", { name: new RegExp(ACTION_TITLE) });
    await expect(action).toBeVisible();
    await action.click();
    await expect(page).toHaveURL(
      new RegExp(`/business/${businessId}/dashboard\\?tab=analytics`),
    );
  });
});
