/**
 * Guest table + public business page crawl for each owner business.
 */
import { test, expect } from "@playwright/test";
import { GUEST_SURFACES } from "./catalog";
import {
  buildReport,
  type Finding,
  type SurfaceResult,
  writeReport,
} from "./collectors";
import { visitSurface } from "./crawl-visit";
import {
  API_BASE,
  listBusinessTables,
  listOwnerBusinesses,
  loginOwnerApi,
  type QaBusiness,
  type QaTable,
} from "./owner-session";
import path from "node:path";

const ENABLED = process.env.PLAYWRIGHT_RUN_QA_PIPELINE === "1";

test.describe("QA pipeline — guest surfaces", () => {
  test.skip(!ENABLED, "Set PLAYWRIGHT_RUN_QA_PIPELINE=1");
  test.describe.configure({ mode: "serial", timeout: 10 * 60_000 });

  let businesses: QaBusiness[] = [];
  let tablesByBiz: Map<number, QaTable[]> = new Map();

  test.beforeAll(async () => {
    const session = await loginOwnerApi();
    businesses = await listOwnerBusinesses(session.api, session.token);
    for (const b of businesses) {
      const tables = await listBusinessTables(
        session.api,
        session.token,
        b.id,
      );
      tablesByBiz.set(b.id, tables);
    }
    await session.api.dispose();
  });

  test("crawl public business pages and guest table journeys", async ({
    page,
    baseURL,
  }) => {
    const startedAt = new Date().toISOString();
    const results: SurfaceResult[] = [];
    for (const biz of businesses) {
      if (biz.custom_url) {
        const bizPage = GUEST_SURFACES.find(
          (s) => s.id === "guest.business-page",
        )!;
        results.push(
          await visitSurface(bizPage, {
            page,
            business: biz,
            interact: false,
            settleMs: 900,
          }),
        );
      }

      const tables = tablesByBiz.get(biz.id) || [];
      const sample = tables.slice(0, 2); // first two tables per business
      for (const table of sample) {
        for (const surface of GUEST_SURFACES.filter((s) =>
          s.path.includes("{tableCode}"),
        )) {
          results.push(
            await visitSurface(surface, {
              page,
              business: biz,
              tokens: { tableCode: table.table_code },
              interact: false,
              settleMs: 900,
            }),
          );
        }
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
    writeReport(report, path.join(outDir, "partial-guest"));

    expect
      .soft(
        report.summary.bySeverity.P0,
        `P0 on guest crawl:\n${findings
          .filter((f) => f.severity === "P0")
          .map((f) => `- ${f.surfaceLabel}: ${f.message}`)
          .join("\n")}`,
      )
      .toBe(0);
  });
});
