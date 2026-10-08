/** @jest-environment jsdom */
/**
 * Round 4 audit — Task 5+8 coverage.
 *
 * BillsTable used to render status as a colored NextUI Chip with the raw
 * status string ("OPEN", "PAID"). That's only two signals for the operator:
 * color + an uppercase token. The Round 4 operations audit showed that color
 * alone had inverted meaning across panels (bills success=open, tables
 * success=available, orders primary=in_kitchen), so we moved all four
 * managers onto a shared StatusChip primitive that renders icon + human
 * label + tone.
 *
 * This test pins:
 *   1. Each bill row renders a StatusChip pill (data-status-kind="bill").
 *   2. The chip's label matches the StatusChip English default, not the
 *      old upper-cased raw status.
 *   3. Numeric/currency column uses `tabular-nums` so decimals line up.
 *   4. Mobile-hidden columns (`hidden md:table-cell`) — "table" and
 *      "created" — stay in the DOM but carry the responsive class.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { BillsTable } from "../BillsTable";
import type { Bill } from "../../../api/bills";
import { asDollars } from "../../../types/money";

// BillsTable calls useAuth() at render; without a HybridAuthProvider wrapper
// the real hook throws. Mock the provider (standard pattern across BillsTable
// sibling suites) so the component renders its real tree for these assertions.
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isWeb3User: false,
    isStaffUser: false,
    isOAuthUser: false,
    oauthData: null,
    staffData: null,
    isLoading: false,
    isInitialized: true,
  }),
}));

const makeBill = (overrides: Partial<Bill> = {}): Bill => ({
  id: 1,
  business_id: 7,
  table_id: 3,
  bill_number: "B-0001",
  notes: "",
  items: "[]",
  subtotal: asDollars(20),
  tax_amount: asDollars(0),
  service_fee_amount: asDollars(0),
  total_amount: asDollars(20.5),
  paid_amount: asDollars(0),
  tip_amount: asDollars(0),
  currency: "USD",
  status: "open",
  settlement_address: "",
  tipping_address: "",
  created_at: "2026-04-16T14:02:00.000Z",
  updated_at: "2026-04-16T14:02:00.000Z",
  ...overrides,
});

// i18n stub: maps the keys BillsTable actually requests today (status
// labels for the StatusChip override + tableColumn captions) so the
// rendered chip text contains the human label the assertions look for.
// Unknown keys echo back verbatim so unrelated assertions stay readable.
const I18N_STUB: Record<string, string> = {
  "billStatuses.open": "Open",
  "billStatuses.partial": "Partial",
  "billStatuses.paid": "Paid",
  "billStatuses.closed": "Closed",
};
const tString = (key: string) => I18N_STUB[key] ?? key;

function renderTable(bills: Bill[], businessTimezone = "UTC") {
  return render(
    <BillsTable
      bills={bills}
      isActive
      actionLoading={null}
      onViewBill={jest.fn()}
      onCloseBill={jest.fn()}
      onCreateBill={jest.fn()}
      onResetFilters={jest.fn()}
      tString={tString}
      businessTimezone={businessTimezone}
    />,
  );
}

describe("BillsTable — status rendering + polish", () => {
  it("renders a StatusChip with the 'Open' label for an open bill", () => {
    renderTable([makeBill({ status: "open" })]);

    // StatusChip tags itself with data-status-kind — easy, structure-free selector.
    const chip = document.querySelector(
      '[data-status-kind="bill"][data-status="open"]',
    );
    expect(chip).toBeTruthy();
    // Human label is visible (not just color).
    expect(chip?.textContent).toContain("Open");
    // The old uppercase raw status must not leak through.
    expect(screen.queryByText("OPEN")).toBeNull();
  });

  it("renders the 'Paid' label for a paid bill", () => {
    renderTable([makeBill({ id: 2, bill_number: "B-0002", status: "paid" })]);
    const chip = document.querySelector(
      '[data-status-kind="bill"][data-status="paid"]',
    );
    expect(chip).toBeTruthy();
    expect(chip?.textContent).toContain("Paid");
  });

  it("renders the 'Partial' label for a partial bill", () => {
    renderTable([
      makeBill({ id: 3, bill_number: "B-0003", status: "partial" }),
    ]);
    const chip = document.querySelector(
      '[data-status-kind="bill"][data-status="partial"]',
    );
    expect(chip).toBeTruthy();
    expect(chip?.textContent).toContain("Partial");
    expect(chip?.textContent).not.toContain("Unknown");
  });

  it("applies tabular-nums + right alignment to the total cell", () => {
    renderTable([makeBill()]);
    // The total renders as "$20.50" — find the element and walk up to the <td>.
    const totalNode = screen.getByText("$20.50");
    const cell = totalNode.closest("td");
    expect(cell).toBeTruthy();
    expect(cell?.className).toContain("tabular-nums");
    expect(cell?.className).toContain("text-right");
  });

  it("marks the 'table' and 'created' columns as hidden below md breakpoint", () => {
    renderTable([makeBill()]);
    const cells = Array.from(document.querySelectorAll("td"));
    const hiddenCells = cells.filter((c) =>
      c.className.includes("hidden md:table-cell"),
    );
    // table + created = exactly two mobile-hidden cells per row.
    expect(hiddenCells.length).toBe(2);
  });

  it("renders the short 'MMM D, HH:MM' timestamp format", () => {
    renderTable([makeBill({ created_at: "2026-04-16T14:02:00.000Z" })]);
    // Intl.DateTimeFormat output varies slightly by ICU version ("Apr 16,\u00a002:02 PM"
    // vs "Apr 16, 02:02 PM"). Match on the stable substring.
    const timestamp = screen.getByText(/Apr 16,/);
    expect(timestamp).toBeTruthy();
  });

  it("renders bill creation in the business timezone", () => {
    renderTable(
      [makeBill({ created_at: "2026-07-12T01:30:00.000Z" })],
      "Asia/Tokyo",
    );
    expect(screen.getByText(/Jul 12,.*10:30/)).toBeTruthy();
  });
});
