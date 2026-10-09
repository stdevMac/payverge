/** @jest-environment jsdom */
/**
 * The guest bill's "≈ USDC" estimate advertises a crypto tender. When the
 * instance reports crypto off (no settlement RPC, or the public demo, whose
 * guard refuses every crypto payment route) the estimate must not render.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

import GuestBill from "@/components/guest/GuestBill";
import { type BillWithItemsResponse } from "@/api/bills";
import { getGuestOrdersByBillNumber } from "@/api/orders";
import { asDollars } from "@/types/money";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import {
  resetInstanceCacheForTests,
  setInstanceForTests,
} from "@/hooks/useInstance";

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
    default: ({
      amount,
      showUSDCConversion,
    }: {
      amount: number;
      showUSDCConversion?: boolean;
    }) => (
      <span
        data-testid="total-converter"
        data-usdc={showUSDCConversion ? "on" : "off"}
      >
        {amount}
      </span>
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

function renderBill() {
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
}

function withInstance(crypto: boolean, demoMode: boolean) {
  setInstanceForTests(
    parseInstanceInfo({
      product_name: "Payverge",
      registration_mode: "invite",
      features: { crypto },
      demo: { enabled: demoMode, mode: demoMode },
    }),
  );
}

describe("GuestBill USDC estimate", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({
      orders: [],
      total: 0,
    });
  });

  afterEach(() => resetInstanceCacheForTests());

  it("hides the estimate on the public demo, which refuses crypto payments", async () => {
    withInstance(false, true);
    renderBill();
    const total = await screen.findByTestId("total-converter");
    expect(total).toHaveAttribute("data-usdc", "off");
  });

  it("keeps the estimate when the instance offers crypto", async () => {
    withInstance(true, false);
    renderBill();
    const total = await screen.findByTestId("total-converter");
    expect(total).toHaveAttribute("data-usdc", "on");
  });
});
