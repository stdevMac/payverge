/** @jest-environment jsdom */
import React from "react";
import { render, waitFor } from "@testing-library/react";

import GuestBill from "@/components/guest/GuestBill";
import {
  getBillByNumber,
  getOpenBillByTableCode,
  type BillWithItemsResponse,
} from "@/api/bills";
import { getGuestOrdersByBillNumber } from "@/api/orders";
import { asDollars } from "@/types/money";

jest.mock("@/api/bills", () => ({
  getOpenBillByTableCode: jest.fn(),
  getBillByNumber: jest.fn(),
  guestBillRef: (b: { public_token?: string }) => b.public_token || "",
  tryGuestBillRef: (b: { bill_number: string; public_token?: string }) =>
    b.public_token?.trim() || b.bill_number || null,
  isActiveBillStatus: (status: string | undefined) =>
    status === "open" || status === "partial",
}));

jest.mock("@/api/orders", () => ({
  getGuestOrdersByBillNumber: jest.fn(),
}));

let pollCallback: (() => Promise<void>) | null = null;
jest.mock("@/hooks/usePolling", () => ({
  usePolling: ({ callback }: { callback: () => Promise<void> }) => {
    pollCallback = callback;
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
  const MockPaymentSection = () => null;
  MockPaymentSection.displayName = "MockPaymentSection";
  return MockPaymentSection;
});

jest.mock("@/components/splitting/GuestBillSplitPanel", () => {
  const MockSplitPanel = () => null;
  MockSplitPanel.displayName = "MockSplitPanel";
  return MockSplitPanel;
});

jest.mock("@/components/common/CurrencyConverter", () => {
  const CurrencyPrice = ({ amount }: { amount: number }) => <span>{amount}</span>;
  return {
    __esModule: true,
    default: CurrencyPrice,
    CurrencyPrice,
  };
});

const buildBill = (total: number): BillWithItemsResponse => ({
  bill: {
    id: 1,
    business_id: 42,
    table_id: 3,
    bill_number: "B42-display-only",
    public_token: "0123456789abcdef0123456789abcdef",
    notes: "",
    items: "[]",
    subtotal: asDollars(total),
    tax_amount: asDollars(0),
    service_fee_amount: asDollars(0),
    total_amount: asDollars(total),
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
});

describe("GuestBill polling resilience", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pollCallback = null;
    jest.spyOn(console, "error").mockImplementation(() => undefined);
    jest.spyOn(console, "warn").mockImplementation(() => undefined);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("keeps a successful bill refresh when the optional order-status poll fails", async () => {
    const refreshedBill = buildBill(24);
    const onBillUpdate = jest.fn();
    (getGuestOrdersByBillNumber as jest.Mock).mockRejectedValue(
      new Error("temporary order endpoint failure"),
    );
    (getBillByNumber as jest.Mock).mockResolvedValue(refreshedBill);
    (getOpenBillByTableCode as jest.Mock).mockRejectedValue(
      new Error("fallback should not run"),
    );

    render(
      <GuestBill
        bill={buildBill(30)}
        business={{ id: 42, name: "Cafe", tax_rate: 0, service_fee_rate: 0 } as any}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
        onBillUpdate={onBillUpdate}
      />,
    );

    await waitFor(() => expect(pollCallback).not.toBeNull());
    await expect(pollCallback!()).resolves.toBeUndefined();
    expect(onBillUpdate).toHaveBeenCalledWith(refreshedBill);
  });
});
