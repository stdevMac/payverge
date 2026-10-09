/** @jest-environment jsdom */
/**
 * Bills row Actions at laptop width: View stays on the row; Close without
 * payment and Print live in an overflow menu so long labels cannot clip.
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ isStaffUser: false }),
}));

jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: [],
    rolePermissions: [],
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: jest.fn(),
  }),
}));

jest.mock("../printers/useIframePrint", () => ({
  useIframePrint: () => ({ print: jest.fn().mockResolvedValue(undefined) }),
}));

jest.mock("@/api/print", () => ({
  createBillPrintJob: jest.fn(),
}));

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: () => ({
    degraded: false,
    blocked: false,
    reconnect: jest.fn(),
  }),
}));

import { BillsTable } from "../BillsTable";
import type { Bill } from "../../../api/bills";
import { asDollars } from "../../../types/money";

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

const tString = (key: string) =>
  ({
    "billStatuses.open": "Open",
    "billStatuses.paid": "Paid",
    "buttons.print": "Print",
    "buttons.closeWithoutPayment": "Close without payment",
    "buttons.moreActions": "More actions",
    "buttons.moreActionsAria": "More actions for bill {number}",
    view: "View",
    billLabel: "bill",
    table: "Table",
    counter: "Counter",
    delivery: "Delivery",
    "tableColumns.billNumber": "Bill",
    "tableColumns.table": "Table",
    "tableColumns.total": "Total",
    "tableColumns.status": "Status",
    "tableColumns.created": "Created",
    "tableColumns.actions": "Actions",
    tableAria: "Bills",
  })[key] ?? key;

function forceDesktopTable() {
  Object.defineProperty(window, "matchMedia", {
    writable: true,
    value: jest.fn().mockImplementation(() => ({
      matches: false,
      media: "",
      onchange: null,
      addListener: jest.fn(),
      removeListener: jest.fn(),
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      dispatchEvent: jest.fn(),
    })),
  });
}

const baseProps = {
  bills: [makeBill()],
  isActive: true,
  actionLoading: null,
  onViewBill: jest.fn(),
  onCloseBill: jest.fn(),
  onCreateBill: jest.fn(),
  onResetFilters: jest.fn(),
  tString,
  businessId: 42,
};

describe("BillsTable actions overflow", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    forceDesktopTable();
  });

  it("keeps View on the row and hides long Close/Print labels behind overflow", () => {
    render(<BillsTable {...baseProps} />);

    const actions = screen.getByTestId("bill-row-actions");
    expect(actions.className).not.toMatch(/flex-wrap/);
    expect(screen.getByRole("button", { name: "View bill B-0001" })).toBeVisible();
    expect(screen.getByTestId("bill-row-overflow")).toHaveAttribute(
      "aria-label",
      "More actions for bill B-0001",
    );

    expect(
      screen.queryByRole("button", { name: /Close without payment/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Print bill/i }),
    ).not.toBeInTheDocument();
    expect(actions).not.toHaveTextContent("Close without payment");
    expect(actions).not.toHaveTextContent("Print");
  });

  it("shows the full Close without payment and Print labels in the overflow menu", async () => {
    const onCloseBill = jest.fn();
    render(<BillsTable {...baseProps} onCloseBill={onCloseBill} />);

    fireEvent.click(screen.getByTestId("bill-row-overflow"));

    const closeItem = await screen.findByRole("menuitem", {
      name: "Close without payment",
    });
    const printItem = screen.getByRole("menuitem", { name: "Print" });
    expect(closeItem).toBeInTheDocument();
    expect(printItem).toBeInTheDocument();

    fireEvent.click(closeItem);
    expect(onCloseBill).toHaveBeenCalledWith(1);
  });

  it("queues a print job from the overflow menu", async () => {
    const { createBillPrintJob } = jest.requireMock("@/api/print");
    (createBillPrintJob as jest.Mock).mockResolvedValueOnce({
      id: 11,
      payload_html: "<html></html>",
    });

    render(<BillsTable {...baseProps} locale="es-ar" />);
    fireEvent.click(screen.getByTestId("bill-row-overflow"));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Print" }));

    await waitFor(() =>
      expect(createBillPrintJob).toHaveBeenCalledWith(42, 1, "es-ar"),
    );
  });

  it("omits Close without payment from overflow on settled bills", async () => {
    render(
      <BillsTable
        {...baseProps}
        isActive={false}
        bills={[makeBill({ status: "paid" })]}
      />,
    );

    fireEvent.click(screen.getByTestId("bill-row-overflow"));
    expect(await screen.findByRole("menuitem", { name: "Print" })).toBeInTheDocument();
    expect(
      screen.queryByRole("menuitem", { name: "Close without payment" }),
    ).not.toBeInTheDocument();
  });
});
