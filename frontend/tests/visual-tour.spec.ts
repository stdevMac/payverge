import { test } from "@playwright/test";

const BASE = "http://localhost:3000";
const TABLE_CODE = "AI-T01";
const OUT_DIR = "/tmp/payverge-screenshots";

const MOBILE = { width: 390, height: 844 };
const DESKTOP = { width: 1280, height: 900 };

const ROUTES = [
  { name: "01-landing", path: `/t/${TABLE_CODE}` },
  { name: "02-menu", path: `/t/${TABLE_CODE}/menu` },
  { name: "03-bill", path: `/t/${TABLE_CODE}/bill` },
];

async function capture(page: any, base: string, route: { name: string; path: string }, label: string) {
  const errors: string[] = [];
  page.on("pageerror", (e: Error) => errors.push(`pageerror: ${e.message}`));
  page.on("console", (msg: any) => {
    if (msg.type() === "error") errors.push(`console.error: ${msg.text()}`);
  });

  await page.goto(`${base}${route.path}`, { waitUntil: "networkidle", timeout: 30000 });
  await page.waitForTimeout(1500);

  // Viewport-only — what users actually see at top-of-fold
  await page.screenshot({ path: `${OUT_DIR}/${label}-${route.name}-fold.png`, fullPage: false });
  // Full document — for "scroll the whole thing" review
  await page.screenshot({ path: `${OUT_DIR}/${label}-${route.name}-full.png`, fullPage: true });

  if (errors.length > 0) {
    console.log(`Errors on ${label} ${route.name}:`);
    errors.slice(0, 5).forEach((e) => console.log(`  ${e}`));
  }
}

test.describe("/t/[tableCode] visual tour", () => {
  for (const route of ROUTES) {
    test(`mobile ${route.name}`, async ({ page }) => {
      await page.setViewportSize(MOBILE);
      await capture(page, BASE, route, "mobile");
    });
    test(`desktop ${route.name}`, async ({ page }) => {
      await page.setViewportSize(DESKTOP);
      await capture(page, BASE, route, "desktop");
    });
  }
});
