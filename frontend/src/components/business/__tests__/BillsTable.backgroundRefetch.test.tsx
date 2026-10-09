/** @jest-environment jsdom */
/**
 * Blocker #4: the L2-37 skeleton must not replace live rows.
 *
 * BillManager passes `loading={listFetching}`, which flips true on every
 * SSE-driven background refetch (payment/close events plus the 60s reconcile).
 * `if (loading) return <skeleton/>` therefore blew away the rendered rows and
 * flashed four pulse bars several times a minute — worse than the empty-flash
 * L2-37 set out to fix. The skeleton belongs to the FIRST load only.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { BillsTable } from "../BillsTable";
import type { Bill } from "../../../api/bills";
import { asDollars } from "../../../types/money";

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

const tString = (key: string) => key;

function renderTable(bills: Bill[], loading: boolean) {
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
      businessTimezone="UTC"
      loading={loading}
    />,
  );
}

it("keeps live rows mounted during a background refetch", () => {
  renderTable([makeBill()], true);

  expect(screen.getByText("B-0001")).toBeInTheDocument();
  expect(screen.queryByTestId("bills-table-loading")).toBeNull();
});

it("still shows the skeleton on the first load (no rows yet)", () => {
  renderTable([], true);

  expect(screen.getByTestId("bills-table-loading")).toBeInTheDocument();
  // and not the empty state — that was the L2-37 regression.
  expect(
    screen.queryByText("emptyState.noActiveBills"),
  ).toBeNull();
});
