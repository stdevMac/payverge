/**
 * Stream A — live deep-link aria-current on the owner dashboard AI tabs.
 * Asserts exactly one sidebar [aria-current="page"] per AI tab deep-link.
 */
import { test, expect, type Page } from "@playwright/test";
import {
  loginOwnerApi,
  seedOwnerSession,
  listOwnerBusinesses,
} from "./owner-session";

const RUN = process.env.PLAYWRIGHT_RUN_QA_PIPELINE === "1";

async function assertSingleSidebarCurrent(page: Page, nameRe: RegExp) {
  const current = page.locator('button[aria-current="page"]');
  await expect(current).toHaveCount(1, { timeout: 20_000 });
  await expect(current).toContainText(nameRe);
}

test.describe("sidebar active tab deep-links (AI tabs)", () => {
  test.skip(!RUN, "set PLAYWRIGHT_RUN_QA_PIPELINE=1 against a local stack");

  test("AI tab deep-links set exactly one aria-current", async ({
    page,
    context,
  }) => {
    const { api, token, cookies } = await loginOwnerApi();
    try {
      await seedOwnerSession(context, cookies);

      const businesses = await listOwnerBusinesses(api, token);
      const business = businesses[0];
      test.skip(!business, "no owner business on this stack");

      const cases: Array<{ tab: string; name: RegExp }> = [
        { tab: "ai-waiter", name: /AI Waiter|Mozo/i },
        { tab: "director-console", name: /Director/i },
        { tab: "marketing", name: /Marketing/i },
      ];

      for (const { tab, name } of cases) {
        const url = `/business/${business!.id}/dashboard?tab=${tab}`;
        await page.goto(url, { waitUntil: "domcontentloaded" });
        await expect(page).toHaveURL(
          new RegExp(`/business/${business!.id}/dashboard`),
        );
        await expect(page).toHaveURL(new RegExp(`[?&]tab=${tab}`));
        expect(page.url()).not.toMatch(/\/admin\b/);
        await assertSingleSidebarCurrent(page, name);
      }
    } finally {
      await api.dispose();
    }
  });
});
