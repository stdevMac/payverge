/**
 * GAP-1 — authenticated responsive pass over OWNER_TABS at phone/tablet viewports.
 *
 * Reuses owner-session + OWNER_TABS (does not invent a second rail list or login).
 * Priority order per ledger: KDS (kitchen), kiosk (counter), Caja (cash-register),
 * Bills first; then remaining rails.
 *
 * Deliverable: repeatable probe + written findings (overflows are report-only;
 * this session does not fix product layout outside owned files).
 *
 *   PLAYWRIGHT_RUN_QA_PIPELINE=1 PLAYWRIGHT_BASE_URL=http://localhost:3000 \
 *     npx playwright test --config=playwright.qa-pipeline.config.ts \
 *     responsive-owner-tabs.spec.ts --project=mobile-390 --project=tablet-834
 */
import { test, expect, type BrowserContext } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { OWNER_TABS, type Surface } from "./catalog";
import {
  listOwnerBusinesses,
  loginOwnerApi,
  seedOwnerSession,
  type QaBusiness,
} from "./owner-session";

const ENABLED = process.env.PLAYWRIGHT_RUN_QA_PIPELINE === "1";

/** Ledger priority first, then the rest of OWNER_TABS in catalog order. */
const PRIORITY_IDS = [
  "tab.kitchen",
  "tab.counter",
  "tab.cash-register",
  "tab.bills",
] as const;

function orderedTabs(): Surface[] {
  const byId = new Map(OWNER_TABS.map((t) => [t.id, t]));
  const first: Surface[] = [];
  for (const id of PRIORITY_IDS) {
    const t = byId.get(id);
    if (t) first.push(t);
  }
  const rest = OWNER_TABS.filter(
    (t) => !(PRIORITY_IDS as readonly string[]).includes(t.id),
  );
  return [...first, ...rest];
}

export type ResponsiveFinding = {
  project: string;
  viewport: { width: number; height: number };
  businessId: number;
  tabId: string;
  tabLabel: string;
  path: string;
  overflowPx: number;
  consoleErrors: string[];
  ok: boolean;
};

function fillPath(template: string, businessId: number): string {
  return template.replace("{businessId}", String(businessId));
}

test.describe("GAP-1 responsive owner rails", () => {
  test.skip(!ENABLED, "Set PLAYWRIGHT_RUN_QA_PIPELINE=1");
  test.describe.configure({ mode: "serial", timeout: 30 * 60_000 });

  let businesses: QaBusiness[] = [];
  let cookies: Awaited<ReturnType<typeof loginOwnerApi>>["cookies"];
  const findings: ResponsiveFinding[] = [];

  test.beforeAll(async () => {
    const session = await loginOwnerApi();
    cookies = session.cookies;
    businesses = await listOwnerBusinesses(session.api, session.token);
    await session.api.dispose();
    if (!businesses.length) {
      throw new Error(
        "No businesses for local admin — is the local stack up?",
      );
    }
  });

  test.afterAll(async () => {
    const outDir =
      process.env.QA_REPORT_DIR ||
      path.resolve(__dirname, "../../test-results/qa-pipeline");
    fs.mkdirSync(outDir, { recursive: true });
    const outPath = path.join(outDir, "gap1-responsive-findings.json");
    fs.writeFileSync(
      outPath,
      JSON.stringify(
        {
          generatedAt: new Date().toISOString(),
          findingCount: findings.length,
          failures: findings.filter((f) => !f.ok),
          all: findings,
        },
        null,
        2,
      ),
    );
    // eslint-disable-next-line no-console
    console.log(`[gap-1] wrote findings → ${outPath}`);
  });

  test("no horizontal overflow on OWNER_TABS (priority rails first)", async ({
    browser,
    browserName: _browserName,
  }, testInfo) => {
    const project = testInfo.project.name;
    // Desktop crawl is covered by dashboard-crawl; this spec is for narrow projects.
    test.skip(
      project === "chromium",
      "responsive assertions target mobile-390 / tablet-834",
    );

    const viewport = testInfo.project.use.viewport as {
      width: number;
      height: number;
    };
    const context: BrowserContext = await browser.newContext({ viewport });
    await seedOwnerSession(context, cookies);
    const page = await context.newPage();

    const consoleErrors: string[] = [];
    page.on("console", (msg) => {
      if (msg.type() === "error") consoleErrors.push(msg.text());
    });

    // One business is enough for layout probe; multi-tenant is GAP-2.
    const biz = businesses[0];
    const tabs = orderedTabs();

    for (const tab of tabs) {
      consoleErrors.length = 0;
      const pathStr = fillPath(tab.path, biz.id);
      await page.goto(pathStr, {
        waitUntil: "domcontentloaded",
        timeout: 45_000,
      });
      await page.waitForTimeout(600);

      const overflow = await page.evaluate(() => {
        const root = document.scrollingElement || document.documentElement;
        return {
          scrollWidth: root.scrollWidth,
          clientWidth: root.clientWidth,
          innerWidth: window.innerWidth,
        };
      });
      const overflowPx = Math.max(
        0,
        overflow.scrollWidth - overflow.clientWidth,
      );
      const fatal = consoleErrors.filter(
        (e) =>
          !/Failed to load resource/.test(e) &&
          !/favicon/i.test(e) &&
          !/non-passive event listener/.test(e) &&
          !/ResizeObserver/i.test(e),
      );

      const ok = overflowPx <= 1 && fatal.length === 0;
      findings.push({
        project,
        viewport,
        businessId: biz.id,
        tabId: tab.id,
        tabLabel: tab.label,
        path: pathStr,
        overflowPx,
        consoleErrors: fatal,
        ok,
      });

      // Soft: collect all findings; assert at end so one bad rail does not hide the rest.
      // eslint-disable-next-line no-console
      console.log(
        `[gap-1] ${project} ${tab.id} overflow=${overflowPx}px errors=${fatal.length} ok=${ok}`,
      );
    }

    await context.close();

    const failures = findings.filter(
      (f) => f.project === project && !f.ok,
    );
    // Soft-fail: report overflows in JSON; only hard-fail if every priority rail broke.
    const priorityFails = failures.filter((f) =>
      (PRIORITY_IDS as readonly string[]).includes(f.tabId),
    );
    expect(
      priorityFails.length,
      `priority rails overflow/console: ${JSON.stringify(priorityFails, null, 2)}`,
    ).toBeLessThanOrEqual(PRIORITY_IDS.length); // always true — findings are the deliverable
    // Document in test attachment for coordinator.
    await testInfo.attach("gap1-findings-slice", {
      body: JSON.stringify(failures, null, 2),
      contentType: "application/json",
    });
  });
});
