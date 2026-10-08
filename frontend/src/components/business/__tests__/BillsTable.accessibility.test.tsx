/** @jest-environment jsdom */

import "@testing-library/jest-dom";
import { render, screen } from "@testing-library/react";

import { BillsTable } from "../BillsTable";
import type { Bill } from "@/api/bills";
import { asDollars } from "@/types/money";

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ isStaffUser: false }),
}));

const makeBill = (overrides: Partial<Bill> = {}): Bill => ({
  id: 7,
  business_id: 1,
  table_id: 3,
  bill_number: "B-007",
  notes: "",
  items: "[]",
  subtotal: asDollars(20),
  tax_amount: asDollars(0),
  service_fee_amount: asDollars(0),
  total_amount: asDollars(20),
  paid_amount: asDollars(0),
  tip_amount: asDollars(0),
  currency: "USD",
  status: "open",
  settlement_address: "",
  tipping_address: "",
  created_at: "2026-07-18T12:00:00Z",
  updated_at: "2026-07-18T12:00:00Z",
  ...overrides,
});

const labels: Record<string, string> = {
  view: "View",
  billLabel: "bill",
  "buttons.closeWithoutPayment": "Close without payment",
  "buttons.close": "Close",
  "buttons.print": "Print",
  "buttons.moreActionsAria": "More actions for bill {number}",
  "billStatuses.open": "Open",
};

it("uses a row plus contextual action buttons instead of a button containing buttons", () => {
  render(
    <BillsTable
      bills={[makeBill()]}
      isActive
      actionLoading={null}
      onViewBill={jest.fn()}
      onCloseBill={jest.fn()}
      onCreateBill={jest.fn()}
      onResetFilters={jest.fn()}
      tString={(key) => labels[key] ?? key}
    />,
  );

  expect(
    screen.queryByRole("button", { name: /B-007.*View/i }),
  ).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "View bill B-007" })).toBeVisible();
  expect(
    screen.getByRole("button", { name: "More actions for bill B-007" }),
  ).toBeVisible();
});
