/** @jest-environment jsdom */
/**
 * Regression: the active-bills list endpoint serves the BillListRow projection,
 * which carries the table's friendly name as a FLAT `table_name` field
 * (populated via `LEFT JOIN tables`), not as a nested `table` relation.
 * BillsTable previously only read `bill.table?.name`, so projection responses
 * fell through to the raw "Table <id>" fallback — operators saw "Table 36"
 * instead of the friendly name. This pins that the flat `table_name` is
 * rendered, while the nested relation still works for endpoints that preload it.
 *
 * Since F2/F3 (50a7e95a) the label routes through `formatEntityName`, which
 * keeps the "Table" orientation word on custom names that don't already embed
 * it ("Window 1" -> "Table Window 1"). The point of these cases is that the
 * friendly name is surfaced and the raw "Table 36" id fallback is gone.
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
  table_id: 36,
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

const tString = (key: string) =>
  ({ table: "Table", counter: "Counter", delivery: "Delivery" })[key] ?? key;

function renderTable(bills: Bill[]) {
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
    />,
  );
}

describe("BillsTable — table label", () => {
  it("renders the flat projection table_name (Window 1), not 'Table <id>'", () => {
    renderTable([makeBill({ table_id: 36, table_name: "Window 1" })]);
    expect(screen.getByText("Table Window 1")).toBeInTheDocument();
    expect(screen.queryByText("Table 36")).toBeNull();
  });

  it("still renders the nested table.name when an endpoint preloads the relation", () => {
    renderTable([
      makeBill({
        table_id: 36,
        table: { id: 36, name: "Bar 2", table_code: "CORE-T09" },
      }),
    ]);
    expect(screen.getByText("Table Bar 2")).toBeInTheDocument();
    expect(screen.queryByText("Table 36")).toBeNull();
  });

  it("falls back to 'Table <id>' only when neither name is present", () => {
    renderTable([makeBill({ table_id: 36 })]);
    expect(screen.getByText("Table 36")).toBeInTheDocument();
  });

  // table_id = 0 is the "no table" sentinel (delivery/counter bills) — the
  // list must never show the raw "Table 0" to operators.
  it("labels a tableless counter bill 'Counter', never 'Table 0'", () => {
    renderTable([makeBill({ table_id: 0, counter_id: 3 })]);
    expect(screen.getByText("Counter")).toBeInTheDocument();
    expect(screen.queryByText("Table 0")).toBeNull();
  });

  it("labels a tableless bill without a counter 'Delivery'", () => {
    renderTable([makeBill({ table_id: 0 })]);
    expect(screen.getByText("Delivery")).toBeInTheDocument();
    expect(screen.queryByText("Table 0")).toBeNull();
  });
});
