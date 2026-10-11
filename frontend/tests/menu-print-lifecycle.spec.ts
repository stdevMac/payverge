import { expect, test } from "@playwright/test";

import { installMenuPrintWindowStub } from "./helpers/print-dialog-fixture";
import {
  listOwnerBusinesses,
  loginOwnerApi,
  seedOwnerSession,
} from "./qa-pipeline/owner-session";

const enabled = process.env.PLAYWRIGHT_RUN_MENU_PRINT_E2E === "1";
const storageState = process.env.PLAYWRIGHT_MENU_PRINT_STORAGE_STATE;
const businessOverride = process.env.PLAYWRIGHT_MENU_PRINT_BUSINESS_ID;

if (storageState) test.use({ storageState });

test.describe("live menu print window lifecycle", () => {
  test.skip(
    !enabled,
    "Set PLAYWRIGHT_RUN_MENU_PRINT_E2E=1 with an authenticated local stack.",
  );

  test("cancel, immediate repeat, and close/reopen leave the menu interactive", async ({
    context,
    page,
  }) => {
    // The first operator-route compile in local Next dev can take well over a
    // minute. Keep the live-only lifecycle gate patient enough to test print
    // behavior instead of failing while the dashboard chunks are still warmup.
    test.setTimeout(180_000);
    let businessRoutes = businessOverride ? [businessOverride] : [];
    let ownerSession: Awaited<ReturnType<typeof loginOwnerApi>> | undefined;
    if (!storageState) {
      ownerSession = await loginOwnerApi();
      await seedOwnerSession(context, ownerSession.cookies);
    }
    if (!businessRoutes.length) {
      ownerSession ??= await loginOwnerApi();
      const businesses = await listOwnerBusinesses(
        ownerSession.api,
        ownerSession.token,
      );
      const preferred = businesses[0];
      expect(
        preferred,
        "The sanitized local owner must have at least one business",
      ).toBeTruthy();
      businessRoutes = [preferred!.custom_url, String(preferred!.id)].filter(
        (route, index, routes): route is string =>
          Boolean(route) && routes.indexOf(route) === index,
      );
    }

    const printCalls = await installMenuPrintWindowStub(page, "cancel");
    const openStudio = page.getByRole("button", {
      name: /design and print this menu|print menu/i,
    });
    for (const businessRoute of businessRoutes) {
      await page
        .goto(`/business/${businessRoute}/dashboard?tab=menu`, {
          waitUntil: "domcontentloaded",
          timeout: 20_000,
        })
        .catch(() => null);
      if (await openStudio.isVisible({ timeout: 4_000 }).catch(() => false))
        break;
    }
    await expect(openStudio).toBeVisible({ timeout: 75_000 });
    const cookieChoice = page.getByRole("button", {
      name: /decline non-essential/i,
    });
    if (await cookieChoice.isVisible().catch(() => false)) {
      await cookieChoice.click();
    }
    await openStudio.click();

    const studio = page.getByTestId("print-workflow-shell");
    await expect(studio).toBeVisible();
    for (let step = 0; step < 4; step += 1) {
      await page.getByRole("button", { name: /^next$/i }).click();
    }

    const printButton = page.getByRole("button", { name: /^print menu$/i });
    await expect(printButton).toBeEnabled();
    const parentCsp = await page.evaluate(() => {
      const noncedElement = Array.from(
        document.querySelectorAll<HTMLScriptElement | HTMLStyleElement>(
          "script, style",
        ),
      ).find((element) => element.nonce);

      return {
        origin: window.location.origin,
        nonce: noncedElement?.nonce ?? null,
      };
    });
    expect(
      parentCsp.nonce,
      "the dashboard must expose its active CSP nonce",
    ).toBeTruthy();
    const popupPromise = page.waitForEvent("popup");
    await printButton.click();
    const popup = await popupPromise;
    const popupCsp = await popup.evaluate(() => {
      const lifecycle = document.querySelector<HTMLScriptElement>(
        "#payverge-print-lifecycle",
      );
      return {
        origin: window.location.origin,
        nonce: lifecycle?.nonce ?? null,
      };
    });
    expect(popupCsp.origin).toBe(parentCsp.origin);
    expect(popupCsp.nonce).toBe(parentCsp.nonce);
    await expect.poll(printCalls, { timeout: 60_000 }).toBe(1);
    await expect(printButton).toBeEnabled();

    await printButton.click();
    await expect.poll(printCalls, { timeout: 60_000 }).toBe(2);
    await expect(page.getByTestId("print-stage-panel")).toBeVisible();

    await expect
      .poll(() => context.pages().length, {
        message: "the completed dedicated print window should close",
        timeout: 30_000,
      })
      .toBe(1);
    await printButton.click();
    await expect.poll(printCalls, { timeout: 60_000 }).toBe(3);
    await expect(printButton).toBeEnabled();

    await page
      .getByRole("button", { name: /^close$/i })
      .filter({ hasText: /^close$/i })
      .click();
    await expect(studio).toBeHidden();
    await expect(openStudio).toBeEnabled();

    await openStudio.click();
    await expect(studio).toBeVisible();
    await expect(
      page.getByRole("heading", { name: /choose a look/i }),
    ).toBeVisible();
    await page.getByRole("button", { name: /^next$/i }).click();
    await expect(
      page.getByRole("heading", { name: /shape the menu/i }),
    ).toBeVisible();
    await ownerSession?.api.dispose();
  });
});
