/**
 * Owner dashboard tab crawl — every catalogued business tab × each owner business.
 *
 * Gated by PLAYWRIGHT_RUN_QA_PIPELINE=1 so it does not run in normal e2e suites.
 */
import { test, expect } from "@playwright/test";
import { OWNER_TABS } from "./catalog";
import {
  buildReport,
  type Finding,
  type SurfaceResult,
  writeReport,
} from "./collectors";
import { visitTabWithSubs } from "./crawl-visit";
import {
  API_BASE,
  listOwnerBusinesses,
  loginOwnerApi,
  seedOwnerSession,
  type QaBusiness,
} from "./owner-session";
import path from "node:path";

const ENABLED = process.env.PLAYWRIGHT_RUN_QA_PIPELINE === "1";

test.describe("QA pipeline — owner dashboard crawl", () => {
  test.skip(!ENABLED, "Set PLAYWRIGHT_RUN_QA_PIPELINE=1");

  test.describe.configure({ mode: "serial", timeout: 20 * 60_000 });

  let businesses: QaBusiness[] = [];
  let cookies: Awaited<ReturnType<typeof loginOwnerApi>>["cookies"];

  test.beforeAll(async () => {
    const session = await loginOwnerApi();
    cookies = session.cookies;
    businesses = await listOwnerBusinesses(session.api, session.token);
    await session.api.dispose();
    if (!businesses.length) {
      throw new Error(
        "No businesses for local admin — is the local stack up with demo data?",
      );
    }
  });

  test("crawl all dashboard tabs for each owner business", async ({
    page,
    context,
    baseURL,
  }) => {
    const startedAt = new Date().toISOString();
    const results: SurfaceResult[] = [];
    await seedOwnerSession(context, cookies);

    // Warm session on dashboard hub.
    await page.goto("/dashboard", {
      waitUntil: "domcontentloaded",
      timeout: 45_000,
    });
    await page.waitForTimeout(1_000);
    if (/login/i.test(page.url())) {
      throw new Error(
        `Session seed failed — still on login (${page.url()}). Use localhost (not 127.0.0.1) for BASE_URL and API so Domain=localhost cookies match.`,
      );
    }

    for (const biz of businesses) {
      // eslint-disable-next-line no-console
      console.log(
        `[qa] business ${biz.id} ${biz.name}`,
      );
      for (const tab of OWNER_TABS) {
        // AI tabs may lock when no LLM is configured — still visit (allowLocked).
        // Skip heavy sub-tab expansion on interact to keep wall clock reasonable;
        // still visit each ?sub= via dedicated path without extra clicks.
        const batch = await visitTabWithSubs(tab, {
          page,
          business: biz,
          interact: tab.subTabs ? false : true,
          settleMs: 400,
        });
        results.push(...batch);
      }
    }

    const findings: Finding[] = results.flatMap((r) => r.findings);
    const outDir =
      process.env.QA_REPORT_DIR ||
      path.resolve(__dirname, "../../test-results/qa-pipeline");
    const report = buildReport(
      startedAt,
      results,
      findings,
      baseURL || "http://localhost:3000",
      API_BASE,
    );
    writeReport(report, path.join(outDir, "partial-dashboard"));

    const p0 = report.summary.bySeverity.P0;
    // Fail the test on P0 so CI turns red; P1+ still appear in the report.
    expect
      .soft(p0, `P0 findings on dashboard crawl:\n${summarize(report)}`)
      .toBe(0);
  });
});

function summarize(report: {
  findings: Array<{ severity: string; surfaceLabel: string; message: string }>;
}): string {
  return report.findings
    .filter((f) => f.severity === "P0" || f.severity === "P1")
    .slice(0, 30)
    .map((f) => `- [${f.severity}] ${f.surfaceLabel}: ${f.message}`)
    .join("\n");
}
