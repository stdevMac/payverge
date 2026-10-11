/**
 * Visual + deeper interaction QA: screenshot every owner tab, open safe controls,
 * assert no error toasts / crash markers, flag empty shells.
 *
 * Critical invariant (criterion 4): each owner-tab PNG must be captured while
 * still on `/business/{id}/dashboard?tab=…` — never after a drift to /admin.
 *
 * Gated by PLAYWRIGHT_RUN_QA_PIPELINE=1. Artifacts under
 * test-results/qa-pipeline/screenshots/.
 */
import { test, expect, type Page } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import { OWNER_TABS, PUBLIC_SURFACES, expandPath } from "./catalog";
import {
  buildReport,
  type Finding,
  type SurfaceResult,
  writeReport,
} from "./collectors";
import {
  API_BASE,
  listOwnerBusinesses,
  loginOwnerApi,
  seedOwnerSession,
  type QaBusiness,
} from "./owner-session";

const ENABLED = process.env.PLAYWRIGHT_RUN_QA_PIPELINE === "1";

const CRASH_COPY =
  /Application error|Unhandled Runtime Error|This page could not be found|chunk load error/i;

function tabKeyFromPath(pathTemplate: string): string {
  const m = pathTemplate.match(/tab=([a-z0-9-]+)/i);
  return m?.[1] || "overview";
}

/** True when the browser is still on the intended owner dashboard tab. */
export function isOwnerDashboardTabUrl(
  finalUrl: string,
  businessId: number,
  tabKey: string,
): boolean {
  try {
    const u = new URL(finalUrl, "http://localhost:3000");
    if (!u.pathname.includes(`/business/${businessId}/dashboard`)) return false;
    // ?tab=fiscal renders the accounting screen; either key counts.
    const tab = u.searchParams.get("tab") || "";
    if (tabKey === "fiscal") return tab === "accounting" || tab === "fiscal";
    return tab === tabKey;
  } catch {
    return false;
  }
}

async function captureSurface(
  page: Page,
  outDir: string,
  name: string,
): Promise<string> {
  fs.mkdirSync(outDir, { recursive: true });
  const file = path.join(outDir, `${name.replace(/[^\w.-]+/g, "_")}.png`);
  await page.screenshot({ path: file, fullPage: true });
  return file;
}

/**
 * Safe in-tab interactions. Never click shell chrome that leaves the business
 * dashboard (Admin link → /admin was wiping every screenshot). Also refuse to
 * change the primary `?tab=` query — sidebar tab buttons live in/near main and
 * would otherwise hop to another primary tab on every visit.
 */
async function deepInteract(
  page: Page,
  stayOnPrefix: string,
  stayTabKey: string,
): Promise<void> {
  const dangerous =
    /\b(delete|remove|void|refund|pay|charge|close bill|destroy|wipe|logout|sign out|disable|revoke|print|confirm|admin|home)\b/i;

  const leaveDanger =
    /\/admin(?:\/|$)|\/login|\/staff\/login|stripe\.com|mailto:|tel:/i;

  // Prefer the primary content region; fall back to body.
  const root = page.locator("main").first();
  if (!(await root.count().catch(() => 0))) return;

  // Only in-panel subtabs/filters — not sidebar primary tab switches.
  // Subtabs usually sit under a panel header, not the outer nav.
  const candidates = root.locator(
    '[role="tablist"] [role="tab"], [data-slot="tab"], [role="radiogroup"] [role="radio"]',
  );
  const n = Math.min(await candidates.count().catch(() => 0), 5);
  for (let i = 0; i < n; i++) {
    const el = candidates.nth(i);
    if (!(await el.isVisible().catch(() => false))) continue;
    const name = ((await el.innerText().catch(() => "")) || "").trim();
    if (!name || dangerous.test(name) || name.length > 60) continue;

    const href = await el.getAttribute("href").catch(() => null);
    if (href && leaveDanger.test(href)) continue;
    // Primary sidebar tabs often use ?tab= in href — skip those.
    if (href && /[?&]tab=/.test(href)) continue;

    const before = page.url();
    await el.click({ timeout: 1_500 }).catch(() => {});
    await page.waitForTimeout(200);
    await page.keyboard.press("Escape").catch(() => {});
    await page.waitForTimeout(100);

    const after = page.url();
    if (!after.includes(stayOnPrefix)) return;
    // If primary tab query changed, restore and stop.
    try {
      const tabAfter = new URL(after).searchParams.get("tab") || "";
      const expected =
        stayTabKey === "fiscal" ? "accounting" : stayTabKey;
      if (tabAfter && tabAfter !== expected && tabAfter !== stayTabKey) {
        await page.goto(before, {
          waitUntil: "domcontentloaded",
          timeout: 15_000,
        });
        return;
      }
    } catch {
      return;
    }
  }
}

