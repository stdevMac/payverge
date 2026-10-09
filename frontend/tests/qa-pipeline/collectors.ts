import type { ConsoleMessage, Page, Request, Response } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";

type Severity = "P0" | "P1" | "P2" | "P3";

export interface Finding {
  severity: Severity;
  surfaceId: string;
  surfaceLabel: string;
  businessId?: number;
  businessName?: string;
  url: string;
  kind:
    | "pageerror"
    | "console"
    | "network"
    | "navigation"
    | "blank"
    | "crash"
    | "gate"
    | "assert";
  message: string;
  detail?: string;
  timestamp: string;
}

export interface SurfaceResult {
  surfaceId: string;
  surfaceLabel: string;
  url: string;
  businessId?: number;
  businessName?: string;
  ok: boolean;
  durationMs: number;
  findings: Finding[];
  finalUrl: string;
  title: string;
}

export interface QaReport {
  startedAt: string;
  finishedAt: string;
  baseURL: string;
  apiBase: string;
  results: SurfaceResult[];
  findings: Finding[];
  summary: {
    surfaces: number;
    passed: number;
    failed: number;
    bySeverity: Record<Severity, number>;
  };
}

const CONSOLE_IGNORE = [
  /Download the React DevTools/i,
  /React DevTools/i,
  /\[HMR\]/i,
  /\[Fast Refresh\]/i,
  /favicon\.ico/i,
  /Failed to load resource: the server responded with a status of 404/i,
  /net::ERR_BLOCKED_BY_CLIENT/i,
  /Third-party cookie will be blocked/i,
  /ResizeObserver loop/i,
  /Non-Error promise rejection captured/i,
  // Config-dependent local noise
  /posthog/i,
  /Sentry/i,
  /Failed to fetch dynamically imported module/i, // sometimes flaky under docker; elevate only if page is blank
];

const NETWORK_IGNORE_PATH = [
  /\/favicon/i,
  /hot-update/i,
  /\/_next\/static\/chunks\/.*\.map/i,
  /posthog/i,
  /sentry\.io/i,
  /google-analytics/i,
  /googletagmanager/i,
];

function shouldIgnoreConsole(text: string): boolean {
  return CONSOLE_IGNORE.some((re) => re.test(text));
}

function shouldIgnoreNetwork(url: string): boolean {
  return NETWORK_IGNORE_PATH.some((re) => re.test(url));
}

export class IssueCollector {
  readonly findings: Finding[] = [];
  private pageErrors: string[] = [];
  private consoleErrors: string[] = [];
  private networkFails: Array<{ url: string; status: number; method: string }> =
    [];
  private attached = false;
  private page: Page | null = null;

  private onConsole = (msg: ConsoleMessage) => {
    if (msg.type() !== "error") return;
    const text = msg.text();
    if (shouldIgnoreConsole(text)) return;
    this.consoleErrors.push(text);
  };

  private onPageError = (err: Error) => {
    this.pageErrors.push(err.message || String(err));
  };

  private onResponse = (res: Response) => {
    const req: Request = res.request();
    const url = res.url();
    if (shouldIgnoreNetwork(url)) return;
    const status = res.status();
    // Only track API / same-origin failures that look real.
    if (status < 400) return;
    // 401/403 on public probes are sometimes expected (gate); still record as P2.
    this.networkFails.push({
      url,
      status,
      method: req.method(),
    });
  };

  attach(page: Page) {
    if (this.attached && this.page === page) return;
    this.detach();
    this.page = page;
    page.on("console", this.onConsole);
    page.on("pageerror", this.onPageError);
    page.on("response", this.onResponse);
    this.attached = true;
  }

  detach() {
    if (!this.page || !this.attached) return;
    this.page.off("console", this.onConsole);
    this.page.off("pageerror", this.onPageError);
    this.page.off("response", this.onResponse);
    this.attached = false;
    this.page = null;
  }

  resetBuffers() {
    this.pageErrors = [];
    this.consoleErrors = [];
    this.networkFails = [];
  }

  addFinding(f: Omit<Finding, "timestamp">) {
    this.findings.push({ ...f, timestamp: new Date().toISOString() });
  }

