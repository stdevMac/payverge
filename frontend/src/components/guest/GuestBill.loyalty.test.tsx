/** @jest-environment jsdom */
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

jest.mock("@/hooks/usePolling", () => ({
  usePolling: ({ callback }: { callback: () => Promise<void> | void }) => {
    void callback();
    return { startPolling: jest.fn(), stopPolling: jest.fn(), isPolling: false };
  },
}));

// Faithful translation mock — mirrors GuestTranslationProvider's interpolation
// (both {{var}} and {var}, applied only when params are passed). This is what
// makes the regression real: if the component forgets to pass params, the
// placeholders survive verbatim and this test catches it.
const LOYALTY_MESSAGES: Record<string, string> = {
  "bill.loyaltyPoints": "You have {count} points",
  "bill.redeemPoints": "Use {points} pts ({discount} off)",
  "bill.loyaltyEstimate": "Worth ~{amount} off your bill",
  "bill.pointsApplied": "{points} points applied — {discount} off",
  "bill.loyaltyDiscount": "Loyalty discount",
  "bill.undoRedemption": "Undo",
};
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string, params?: Record<string, string | number>) => {
      const value = LOYALTY_MESSAGES[key] ?? key;
      if (!params) return value;
      return value.replace(
        /\{\{(\w+)\}\}|\{(\w+)\}/g,
        (match: string, doubleKey: string, singleKey: string) => {
          const paramKey = doubleKey || singleKey;
          return params[paramKey] !== undefined && params[paramKey] !== null
            ? params[paramKey].toString()
            : match;
        },
      );
    },
  }),
}));

// Authenticated customer so the loyalty-redemption card mounts.
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
  const CurrencyPrice = ({
    amount,
    displayCurrency,
  }: {
    amount: number;
    displayCurrency?: string;
  }) => (
    <span>
      {new Intl.NumberFormat("en", {
        style: "currency",
        currency: displayCurrency || "USD",
      }).format(amount)}
    </span>
  );
  return {
    __esModule: true,
    default: CurrencyPrice,
    CurrencyPrice,
  };
});

