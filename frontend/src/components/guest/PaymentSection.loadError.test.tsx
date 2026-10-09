/** @jest-environment jsdom */
import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";

import PaymentSection from "@/components/guest/PaymentSection";
import { paymentPluginAPI } from "@/api/plugins";
import toast from "react-hot-toast";

// Audit C-03: a transient plugin-load failure must not silently leave the guest
// with only "Cashier" (indistinguishable from a cash-only business). It should
// surface an error + a retry instead of silently steering them to cash.

jest.mock("@/api/plugins", () => ({
  paymentPluginAPI: {
    getBusinessPaymentPlugins: jest.fn(),
    createPluginPayment: jest.fn(),
    getPluginPaymentStatus: jest.fn(),
  },
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
}));

// Stable `t` (module-level) so it mirrors production: useContext returns the
// same value object across a component's own re-renders, so `t` does not change
// per render. A per-render `t` would churn loadPlugins's useCallback identity.
const stableT = (key: string) => key;
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t: stableT, currentLanguage: "en" }),
}));

jest.mock("@/components/common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => <span>{amount}</span>,
}));

jest.mock("@/components/payment/PaymentProcessor", () => () => null);
jest.mock("@/components/payment/CrossChainPayment", () => () => null);
const renderSection = () =>
  render(
    <PaymentSection
      billId={1}
      billToken="B42-opaque"
      businessId={42}
      amount={30}
      businessName="Cafe"
      businessAddress="0x1111111111111111111111111111111111111111"
      tipAddress="0x456"
      tableCode="T1"
      onPaymentComplete={jest.fn()}
      onCashierPayment={jest.fn()}
    />,
  );

async function flushBackoffRetries() {
  // loadPlugins waits 250*attempt^2 between attempts (250ms, then 1000ms).
  await act(async () => {
    jest.advanceTimersByTime(250);
  });
  await act(async () => {
    jest.advanceTimersByTime(1000);
  });
}

describe("PaymentSection plugin-load failure (C-03)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest.useFakeTimers();
    window.history.replaceState({}, "", "/t/T1/bill");
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it("toasts and shows a retry when the plugin fetch fails after auto-retries", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockRejectedValue(
      new Error("network"),
    );

    renderSection();
    await flushBackoffRetries();

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("payment.methodsLoadError"),
    );
    // Auto-retry with backoff before surfacing the error (3 attempts).
    expect(paymentPluginAPI.getBusinessPaymentPlugins).toHaveBeenCalledTimes(3);
    // Inline retry affordance is shown (reuses the translated bill.retry label).
    expect(screen.getByRole("button", { name: "bill.retry" })).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "payment.methodsLoadError",
    );
    expect(screen.queryByText("bill.payWithCashier")).not.toBeInTheDocument();
    expect(screen.queryByText("bill.cashier")).not.toBeInTheDocument();
  });

  it("recovers silently when the first paint fails then a backoff retry succeeds", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock)
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce({
        plugins: [{ name: "paypal", display_name: "PayPal", is_enabled: true }],
      });

    renderSection();
    await flushBackoffRetries();

    await waitFor(() => expect(screen.getByText("PayPal")).toBeInTheDocument());
    expect(toast.error).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("button", { name: "bill.retry" }),
    ).not.toBeInTheDocument();
  });

  it("recovers when manual retry succeeds: error clears and the method appears", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock)
      .mockRejectedValueOnce(new Error("network"))
      .mockRejectedValueOnce(new Error("network"))
      .mockRejectedValueOnce(new Error("network"))
      .mockResolvedValueOnce({
        plugins: [{ name: "paypal", display_name: "PayPal", is_enabled: true }],
      });

    renderSection();
    await flushBackoffRetries();

    const retry = await screen.findByRole("button", { name: "bill.retry" });
    fireEvent.click(retry);

    await waitFor(() => expect(screen.getByText("PayPal")).toBeInTheDocument());
    expect(
      screen.queryByRole("button", { name: "bill.retry" }),
    ).not.toBeInTheDocument();
  });
});
