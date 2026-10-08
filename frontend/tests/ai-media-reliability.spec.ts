import { expect, test, type Locator } from "@playwright/test";

const enabled = process.env.PLAYWRIGHT_RUN_PAID_AI_E2E === "1";
const businessId = process.env.PLAYWRIGHT_AI_BUSINESS_ID ?? "";
const businessSlug = process.env.PLAYWRIGHT_AI_BUSINESS_SLUG ?? "";
const tableCode = process.env.PLAYWRIGHT_AI_TABLE_CODE ?? "";
const storageState = process.env.PLAYWRIGHT_AI_STORAGE_STATE;
// Where generated images are served from: same-origin /media/ (local storage,
// the default) unless PLAYWRIGHT_AI_MEDIA_HOST names the S3/CDN host that the
// deployment lists in MEDIA_ORIGINS.
const mediaHost = process.env.PLAYWRIGHT_AI_MEDIA_HOST?.trim() ?? "";

if (storageState) {
  test.use({ storageState });
}

/** Selector for an <img> whose src (raw, or inside /_next/image?url=) contains `part`. */
function imgWithSrc(part: string): string {
  const encoded = encodeURIComponent(part);
  return encoded === part
    ? `img[src*="${part}"]`
    : `img[src*="${part}"], img[src*="${encoded}"]`;
}

/** Path of the media file behind an <img> src, unwrapping the optimizer URL. */
function mediaPath(src: string, base: string): string {
  const url = new URL(src, base);
  const inner = url.pathname === "/_next/image" ? url.searchParams.get("url") : null;
  return new URL(inner ?? url.href, base).pathname;
}

async function expectLoaded(locator: Locator) {
  await expect(locator).toBeVisible();
  await expect
    .poll(() =>
      locator.evaluate((node: HTMLImageElement) => ({
        complete: node.complete,
        width: node.naturalWidth,
        height: node.naturalHeight,
      })),
    )
    .toMatchObject({ complete: true });
  expect(await locator.evaluate((node: HTMLImageElement) => node.naturalWidth)).toBeGreaterThan(0);
  expect(await locator.evaluate((node: HTMLImageElement) => node.naturalHeight)).toBeGreaterThan(0);
}

test.describe("paid AI image delivery", () => {
  test.skip(
    !enabled,
    "Set PLAYWRIGHT_RUN_PAID_AI_E2E=1; this test consumes one image credit",
  );

  test.beforeAll(() => {
    expect(businessId).toMatch(/^\d+$/);
    expect(businessSlug).not.toBe("");
    expect(tableCode).not.toBe("");
    expect(storageState, "Set PLAYWRIGHT_AI_STORAGE_STATE to an authenticated state file").toBeTruthy();
  });

  test("generated dish image renders in editor, table menu, and storefront", async ({ page }) => {
    test.setTimeout(180_000);
    const cspFailures: string[] = [];
    page.on("console", (message) => {
      const text = message.text();
      if (/content security policy|refused to load/i.test(text) && /image/i.test(text)) {
        cspFailures.push(text);
      }
    });

    await page.goto(`/business/${businessId}/dashboard?tab=menu`);
    await page.getByText("QA Image Dish", { exact: true }).click();
    await page.getByRole("button", { name: /generate photo/i }).first().click();
    const editorImage = page
      .getByRole("dialog")
      .locator(imgWithSrc(mediaHost || "/media/"))
      .last();
    await expectLoaded(editorImage);
    const generatedURL = await editorImage.getAttribute("src");
    expect(generatedURL).toBeTruthy();

    await page.getByRole("button", { name: /save/i }).last().click();
    const imagePath = mediaPath(generatedURL!, page.url());
    await page.goto(`/t/${tableCode}/menu`);
    await expectLoaded(page.locator(imgWithSrc(imagePath)).first());
    await page.goto(`/b/${businessSlug}`);
    await expectLoaded(page.locator(imgWithSrc(imagePath)).first());

    expect(cspFailures).toEqual([]);
  });
});
