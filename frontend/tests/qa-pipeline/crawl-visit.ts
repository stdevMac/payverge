import type { Page } from "@playwright/test";
import {
  detectCrashOrBlank,
  globalCollector,
  globalResults,
  type SurfaceResult,
} from "./collectors";
import { expandPath, type Surface } from "./catalog";
import { safeClickAround, type QaBusiness } from "./owner-session";

export interface VisitContext {
  page: Page;
  business?: QaBusiness;
  tokens?: Record<string, string | number>;
  /** Click safe tabs/filters after load */
  interact?: boolean;
  settleMs?: number;
}

export async function visitSurface(
  surface: Surface,
  ctx: VisitContext,
): Promise<SurfaceResult> {
  const tokens: Record<string, string | number> = {
    ...(ctx.business
      ? {
          businessId: ctx.business.id,
          customUrl: ctx.business.custom_url,
        }
      : {}),
    ...(ctx.tokens || {}),
  };
  const url = expandPath(surface.path, tokens);
  const started = Date.now();
  const meta = {
    surfaceId: surface.id,
    surfaceLabel: surface.label,
    url,
    businessId: ctx.business?.id,
    businessName: ctx.business?.name,
    allowLocked: surface.allowLocked,
  };

  globalCollector.attach(ctx.page);
  globalCollector.resetBuffers();

  let navError: string | null = null;
  // Progress breadcrumb for long crawls (Playwright list reporter picks this up).
  // eslint-disable-next-line no-console
  console.log(
    `[qa] → ${surface.id}${ctx.business ? ` @${ctx.business.id}` : ""} ${url}`,
  );
  try {
    await ctx.page.goto(url, {
      // Never wait for networkidle — dashboard SSE + polling keep the network busy forever.
      waitUntil: "domcontentloaded",
      timeout: 30_000,
    });
    // Short settle for client data; keep this low so ~80 surfaces finish in minutes.
    await ctx.page.waitForTimeout(ctx.settleMs ?? 500);

    if (ctx.interact !== false) {
      await safeClickAround(ctx.page);
      await ctx.page.waitForTimeout(200);
    }
  } catch (err) {
    navError = err instanceof Error ? err.message : String(err);
  }

  const visitFindings = globalCollector.flushVisit(meta);
  await detectCrashOrBlank(ctx.page, meta, globalCollector);

  if (navError) {
    globalCollector.addFinding({
      severity: "P0",
      surfaceId: meta.surfaceId,
      surfaceLabel: meta.surfaceLabel,
      businessId: meta.businessId,
      businessName: meta.businessName,
      url: meta.url,
      kind: "navigation",
      message: `Navigation failed: ${navError.slice(0, 300)}`,
    });
  }

  // Attach any findings added by detectCrashOrBlank after flush.
  const extra = globalCollector.findings.filter(
    (f) =>
      f.surfaceId === meta.surfaceId &&
      f.url === meta.url &&
      !visitFindings.includes(f),
  );
  // extra already in globalCollector.findings

  const findingsForSurface = globalCollector.findings.filter(
    (f) => f.surfaceId === meta.surfaceId && f.url === meta.url,
  );
  // Prefer findings tagged with this exact visit URL (includes post-flush adds).
  const allForVisit = [
    ...visitFindings,
    ...extra.filter((f) => !visitFindings.some((v) => v.message === f.message)),
  ];
  // Deduplicate by message+kind
  const dedup = new Map<string, (typeof allForVisit)[number]>();
  for (const f of allForVisit) {
    dedup.set(`${f.kind}|${f.message}`, f);
  }
  const findings = [...dedup.values()];

  // Also pull any added after flush that share surfaceId (crash detection).
  for (const f of findingsForSurface) {
    const key = `${f.kind}|${f.message}`;
    if (!dedup.has(key)) {
      dedup.set(key, f);
      findings.push(f);
    }
  }

  const blocking = findings.some(
    (f) => f.severity === "P0" || f.severity === "P1",
  );
  const result: SurfaceResult = {
    surfaceId: surface.id,
    surfaceLabel: surface.label,
    url,
    businessId: ctx.business?.id,
    businessName: ctx.business?.name,
    ok: !blocking && !navError,
    durationMs: Date.now() - started,
    findings,
    finalUrl: ctx.page.url(),
    title: await ctx.page.title().catch(() => ""),
  };
  globalResults.push(result);
  return result;
}

/** Visit a tab and each of its ?sub= sub-tabs as separate surfaces. */
export async function visitTabWithSubs(
  surface: Surface,
  ctx: VisitContext,
): Promise<SurfaceResult[]> {
  const results: SurfaceResult[] = [await visitSurface(surface, ctx)];
  if (!surface.subTabs?.length || !ctx.business) return results;

  for (const sub of surface.subTabs) {
    const subSurface: Surface = {
      ...surface,
      id: `${surface.id}.sub.${sub}`,
      label: `${surface.label} → ${sub}`,
      path: `/business/{businessId}/dashboard?tab=${tabKeyFromPath(surface.path)}&sub=${sub}`,
      kind: "owner-subtab",
    };
    results.push(await visitSurface(subSurface, ctx));
  }
  return results;
}

function tabKeyFromPath(path: string): string {
  const m = path.match(/tab=([a-z0-9-]+)/i);
  return m?.[1] || "overview";
}
