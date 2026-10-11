/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";

import GuestBill from "@/components/guest/GuestBill";
import { type BillWithItemsResponse } from "@/api/bills";
import { getGuestOrdersByBillNumber } from "@/api/orders";
import { asDollars } from "@/types/money";

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
}));

jest.mock("@/hooks/usePolling", () => ({
  usePolling: () => ({
    startPolling: jest.fn(),
    stopPolling: jest.fn(),
    isPolling: false,
  }),
}));

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({ customer: null, isAuthenticated: false }),
}));

// PaymentSection mock exposes a button that fires onPaymentComplete with the
// FULL payment detail object (method + tx id), so we can assert GuestBill
// forwards them to its parent onPaymentComplete prop.
jest.mock("@/components/guest/PaymentSection", () => {
  const MockPaymentSection = ({
    onPaymentComplete,
  }: {
    onPaymentComplete: (d: {
      totalPaid: number;
      tipAmount: number;
      paymentMethod: string;
      transactionId?: string;
    }) => void;
  }) => (
    <button
      data-testid="complete-payment"
      onClick={() =>
        onPaymentComplete({
          totalPaid: 45.8,
          tipAmount: 5,
          paymentMethod: "cross_chain_payment",
          transactionId: "0xdeadbeef",
        })
      }
    >
      complete
    </button>
  );
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

const openBill: BillWithItemsResponse = {
  bill: {
    id: 1,
    business_id: 42,
    table_id: 3,
    bill_number: "B42-opaque",
    notes: "",
    items: "[]",
    subtotal: asDollars(40),
    tax_amount: asDollars(0),
    service_fee_amount: asDollars(0),
    total_amount: asDollars(45.8),
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

describe("GuestBill onPaymentComplete forwarding", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({
      orders: [],
      total: 0,
    });
  });

  it("forwards paymentMethod and transactionId to the parent handler", async () => {
    const onPaymentComplete = jest.fn();

    render(
      <GuestBill
        bill={openBill}
        business={
          { id: 42, name: "Cafe", tax_rate: 0, service_fee_rate: 0 } as any
        }
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={onPaymentComplete}
      />,
    );

    fireEvent.click(await screen.findByTestId("complete-payment"));

    await waitFor(() => {
      expect(onPaymentComplete).toHaveBeenCalledWith({
        totalPaid: 45.8,
        tipAmount: 5,
        paymentMethod: "cross_chain_payment",
        transactionId: "0xdeadbeef",
      });
    });
  });
});
