/** @jest-environment jsdom */
/**
 * #528 — kitchen-accepted guest cancel must not say "already cancelled",
 * and a 409 must refresh the pending row so Cancel does not linger.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import toast from "react-hot-toast";

import GuestBill from "@/components/guest/GuestBill";
import { getBillByNumber, type BillWithItemsResponse } from "@/api/bills";
import {
  getGuestOrdersByBillNumber,
  guestCancelOrder,
  type Order,
} from "@/api/orders";
import { asDollars } from "@/types/money";

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
  useGuestTranslation: () => ({ t: (key: string) => key, currentLanguage: "en" }),
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
  const CurrencyPrice = ({ amount }: { amount: number }) => <span>{amount}</span>;
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
    public_token: "tok-guest-bill",
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
    id: 7,
    bill_id: 1,
    business_id: 42,
    order_number: "O-7",
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

function sanitized409(code: string, error: string) {
  return Object.assign(new Error("request failed"), {
    status: 409,
    code,
    response: { status: 409, data: { code, error } },
  });
}

async function confirmCancel() {
  const user = userEvent.setup();
  const cancel = await screen.findByRole("button", { name: "orders.cancel" });
  await user.click(cancel);
  await user.click(screen.getByRole("button", { name: "orders.cancel" }));
}

describe("GuestBill cancel 409 (#528)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (getBillByNumber as jest.Mock).mockResolvedValue(bill);
  });

  it("shows kitchen-accepted copy and drops Cancel after the list refreshes", async () => {
    let current = [makeOrder({ status: "pending" })];
    (getGuestOrdersByBillNumber as jest.Mock).mockImplementation(() =>
      Promise.resolve({ orders: current, total: current.length }),
    );
    (guestCancelOrder as jest.Mock).mockImplementation(async () => {
      current = [makeOrder({ status: "approved" })];
      throw sanitized409(
        "order_already_accepted",
        "This order has already been accepted by the kitchen",
      );
    });

    render(
      <GuestBill
        bill={bill}
        business={business}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
        onBillUpdate={jest.fn()}
      />,
    );

    await confirmCancel();

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("orders.alreadyAccepted");
    });
    expect(toast.error).not.toHaveBeenCalledWith("orders.alreadyCancelled");
    expect(guestCancelOrder).toHaveBeenCalledWith("T1", 7);
    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: "orders.cancel" }),
      ).not.toBeInTheDocument();
    });
    expect(screen.getByText("orders.statusAccepted")).toHaveAttribute(
      "aria-current",
      "step",
    );
    expect(getBillByNumber).toHaveBeenCalled();
  });

  it("shows cancelled copy on a double-cancel 409 and does not claim staff cancelled", async () => {
    let current = [makeOrder({ status: "pending" })];
    (getGuestOrdersByBillNumber as jest.Mock).mockImplementation(() =>
      Promise.resolve({ orders: current, total: current.length }),
    );
    (guestCancelOrder as jest.Mock).mockImplementation(async () => {
      current = [makeOrder({ status: "cancelled", cancelled_by: "guest" })];
      throw sanitized409(
        "order_already_cancelled",
        "This order has already been cancelled",
      );
    });

    render(
      <GuestBill
        bill={bill}
        business={business}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
        onBillUpdate={jest.fn()}
      />,
    );

    await confirmCancel();

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("orders.alreadyCancelled");
    });
    expect(toast.error).not.toHaveBeenCalledWith("orders.alreadyAccepted");
    expect(toast).not.toHaveBeenCalledWith(
      "orders.staffCancelledToast",
      expect.anything(),
    );
    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: "orders.cancel" }),
      ).not.toBeInTheDocument();
    });
  });

  it("still toasts the conflict when the post-409 refresh fails", async () => {
    jest.spyOn(console, "warn").mockImplementation(() => undefined);
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({
      orders: [makeOrder({ status: "pending" })],
      total: 1,
    });
    (guestCancelOrder as jest.Mock).mockImplementation(async () => {
      (getGuestOrdersByBillNumber as jest.Mock).mockRejectedValue(
        new Error("poll failed"),
      );
      throw sanitized409(
        "order_already_accepted",
        "This order has already been accepted by the kitchen",
      );
    });

    render(
      <GuestBill
        bill={bill}
        business={business}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
        onBillUpdate={jest.fn()}
      />,
    );

    await confirmCancel();

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("orders.alreadyAccepted");
    });
  });
});
