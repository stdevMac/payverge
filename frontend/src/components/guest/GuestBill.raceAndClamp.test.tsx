/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

import GuestBill from "@/components/guest/GuestBill";
import { type BillWithItemsResponse } from "@/api/bills";
import { getBillByNumber } from "@/api/bills";
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

// Capture the poll callback so we can drive loadBillData deterministically and
// create an out-of-order resolution (the real race E-02 guards against).
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

// Expose PaymentSection's `amount` prop so we can assert the clamped value.
jest.mock("@/components/guest/PaymentSection", () => {
  const MockPaymentSection = ({ amount }: { amount: number }) => (
    <span data-testid="pay-amount">{amount}</span>
  );
  MockPaymentSection.displayName = "MockPaymentSection";
  return MockPaymentSection;
});

jest.mock("@/components/splitting/GuestBillSplitPanel", () => {
  const MockGuestBillSplitPanel = () => null;
  MockGuestBillSplitPanel.displayName = "MockGuestBillSplitPanel";
  return MockGuestBillSplitPanel;
});

jest.mock("@/components/common/CurrencyConverter", () => {
  const CurrencyPrice = ({ amount }: { amount: number }) => <span>{amount}</span>;
  return {
    __esModule: true,
    default: ({ amount }: { amount: number }) => <span>{amount}</span>,
    CurrencyPrice,
  };
});

const buildBill = (
  overrides: Partial<BillWithItemsResponse["bill"]>,
): BillWithItemsResponse => ({
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
    ...overrides,
  },
  items: [],
});

const business = { id: 42, name: "Cafe", tax_rate: 0, service_fee_rate: 0 } as any;

describe("GuestBill bill-load race guard (E-02)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pollCallback = null;
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({
      orders: [],
      total: 0,
    });
  });

  it("drops a stale 'open' response that resolves after a fresh 'paid' response", async () => {
    const onBillUpdate = jest.fn();

    // Two in-flight loadBillData calls: the first (older) resolves to the now-
    // outdated OPEN bill; the second (newer) resolves to PAID. We resolve them
    // OUT OF ORDER (newer first, then older) to simulate the production race.
    let resolveOpen!: (v: BillWithItemsResponse) => void;
    let resolvePaid!: (v: BillWithItemsResponse) => void;
    const openPromise = new Promise<BillWithItemsResponse>((r) => {
      resolveOpen = r;
    });
    const paidPromise = new Promise<BillWithItemsResponse>((r) => {
      resolvePaid = r;
    });
    (getBillByNumber as jest.Mock)
      .mockReturnValueOnce(openPromise)
      .mockReturnValueOnce(paidPromise);

    render(
      <GuestBill
        bill={buildBill({})}
        business={business}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
        onBillUpdate={onBillUpdate}
      />,
    );

    await waitFor(() => expect(pollCallback).not.toBeNull());

    // Fire two polls — load #1 (older) and load #2 (newer) are both in flight.
    const poll1 = pollCallback!();
    const poll2 = pollCallback!();

    // Resolve the NEWER load first (paid), then the OLDER (open).
    resolvePaid(buildBill({ status: "paid", paid_amount: asDollars(45.8) }));
    resolveOpen(buildBill({ status: "open", paid_amount: asDollars(0) }));
    await Promise.all([poll1, poll2]);

    // The guard must drop the stale OPEN write; the last published status is PAID.
    await waitFor(() => expect(onBillUpdate).toHaveBeenCalled());
    const lastArg = onBillUpdate.mock.calls.at(-1)?.[0] as BillWithItemsResponse;
    expect(lastArg.bill.status).toBe("paid");
    // The stale open bill must never have been published.
    const publishedOpen = onBillUpdate.mock.calls.some(
      ([arg]) => (arg as BillWithItemsResponse).bill.status === "open",
    );
    expect(publishedOpen).toBe(false);
  });
});

describe("GuestBill remaining-amount clamp (B-02)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pollCallback = null;
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({
      orders: [],
      total: 0,
    });
  });

  it("never feeds PaymentSection a negative amount on an overpaid partial bill", async () => {
    // paid_amount > total_amount (e.g. a large tip pushed paid over total) on a
    // still-payable partial bill. The payment base must clamp to 0, not -10.
    render(
      <GuestBill
        bill={buildBill({
          status: "partial",
          total_amount: asDollars(40),
          paid_amount: asDollars(50),
        })}
        business={business}
        tableCode="T1"
        selectedLanguage="en"
        onPaymentComplete={jest.fn()}
      />,
    );

    const amount = await screen.findByTestId("pay-amount");
    expect(Number(amount.textContent)).toBe(0);
  });
});