test.describe("QA pipeline — visual + deep interact", () => {
  test.skip(!ENABLED, "Set PLAYWRIGHT_RUN_QA_PIPELINE=1");
  test.describe.configure({ mode: "serial", timeout: 25 * 60_000 });

  let businesses: QaBusiness[] = [];
  let cookies: Awaited<ReturnType<typeof loginOwnerApi>>["cookies"];

  test.beforeAll(async () => {
    const session = await loginOwnerApi();
    cookies = session.cookies;
    businesses = await listOwnerBusinesses(session.api, session.token);
    await session.api.dispose();
    if (!businesses.length) {
      throw new Error("No businesses for visual crawl");
    }
  });

  test("screenshot and deeply exercise owner tabs + public pages", async ({
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

    await seedOwnerSession(context, cookies);
    // Pre-set cookie consent so screenshots aren't blocked by the privacy modal.
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
    // Land on first business dashboard — never /admin.
    const warmBiz = businesses[0];
    await page.goto(
      `/business/${warmBiz.id}/dashboard?tab=overview`,
      { waitUntil: "domcontentloaded", timeout: 45_000 },
    );
    await page.waitForTimeout(800);
    // Dismiss residual consent UI if still visible.
    const decline = page.getByRole("button", {
      name: /decline non-essential|accept all|got it/i,
    });
    if (await decline.first().isVisible().catch(() => false)) {
      await decline.first().click().catch(() => {});
      await page.waitForTimeout(300);
    }
    expect(page.url(), "must stay authenticated on business dashboard").toMatch(
      /\/business\/\d+\/dashboard/,
    );
    expect(page.url(), "must not land on platform admin").not.toMatch(/\/admin/);

    const ordered = businesses;

    for (const biz of ordered) {
      for (const tab of OWNER_TABS) {
        const tabKey = tabKeyFromPath(tab.path);
        const url = expandPath(tab.path, { businessId: biz.id });
        const stayOnPrefix = `/business/${biz.id}/dashboard`;
        const t0 = Date.now();
        const surfaceId = `visual.${tab.id}.${biz.id}`;
        try {
          await page.goto(url, {
            waitUntil: "domcontentloaded",
            timeout: 30_000,
          });
          await page.waitForTimeout(700);

          // Hard gate: must be on the intended owner tab BEFORE any interact.
          const urlAfterLoad = page.url();
          if (!isOwnerDashboardTabUrl(urlAfterLoad, biz.id, tabKey)) {
            findings.push({
              severity: "P0",
              surfaceId,
              surfaceLabel: `Visual ${tab.label}`,
              businessId: biz.id,
              businessName: biz.name,
              url,
              kind: "navigation",
              message: `Owner tab load drifted before screenshot: expected business ${biz.id} tab=${tabKey}, got ${urlAfterLoad}`,
              timestamp: new Date().toISOString(),
            });
            // Still try to recover once.
            await page.goto(url, {
              waitUntil: "domcontentloaded",
              timeout: 30_000,
            });
            await page.waitForTimeout(500);
          }

          // Canonical evidence: screenshot WHILE on the intended tab (before interact).
          const shot = await captureSurface(
            page,
            shotDir,
            `${biz.id}-${tab.id}`,
          );
          const urlAtShot = page.url();
          if (!isOwnerDashboardTabUrl(urlAtShot, biz.id, tabKey)) {
            findings.push({
              severity: "P0",
              surfaceId,
              surfaceLabel: `Visual ${tab.label}`,
              businessId: biz.id,
              businessName: biz.name,
              url,
              kind: "assert",
              message: `Screenshot taken off-tab: expected tab=${tabKey} biz=${biz.id}, finalUrl=${urlAtShot}`,
              timestamp: new Date().toISOString(),
            });
          }

          // Body checks on the real owner surface.
          const body = (
            (await page.locator("body").innerText().catch(() => "")) || ""
          ).replace(/\s+/g, " ");
          // Platform admin shell markers must never appear on owner-tab shots.
          if (
            /command center|platform admin|\/admin\/stats/i.test(body) &&
            !/business dashboard/i.test(body)
          ) {
            // Soft: only flag if URL is also admin
            if (/\/admin(?:\/|\?|$)/.test(page.url())) {
              findings.push({
                severity: "P0",
                surfaceId,
                surfaceLabel: `Visual ${tab.label}`,
                businessId: biz.id,
                businessName: biz.name,
                url,
                kind: "assert",
                message: `Owner-tab capture landed on admin shell: ${page.url()}`,
                timestamp: new Date().toISOString(),
              });
            }
          }
          if (CRASH_COPY.test(body) && !tab.allowLocked) {
            const mainText = (
              (await page.locator("main").innerText().catch(() => "")) || ""
            ).trim();
            if (
              mainText.length < 80 ||
              /Something went wrong on our end/i.test(mainText)
            ) {
              findings.push({
                severity: "P0",
                surfaceId,
                surfaceLabel: `Visual ${tab.label}`,
                businessId: biz.id,
                businessName: biz.name,
                url,
                kind: "crash",
                message: `Crash/error copy on ${tab.label}: ${body.slice(0, 160)}`,
                timestamp: new Date().toISOString(),
              });
            }
          }

          // Safe interact AFTER evidence capture; re-assert URL after.
          await deepInteract(page, stayOnPrefix, tabKey);
          await page.waitForTimeout(200);
          if (!isOwnerDashboardTabUrl(page.url(), biz.id, tabKey)) {
            findings.push({
              severity: "P1",
              surfaceId,
              surfaceLabel: `Visual ${tab.label}`,
              businessId: biz.id,
              businessName: biz.name,
              url,
              kind: "navigation",
              message: `deepInteract navigated away from tab=${tabKey}: ${page.url()}`,
              timestamp: new Date().toISOString(),
            });
            // Restore intended tab for the next iteration cleanliness.
            await page.goto(url, {
              waitUntil: "domcontentloaded",
              timeout: 30_000,
            });
          }

          const visitFindings = findings.filter((f) => f.surfaceId === surfaceId);
          results.push({
            surfaceId,
            surfaceLabel: `Visual ${tab.label}`,
            url,
            businessId: biz.id,
            businessName: biz.name,
            ok: !visitFindings.some(
              (f) => f.severity === "P0" || f.severity === "P1",
            ),
            durationMs: Date.now() - t0,
            findings: visitFindings,
            finalUrl: urlAtShot,
            title: shot,
          });
        } catch (err) {
          const msg = err instanceof Error ? err.message : String(err);
          const f: Finding = {
            severity: "P0",
            surfaceId,
            surfaceLabel: `Visual ${tab.label}`,
            businessId: biz.id,
            businessName: biz.name,
            url,
            kind: "navigation",
            message: msg.slice(0, 300),
            timestamp: new Date().toISOString(),
          };
          findings.push(f);
          results.push({
            surfaceId,
            surfaceLabel: `Visual ${tab.label}`,
            url,
            businessId: biz.id,
            businessName: biz.name,
            ok: false,
            durationMs: Date.now() - t0,
            findings: [f],
            finalUrl: page.url(),
            title: "",
          });
        }
      }
    }

    // Public pages — light visual pass
    for (const surface of PUBLIC_SURFACES.slice(0, 8)) {
      const url = surface.path;
      const t0 = Date.now();
      await page.goto(url, { waitUntil: "domcontentloaded", timeout: 20_000 });
      await page.waitForTimeout(400);
      const shot = await captureSurface(
        page,
        shotDir,
        `public-${surface.id}`,
      );
      results.push({
        surfaceId: `visual.${surface.id}`,
        surfaceLabel: `Visual ${surface.label}`,
        url,
        ok: true,
        durationMs: Date.now() - t0,
        findings: [],
        finalUrl: page.url(),
        title: shot,
      });
    }

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
    writeReport(report, path.join(outDir, "partial-visual"));

    // Fail hard if any owner-tab shot was off-dashboard.
    const offTab = findings.filter(
      (f) =>
        f.severity === "P0" &&
        (f.message.includes("off-tab") ||
          f.message.includes("admin shell") ||
          f.message.includes("drifted")),
    );
    expect(
      offTab,
      `Owner-tab screenshots must stay on /business/:id/dashboard?tab=…\n${offTab
        .map((f) => f.message)
        .join("\n")}`,
    ).toHaveLength(0);

    expect
      .soft(
        report.summary.bySeverity.P0,
        `P0 visual findings:\n${findings
          .filter((f) => f.severity === "P0")
          .map((f) => f.message)
          .join("\n")}`,
      )
      .toBe(0);
  });
});

// Lightweight pure-function coverage for the URL guard (no browser).
test.describe("isOwnerDashboardTabUrl unit", () => {
  test("accepts matching business tab URLs", () => {
    expect(
      isOwnerDashboardTabUrl(
        "http://localhost:3000/business/50/dashboard?tab=bills&billTab=history",
        50,
        "bills",
      ),
    ).toBe(true);
  });
  test("rejects admin and wrong business", () => {
    expect(
      isOwnerDashboardTabUrl("http://localhost:3000/admin", 50, "overview"),
    ).toBe(false);
    expect(
      isOwnerDashboardTabUrl(
        "http://localhost:3000/business/51/dashboard?tab=overview",
        50,
        "overview",
      ),
    ).toBe(false);
  });
});
