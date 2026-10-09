import { test, expect } from "@playwright/test";

const BASE = "http://localhost:3000";
const TABLE_CODE = "AI-T01";
const OUT_DIR = "/tmp/payverge-screenshots";
const MOBILE = { width: 390, height: 844 };

const ROUTES = [
  { name: "01-landing", path: `/t/${TABLE_CODE}` },
  { name: "02-menu", path: `/t/${TABLE_CODE}/menu` },
  { name: "03-bill", path: `/t/${TABLE_CODE}/bill` },
];

test.describe("bottom-nav overlap audit", () => {
  for (const route of ROUTES) {
    test(`${route.name} — measure nav vs last content at scroll bottom`, async ({ page }) => {
      await page.setViewportSize(MOBILE);
      await page.goto(`${BASE}${route.path}`, {
        waitUntil: "networkidle",
        timeout: 30000,
      });
      await page.waitForTimeout(1500);

      // Scroll to the absolute bottom
      await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
      await page.waitForTimeout(500);

      // Capture viewport at bottom-of-scroll (what the user actually sees)
      await page.screenshot({
        path: `${OUT_DIR}/scroll-bottom-${route.name}.png`,
        fullPage: false,
      });

      // Measure nav and the last visible *leaf* content element above it.
      // We exclude wrapper containers (whose bottom naturally meets body
      // bottom because they hold pb-32 padding) and look for actual leaf
      // content with paint: text nodes, buttons, links, images, headings.
      const measurements = await page.evaluate(() => {
        const nav = document.querySelector(".fixed.bottom-0") as HTMLElement | null;
        const navRect = nav?.getBoundingClientRect();
        const navHeight = navRect?.height ?? 0;
        const navTop = navRect?.top ?? 0;

        const SELECTOR =
          "p, h1, h2, h3, button, a, span, [class*='font-mono'], svg";
        const all = Array.from(document.querySelectorAll(SELECTOR)) as HTMLElement[];
        const docBottom = document.documentElement.scrollHeight;

        const candidates = all
          .filter((el) => {
            const r = el.getBoundingClientRect();
            const cs = getComputedStyle(el);
            if (nav && (el === nav || nav.contains(el))) return false;
            if (cs.display === "none" || cs.visibility === "hidden") return false;
            if (r.width === 0 || r.height === 0) return false;
            // Only consider elements visible in the current (scrolled) viewport,
            // since we just scrolled to the bottom.
            return r.top < window.innerHeight && r.bottom > 0;
          })
          .sort(
            (a, b) =>
              b.getBoundingClientRect().bottom -
              a.getBoundingClientRect().bottom,
          );

        const last = candidates[0];
        const lastRect = last?.getBoundingClientRect();
        // SVG elements have SVGAnimatedString classes; coerce to string.
        const classNameStr =
          last && typeof last.className === "string" ? last.className : "";
        const lastTag = last
          ? `${last.tagName.toLowerCase()}${classNameStr ? `.${classNameStr.split(/\s+/).slice(0, 3).join(".")}` : ""}`
          : null;
        const lastText = last?.innerText?.trim().slice(0, 80) || "";

        return {
          viewportHeight: window.innerHeight,
          docScrollHeight: document.documentElement.scrollHeight,
          scrollY: window.scrollY,
          navHeight,
          navTopInViewport: navTop,
          lastContent: last
            ? {
                tag: lastTag,
                text: lastText,
                bottomInViewport: lastRect!.bottom,
                topInViewport: lastRect!.top,
                hiddenBehindNav: lastRect!.bottom > navTop,
                clearanceFromNav: navTop - lastRect!.bottom,
              }
            : null,
        };
      });

      console.log(`\n=== ${route.name} ===`);
      console.log(JSON.stringify(measurements, null, 2));

      // Soft assertion — log the verdict instead of failing
      if (measurements.lastContent) {
        if (measurements.lastContent.hiddenBehindNav) {
          console.log(
            `❌ Last content is HIDDEN behind nav by ${Math.abs(measurements.lastContent.clearanceFromNav)}px`,
          );
        } else {
          console.log(
            `✅ Last content clears nav by ${measurements.lastContent.clearanceFromNav}px`,
          );
        }
      }

      expect(measurements.navHeight).toBeGreaterThan(0);
    });
  }
});
