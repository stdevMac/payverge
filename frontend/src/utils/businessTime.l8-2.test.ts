import fs from "fs";
import path from "path";
import {
  DATE_LONG,
  DATE_MONTH_DAY,
  DATE_SHORT,
  DATE_TIME_SHORT,
  RELATIVE,
  TIME_SHORT,
  formatBusinessDateTime,
} from "./businessTime";

const ROOT = path.resolve(__dirname, "../components/business");

/** Call sites that must use named presets (L8-2 / N-5). */
const PRESET_SITES = [
  "AiWaiter/AiWaiterDashboard.tsx",
  "accounting/InvoicesTab.tsx",
  "BillsTable.tsx",
  "CashRegisterDashboard.tsx",
  "BillDisplayMode.tsx",
  "PendingOrdersSection.tsx",
  "fiscal/FiscalDashboard.tsx",
  "accounting/IssueInvoiceDrawer.tsx",
  "accounting/ReceiptDetailDrawer.tsx",
  "BillDetailsModal.tsx",
  "StaffManagement.tsx",
  "ReservationManager.tsx",
  "ReservationApprovalQueue.tsx",
  "InventoryManager.tsx",
  "ReservationsTodayCard.tsx",
];

function readSite(rel: string): string {
  return fs.readFileSync(path.join(ROOT, rel), "utf8");
}

describe("L8-2 / N-5 datetime presets", () => {
  it("exports DATE_SHORT, DATE_MONTH_DAY, DATE_TIME_SHORT, DATE_LONG, TIME_SHORT, RELATIVE", () => {
    expect(DATE_SHORT).toMatchObject({
      year: "numeric",
      month: "short",
      day: "numeric",
    });
    expect(DATE_MONTH_DAY).toMatchObject({
      month: "short",
      day: "numeric",
    });
    expect(DATE_MONTH_DAY).not.toHaveProperty("year");
    expect(DATE_TIME_SHORT).toMatchObject({
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    });
    expect(DATE_LONG).toMatchObject({
      dateStyle: "full",
      timeStyle: "short",
    });
    expect(TIME_SHORT).toMatchObject({
      hour: "numeric",
      minute: "2-digit",
    });
    expect(RELATIVE).toEqual({ kind: "relative" });
  });

  it("DATE_TIME_SHORT always includes a year (fixes bills without year)", () => {
    const formatted = formatBusinessDateTime(
      "2026-07-08T20:53:00Z",
      "en",
      "UTC",
      DATE_TIME_SHORT,
    );
    expect(formatted).toMatch(/2026/);
    // Same instant under es still carries the year (no bare "8 jul, 20:53").
    const es = formatBusinessDateTime(
      "2026-07-08T20:53:00Z",
      "es",
      "UTC",
      DATE_TIME_SHORT,
    );
    expect(es).toMatch(/2026/);
  });

  // MIN-1: 24h hourCycle so "2:25" never looks like a half-applied 12h clock.
  it("DATE_TIME_SHORT uses 24h hourCycle (no ambiguous 12h times)", () => {
    const afternoon = formatBusinessDateTime(
      "2026-07-17T14:25:00Z",
      "en",
      "UTC",
      DATE_TIME_SHORT,
    );
    // h23 always emits a two-digit hour; 14:25 never becomes "2:25".
    expect(afternoon).toMatch(/14:25/);
    expect(afternoon).not.toMatch(/\b2:25\b/);
  });

  it("same instant formats with year across operator locales (N-5 consistency)", () => {
    const instant = "2026-07-08T20:53:00Z";
    for (const locale of ["en", "es"] as const) {
      const short = formatBusinessDateTime(instant, locale, "UTC", DATE_SHORT);
      const time = formatBusinessDateTime(
        instant,
        locale,
        "UTC",
        DATE_TIME_SHORT,
      );
      const long = formatBusinessDateTime(instant, locale, "UTC", DATE_LONG);
      expect(short).toMatch(/2026/);
      expect(time).toMatch(/2026/);
      expect(long).toMatch(/2026/);
    }
  });

  // Structural call-site guard (secondary). Primary DOM proof lives in
  // BillsTable.l8-2-datetime.test.tsx — greps alone are D1 pass-on-revert.
  it.each(PRESET_SITES)(
    "%s uses named DATE_* presets (no ad-hoc month/dateStyle option objects)",
    (rel) => {
      const src = readSite(rel);
      expect(src).toMatch(/formatBusinessDateTime/);
      expect(src).toMatch(/DATE_(SHORT|MONTH_DAY|TIME_SHORT|LONG)|TIME_SHORT/);
      // Revert-proof: inline option objects on formatBusinessDateTime must not return.
      expect(src).not.toMatch(
        /formatBusinessDateTime\([\s\S]{0,120}\{\s*(month|dateStyle|year):/,
      );
    },
  );

  it("TableRow keeps relative timeAgo as the RELATIVE sibling path", () => {
    const src = readSite("tables/TableRow.tsx");
    expect(src).toMatch(/function timeAgo/);
    expect(src).not.toMatch(/formatBusinessDateTime/);
  });
});

