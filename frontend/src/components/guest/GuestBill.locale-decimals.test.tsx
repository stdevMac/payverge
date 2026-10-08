/**
 * Issue #561 — guest bill tax/service-fee sublabels and the USDC chip must
 * follow the diner locale's decimal convention (de: 8,875% / USDC 34,99),
 * not raw JS number interpolation / toFixed.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

import GuestBill from "@/components/guest/GuestBill";
import { type BillWithItemsResponse } from "@/api/bills";
import { getGuestOrdersByBillNumber } from "@/api/orders";
import { asDollars } from "@/types/money";

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
    currentLanguage: "de",
    t: (key: string, params?: Record<string, string | number>) => {
      const templates: Record<string, string> = {
        "bill.tax": "Steuer ({rate}%)",
        "bill.serviceFee": "Servicegebühr ({rate}%)",
        "bill.summary": "Zusammenfassung",
        "bill.subtotal": "Zwischensumme",
        "bill.total": "Gesamt",
      };
      const template = templates[key] ?? key;
      if (!params) return template;
      return Object.entries(params).reduce(
        (s, [k, v]) => s.replaceAll(`{${k}}`, String(v)),
        template,
      );
    },
  }),
}));

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({ customer: null, isAuthenticated: false }),
}));

jest.mock("@/components/guest/PaymentSection", () => {
  const MockPaymentSection = () => <div data-testid="payment-section" />;
  MockPaymentSection.displayName = "MockPaymentSection";
  return MockPaymentSection;
});

jest.mock("@/api/currency", () => {
  const actual = jest.requireActual("@/api/currency") as typeof import("@/api/currency");
  return {
    ...actual,
    convertAmount: jest.fn(async (amount: number) => ({
      converted_amount: amount,
    })),
    getGuestMenuTranslations: jest.fn(async () => ({ translations: {} })),
  };
});

const bill: BillWithItemsResponse = {
  bill: {
    id: 1,
    business_id: 42,
    table_id: 3,
    bill_number: "B42-locale",
    notes: "",
    items: "[]",
    subtotal: asDollars(30),
    tax_amount: asDollars(2.66),
    service_fee_amount: asDollars(2.33),
    total_amount: asDollars(34.99),
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

describe("GuestBill locale decimals (#561)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({
      orders: [],
      total: 0,
    });
  });

  it("formats de-DE tax/service rates and the USDC chip with comma decimals", async () => {
    render(
      <GuestBill
        bill={bill}
        business={
          {
            id: 42,
            name: "Cafe",
            tax_rate: 8.875,
            service_fee_rate: 10.5,
          } as any
        }
        tableCode="T1"
        selectedLanguage="de"
        onPaymentComplete={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Steuer (8,875%)")).toBeInTheDocument();
    });
    expect(screen.getByText("Servicegebühr (10,5%)")).toBeInTheDocument();
    expect(screen.queryByText(/8\.875/)).not.toBeInTheDocument();
    expect(screen.queryByText(/10\.5%/)).not.toBeInTheDocument();

    const chip = await screen.findByText(/USDC/);
    expect(chip.textContent).toContain("34,99");
    expect(chip.textContent).not.toContain("34.99");
  });
});
