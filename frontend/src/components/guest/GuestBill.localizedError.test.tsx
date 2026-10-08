/** @jest-environment jsdom */
/**
 * H1 (frontend) regression lock: the migrated GuestBill loyalty error path must
 * localize a CODED backend error through the live translateApiError layer, in
 * the diner's selected storefront language — not surface the raw English string.
 *
 * Before the fix, the redeem/undo catch-blocks did
 *   apiErrorDetail(err) || errMessage(err) || "Failed to redeem points"
 * which returned the backend `error` string verbatim (English) regardless of
 * locale. Now they call getLocalizedApiError(err, currentLanguage), so a coded
 * envelope resolves to its apiErrors.json translation (es-AR voseo here).
 */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";

import GuestBill from "@/components/guest/GuestBill";
import {
  getOpenBillByTableCode,
  type BillWithItemsResponse,
} from "@/api/bills";
import { getGuestOrdersByBillNumber } from "@/api/orders";
import { crmAPI } from "@/api/crm";
import { getLoyaltyRate, redeemPoints } from "@/api/loyalty";
import { asDollars } from "@/types/money";

const toastErrors: string[] = [];
jest.mock("react-hot-toast", () => {
  const toast: any = (msg: string) => toastErrors.push(msg);
  toast.error = (msg: string) => toastErrors.push(msg);
  toast.success = jest.fn();
  return { __esModule: true, default: toast };
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
}));

jest.mock("@/api/crm", () => ({
  crmAPI: { getBusinesses: jest.fn() },
}));

jest.mock("@/api/loyalty", () => ({
  getLoyaltyRate: jest.fn(),
  redeemPoints: jest.fn(),
  undoRedemption: jest.fn(),
}));

jest.mock("@/api/currency", () => ({
  getGuestMenuTranslations: jest.fn().mockResolvedValue({ translations: {} }),
}));

jest.mock("@/hooks/usePolling", () => ({
  usePolling: ({ callback }: { callback: () => Promise<void> | void }) => {
    void callback();
    return { startPolling: jest.fn(), stopPolling: jest.fn(), isPolling: false };
  },
}));

// Diner is in Argentine Spanish; the translator exposes `currentLanguage`
// (which the real provider does and the migrated catch-block reads).
const MESSAGES: Record<string, string> = {
  "bill.redeemPoints": "Use {points} pts ({discount} off)",
  "bill.loyaltyPoints": "You have {count} points",
  "bill.loyaltyEstimate": "Worth ~{amount} off your bill",
};
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    currentLanguage: "es-AR",
    t: (key: string, params?: Record<string, string | number>) => {
      const value = MESSAGES[key] ?? key;
      if (!params) return value;
      return value.replace(/\{(\w+)\}/g, (m, k) =>
        params[k] !== undefined ? String(params[k]) : m,
      );
    },
  }),
}));

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({
    customer: { id: 7, email: "guest@example.com" },
    isAuthenticated: true,
  }),
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

const openBill: BillWithItemsResponse = {
  bill: {
    id: 1,
    business_id: 42,
    table_id: 3,
    bill_number: "B42-loyalty-err",
    notes: "",
    items: "[]",
    subtotal: asDollars(30),
    tax_amount: asDollars(0),
    service_fee_amount: asDollars(0),
    total_amount: asDollars(30),
    paid_amount: asDollars(0),
    tip_amount: asDollars(0),
    currency: "EUR",
    status: "open",
    settlement_address: "0x123",
    tipping_address: "0x456",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  },
  items: [],
};

describe("GuestBill loyalty error is localized (H1)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    toastErrors.length = 0;
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({ orders: [], total: 0 });
    (getOpenBillByTableCode as jest.Mock).mockResolvedValue(openBill);
    (crmAPI.getBusinesses as jest.Mock).mockResolvedValue([
      { business_id: 42, loyalty_points: 500 },
    ]);
    (getLoyaltyRate as jest.Mock).mockResolvedValue(0);
  });

  it("surfaces a CODED redeem failure in the diner's locale (es-AR voseo), not raw English", async () => {
    // Backend rejects with a structured code + an English `error` fallback.
    (redeemPoints as jest.Mock).mockRejectedValue({
      status: 403,
      response: {
        status: 403,
        data: { code: "AUTH_FORBIDDEN", error: "You do not have access to this action." },
      },
    });

    render(
      <GuestBill
        bill={openBill}
        business={{ id: 42, name: "Café", tax_rate: 0, service_fee_rate: 0 } as any}
        tableCode="T1"
        selectedLanguage="es-AR"
        defaultCurrency="EUR"
        displayCurrency="EUR"
        onPaymentComplete={jest.fn()}
        onBillUpdate={jest.fn()}
      />,
    );

    const redeemButton = await screen.findByText(/Use 500 pts/);
    fireEvent.click(redeemButton);

    await waitFor(() => {
      expect(toastErrors).toContain("No tenés acceso a esta acción.");
    });
    // The raw English string is NOT what the diner sees.
    expect(toastErrors).not.toContain("You do not have access to this action.");
  });

  it("surfaces already-applied redeem as loyalty copy, not form validation", async () => {
    (redeemPoints as jest.Mock).mockRejectedValue({
      status: 409,
      response: {
        status: 409,
        data: {
          code: "loyalty_discount_already_applied",
          error:
            "a loyalty discount is already applied to this bill; undo it before redeeming again",
        },
      },
    });

    render(
      <GuestBill
        bill={openBill}
        business={{ id: 42, name: "Café", tax_rate: 0, service_fee_rate: 0 } as any}
        tableCode="T1"
        selectedLanguage="es-AR"
        defaultCurrency="EUR"
        displayCurrency="EUR"
        onPaymentComplete={jest.fn()}
        onBillUpdate={jest.fn()}
      />,
    );

    const redeemButton = await screen.findByText(/Use 500 pts/);
    fireEvent.click(redeemButton);

    await waitFor(() => {
      expect(toastErrors).toContain(
        "Ya hay un descuento de lealtad en esta cuenta. Anulalo antes de canjear de nuevo.",
      );
    });
    expect(toastErrors.join(" ")).not.toMatch(/formulario/i);
  });
});
