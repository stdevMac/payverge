/**
 * Runs last (zzz- prefix) and merges partial reports written by each crawl suite.
 */
import { test, expect } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import {
  buildReport,
  type Finding,
  type QaReport,
  type SurfaceResult,
  writeReport,
} from "./collectors";
import { API_BASE } from "./owner-session";

const ENABLED = process.env.PLAYWRIGHT_RUN_QA_PIPELINE === "1";

test.describe("QA pipeline — finalize report", () => {
  test.skip(!ENABLED, "Set PLAYWRIGHT_RUN_QA_PIPELINE=1");
  test.describe.configure({ mode: "serial" });

  test("merge partial reports into report.json + report.md", async ({
    baseURL,
  }) => {
    const outDir =
      process.env.QA_REPORT_DIR ||
      path.resolve(__dirname, "../../test-results/qa-pipeline");

    const partialDirs = [
      "partial-dashboard",
      "partial-public",
      "partial-guest",
      "partial-guest-journey",
      "partial-visual",
      "partial-full-pages",
    ].map((d) => path.join(outDir, d));

    const results: SurfaceResult[] = [];
    const findings: Finding[] = [];
    let startedAt = new Date().toISOString();

    for (const dir of partialDirs) {
      const jsonPath = path.join(dir, "report.json");
      if (!fs.existsSync(jsonPath)) continue;
      const partial = JSON.parse(
        fs.readFileSync(jsonPath, "utf8"),
      ) as QaReport;
      if (partial.startedAt < startedAt) startedAt = partial.startedAt;
      results.push(...(partial.results || []));
      findings.push(...(partial.findings || []));
    }

    // Deduplicate findings (same surface + kind + message).
    const seen = new Set<string>();
    const dedupedFindings: Finding[] = [];
    for (const f of findings) {
      const key = `${f.surfaceId}|${f.kind}|${f.message}|${f.businessId || ""}`;
      if (seen.has(key)) continue;
      seen.add(key);
      dedupedFindings.push(f);
    }

    const report = buildReport(
      startedAt,
      results,
      dedupedFindings,
      baseURL || "http://localhost:3000",
      API_BASE,
    );
    const paths = writeReport(report, outDir);

    // eslint-disable-next-line no-console
    console.log(
      `\n=== QA pipeline report ===\nJSON: ${paths.json}\nMD:   ${paths.md}\n` +
        `Surfaces: ${report.summary.surfaces} pass=${report.summary.passed} fail=${report.summary.failed}\n` +
        `Findings: P0=${report.summary.bySeverity.P0} P1=${report.summary.bySeverity.P1} P2=${report.summary.bySeverity.P2} P3=${report.summary.bySeverity.P3}\n`,
    );

    expect(
      report.summary.surfaces,
      "expected at least one surface result from partial reports",
    ).toBeGreaterThan(0);

    // Print top findings for the console log of the runner.
    const top = dedupedFindings
      .filter((f) => f.severity === "P0" || f.severity === "P1")
      .slice(0, 40);
    if (top.length) {
      // eslint-disable-next-line no-console
      console.log(
        "Top findings:\n" +
          top
            .map(
              (f) =>
                `  [${f.severity}] ${f.surfaceLabel}${f.businessName ? " @ " + f.businessName : ""}: ${f.message.slice(0, 160)}`,
            )
            .join("\n"),
      );
    }
  });
});
