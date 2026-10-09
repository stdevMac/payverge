import { test, expect } from "@playwright/test";

const BASE = "http://localhost:3000";
const TABLE_CODE = "AI-T01";
const OUT_DIR = "/tmp/payverge-screenshots";
const MOBILE = { width: 390, height: 844 };

// Seed the menu cart via localStorage before navigating, so the FAB cart pill
// renders and we can measure whether it occludes the last menu content.
const seededCart = JSON.stringify({
  items: [
    {
      itemType: "menu_item",
      name: "Truffle Labneh Tart",
      price: 56,
      quantity: 2,
      menuItemId: "demo-truffle-labneh",
    },
  ],
  timestamp: Date.now(),
  tableCode: TABLE_CODE,
});

test("menu — FAB cart vs last content at scroll bottom", async ({ page }) => {
  await page.setViewportSize(MOBILE);

  // Pre-seed localStorage on the same origin before the menu page hydrates.
  await page.goto(`${BASE}/t/${TABLE_CODE}`, { waitUntil: "domcontentloaded" });
  await page.evaluate(
    ([key, value]) => {
      localStorage.setItem(key, value);
    },
    [`payverge_cart_${TABLE_CODE}`, seededCart],
  );

  await page.goto(`${BASE}/t/${TABLE_CODE}/menu`, {
    waitUntil: "networkidle",
    timeout: 30000,
  });
  await page.waitForTimeout(1500);

  // Scroll to the absolute bottom
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  await page.waitForTimeout(500);

  await page.screenshot({
    path: `${OUT_DIR}/scroll-bottom-menu-with-fab.png`,
    fullPage: false,
  });

  const measurements = await page.evaluate(() => {
    const nav = document.querySelector(".fixed.bottom-0") as HTMLElement | null;
    const fab = Array.from(
      document.querySelectorAll("button.fixed.left-1\\/2"),
    )[0] as HTMLElement | undefined;

    const navRect = nav?.getBoundingClientRect();
    const fabRect = fab?.getBoundingClientRect();

    // Find the last leaf content visible on screen
    const SELECTOR = "p, h1, h2, h3, button, a, span, [class*='font-mono']";
    const all = Array.from(document.querySelectorAll(SELECTOR)) as HTMLElement[];
    const candidates = all
      .filter((el) => {
        const r = el.getBoundingClientRect();
        const cs = getComputedStyle(el);
        if (nav && (el === nav || nav.contains(el))) return false;
        if (fab && (el === fab || fab.contains(el))) return false;
        if (cs.display === "none" || cs.visibility === "hidden") return false;
        if (r.width === 0 || r.height === 0) return false;
        return r.top < window.innerHeight && r.bottom > 0;
      })
      .sort(
        (a, b) =>
          b.getBoundingClientRect().bottom - a.getBoundingClientRect().bottom,
      );

    const last = candidates[0];
    const lastRect = last?.getBoundingClientRect();
    const classNameStr =
      last && typeof last.className === "string" ? last.className : "";
    const lastTag = last
      ? `${last.tagName.toLowerCase()}${classNameStr ? `.${classNameStr.split(/\s+/).slice(0, 3).join(".")}` : ""}`
      : null;
    const lastText = last?.innerText?.trim().slice(0, 80) || "";

    return {
      viewportHeight: window.innerHeight,
      nav: navRect ? { top: navRect.top, height: navRect.height } : null,
      fab: fabRect
        ? { top: fabRect.top, bottom: fabRect.bottom, height: fabRect.height }
        : null,
      lastContent: last
        ? {
            tag: lastTag,
            text: lastText,
            bottom: lastRect!.bottom,
            top: lastRect!.top,
          }
        : null,
      fabCoversContent:
        last && fabRect
          ? lastRect!.bottom > fabRect.top && lastRect!.top < fabRect.bottom
          : null,
      navCoversContent:
        last && navRect ? lastRect!.bottom > navRect.top : null,
      contentClearsFab:
        last && fabRect ? fabRect.top - lastRect!.bottom : null,
    };
  });

  console.log("\n=== menu with FAB ===");
  console.log(JSON.stringify(measurements, null, 2));
  if (measurements.fab) {
    console.log(
      measurements.fabCoversContent
        ? `❌ FAB OVERLAPS last content`
        : `✅ Last content clears FAB by ${measurements.contentClearsFab}px`,
    );
  } else {
    console.log("⚠️  FAB not rendered — cart did not seed correctly");
  }

  expect(measurements.nav).not.toBeNull();
});
