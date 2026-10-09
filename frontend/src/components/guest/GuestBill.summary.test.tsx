/** @jest-environment jsdom */
import React from "react";
import { render, screen, within } from "@testing-library/react";

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

jest.mock("@/components/guest/PaymentSection", () => {
  const MockPaymentSection = () => <div data-testid="payment-section" />;
  MockPaymentSection.displayName = "MockPaymentSection";
  return MockPaymentSection;
});

jest.mock("@/components/common/CurrencyConverter", () => {
  const CurrencyPrice = ({ amount }: { amount: number }) => (
    <span data-testid="price">{amount}</span>
  );
  return {
    __esModule: true,
    default: ({ amount }: { amount: number }) => (
      <span data-testid="price">{amount}</span>
    ),
    CurrencyPrice,
  };
});

const taxedBill: BillWithItemsResponse = {
  bill: {
    id: 1,
    business_id: 42,
    table_id: 3,
    bill_number: "B42-tax",
    notes: "",
    items: "[]",
    subtotal: asDollars(40),
    tax_amount: asDollars(3.2),
    service_fee_amount: asDollars(0),
    total_amount: asDollars(43.2),
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

describe("GuestBill summary", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({
      orders: [],
      total: 0,
    });
  });

  it("renders a Subtotal row so the arithmetic is legible", async () => {
    render(
      <GuestBill
        bill={taxedBill}
        business={
          { id: 42, name: "Cafe", tax_rate: 8, service_fee_rate: 0 } as any
        }
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
      />,
    );

    // The summary section (header bill.summary) must include a Subtotal row
    // with the subtotal value (40) — not jump straight from Tax to Total.
    const summaryHeading = await screen.findByText("bill.summary");
    const summaryCard = summaryHeading.closest("div")!;
    const subtotalLabel = within(summaryCard).getByText("bill.subtotal");
    const subtotalRow = subtotalLabel.closest("div")!;
    expect(
      within(subtotalRow).getByTestId("price").textContent,
    ).toBe("40");
  });

  it("formats bill created_at in the venue timezone, not the device timezone", async () => {
    // 2026-07-23T01:14:46Z is 9:14 PM on Jul 22 in America/New_York, but
    // 5:14 AM on Jul 23 in Asia/Dubai (the audit host TZ). Guests at the
    // table must see restaurant-local time (FIND-013 honesty for bill stamps).
    const nyBill = {
      ...taxedBill,
      bill: {
        ...taxedBill.bill,
        created_at: "2026-07-23T01:14:46.475623Z",
      },
    };
    render(
      <GuestBill
        bill={nyBill}
        business={
          {
            id: 42,
            name: "Cafe",
            tax_rate: 8,
            service_fee_rate: 0,
            timezone: "America/New_York",
          } as any
        }
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
      />,
    );

    // Accept either 9:14 PM or 21:14 depending on locale hour12 preference.
    expect(
      await screen.findByText(/Jul 22, 2026,\s*(9:14\s*PM|21:14)/i),
    ).toBeInTheDocument();
    expect(screen.queryByText(/5:14\s*AM/i)).not.toBeInTheDocument();
  });
});
