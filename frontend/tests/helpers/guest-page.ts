import type { Page } from "@playwright/test";

/** Record an essential-only consent choice before the app hydrates. */
export async function prepareGuestPage(page: Page): Promise<void> {
  await page.addInitScript(() => {
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
}
