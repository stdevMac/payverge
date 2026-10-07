/** @jest-environment jsdom */

import "@testing-library/jest-dom";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import { BillsTable } from "../BillsTable";
import type { Bill } from "@/api/bills";
import { asDollars } from "@/types/money";

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ isStaffUser: false }),
}));

const makeBill = (overrides: Partial<Bill> = {}): Bill =>
  ({
    id: 88,
    business_id: 1,
    table_id: 0,
    bill_number: "DEMO-DEL-88",
    notes: "",
    items: "[]",
    subtotal: asDollars(20),
    tax_amount: asDollars(0),
    service_fee_amount: asDollars(0),
    total_amount: asDollars(20),
    paid_amount: asDollars(20),
    tip_amount: asDollars(0),
    currency: "USD",
    status: "paid",
    settlement_address: "",
    tipping_address: "",
    created_at: "2026-07-18T12:00:00Z",
    updated_at: "2026-07-18T12:00:00Z",
    ...overrides,
  }) as Bill;

const labels: Record<string, string> = {
  view: "View",
  billLabel: "bill",
  delivery: "Delivery",
  table: "Table",
  counter: "Counter",
  "buttons.closeWithoutPayment": "Close",
  "buttons.close": "Close",
  "buttons.print": "Print",
  "billStatuses.paid": "Paid",
  "billStatuses.open": "Open",
  "bills.copyBillNumber": "Copy bill number for support",
  "bills.copiedBillNumber": "Copied",
};

describe("BillsTable delivery drill-in (Task 8)", () => {
  it("exposes View for delivery bills and strips DEMO- from the display number", async () => {
    const onViewBill = jest.fn();
    render(
      <BillsTable
        bills={[makeBill({ table_id: 0 })]}
        isActive={false}
        actionLoading={null}
        onViewBill={onViewBill}
        onCloseBill={jest.fn()}
        onCreateBill={jest.fn()}
        onResetFilters={jest.fn()}
        tString={(key) => labels[key] ?? key}
      />,
    );

    // Honest display: DEMO- stripped.
    expect(screen.getByText(/DEL-88/)).toBeInTheDocument();
    expect(screen.queryByText(/DEMO-/i)).not.toBeInTheDocument();

    const view = screen.getByRole("button", { name: /View bill/i });
    expect(view).toBeVisible();
    await userEvent.click(view);
    expect(onViewBill).toHaveBeenCalledWith(88);
  });
});
