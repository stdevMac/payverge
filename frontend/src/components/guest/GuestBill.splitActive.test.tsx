/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

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

jest.mock("@/api/orders", () => ({ getGuestOrdersByBillNumber: jest.fn() }));

jest.mock("@/hooks/usePolling", () => ({
  usePolling: () => ({ startPolling: jest.fn(), stopPolling: jest.fn(), isPolling: false }),
}));

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t: (key: string) => key }),
}));

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({ customer: null, isAuthenticated: false }),
}));

// The full-bill PaymentSection is identified by this testid.
jest.mock("@/components/guest/PaymentSection", () => {
  const MockPaymentSection = () => <div data-testid="full-bill-pay">full bill pay</div>;
  MockPaymentSection.displayName = "MockPaymentSection";
  return MockPaymentSection;
});

// The split panel exposes buttons to toggle its "active share" state so we can
// assert the parent hides/reveals the full-bill section accordingly.
jest.mock("@/components/splitting/GuestBillSplitPanel", () => {
  const MockPanel = ({ onActiveChange }: { onActiveChange?: (a: boolean) => void }) => (
    <div>
      <button data-testid="hold-share" onClick={() => onActiveChange?.(true)}>
        hold
      </button>
      <button data-testid="release-share" onClick={() => onActiveChange?.(false)}>
        release
      </button>
    </div>
  );
  MockPanel.displayName = "MockGuestBillSplitPanel";
  return MockPanel;
});

jest.mock("@/components/common/CurrencyConverter", () => {
  const CurrencyPrice = ({ amount }: { amount: number }) => <span>{amount}</span>;
  return { __esModule: true, default: CurrencyPrice, CurrencyPrice };
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

describe("GuestBill hides the full-bill Pay Now while a split share is active", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({ orders: [], total: 0 });
  });

  it("hides the full-bill section when a share is held and restores it on release", async () => {
    render(
      <GuestBill
        bill={openBill}
        business={{ id: 42, name: "Cafe", tax_rate: 0, service_fee_rate: 0 } as any}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
      />,
    );

    // Full-bill pay is primary; split is collapsed by default.
    expect(await screen.findByTestId("full-bill-pay")).toBeInTheDocument();
    expect(screen.getByTestId("guest-bill-split-toggle")).toHaveAttribute(
      "aria-expanded",
      "false",
    );

    // Expand split disclosure to reach the panel controls.
    fireEvent.click(screen.getByTestId("guest-bill-split-toggle"));
    expect(await screen.findByTestId("hold-share")).toBeInTheDocument();

    // Holding a split share hides the full-bill Pay Now (can't pay the whole
    // bill at an amount that ignores held shares).
    fireEvent.click(screen.getByTestId("hold-share"));
    await waitFor(() =>
      expect(screen.queryByTestId("full-bill-pay")).not.toBeInTheDocument(),
    );

    // Releasing the share restores it.
    fireEvent.click(screen.getByTestId("release-share"));
    expect(await screen.findByTestId("full-bill-pay")).toBeInTheDocument();
  });
});
