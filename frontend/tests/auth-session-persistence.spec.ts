import { createHash } from "node:crypto";
import {
  expect,
  test,
  type APIRequestContext,
  type BrowserContext,
  type Page,
  type Request,
  type Response,
  type Route,
  type TestInfo,
} from "@playwright/test";
import { API_BASE, MANAGER_EMAIL } from "./helpers/journeys";
import { loginStaff } from "./helpers/staff-login";

const AUTH_E2E_ENABLED = process.env.PLAYWRIGHT_RUN_AUTH_E2E === "1";
const SESSION_HINT_KEY = "payverge_had_session";
const ACCESS_COOKIE_NAMES = /^(session_token|staff_token|customer_token)$/;
const INITIAL_PROBE_TIMEOUT_MS = 10_000;
const SESSION_INFO_ENDPOINT = apiEndpoint("auth/session-info");
const REFRESH_ENDPOINT = apiEndpoint("auth/refresh");
const STAFF_LOGOUT_ENDPOINT = apiEndpoint("staff/logout");

// Auth traces may retain cookie request/response headers. This spec exercises
// real refresh credentials, so never record a trace even when the global
// Playwright configuration requests one on failure.
test.use({ trace: "off" });

interface StaffSessionInfo {
  authenticated?: boolean;
  type?: string;
  business_id?: number;
  business_name?: string;
}

interface SessionProbeBarrier {
  dispose: () => Promise<void>;
  handler: (route: Route) => Promise<void>;
  pattern: string;
  waitUntilReleased: () => Promise<void>;
}

type ReleaseRoute = (route: Route) => Promise<void>;

function apiEndpoint(path: string): URL {
  const normalizedBase = API_BASE.replace(/\/+$/, "");
  const normalizedPath = path.replace(/^\/+/, "");
  return new URL(`${normalizedBase}/${normalizedPath}`);
}

function isExactEndpoint(url: string, endpoint: URL): boolean {
  const candidate = new URL(url);
  return (
    candidate.origin === endpoint.origin &&
    candidate.pathname === endpoint.pathname
  );
}

function requireCookieValue(
  cookies: Array<{ name: string; value: string }>,
  name: string,
): string {
  const value = cookies.find((cookie) => cookie.name === name)?.value;
  if (!value) throw new Error(`expected non-empty ${name} cookie`);
  return value;
}

