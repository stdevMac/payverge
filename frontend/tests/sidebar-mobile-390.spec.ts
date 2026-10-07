import { test, expect, type BrowserContext } from "@playwright/test";

import { API_BASE, MANAGER_EMAIL, resolveBusinessId } from "./helpers/journeys";
import { loginStaff } from "./helpers/staff-login";

const MOBILE = { width: 390, height: 844 };
const STAFF_EMAIL = process.env.PLAYWRIGHT_OPERATOR_EMAIL || MANAGER_EMAIL;

let businessId = 0;

const TABS = [
  "dashboard",
  "bills",
  "accounting",
  "fiscal",
  "kitchen",
  "menu",
  "tables",
  "counter",
  "staff",
  "inventory",
  "plugins",
  "settings",
];

test.describe.serial("sidebar tabs at 390px", () => {
  let context: BrowserContext;

  test.beforeAll(async ({ browser, playwright }) => {
    context = await browser.newContext({ viewport: MOBILE });
    businessId = process.env.PLAYWRIGHT_BUSINESS_ID
      ? Number(process.env.PLAYWRIGHT_BUSINESS_ID)
      : await resolveBusinessId();
    const apiRequest = await playwright.request.newContext({
      baseURL: API_BASE,
    });
    try {
      await loginStaff(apiRequest, STAFF_EMAIL);
      const state = await apiRequest.storageState();
      await context.addCookies(state.cookies);
    } finally {
      await apiRequest.dispose();
    }
  });

  test.afterAll(async () => {
    await context.close();
  });

  for (const tab of TABS) {
    test(`${tab} — no horizontal overflow, narrow media query active`, async () => {
      const page = await context.newPage();
      try {
        await page.goto(`/business/${businessId}/dashboard?tab=${tab}`);
        await page.waitForLoadState("networkidle", { timeout: 15_000 });

        const mqState = await page.evaluate(() => ({
          innerWidth: window.innerWidth,
          mqMobile: window.matchMedia("(max-width: 640px)").matches,
          mqDesktop: window.matchMedia("(min-width: 1024px)").matches,
        }));
        expect(mqState.innerWidth).toBe(390);
        expect(mqState.mqMobile).toBe(true);
        expect(mqState.mqDesktop).toBe(false);

        const overflow = await page.evaluate(() => {
          const root = document.scrollingElement || document.documentElement;
          return {
            scrollWidth: root.scrollWidth,
            clientWidth: root.clientWidth,
          };
        });
        expect(overflow.scrollWidth).toBeLessThanOrEqual(overflow.clientWidth + 1);

        const consoleErrors: string[] = [];
        page.on("console", (msg) => {
          if (msg.type() === "error") consoleErrors.push(msg.text());
        });
        await page.waitForTimeout(500);
        const fatal = consoleErrors.filter(
          (e) =>
            !/Failed to load resource/.test(e) &&
            !/favicon/i.test(e) &&
            !/non-passive event listener/.test(e),
        );
        expect(fatal, `console errors on ${tab}: ${fatal.join("\n")}`).toEqual([]);
      } finally {
        await page.close();
      }
    });
  }
});
