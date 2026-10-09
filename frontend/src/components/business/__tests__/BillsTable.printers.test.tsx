/** @jest-environment jsdom */
/**
 * Print row action in BillsTable — gated on print:bill for staff, always on for owners.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

let mockIsStaffUser = false;
let mockStaffPermissions: string[] = [];

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isStaffUser: mockIsStaffUser,
  }),
}));

jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: mockStaffPermissions,
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

const I18N_STUB: Record<string, string> = {
  "billStatuses.open": "Open",
  "billStatuses.partial": "Partial",
  "billStatuses.paid": "Paid",
  "billStatuses.closed": "Closed",
  "buttons.print": "Print",
  "buttons.closeWithoutPayment": "Close without payment",
  "buttons.moreActionsAria": "More actions for bill {number}",
  view: "View",
  billLabel: "bill",
};
const tString = (key: string) => I18N_STUB[key] ?? key;

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

async function openPrintMenu() {
  fireEvent.click(screen.getByTestId("bill-row-overflow"));
  return screen.findByRole("menuitem", { name: "Print" });
}

describe("BillsTable — Print row action", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockIsStaffUser = false;
    mockStaffPermissions = [];
  });

  it("renders a Print action for owners", async () => {
    render(<BillsTable {...baseProps} />);
    expect(await openPrintMenu()).toBeInTheDocument();
  });

  it("renders Print for staff with print:bill", async () => {
    mockIsStaffUser = true;
    mockStaffPermissions = ["print:bill"];
    render(<BillsTable {...baseProps} />);
    expect(await openPrintMenu()).toBeInTheDocument();
  });

  it("does not render Print for staff without print:bill", async () => {
    mockIsStaffUser = true;
    mockStaffPermissions = ["bills:read"];
    render(<BillsTable {...baseProps} />);
    fireEvent.click(screen.getByTestId("bill-row-overflow"));
    expect(await screen.findByRole("menuitem", { name: "Close without payment" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Print" })).toBeNull();
  });

  it("passes the operator locale to createBillPrintJob", async () => {
    const { createBillPrintJob } = jest.requireMock("@/api/print");
    (createBillPrintJob as jest.Mock).mockResolvedValueOnce({
      id: 11,
      payload_html: "<html></html>",
    });
    render(<BillsTable {...baseProps} locale="es-ar" />);
    fireEvent.click(await openPrintMenu());
    await waitFor(() =>
      expect(createBillPrintJob).toHaveBeenCalledWith(42, 1, "es-ar"),
    );
  });
});
