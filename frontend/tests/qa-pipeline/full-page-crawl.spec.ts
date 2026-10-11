/**
 * Full App Router page crawl — every inventory page that can be resolved
 * without inventing secrets (invite tokens, random delivery numbers).
 *
 * Complements tab/guest/visual crawls with admin, legal and auth pages.
 */
import { test, expect } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import {
  APP_PAGES,
  expandPagePath,
  pathNeedsTokens,
  type AppPage,
} from "./page-inventory";
import {
  buildReport,
  type Finding,
  type SurfaceResult,
  writeReport,
} from "./collectors";
import {
  API_BASE,
  listBusinessTables,
  listOwnerBusinesses,
  loginOwnerApi,
  seedOwnerSession,
} from "./owner-session";

const ENABLED = process.env.PLAYWRIGHT_RUN_QA_PIPELINE === "1";

const HARD_CRASH =
  /Application error|Unhandled Runtime Error|This page could not be found|Internal Server Error|chunk load error/i;

test.describe("QA pipeline — full page inventory crawl", () => {
  test.skip(!ENABLED, "Set PLAYWRIGHT_RUN_QA_PIPELINE=1");
  test.describe.configure({ mode: "serial", timeout: 20 * 60_000 });

  test("visit all resolvable App Router pages", async ({
    page,
    context,
    baseURL,
  }) => {
    const startedAt = new Date().toISOString();
    const results: SurfaceResult[] = [];
    const findings: Finding[] = [];
    const shotDir =
      process.env.QA_SCREENSHOT_DIR ||
      path.resolve(__dirname, "../../test-results/qa-pipeline/screenshots");
    fs.mkdirSync(shotDir, { recursive: true });

    // Consent
    await context.addInitScript(() => {
      window.localStorage.setItem(
        "payverge_cookie_consent",
        JSON.stringify({
          version: 1,
          analytics: false,
          marketing: false,
          decidedAt: new Date().toISOString(),
        }),
      );
    });

    const session = await loginOwnerApi();
    const businesses = await listOwnerBusinesses(session.api, session.token);
    expect(businesses.length).toBeGreaterThan(0);
    const biz = businesses[0];
    const tables = await listBusinessTables(
      session.api,
      session.token,
      biz.id,
    );

    // Optional dynamic tokens from live APIs / DB-backed public endpoints.
    let deliveryNumber = "";
    let billId = "";
    try {
      // Open bills for deep-link alternative-payments page.
      const billsResp = await session.api.get(
        `${API_BASE}/inside/businesses/${biz.id}/bills/open`,
        { headers: { Authorization: `Bearer ${session.token}` } },
      );
      if (billsResp.ok()) {
        const body = await billsResp.json();
        const bills: Array<{ id: number }> = Array.isArray(body)
          ? body
          : (body.bills ?? body.data ?? []);
        if (bills[0]?.id) billId = String(bills[0].id);
      }
    } catch {
      /* optional */
    }
    try {
      // Delivery list if operator has delivery:read
      const delResp = await session.api.get(
        `${API_BASE}/inside/businesses/${biz.id}/delivery/orders?limit=5`,
        { headers: { Authorization: `Bearer ${session.token}` } },
      );
      if (delResp.ok()) {
        const body = await delResp.json();
        const rows: Array<{ delivery_number?: string; number?: string }> =
          Array.isArray(body)
            ? body
            : (body.orders ?? body.data ?? body.items ?? []);
        const num =
          rows.find((r) => r.delivery_number || r.number)?.delivery_number ||
          rows.find((r) => r.delivery_number || r.number)?.number;
        if (num) deliveryNumber = String(num);
      }
    } catch {
      /* optional */
    }

    const cookies = session.cookies;
    await session.api.dispose();

    // Fallback demo delivery numbers known from the demo seed when API shape varies.
    if (!deliveryNumber) {
      deliveryNumber = "DEL-DEMO-8-50-20260623-001";
    }

    const tokens: Record<string, string | number> = {
      businessId: biz.id,
      customUrl: biz.custom_url,
      tableCode: tables[0]?.table_code || "missing-table",
      deliveryNumber,
      billId: billId || "0",
    };

    // Seed owner/admin session (local admin is role=admin + owns demos).
    await seedOwnerSession(context, cookies);

    const deferred: AppPage[] = [];
    let visited = 0;

    for (const appPage of APP_PAGES) {
      if (appPage.auth === "skip") {
        deferred.push(appPage);
        results.push({
          surfaceId: appPage.id,
          surfaceLabel: appPage.label,
          url: appPage.path,
          ok: true,
          durationMs: 0,
          findings: [
            {
              severity: "P3",
              surfaceId: appPage.id,
              surfaceLabel: appPage.label,
              url: appPage.path,
              kind: "assert",
              message: `Deferred: ${appPage.note || "dynamic token required"}`,
              timestamp: new Date().toISOString(),
            },
          ],
          finalUrl: appPage.path,
          title: "deferred",
        });
        continue;
      }

      if (appPage.auth === "staff") {
        // Without staff OTP, expect bounce to login — still exercise the route.
        // Record as soft check rather than fail.
      }

      let url: string;
      try {
        const needed = pathNeedsTokens(appPage.path);
        for (const k of needed) {
          if (!tokens[k]) throw new Error(`no token ${k}`);
        }
        url = expandPagePath(appPage.path, tokens);
      } catch (err) {
        deferred.push(appPage);
        continue;
      }

      const t0 = Date.now();
      const visitFindings: Finding[] = [];
      try {
        const resp = await page.goto(url, {
          waitUntil: "domcontentloaded",
          timeout: 30_000,
        });
        await page.waitForTimeout(500);

        const status = resp?.status() ?? 0;
        const finalUrl = page.url();
        const body = (
          (await page.locator("body").innerText().catch(() => "")) || ""
        ).replace(/\s+/g, " ");

        // Screenshot for inventory evidence
        const shotName = `fullpage-${appPage.id}`;
        await page
          .screenshot({
            path: path.join(shotDir, `${shotName.replace(/[^\w.-]+/g, "_")}.png`),
            fullPage: true,
          })
          .catch(() => {});

        if (status >= 500) {
          visitFindings.push({
            severity: "P0",
            surfaceId: appPage.id,
            surfaceLabel: appPage.label,
            url,
            kind: "network",
            message: `HTTP ${status} loading ${url}`,
            timestamp: new Date().toISOString(),
          });
        }

        if (HARD_CRASH.test(body) && body.length < 400) {
          visitFindings.push({
            severity: "P0",
            surfaceId: appPage.id,
            surfaceLabel: appPage.label,
            url,
            kind: "crash",
            message: `Crash shell: ${body.slice(0, 160)}`,
            timestamp: new Date().toISOString(),
          });
        }

        // Owner/admin pages must not bounce to login unexpectedly.
        if (
          (appPage.auth === "owner" || appPage.auth === "admin") &&
          /\/(business\/)?login|\/staff\/login/i.test(finalUrl) &&
          !/register/i.test(url)
        ) {
          visitFindings.push({
            severity: "P0",
            surfaceId: appPage.id,
            surfaceLabel: appPage.label,
            url,
            kind: "navigation",
            message: `Auth bounce to login: ${finalUrl}`,
            timestamp: new Date().toISOString(),
          });
        }

        // Staff home without staff session may redirect — P2 note only.
        if (
          appPage.auth === "staff" &&
          /login/i.test(finalUrl)
        ) {
          visitFindings.push({
            severity: "P3",
            surfaceId: appPage.id,
            surfaceLabel: appPage.label,
            url,
            kind: "navigation",
            message: `Staff route redirected to login (expected without staff OTP): ${finalUrl}`,
            timestamp: new Date().toISOString(),
          });
        }

        // Empty body
        if (body.trim().length < 20) {
          visitFindings.push({
            severity: "P1",
            surfaceId: appPage.id,
            surfaceLabel: appPage.label,
            url,
            kind: "blank",
            message: `Near-empty body (${body.length} chars)`,
            timestamp: new Date().toISOString(),
          });
        }

        findings.push(...visitFindings);
        results.push({
          surfaceId: appPage.id,
          surfaceLabel: `[${appPage.group}] ${appPage.label}`,
          url,
          businessId: biz.id,
          businessName: biz.name,
          ok: !visitFindings.some(
            (f) => f.severity === "P0" || f.severity === "P1",
          ),
          durationMs: Date.now() - t0,
          findings: visitFindings,
          finalUrl,
          title: await page.title().catch(() => ""),
        });
        visited++;
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        const f: Finding = {
          severity: "P0",
          surfaceId: appPage.id,
          surfaceLabel: appPage.label,
          url,
          kind: "navigation",
          message: msg.slice(0, 300),
          timestamp: new Date().toISOString(),
        };
        findings.push(f);
        results.push({
          surfaceId: appPage.id,
          surfaceLabel: `[${appPage.group}] ${appPage.label}`,
          url,
          ok: false,
          durationMs: Date.now() - t0,
          findings: [f],
          finalUrl: page.url(),
          title: "",
        });
      }
    }

    // Write inventory list for the report consumers.
    const invPath = path.join(
      process.env.QA_REPORT_DIR ||
        path.resolve(__dirname, "../../test-results/qa-pipeline"),
      "page-inventory.md",
    );
    fs.mkdirSync(path.dirname(invPath), { recursive: true });
    const invLines = [
      "# Frontend page inventory crawl",
      "",
      `Visited: ${visited}`,
      `Deferred (dynamic): ${deferred.length}`,
      `Inventory entries: ${APP_PAGES.length}`,
      "",
      "## Results",
      "",
      ...results.map(
        (r) =>
          `- ${r.ok ? "PASS" : "FAIL"} **${r.surfaceLabel}** \`${r.url}\` → \`${r.finalUrl}\` (${r.durationMs}ms)`,
      ),
      "",
      "## Deferred",
      "",
      ...deferred.map((d) => `- \`${d.path}\` — ${d.note || "skipped"}`),
      "",
    ];
    fs.writeFileSync(invPath, invLines.join("\n"));

    const outDir =
      process.env.QA_REPORT_DIR ||
      path.resolve(__dirname, "../../test-results/qa-pipeline");
    const report = buildReport(
      startedAt,
      results,
      findings.filter((f) => f.severity === "P0" || f.severity === "P1"),
      baseURL || "http://localhost:3000",
      API_BASE,
    );
    writeReport(report, path.join(outDir, "partial-full-pages"));

    // eslint-disable-next-line no-console
    console.log(
      `[qa] full-page crawl visited=${visited} deferred=${deferred.length} P0=${report.summary.bySeverity.P0} P1=${report.summary.bySeverity.P1}`,
    );

    expect(visited, "must visit a substantial page set").toBeGreaterThan(40);
    expect
      .soft(
        report.summary.bySeverity.P0,
        `P0 full-page findings:\n${findings
          .filter((f) => f.severity === "P0")
          .map((f) => `- ${f.surfaceLabel}: ${f.message}`)
          .join("\n")}`,
      )
      .toBe(0);
  });
});
