import { expect, test, type Browser, type Page } from "@playwright/test";

const ROUTES = [
  "/",
  "/privacy-policy",
  "/terms-and-conditions",
  "/refund",
] as const;

const VIEWPORTS = [
  { width: 320, height: 568 },
  { width: 390, height: 844 },
] as const;

async function localeCookie(
  browser: Browser,
  baseURL: string,
  javaScriptEnabled = true,
) {
  const origin = new URL(baseURL);
  const context = await browser.newContext({ javaScriptEnabled });
  await context.addCookies([
    {
      name: "payverge_locale",
      value: "es-AR",
      domain: origin.hostname,
      path: "/",
      sameSite: "Lax",
      secure: origin.protocol === "https:",
    },
  ]);
  return context;
}

function captureFatalBrowserErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(`pageerror: ${error.message}`));
  page.on("console", (message) => {
    if (message.type() === "error") {
      errors.push(`console.error: ${message.text()}`);
    }
  });
  return errors;
}

test.describe("request locale hydration contract", () => {
  for (const viewport of VIEWPORTS) {
    for (const route of ROUTES) {
      test(`${route} stays es-AR without hydration errors at ${viewport.width}px`, async ({
        browser,
        baseURL,
      }) => {
        const appURL = baseURL ?? "http://localhost:3001";
        const target = new URL(route, appURL).toString();

        const ssrContext = await localeCookie(browser, appURL, false);
        const ssrPage = await ssrContext.newPage();
        await ssrPage.setViewportSize(viewport);
        await ssrPage.goto(target, { waitUntil: "domcontentloaded" });
        const ssrLang = await ssrPage.locator("html").getAttribute("lang");
        const ssrHeading = (await ssrPage.locator("h1").first().innerText()).trim();
        await ssrContext.close();

        const hydratedContext = await localeCookie(browser, appURL);
        await hydratedContext.addInitScript(() => {
          // A stale pre-cookie preference must not override request-derived SSR.
          window.localStorage.setItem("locale", "en");
          const snapshots: Array<{ lang: string; heading: string }> = [];
          const capture = () => {
            const next = {
              lang: document.documentElement?.lang ?? "",
              heading: document.querySelector("h1")?.textContent?.trim() ?? "",
            };
            const previous = snapshots[snapshots.length - 1];
            if (
              !previous ||
              previous.lang !== next.lang ||
              previous.heading !== next.heading
            ) {
              snapshots.push(next);
            }
          };
          Object.defineProperty(window, "__legalLocaleSnapshots", {
            value: snapshots,
            configurable: false,
          });
          document.addEventListener("DOMContentLoaded", capture, { once: true });
          new MutationObserver(capture).observe(document, {
            subtree: true,
            childList: true,
            characterData: true,
            attributes: true,
            attributeFilter: ["lang"],
          });
        });
        const page = await hydratedContext.newPage();
        await page.setViewportSize(viewport);
        const browserErrors = captureFatalBrowserErrors(page);
        await page.goto(target, { waitUntil: "networkidle" });

        await expect(page.locator("html")).toHaveAttribute("lang", "es-AR");
        await expect(page.locator("h1").first()).toHaveText(ssrHeading);
        expect(ssrLang).toBe("es-AR");
        expect(browserErrors).toEqual([]);

        const snapshots = await page.evaluate(
          () =>
            (window as typeof window & {
              __legalLocaleSnapshots?: Array<{
                lang: string;
                heading: string;
              }>;
            }).__legalLocaleSnapshots ?? [],
        );
        const observedLanguages = [
          ...new Set(snapshots.map(({ lang }) => lang).filter(Boolean)),
        ];
        const observedHeadings = [
          ...new Set(snapshots.map(({ heading }) => heading).filter(Boolean)),
        ];
        expect(observedLanguages).toEqual(["es-AR"]);
        expect(observedHeadings).toEqual([ssrHeading]);

        const visibleSwitchers = page.getByRole("button", {
          name: /change language|cambiar idioma/i,
        });
        await expect(visibleSwitchers).toHaveCount(1);
        await expect(visibleSwitchers).toBeVisible();

        await hydratedContext.close();
      });
    }
  }

  test("standalone auth locale control remains visible on desktop", async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/forgot-password", { waitUntil: "networkidle" });

    const switcher = page.getByRole("button", {
      name: /change language|cambiar idioma/i,
    });
    await expect(switcher).toHaveCount(1);
    await expect(switcher).toBeVisible();
  });
});