  /**
   * Snapshot buffers into findings for one surface visit, then clear buffers.
   */
  flushVisit(meta: {
    surfaceId: string;
    surfaceLabel: string;
    url: string;
    businessId?: number;
    businessName?: string;
    allowLocked?: boolean;
  }): Finding[] {
    const out: Finding[] = [];
    const base = {
      surfaceId: meta.surfaceId,
      surfaceLabel: meta.surfaceLabel,
      businessId: meta.businessId,
      businessName: meta.businessName,
      url: meta.url,
      timestamp: new Date().toISOString(),
    } as const;

    for (const msg of this.pageErrors) {
      const f: Finding = {
        ...base,
        severity: "P0",
        kind: "pageerror",
        message: msg,
      };
      out.push(f);
      this.findings.push(f);
    }

    for (const msg of this.consoleErrors) {
      const f: Finding = {
        ...base,
        severity: "P1",
        kind: "console",
        message: msg.slice(0, 500),
      };
      out.push(f);
      this.findings.push(f);
    }

    for (const n of this.networkFails) {
      // On lockable AI tabs (allowLocked) a 402 is the per-business AI budget
      // being exhausted (ai_budget_exceeded, backend/internal/server/ai_errors.go),
      // an expected lock. There is no plan paywall.
      const isApi = /\/api\//.test(n.url);
      if (!isApi && n.status === 404) continue;

      let severity: Severity = "P2";
      if (n.status >= 500) severity = "P0";
      else if (n.status === 401 || n.status === 403) severity = "P1";
      else if (n.status === 402 && meta.allowLocked) continue; // AI budget exhausted
      else if (n.status === 402) severity = "P1";
      else if (n.status >= 400) severity = "P2";

      // Navigation document failures are worse.
      if (n.method === "GET" && !isApi && n.status >= 500) severity = "P0";

      const f: Finding = {
        ...base,
        severity,
        kind: "network",
        message: `${n.method} ${n.status} ${shortUrl(n.url)}`,
        detail: n.url,
      };
      out.push(f);
      this.findings.push(f);
    }

    this.resetBuffers();
    return out;
  }
}

function shortUrl(url: string): string {
  try {
    const u = new URL(url);
    return u.pathname + (u.search ? "?…" : "");
  } catch {
    return url.slice(0, 120);
  }
}

export async function detectCrashOrBlank(
  page: Page,
  meta: {
    surfaceId: string;
    surfaceLabel: string;
    url: string;
    businessId?: number;
    businessName?: string;
    allowLocked?: boolean;
  },
  collector: IssueCollector,
): Promise<void> {
  const bodyText = ((await page.locator("body").innerText().catch(() => "")) || "")
    .replace(/\s+/g, " ")
    .trim();

  // Full-page crash markers only — avoid flagging ephemeral server-error toasts
  // ("Something went wrong on our end") that leave the dashboard shell intact.
  const hardCrashMarkers = [
    /Application error/i,
    /Unhandled Runtime Error/i,
    /This page could not be found/i,
    /Internal Server Error/i,
    /chunk load error/i,
  ];
  for (const re of hardCrashMarkers) {
    if (re.test(bodyText)) {
      const sev: Severity =
        /could not be found/i.test(bodyText) && !meta.allowLocked
          ? "P1"
          : "P0";
      collector.addFinding({
        severity: sev,
        surfaceId: meta.surfaceId,
        surfaceLabel: meta.surfaceLabel,
        businessId: meta.businessId,
        businessName: meta.businessName,
        url: meta.url,
        kind: "crash",
        message: `Crash/error marker in body: ${bodyText.slice(0, 160)}`,
      });
      return;
    }
  }
  // Soft server error only when main content is essentially empty / error-only.
  if (
    /Something went wrong on our end/i.test(bodyText) &&
    bodyText.length < 280
  ) {
    collector.addFinding({
      severity: "P0",
      surfaceId: meta.surfaceId,
      surfaceLabel: meta.surfaceLabel,
      businessId: meta.businessId,
      businessName: meta.businessName,
      url: meta.url,
      kind: "crash",
      message: `Page appears error-only: ${bodyText.slice(0, 160)}`,
    });
  }

  // Very short body after settle often means blank shell.
  if (bodyText.length < 40) {
    collector.addFinding({
      severity: "P1",
      surfaceId: meta.surfaceId,
      surfaceLabel: meta.surfaceLabel,
      businessId: meta.businessId,
      businessName: meta.businessName,
      url: meta.url,
      kind: "blank",
      message: `Suspiciously short page body (${bodyText.length} chars)`,
      detail: bodyText.slice(0, 200),
    });
  }

  // Unexpected bounce to login while crawling authenticated owner tabs.
  if (
    meta.surfaceId.startsWith("tab.") &&
    /\/(business\/)?login|\/staff\/login/i.test(page.url())
  ) {
    collector.addFinding({
      severity: "P0",
      surfaceId: meta.surfaceId,
      surfaceLabel: meta.surfaceLabel,
      businessId: meta.businessId,
      businessName: meta.businessName,
      url: meta.url,
      kind: "navigation",
      message: `Owner tab redirected to login: ${page.url()}`,
    });
  }
}

