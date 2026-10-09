/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

// The dashboard page is a ~1700-line client god-component with no mountable
// test; per the established convention (pollGating.test.ts) we source-lock the
// two IA-1 hardening invariants so they can't silently regress.
const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../page.tsx"),
  "utf-8",
);
const TAB_PARAMS_SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../tabParams.ts"),
  "utf-8",
);

describe("dashboard Accounting/Invoices IA hardening (Task 32)", () => {
  it("keeps ?tab=fiscal as a first-class tab so the Setup sidebar entry highlights", () => {
    // Task 32 restored fiscal to SECONDARY_TABS. The URL-driven effect must
    // set activeTab to the raw "fiscal" key (no rewrite to accounting).
    expect(SOURCE).not.toMatch(
      /const\s+normalizedTab\s*=\s*tabFromUrl === "fiscal"/,
    );
    expect(SOURCE).not.toMatch(/applyNormalizedFiscalUrl/);
    // Content still shares the AccountingDashboard invoices surface.
    expect(SOURCE).toMatch(/case\s+"fiscal":/);
  });

  it("allowlists the ?sub= param via resolveSubTab so a crafted value can't blank the Accounting panel", () => {
    expect(SOURCE).toMatch(
      /const\s+ACCOUNTING_SUB_TABS\s*=\s*\[[\s\S]*?"overview"[\s\S]*?"entries"[\s\S]*?"payroll"[\s\S]*?"invoices"[\s\S]*?"reports"[\s\S]*?"outstanding"[\s\S]*?\]/,
    );
    // Task 31: resolveSubTab returns status=unknown for bad keys (named not-found).
    expect(SOURCE).toMatch(/resolveSubTab\(/);
    expect(SOURCE).toMatch(/accountingUnknownSub/);
    expect(SOURCE).toMatch(/dashboard-sub-not-found/);
  });
});

describe("dashboard invalid ?tab= normalization (Task 31 / F15)", () => {
  it("names an unknown tab key in a not-found state rather than silently redirecting", () => {
    // Task 31: unknown ?tab= keeps the URL and surfaces a not-found panel
    // that names the bad key. Safe aliases still rewrite.
    expect(SOURCE).toMatch(/const\s+aliased\s*=\s*TAB_ALIASES\[tabFromUrl\]/);
    expect(SOURCE).toMatch(/setUnknownTabKey\(tabFromUrl\)/);
    expect(SOURCE).toMatch(/setActiveTab\("__unknown__"\)/);
    expect(SOURCE).toMatch(/dashboard-tab-not-found/);
    expect(SOURCE).toMatch(/error\.tabNotFoundBody/);
  });

  it("aliases obvious singulars onto their canonical plural slug", () => {
    // Session P extracted TAB_ALIASES into ./tabParams so it is unit-testable;
    // page.tsx still consumes it (asserted above), the table itself lives here.
    expect(TAB_PARAMS_SOURCE).toMatch(
      /const\s+TAB_ALIASES\s*:\s*Record<string,\s*string>/,
    );
    expect(TAB_PARAMS_SOURCE).toMatch(/plugin:\s*"plugins"/);
    // Marketing/docs often use ?tab=director; TabKey is director-console.
    expect(TAB_PARAMS_SOURCE).toMatch(/director:\s*"director-console"/);
    // Every alias target must be a real valid tab slug.
    const aliasBlock = TAB_PARAMS_SOURCE.match(
      /const\s+TAB_ALIASES[\s\S]*?\};/,
    )?.[0];
    expect(aliasBlock).toBeTruthy();
    const targets = [...(aliasBlock ?? "").matchAll(/:\s*"([a-z-]+)"/g)].map(
      (m) => m[1],
    );
    expect(targets.length).toBeGreaterThan(0);
    // Alias targets must be real rail keys. Since the registry consolidation,
    // membership lives in TAB_REGISTRY (page.tsx derives validTabs from it).
    const REGISTRY_SOURCE = fs.readFileSync(
      path.resolve(
        __dirname,
        "../../../../../../components/business/tabs/tabRegistry.tsx",
      ),
      "utf-8",
    );
    const registryBlock =
      REGISTRY_SOURCE.match(
        /export const TAB_REGISTRY = \{[\s\S]*?\} as const/,
      )?.[0] ?? "";
    expect(registryBlock).toBeTruthy();
    for (const target of targets) {
      expect(registryBlock).toMatch(
        new RegExp(`(?:^|[\\s{])"?${target}"?\\s*:\\s*\\{`),
      );
    }
  });
});

describe("NEW-15 fiscal entry heading", () => {
  it("passes fiscalEntry when the active tab is fiscal", () => {
    expect(SOURCE).toMatch(/fiscalEntry=\{visibleActiveTab === "fiscal"\}/);
  });

  it("AccountingTab titles fiscal entry as a focused Setup destination (no Accounting strip)", () => {
    const accountingTab = fs.readFileSync(
      path.resolve(
        __dirname,
        "../../../../../../components/business/accounting/AccountingTab.tsx",
      ),
      "utf-8",
    );
    expect(accountingTab).toMatch(/fiscalEntry/);
    expect(accountingTab).toMatch(/fiscal\.dashboard\.title/);
    expect(accountingTab).toMatch(/fiscalEntry\.eyebrow/);
    // Distinct destination: hide Accounting SegmentedTabs when fiscalEntry.
    expect(accountingTab).toMatch(/!fiscalEntry\s*\?\s*\(/);
  });
});