function credentialDigest(value: string): string {
  return createHash("sha256").update(value).digest("hex");
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

test("fixture context resolves the configured app base URL", async ({
  page,
  baseURL,
}) => {
  if (!baseURL) throw new Error("Playwright app baseURL is required");
  await page.route("**/*", (route) =>
    route.fulfill({
      contentType: "text/html",
      body: "<main>base URL probe</main>",
    }),
  );
  const relativePath = "/business/1/dashboard?tab=overview";

  await page.goto(relativePath);

  expect(page.url()).toBe(new URL(relativePath, baseURL).toString());
});

test("API endpoint matching rejects lookalike origins and paths", () => {
  const lookalikeOrigin = new URL(SESSION_INFO_ENDPOINT.href);
  lookalikeOrigin.hostname = "lookalike.invalid";
  const lookalikePath = new URL(SESSION_INFO_ENDPOINT.href);
  lookalikePath.pathname = `${lookalikePath.pathname}/extra`;

  expect(
    isExactEndpoint(SESSION_INFO_ENDPOINT.href, SESSION_INFO_ENDPOINT),
  ).toBe(true);
  expect(isExactEndpoint(lookalikeOrigin.href, SESSION_INFO_ENDPOINT)).toBe(
    false,
  );
  expect(isExactEndpoint(lookalikePath.href, SESSION_INFO_ENDPOINT)).toBe(
    false,
  );
});

async function isAuthenticatedStaffSession(
  response: Response,
): Promise<boolean> {
  if (
    response.request().method() !== "GET" ||
    response.status() !== 200 ||
    !isExactEndpoint(response.url(), SESSION_INFO_ENDPOINT)
  ) {
    return false;
  }

  try {
    const body = (await response.json()) as StaffSessionInfo;
    return body.authenticated === true && body.type === "staff";
  } catch {
    return false;
  }
}

function routePage(route: Route): Page | undefined {
  try {
    return route.request().frame().page();
  } catch {
    return undefined;
  }
}

/**
 * Hold initial session-info probes until every expected Page realm has reached
 * the server. Duplicate probes from one page stay queued but do not satisfy
 * another page's slot. All queued routes are released on success, timeout, or
 * teardown so a failed test cannot leave handlers pending.
 */
function createInitialSessionProbeBarrier(
  expectedPages: readonly Page[],
  options: {
    releaseRoute?: ReleaseRoute;
    timeoutMs?: number;
  } = {},
): SessionProbeBarrier {
  const expectedPageSet = new Set(expectedPages);
  if (
    expectedPageSet.size !== expectedPages.length ||
    expectedPages.length < 2
  ) {
    throw new Error(
      "session probe barrier requires at least two distinct pages",
    );
  }

  const releaseRoute =
    options.releaseRoute ?? ((route: Route) => route.continue());
  const timeoutMs = options.timeoutMs ?? INITIAL_PROBE_TIMEOUT_MS;
  const queuedRoutes: Route[] = [];
  const seenPages = new Set<Page>();
  let failure: Error | undefined;
  let finished = false;
  let settlePromise: Promise<void> | undefined;
  let resolveFinished!: () => void;
  const finishedPromise = new Promise<void>((resolve) => {
    resolveFinished = resolve;
  });

  const releaseQueuedRoutes = async (): Promise<Error | undefined> => {
    const results = await Promise.allSettled(
      queuedRoutes.splice(0).map((route) => releaseRoute(route)),
    );
    return results.some((result) => result.status === "rejected")
      ? new Error("failed to release one or more session-info probes")
      : undefined;
  };

  let timeout: ReturnType<typeof setTimeout>;
  const settle = (settleFailure?: Error): Promise<void> => {
    if (settlePromise) return settlePromise;
    // Fence new arrivals before draining the queue. A request that arrives
    // during the asynchronous drain takes the direct-release branch instead
    // of being appended behind the splice and stranded.
    finished = true;
    settlePromise = (async () => {
      clearTimeout(timeout);
      const releaseFailure = await releaseQueuedRoutes();
      failure = settleFailure ?? releaseFailure;
      resolveFinished();
    })();
    return settlePromise;
  };

  timeout = setTimeout(() => {
    void settle(
      new Error(
        `timed out waiting for initial session-info probes (${seenPages.size}/${expectedPageSet.size} pages)`,
      ),
    );
  }, timeoutMs);

  return {
    pattern: SESSION_INFO_ENDPOINT.href,
    handler: async (route: Route): Promise<void> => {
      if (finished) {
        await releaseRoute(route);
        return;
      }

      const page = routePage(route);
      if (!page || !expectedPageSet.has(page)) {
        await releaseRoute(route);
        return;
      }

      queuedRoutes.push(route);
      seenPages.add(page);
      if (seenPages.size === expectedPageSet.size) {
        await settle();
      }
      await finishedPromise;
    },
    waitUntilReleased: async (): Promise<void> => {
      await finishedPromise;
      if (failure) throw failure;
    },
    dispose: async (): Promise<void> => {
      await settle();
    },
  };
}

function fakeSessionProbeRoute(
  page: Page,
  label: string,
  released: string[],
): Route {
  return {
    request: () => ({
      frame: () => ({ page: () => page }),
    }),
    continue: async () => {
      released.push(label);
    },
  } as unknown as Route;
}

test("session probe barrier waits for one request from each page", async () => {
  const firstPage = {} as Page;
  const secondPage = {} as Page;
  const released: string[] = [];
  const barrier = createInitialSessionProbeBarrier([firstPage, secondPage], {
    timeoutMs: 1_000,
  });

  try {
    const firstProbe = barrier.handler(
      fakeSessionProbeRoute(firstPage, "first", released),
    );
    const duplicateProbe = barrier.handler(
      fakeSessionProbeRoute(firstPage, "duplicate", released),
    );
    await Promise.resolve();

    expect(released).toHaveLength(0);

    const secondProbe = barrier.handler(
      fakeSessionProbeRoute(secondPage, "second", released),
    );
    await Promise.all([
      firstProbe,
      duplicateProbe,
      secondProbe,
      barrier.waitUntilReleased(),
    ]);
    expect(released.sort()).toEqual(["duplicate", "first", "second"]);
  } finally {
    await barrier.dispose();
  }
});

test("session probe barrier times out with a bounded page-count error", async () => {
  const firstPage = {} as Page;
  const secondPage = {} as Page;
  const released: string[] = [];
  const barrier = createInitialSessionProbeBarrier([firstPage, secondPage], {
    timeoutMs: 20,
  });

  try {
    const firstProbe = barrier.handler(
      fakeSessionProbeRoute(firstPage, "first", released),
    );
    await expect(barrier.waitUntilReleased()).rejects.toThrow(
      "timed out waiting for initial session-info probes (1/2 pages)",
    );
    await firstProbe;
    expect(released).toEqual(["first"]);
  } finally {
    await barrier.dispose();
  }
});

test("session probe barrier releases requests that arrive while settling", async () => {
  const firstPage = {} as Page;
  const secondPage = {} as Page;
  const released: string[] = [];
  let allowRelease!: () => void;
  const releaseAllowed = new Promise<void>((resolve) => {
    allowRelease = resolve;
  });
  const barrier = createInitialSessionProbeBarrier([firstPage, secondPage], {
    timeoutMs: 1_000,
    releaseRoute: async (route) => {
      await releaseAllowed;
      await route.continue();
    },
  });

  try {
    const firstProbe = barrier.handler(
      fakeSessionProbeRoute(firstPage, "first", released),
    );
    const secondProbe = barrier.handler(
      fakeSessionProbeRoute(secondPage, "second", released),
    );
    await Promise.resolve();
    const lateProbe = barrier.handler(
      fakeSessionProbeRoute(firstPage, "late", released),
    );

    allowRelease();
    await Promise.all([
      firstProbe,
      secondProbe,
      lateProbe,
      barrier.waitUntilReleased(),
    ]);
    expect(released.sort()).toEqual(["first", "late", "second"]);
  } finally {
    allowRelease();
    await barrier.dispose();
  }
});

async function logoutStaffSession(
  context: BrowserContext,
  bootstrapRequest: APIRequestContext,
  appOrigin: string,
): Promise<void> {
  const statuses: string[] = [];
  const attempts: Array<{
    label: string;
    run: () => Promise<{ ok(): boolean; status(): number }>;
  }> = [
    {
      label: "browser",
      run: () =>
        context.request.post(STAFF_LOGOUT_ENDPOINT.href, {
          headers: { Origin: appOrigin },
        }),
    },
    {
      label: "bootstrap",
      run: () =>
        bootstrapRequest.post(STAFF_LOGOUT_ENDPOINT.href, {
          headers: { Origin: appOrigin },
        }),
    },
  ];

  for (const attempt of attempts) {
    try {
      const response = await attempt.run();
      statuses.push(`${attempt.label}=${response.status()}`);
      if (response.ok()) return;
    } catch {
      statuses.push(`${attempt.label}=network_error`);
    }
  }

  throw new Error(`staff logout cleanup failed (${statuses.join(", ")})`);
}

function recordCleanupFailure(
  testInfo: TestInfo,
  primaryFailure: unknown,
  cleanupFailure: unknown,
): void {
  const message =
    cleanupFailure instanceof Error
      ? cleanupFailure.message
      : "unknown staff logout cleanup failure";
  if (primaryFailure) {
    testInfo.annotations.push({ type: "cleanup-error", description: message });
    return;
  }
  throw cleanupFailure;
}

test.describe("auth session persistence across tabs", () => {
  test.skip(
    !AUTH_E2E_ENABLED,
    "Set PLAYWRIGHT_RUN_AUTH_E2E=1 (requires a running seeded stack)",
  );

  test("one refresh restores two independently loaded protected pages", async ({
    context,
    baseURL,
    playwright,
  }, testInfo) => {
    test.setTimeout(60_000);
    if (!baseURL) throw new Error("Playwright app baseURL is required");

    const appOrigin = new URL(baseURL).origin;
    const apiRequest = await playwright.request.newContext({
      baseURL: API_BASE,
    });
    let primaryFailure: unknown;
    let sessionCreated = false;

    try {
      await loginStaff(apiRequest, MANAGER_EMAIL);
      sessionCreated = true;

      const bootstrapSessionResponse = await apiRequest.get(
        SESSION_INFO_ENDPOINT.href,
      );
      expect(
        bootstrapSessionResponse.ok(),
        `authenticated session preflight failed: ${bootstrapSessionResponse.status()}`,
      ).toBeTruthy();
      const bootstrapSession =
        (await bootstrapSessionResponse.json()) as StaffSessionInfo;
      expect(bootstrapSession).toMatchObject({
        authenticated: true,
        type: "staff",
      });
      expect(bootstrapSession.business_id).toBeGreaterThan(0);
      expect(Boolean(bootstrapSession.business_name)).toBe(true);
      const businessId = bootstrapSession.business_id!;
      const businessName = bootstrapSession.business_name!;

      const authenticatedState = await apiRequest.storageState();
      const initialRefreshDigest = credentialDigest(
        requireCookieValue(authenticatedState.cookies, "refresh_token"),
      );

      // The fixture-owned context preserves the active project's desktop or
      // mobile device settings while both pages share cookies/localStorage.
      await context.addCookies(authenticatedState.cookies);
      await context.clearCookies({ name: ACCESS_COOKIE_NAMES });
      await context.addInitScript((sessionHintKey) => {
        window.localStorage.setItem(sessionHintKey, "1");
      }, SESSION_HINT_KEY);

      const cookiesAfterAccessExpiry = await context.cookies();
      const retainedRefreshDigest = credentialDigest(
        requireCookieValue(cookiesAfterAccessExpiry, "refresh_token"),
      );
      expect(retainedRefreshDigest).toBe(initialRefreshDigest);
      expect(
        cookiesAfterAccessExpiry.filter((cookie) =>
          ACCESS_COOKIE_NAMES.test(cookie.name),
        ),
      ).toHaveLength(0);

      const pages = await Promise.all([context.newPage(), context.newPage()]);
      const sessionProbeBarrier = createInitialSessionProbeBarrier(pages);
      let refreshRequestCount = 0;
      const onRequest = (request: Request): void => {
        if (
          request.method() === "POST" &&
          isExactEndpoint(request.url(), REFRESH_ENDPOINT)
        ) {
          refreshRequestCount += 1;
        }
      };

      // Register context/page listeners before either protected navigation.
      context.on("request", onRequest);
      await context.route(
        sessionProbeBarrier.pattern,
        sessionProbeBarrier.handler,
      );

      try {
        const authenticatedSessions = pages.map((page) =>
          page.waitForResponse(isAuthenticatedStaffSession),
        );
        const protectedPath = `/business/${businessId}/dashboard?tab=overview`;
        const protectedURL = new URL(protectedPath, baseURL).href;

        await Promise.all([
          ...pages.map((page) =>
            page.goto(protectedPath, { waitUntil: "domcontentloaded" }),
          ),
          sessionProbeBarrier.waitUntilReleased(),
        ]);

        const recoveredSessions = await Promise.all(authenticatedSessions);
        for (const [index, response] of recoveredSessions.entries()) {
          const session = (await response.json()) as StaffSessionInfo;
          expect(session).toMatchObject({
            authenticated: true,
            type: "staff",
            business_id: businessId,
          });
          await expect(pages[index]).toHaveURL(protectedURL);
          await expect(
            pages[index].getByRole("heading", {
              level: 1,
              name: new RegExp(escapeRegExp(businessName), "i"),
            }),
          ).toBeVisible();
          await expect(pages[index].getByLabel("Loading session")).toHaveCount(
            0,
          );
          expect(
            await pages[index].evaluate(
              (sessionHintKey) => window.localStorage.getItem(sessionHintKey),
              SESSION_HINT_KEY,
            ),
          ).toBe("1");
        }

        expect(refreshRequestCount).toBe(1);
        const rotatedRefreshDigest = credentialDigest(
          requireCookieValue(await context.cookies(), "refresh_token"),
        );
        expect(rotatedRefreshDigest).not.toBe(initialRefreshDigest);
      } finally {
        await sessionProbeBarrier.dispose();
        context.off("request", onRequest);
        await context.unroute(
          sessionProbeBarrier.pattern,
          sessionProbeBarrier.handler,
        );
      }
    } catch (error) {
      primaryFailure = error;
      throw error;
    } finally {
      let cleanupFailure: unknown;
      if (sessionCreated) {
        try {
          await logoutStaffSession(context, apiRequest, appOrigin);
        } catch (error) {
          cleanupFailure = error;
        }
      }
      try {
        await apiRequest.dispose();
      } catch (error) {
        cleanupFailure ??= error;
      }
      if (cleanupFailure) {
        recordCleanupFailure(testInfo, primaryFailure, cleanupFailure);
      }
    }
  });
});