export function buildReport(
  startedAt: string,
  results: SurfaceResult[],
  findings: Finding[],
  baseURL: string,
  apiBase: string,
): QaReport {
  const bySeverity: Record<Severity, number> = {
    P0: 0,
    P1: 0,
    P2: 0,
    P3: 0,
  };
  for (const f of findings) bySeverity[f.severity] += 1;
  const failed = results.filter((r) => !r.ok).length;
  return {
    startedAt,
    finishedAt: new Date().toISOString(),
    baseURL,
    apiBase,
    results,
    findings,
    summary: {
      surfaces: results.length,
      passed: results.length - failed,
      failed,
      bySeverity,
    },
  };
}

export function writeReport(report: QaReport, outDir: string): {
  json: string;
  md: string;
} {
  fs.mkdirSync(outDir, { recursive: true });
  const jsonPath = path.join(outDir, "report.json");
  const mdPath = path.join(outDir, "report.md");
  fs.writeFileSync(jsonPath, JSON.stringify(report, null, 2));
  fs.writeFileSync(mdPath, renderMarkdown(report));
  return { json: jsonPath, md: mdPath };
}

function renderMarkdown(report: QaReport): string {
  const lines: string[] = [];
  lines.push(`# Prod-local QA pipeline report`);
  lines.push("");
  lines.push(`- Started: ${report.startedAt}`);
  lines.push(`- Finished: ${report.finishedAt}`);
  lines.push(`- Base URL: ${report.baseURL}`);
  lines.push(`- API: ${report.apiBase}`);
  lines.push(
    `- Surfaces: ${report.summary.surfaces} (pass ${report.summary.passed} / fail ${report.summary.failed})`,
  );
  lines.push(
    `- Findings: P0=${report.summary.bySeverity.P0} P1=${report.summary.bySeverity.P1} P2=${report.summary.bySeverity.P2} P3=${report.summary.bySeverity.P3}`,
  );
  lines.push("");

  if (report.findings.length === 0) {
    lines.push("No findings. 🎉");
    return lines.join("\n");
  }

  lines.push("## Findings");
  lines.push("");
  const order: Severity[] = ["P0", "P1", "P2", "P3"];
  for (const sev of order) {
    const group = report.findings.filter((f) => f.severity === sev);
    if (!group.length) continue;
    lines.push(`### ${sev} (${group.length})`);
    lines.push("");
    for (const f of group) {
      const biz = f.businessName
        ? ` · ${f.businessName}`
        : "";
      lines.push(
        `- **${f.surfaceLabel}**${biz} — \`${f.kind}\`: ${f.message.replace(/\n/g, " ")}`,
      );
      lines.push(`  - URL: ${f.url}`);
    }
    lines.push("");
  }

  lines.push("## Surface results");
  lines.push("");
  lines.push("| Status | Surface | Business | ms | Findings | URL |");
  lines.push("|---|---|---|---:|---:|---|");
  for (const r of report.results) {
    lines.push(
      `| ${r.ok ? "PASS" : "FAIL"} | ${r.surfaceLabel} | ${r.businessName || "—"} | ${r.durationMs} | ${r.findings.length} | \`${r.finalUrl}\` |`,
    );
  }
  lines.push("");
  return lines.join("\n");
}

/** Global report accumulator shared across specs in one worker. */
export const globalResults: SurfaceResult[] = [];
export const globalCollector = new IssueCollector();
