/** @jest-environment jsdom */
/**
 * PV-LIVE-20260720-001: GuestBill must not throw into TableErrorBoundary when
 * the open-bill API returns a blank public_token. Shows recoverable UI instead.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

import GuestBill from "@/components/guest/GuestBill";
import type { BillWithItemsResponse } from "@/api/bills";
import { asDollars } from "@/types/money";

jest.mock("@/api/bills", () => {
  const actual = jest.requireActual("@/api/bills");
  return {
    ...actual,
    getOpenBillByTableCode: jest.fn(),
    getBillByNumber: jest.fn(),
    isActiveBillStatus: (status: string | undefined) =>
      status === "open" || status === "partial",
  };
});

jest.mock("@/api/orders", () => ({
  getGuestOrdersByBillNumber: jest.fn().mockResolvedValue({ orders: [], total: 0 }),
}));

jest.mock("@/hooks/usePolling", () => ({
  usePolling: () => ({
    startPolling: jest.fn(),
    stopPolling: jest.fn(),
    isPolling: false,
  }),
}));

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
    currentLanguage: "en",
  }),
}));

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({ customer: null, isAuthenticated: false }),
}));

jest.mock("@/components/guest/PaymentSection", () => {
  const Mock = () => <div data-testid="payment-section" />;
  Mock.displayName = "MockPaymentSection";
  return Mock;
});

jest.mock("@/components/splitting/GuestBillSplitPanel", () => {
  const Mock = () => <div data-testid="split-panel" />;
  Mock.displayName = "MockGuestBillSplitPanel";
  return Mock;
});

jest.mock("@/components/common/CurrencyConverter", () => {
  const CurrencyPrice = ({ amount }: { amount: number }) => <span>{amount}</span>;
  return {
    __esModule: true,
    default: ({ amount }: { amount: number }) => <span>{amount}</span>,
    CurrencyPrice,
  };
});

describe("GuestBill missing public_token (PV-LIVE-20260720-001)", () => {
  const blankTokenBill: BillWithItemsResponse = {
    bill: {
      id: 1,
      business_id: 42,
      table_id: 3,
      bill_number: "B42-no-token",
      public_token: "",
      notes: "",
      items: "[]",
      subtotal: asDollars(18.3),
      tax_amount: asDollars(0),
      service_fee_amount: asDollars(0),
      total_amount: asDollars(18.3),
      paid_amount: asDollars(0),
      tip_amount: asDollars(0),
      currency: "USD",
      status: "open",
      settlement_address: "0x123",
      tipping_address: "0x456",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    },
    items: [
      {
        id: "item-1",
        menu_item_id: "menu-item-1",
        name: "Harvest Bowl",
        price: asDollars(18.3),
        quantity: 1,
        options: [],
        subtotal: asDollars(18.3),
      },
    ],
  };

  it("renders a recoverable alert instead of throwing when public_token is blank", () => {
    expect(() =>
      render(
        <GuestBill
          bill={blankTokenBill}
          business={{
            id: 42,
            name: "AI Pro Lounge",
            tax_rate: 0,
            service_fee_rate: 0,
          } as never}
          tableCode="demo-2-ai-pro-table-02"
          onPaymentComplete={jest.fn()}
        />,
      ),
    ).not.toThrow();

    expect(screen.getByTestId("guest-bill-missing-token")).toBeInTheDocument();
    expect(screen.getByText("bill.missingAccessTokenTitle")).toBeInTheDocument();
    expect(screen.getByText("bill.missingAccessTokenBody")).toBeInTheDocument();
    expect(screen.queryByTestId("payment-section")).not.toBeInTheDocument();
    expect(screen.queryByTestId("split-panel")).not.toBeInTheDocument();
  });
});
