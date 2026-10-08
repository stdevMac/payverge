/** @jest-environment jsdom */
/**
 * GuestBill cashier path must localize coded alternative-payment failures
 * through presentGuestPaymentError — never surface raw backend English.
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
import { getLoyaltyRate } from "@/api/loyalty";
import { requestAlternativePayment } from "@/api/alternativePayments";
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

jest.mock("@/api/alternativePayments", () => ({
  requestAlternativePayment: jest.fn(),
}));

jest.mock("@/hooks/usePolling", () => ({
  usePolling: ({ callback }: { callback: () => Promise<void> | void }) => {
    void callback();
    return { startPolling: jest.fn(), stopPolling: jest.fn(), isPolling: false };
  },
}));

const LOCALIZED: Record<string, string> = {
  "bill.cashierGuestLabel": "Guest at counter",
  "bill.cashierAmountInvalid": "Nothing left to pay",
  "bill.cashier": "Cashier",
  "bill.cashierInstructions":
    "Go to the cashier and pay with cash, card, or any other method they accept.",
  "bill.cashierRequestSentNote":
    "Your table has been notified. Pay at the counter — staff will confirm once they receive your payment.",
  "bill.cashierUpdateNote":
    "The cashier will update your payment status automatically",
  "common.close": "Close",
  "payment.errors.pluginUnavailable":
    "This payment method is temporarily unavailable.",
  "payment.errors.cashierRequestFailed":
    "Could not reach the cashier. Please try again or ask staff.",
  "payment.errors.billNotPayable":
    "This bill can no longer accept that payment. Refresh and check the remaining balance, or ask staff.",
  "payment.errors.idempotencyConflict":
    "That payment request was already sent with different details. Refresh and try again.",
  "payment.errors.generic": "Something went wrong. Please try again.",
};

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    currentLanguage: "en",
    t: (key: string, params?: Record<string, string | number>) => {
      const value = LOCALIZED[key] ?? key;
      if (!params) return value;
      return value.replace(/\{(\w+)\}/g, (m, k) =>
        params[k] !== undefined ? String(params[k]) : m,
      );
    },
  }),
}));

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({
    customer: null,
    isAuthenticated: false,
  }),
}));

jest.mock("@/components/guest/PaymentSection", () => {
  const MockPaymentSection = (props: {
    onCashierPayment?: (details: {
      totalPaid: number;
      tipAmount: number;
      paymentMethod: string;
    }) => void;
    amount: number;
  }) => (
    <button
      type="button"
      data-testid="full-bill-cashier"
      onClick={() =>
        props.onCashierPayment?.({
          totalPaid: props.amount,
          tipAmount: 0,
          paymentMethod: "cash",
        })
      }
    >
      Cashier
    </button>
  );
  MockPaymentSection.displayName = "MockPaymentSection";
  return MockPaymentSection;
});

jest.mock("@/components/splitting/GuestBillSplitPanel", () => {
  const Mock = () => null;
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

const openBill: BillWithItemsResponse = {
  bill: {
    id: 1,
    business_id: 42,
    table_id: 3,
    bill_number: "B42-cashier-err",
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

describe("GuestBill cashier payment error is localized", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    toastErrors.length = 0;
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({
      orders: [],
      total: 0,
    });
    (getOpenBillByTableCode as jest.Mock).mockResolvedValue(openBill);
    (crmAPI.getBusinesses as jest.Mock).mockResolvedValue([]);
    (getLoyaltyRate as jest.Mock).mockResolvedValue(0);
  });

  it("surfaces a coded cashier request failure as localized toast, not raw English", async () => {
    const englishBackend = "Plugin is not available for this business";
    const err: any = new Error(englishBackend);
    err.response = {
      status: 503,
      data: { code: "plugin_unavailable", error: englishBackend },
    };
    (requestAlternativePayment as jest.Mock).mockRejectedValue(err);

    render(
      <GuestBill
        bill={openBill}
        business={{ id: 42, name: "Café", tax_rate: 0, service_fee_rate: 0 } as any}
        tableCode="T1"
        selectedLanguage="en"
        defaultCurrency="USD"
        displayCurrency="USD"
        onPaymentComplete={jest.fn()}
        onBillUpdate={jest.fn()}
      />,
    );

    fireEvent.click(await screen.findByTestId("full-bill-cashier"));

    await waitFor(() => {
      expect(toastErrors).toContain(
        "This payment method is temporarily unavailable.",
      );
    });
    expect(toastErrors).not.toContain(englishBackend);
  });

  it("shows pay-at-counter confirmation only after a successful cashier request", async () => {
    (requestAlternativePayment as jest.Mock).mockResolvedValue({
      success: true,
      message: "Alternative payment request sent to business owner",
      requestId: "99",
    });

    render(
      <GuestBill
        bill={openBill}
        business={{ id: 42, name: "Café", tax_rate: 0, service_fee_rate: 0 } as any}
        tableCode="T1"
        selectedLanguage="en"
        defaultCurrency="USD"
        displayCurrency="USD"
        onPaymentComplete={jest.fn()}
        onBillUpdate={jest.fn()}
      />,
    );

    fireEvent.click(await screen.findByTestId("full-bill-cashier"));

    await waitFor(() => {
      expect(
        screen.getByText(
          "Your table has been notified. Pay at the counter — staff will confirm once they receive your payment.",
        ),
      ).toBeInTheDocument();
    });
    expect(toastErrors).toHaveLength(0);
  });

  it("maps bill_not_payable to a specific localized cashier conflict toast", async () => {
    const englishBackend = "Payment request exceeds the bill's outstanding balance";
    const err: any = new Error(englishBackend);
    err.response = {
      status: 409,
      data: { code: "bill_not_payable", error: englishBackend },
    };
    (requestAlternativePayment as jest.Mock).mockRejectedValue(err);

    render(
      <GuestBill
        bill={openBill}
        business={{ id: 42, name: "Café", tax_rate: 0, service_fee_rate: 0 } as any}
        tableCode="T1"
        selectedLanguage="en"
        defaultCurrency="USD"
        displayCurrency="USD"
        onPaymentComplete={jest.fn()}
        onBillUpdate={jest.fn()}
      />,
    );

    fireEvent.click(await screen.findByTestId("full-bill-cashier"));

    await waitFor(() => {
      expect(toastErrors).toContain(
        "This bill can no longer accept that payment. Refresh and check the remaining balance, or ask staff.",
      );
    });
    expect(toastErrors).not.toContain(englishBackend);
    expect(toastErrors).not.toContain("Something went wrong. Please try again.");
  });
  it("treats a repeat cashier request (payment_request_pending) as already sent", async () => {
    const englishBackend = "A payment request for this bill is already waiting for staff";
    const err: any = new Error(englishBackend);
    err.response = {
      status: 409,
      data: { code: "payment_request_pending", error: englishBackend },
    };
    (requestAlternativePayment as jest.Mock).mockRejectedValue(err);

    render(
      <GuestBill
        bill={openBill}
        business={{ id: 42, name: "Café", tax_rate: 0, service_fee_rate: 0 } as any}
        tableCode="T1"
        selectedLanguage="en"
        defaultCurrency="USD"
        displayCurrency="USD"
        onPaymentComplete={jest.fn()}
        onBillUpdate={jest.fn()}
      />,
    );

    fireEvent.click(await screen.findByTestId("full-bill-cashier"));

    await waitFor(() => {
      expect(
        screen.getByText(
          "Your table has been notified. Pay at the counter — staff will confirm once they receive your payment.",
        ),
      ).toBeInTheDocument();
    });
    expect(toastErrors).toHaveLength(0);
  });
});
