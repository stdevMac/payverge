/**
 * Public / marketing surface crawl from the surface catalog.
 */
import { test, expect } from "@playwright/test";
import { PUBLIC_SURFACES } from "./catalog";
import {
  buildReport,
  type Finding,
  type SurfaceResult,
  writeReport,
} from "./collectors";
import { visitSurface } from "./crawl-visit";
import { API_BASE } from "./owner-session";
import path from "node:path";

const ENABLED = process.env.PLAYWRIGHT_RUN_QA_PIPELINE === "1";

test.describe("QA pipeline — public surfaces", () => {
  test.skip(!ENABLED, "Set PLAYWRIGHT_RUN_QA_PIPELINE=1");
  test.describe.configure({ mode: "serial", timeout: 10 * 60_000 });

  test("crawl marketing and auth entry pages", async ({ page, baseURL }) => {
    const startedAt = new Date().toISOString();
    const results: SurfaceResult[] = [];
    for (const surface of PUBLIC_SURFACES) {
      results.push(
        await visitSurface(surface, { page, interact: false, settleMs: 600 }),
      );
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
    writeReport(report, path.join(outDir, "partial-public"));

    expect
      .soft(
        report.summary.bySeverity.P0,
        `P0 on public crawl:\n${findings
          .filter((f) => f.severity === "P0")
          .map((f) => f.message)
          .join("\n")}`,
      )
      .toBe(0);
  });
});