const openBill: BillWithItemsResponse = {
  bill: {
    id: 1,
    business_id: 42,
    table_id: 3,
    bill_number: "B42-loyalty",
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

describe("GuestBill loyalty-redemption card", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (getGuestOrdersByBillNumber as jest.Mock).mockResolvedValue({ orders: [], total: 0 });
    (getOpenBillByTableCode as jest.Mock).mockResolvedValue(openBill);
    // 500 points at the default rate of 100 pts/unit => 5.00 of base currency.
    (crmAPI.getBusinesses as jest.Mock).mockResolvedValue([
      { business_id: 42, loyalty_points: 500 },
    ]);
    (getLoyaltyRate as jest.Mock).mockResolvedValue(0); // keep default rate (100)
  });

  it("interpolates loyalty placeholders and formats the discount in the business currency", async () => {
    render(
      <GuestBill
        bill={openBill}
        business={{ id: 42, name: "Café", tax_rate: 0, service_fee_rate: 0 } as any}
        tableCode="T1"
        selectedLanguage="en"
        defaultCurrency="EUR"
        displayCurrency="EUR"
        onPaymentComplete={jest.fn()}
        onBillUpdate={jest.fn()}
      />,
    );

    // The "You have N points" line proves the {count} placeholder is interpolated.
    await waitFor(() => {
      expect(screen.getByText("You have 500 points")).toBeInTheDocument();
    });


    // Discount amounts go through CurrencyPrice (display currency). The mock
    // emits the raw amount in its own span, so match the button/estimate by
    // combined text content rather than a single formatted string node.
    expect(
      screen.getByText(
        (_content, el) =>
          el?.tagName === "BUTTON" && /Use 500 pts \(.*5.* off\)/.test(el.textContent ?? ""),
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        (_content, el) =>

          el?.tagName === "P" && /Worth ~.*5.* off your bill/.test(el.textContent ?? ""),
      ),
    ).toBeInTheDocument();

    // No literal placeholders or hardcoded dollar discount survive.
    expect(screen.queryByText(/\{points\}|\{discount\}|\{count\}|\{amount\}/)).toBeNull();
    expect(screen.queryByText(/\$5\.00/)).toBeNull();
  });

  // A bill that ALREADY carries a loyalty discount (e.g. the guest reloads the
  // page, or an operator opens it) must explain the reduced total with a
  // persistent "Loyalty discount" line in the summary — NOT only the ephemeral
  // green "points applied" chip, which is session-state set after clicking
  // Redeem. Before this, the backend baked the discount into total_amount but
  // the summary showed tax+fee+items that no longer reconciled with the lower
  // Total, with nothing explaining the gap.
  it("renders a persistent loyalty-discount line from the bill (no redeem click needed)", async () => {
    const discountedBill: BillWithItemsResponse = {
      bill: {
        ...openBill.bill,
        bill_number: "B42-discounted",
        subtotal: asDollars(30),
        total_amount: asDollars(25), // net of a 5.00 loyalty discount
        loyalty_discount: asDollars(5),
      },
      items: [],
    };
    (getOpenBillByTableCode as jest.Mock).mockResolvedValue(discountedBill);
    // Authenticated guest with no remaining balance: Redeem stays hidden, but
    // Undo must still render from the payload (not from session redeem state).
    (crmAPI.getBusinesses as jest.Mock).mockResolvedValue([
      { business_id: 42, loyalty_points: 0 },
    ]);

    render(
      <GuestBill
        bill={discountedBill}
        business={{ id: 42, name: "Café", tax_rate: 0, service_fee_rate: 0 } as any}
        tableCode="T1"
        selectedLanguage="en"
        defaultCurrency="EUR"
        displayCurrency="EUR"
        onPaymentComplete={jest.fn()}
        onBillUpdate={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("Loyalty discount")).toBeInTheDocument();
    });
    expect(screen.getByText("Undo")).toBeInTheDocument();
    expect(screen.queryByText(/Use \d+ pts/)).toBeNull();
    expect(
      screen.getByText(
        (_content, el) =>
          el?.tagName === "P" && /500 points applied/.test(el.textContent ?? ""),
      ),
    ).toBeInTheDocument();
    // The discount renders as a negative credit. The CurrencyPrice mock emits
    // the raw amount in its own span, so match the line's combined text content.
    expect(
      screen.getByText(
        (_content, el) =>
          el?.tagName === "DD" && /-\s*€5\.00/.test(el.textContent ?? ""),
      ),
    ).toBeInTheDocument();
  });

  // Over-redemption guard: a guest whose point value exceeds the bill must not be
  // shown (or charged) their whole balance. With a €2.00 bill and 500 points worth
  // €5.00 at 100 pts/unit, the card must advertise only €2.00 off / 200 pts, and
  // tapping Redeem must request exactly 200 points — never the full 500, whose
  // surplus value would otherwise be silently destroyed.
  it("clamps the loyalty estimate and redeem request to the bill total", async () => {
    const smallBill: BillWithItemsResponse = {
      bill: {
        ...openBill.bill,
        bill_number: "B42-small",
        subtotal: asDollars(2),
        total_amount: asDollars(2),
        paid_amount: asDollars(0),
      },
      items: [],
    };
    (getOpenBillByTableCode as jest.Mock).mockResolvedValue(smallBill);
    (redeemPoints as jest.Mock).mockResolvedValue({
      points_deducted: 200,
      discount_cents: 200,
      remaining_points: 300,
    });

    render(
      <GuestBill
        bill={smallBill}
        business={{ id: 42, name: "Café", tax_rate: 0, service_fee_rate: 0 } as any}
        tableCode="T1"
        selectedLanguage="en"
        defaultCurrency="EUR"
        displayCurrency="EUR"
        onPaymentComplete={jest.fn()}
        onBillUpdate={jest.fn()}
      />,
    );


    // Estimate and button reflect the clamped 200 pts / 2.00, not 500 / 5.00.
    // CurrencyPrice is mocked as the raw amount, so match combined text.
    const redeemButtonMatcher = (_content: string, el: Element | null) =>
      el?.tagName === "BUTTON" && /Use 200 pts \(.*2.* off\)/.test(el.textContent ?? "");

    await waitFor(() => {
      expect(screen.getByText(redeemButtonMatcher)).toBeInTheDocument();
    });
    expect(
      screen.getByText(
        (_content, el) =>
          el?.tagName === "P" && /Worth ~.*2.* off your bill/.test(el.textContent ?? ""),
      ),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Use 500 pts/)).toBeNull();

    fireEvent.click(screen.getByText(redeemButtonMatcher));

    // The request sends the clamped points, so the backend never deducts the
    // surplus 300 points.
    await waitFor(() => {
      expect(redeemPoints).toHaveBeenCalledWith("T1", 200, "B42-small");
    });
  });
});
