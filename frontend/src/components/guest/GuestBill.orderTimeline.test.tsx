/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

import GuestBill from "@/components/guest/GuestBill";
import { getGuestOrdersByBillNumber, type Order } from "@/api/orders";
import { asDollars } from "@/types/money";
import toast from "react-hot-toast";
import type { BillWithItemsResponse } from "@/api/bills";

jest.mock("react-hot-toast", () => {
  const toastFn = Object.assign(jest.fn(), {
    success: jest.fn(),
    error: jest.fn(),
    dismiss: jest.fn(),
  });
  return { __esModule: true, default: toastFn };
});

jest.mock("@/api/bills", () => ({
  getOpenBillByTableCode: jest.fn(),
  getBillByNumber: jest.fn(),
  // Real fallback behavior: token-first, bill_number for pre-migration bills.
  guestBillRef: (b: { bill_number: string; public_token?: string }) =>
    b.public_token || b.bill_number,
  tryGuestBillRef: (b: { bill_number: string; public_token?: string }) =>
    b.public_token?.trim() || b.bill_number || null,
  isActiveBillStatus: (status: string | undefined) =>
    status === "open" || status === "partial",
}));

jest.mock("@/api/orders", () => ({
  getGuestOrdersByBillNumber: jest.fn(),
  guestCancelOrder: jest.fn(),
  parseOrderItems: (items: unknown) => {
    if (Array.isArray(items)) return items;
    if (typeof items !== "string" || !items.trim()) return [];
    try {
      const parsed = JSON.parse(items) as unknown;
      return Array.isArray(parsed) ? parsed : [];
    } catch {
      return [];
    }
  },
}));

jest.mock("@/hooks/usePolling", () => ({
  usePolling: ({ callback }: { callback: () => Promise<void> | void }) => {
    void callback();
    return { startPolling: jest.fn(), stopPolling: jest.fn(), isPolling: false };
  },
}));

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({ customer: null, isAuthenticated: false }),
}));

jest.mock("@/components/guest/PaymentSection", () => {
  const MockPaymentSection = () => <div data-testid="payment-section" />;
  MockPaymentSection.displayName = "MockPaymentSection";
  return MockPaymentSection;
});

jest.mock("@/components/common/CurrencyConverter", () => {
  const CurrencyPrice = ({ amount }: { amount: number }) => (
    <span>{amount}</span>
  );
  return {
    __esModule: true,
    default: ({ amount }: { amount: number }) => <span>{amount}</span>,
    CurrencyPrice,
  };
});

const bill: BillWithItemsResponse = {
  bill: {
    id: 1,
    business_id: 42,
    table_id: 3,
    bill_number: "B42-opaque",
    notes: "",
    items: "[]",
    subtotal: asDollars(30),
    tax_amount: asDollars(0),
    service_fee_amount: asDollars(0),
    total_amount: asDollars(30),
    paid_amount: asDollars(0),
    tip_amount: asDollars(0),
    currency: "USD",
    status: "open",
    settlement_address: "0x123",
    tipping_address: "0x456",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
  items: [],
};

const makeOrder = (overrides: Partial<Order>): Order =>
  ({
    id: 1,
    bill_id: 1,
    business_id: 42,
    order_number: "O-1",
    status: "pending",
    items: JSON.stringify([
      {
        menu_item_name: "Burger",
        quantity: 1,
        subtotal: 10,
        options: [],
        special_requests: "",
      },
    ]),
    currency: "USD",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...overrides,
  }) as Order;

const business = {
  id: 42,
  name: "Cafe",
  tax_rate: 0,
  service_fee_rate: 0,
} as any;

describe("GuestBill order status timeline (G-6)", () => {
  beforeEach(() => jest.clearAllMocks());

  it("renders approved / in_kitchen / ready orders with the stepper (not just pending)", async () => {
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({
      orders: [makeOrder({ id: 1, status: "in_kitchen" })],
      total: 1,
    });

    render(
      <GuestBill
        bill={bill}
        business={business}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("guest-bill-kitchen-toggle")).toBeInTheDocument();
    });
    // Kitchen status is expanded by default so diners see pending→kitchen→ready.
    expect(screen.getByTestId("guest-bill-kitchen-toggle")).toHaveAttribute(
      "aria-expanded",
      "true",
    );
    expect(screen.getByText(/Burger/)).toBeInTheDocument();
    expect(screen.getByText("orders.statusInKitchen")).toHaveAttribute(
      "aria-current",
      "step",
    );
    expect(screen.queryByText("orders.cancel")).not.toBeInTheDocument();
  });

  it("shows the backend cancel_reason on cancelled orders", async () => {
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({
      orders: [
        makeOrder({ id: 2, status: "cancelled", cancel_reason: "Out of buns" }),
      ],
      total: 1,
    });

    render(
      <GuestBill
        bill={bill}
        business={business}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText(/Out of buns/)).toBeInTheDocument();
    });
    expect(screen.getByText(/orders.cancelReasonLabel/)).toBeInTheDocument();
  });

  it("announces a cancelled line to screen readers without dimming the row", async () => {
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({
      orders: [makeOrder({ id: 4, status: "cancelled" })],
      total: 1,
    });

    render(
      <GuestBill
        bill={bill}
        business={business}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
      />,
    );

    const name = await screen.findByText(/Burger/);
    const row = name.closest("li");
    expect(row).toBeTruthy();
    expect(row).not.toHaveClass("opacity-70");
    expect(row?.querySelector(".sr-only")).toHaveTextContent(
      /orders\.cancelled/,
    );
  });

  it("toasts exactly once when a staff cancellation is first observed", async () => {
    (getGuestOrdersByBillNumber as jest.Mock)
      .mockResolvedValueOnce({
        orders: [makeOrder({ id: 3, status: "pending" })],
        total: 1,
      })
      .mockResolvedValue({
        orders: [
          makeOrder({ id: 3, status: "cancelled", cancelled_by: "staff:7" }),
        ],
        total: 1,
      });

    render(
      <GuestBill
        bill={bill}
        business={business}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
      />,
    );

    await waitFor(() => {
      const staffCancelToasts = (
        toast as unknown as jest.Mock
      ).mock.calls.filter(
        (call: [string]) => call[0] === "orders.staffCancelledToast",
      );
      expect(staffCancelToasts).toHaveLength(1);
    });
  });
});
