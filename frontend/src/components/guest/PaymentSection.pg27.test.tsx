/** @jest-environment jsdom */
/**
 * PG-27: payment-method selector cards must be type="button" (not implicit
 * submit). Mounts real PaymentSection — not a source-file grep.
 *
 * Duplicate-CTA note: cashier is a method *selector* card; the single primary
 * pay button becomes `bill.payWithCashier` only after selection. That is
 * select-then-confirm UX, not two independent cashier action paths.
 */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import PaymentSection from "@/components/guest/PaymentSection";
import { paymentPluginAPI } from "@/api/plugins";

jest.mock("next/dynamic", () => ({
  __esModule: true,
  default: () => () => null,
}));

jest.mock("@/api/plugins", () => ({
  paymentPluginAPI: {
    getBusinessPaymentPlugins: jest.fn(),
    createPluginPayment: jest.fn(),
    getPluginPaymentStatus: jest.fn(),
  },
}));

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
    currentLanguage: "en",
  }),
}));

jest.mock("@/components/common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => <span>{amount}</span>,
}));

jest.mock("@/components/payment/PaymentProcessor", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("@/components/payment/CrossChainPayment", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("@/components/payment/PaymentStatusChecker", () => ({
  __esModule: true,
  default: () => null,
}));

const baseProps = {
  billId: 1,
  billToken: "tok",
  businessId: 1,
  amount: 20,
  businessName: "Cafe",
  businessAddress: "0xabc",
  tipAddress: "0xdef",
  tableCode: "T1",
  defaultCurrency: "USD",
  displayCurrency: "USD",
  onPaymentComplete: jest.fn(),
  onCashierPayment: jest.fn(),
};

beforeEach(() => {
  jest.clearAllMocks();
  (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
    plugins: [],
    counter_settlement_ready: true,
  });
});

describe("PG-27 PaymentSection method cards (real component)", () => {
  it("renders method selector cards as type=button (no form submit)", async () => {
    const { container } = render(<PaymentSection {...baseProps} />);

    await waitFor(() =>
      expect(screen.getByText("bill.cashier")).toBeInTheDocument(),
    );

    // Method grid: each method is a bare <button type="button">.
    const methodButtons = container.querySelectorAll(
      "div.grid button[type='button']",
    );
    expect(methodButtons.length).toBeGreaterThanOrEqual(1);

    // Cashier card must not be type=submit (audit PG-27).
    const cashierLabel = screen.getByText("bill.cashier");
    const cashierBtn = cashierLabel.closest("button");
    expect(cashierBtn).not.toBeNull();
    expect(cashierBtn).toHaveAttribute("type", "button");
    expect(cashierBtn?.getAttribute("type")).not.toBe("submit");
  });

  it("uses one primary pay CTA after selecting cashier (not a second independent cashier path)", async () => {
    render(<PaymentSection {...baseProps} />);

    await waitFor(() =>
      expect(screen.getByText("bill.cashier")).toBeInTheDocument(),
    );
    fireEvent.click(screen.getByText("bill.cashier"));

    // Single primary action for cashier — select-then-confirm, not a second
    // top-level "Pagar en Caja" sibling of the method card.
    expect(screen.getByText("bill.payWithCashier")).toBeInTheDocument();
    const payButtons = screen.getAllByText("bill.payWithCashier");
    expect(payButtons).toHaveLength(1);
  });
});
