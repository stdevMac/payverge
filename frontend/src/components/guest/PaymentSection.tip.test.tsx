/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";

import PaymentSection from "@/components/guest/PaymentSection";
import { paymentPluginAPI } from "@/api/plugins";

jest.mock("@/api/plugins", () => ({
  paymentPluginAPI: {
    getBusinessPaymentPlugins: jest.fn(),
    createPluginPayment: jest.fn(),
    getPluginPaymentStatus: jest.fn(),
  },
}));

// Match the provider contract: PaymentSection state updates do not recreate
// the context's translation function. An inline function here churns
// loadPlugins and can race the user's selected payment method in a slow suite.
const stableT = (key: string) => key;
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: stableT,
    currentLanguage: "en",
  }),
}));

// Render CurrencyPrice as its raw numeric amount so the tip summary line and the
// preset captions are assertable.
jest.mock("@/components/common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => (
    <span data-testid="price">{amount}</span>
  ),
}));

jest.mock("@/components/payment/PaymentProcessor", () => () => null);
jest.mock("@/components/payment/CrossChainPayment", () => () => null);

// NextUI Input mock that forwards inputMode so we can assert the numeric keyboard.
jest.mock("@nextui-org/react", () => ({
  Button: ({
    children,
    onPress,
    onClick,
    isDisabled,
  }: {
    children: React.ReactNode;
    onPress?: () => void;
    onClick?: () => void;
    isDisabled?: boolean;
  }) => (
    <button
      type="button"
      disabled={isDisabled}
      onClick={() => {
        onPress?.();
        onClick?.();
      }}
    >
      {children}
    </button>
  ),
  Input: ({
    label,
    placeholder,
    value,
    onChange,
    inputMode,
  }: {
    label?: string;
    placeholder?: string;
    value?: string;
    onChange?: (event: React.ChangeEvent<HTMLInputElement>) => void;
    inputMode?: React.HTMLAttributes<HTMLInputElement>["inputMode"];
  }) => (
    <label>
      <span>{label}</span>
      <input
        data-testid="custom-tip-input"
        inputMode={inputMode}
        placeholder={placeholder}
        value={value}
        onChange={onChange}
      />
    </label>
  ),
}));


const baseProps = {
  billId: 1,
  billToken: "B1",
  businessId: 42,
  amount: 100,
  businessName: "Cafe",
  businessAddress: "0x123",
  tipAddress: "0x456",
  tableCode: "T1",
  onPaymentComplete: jest.fn(),
  onCashierPayment: jest.fn(),
};

function renderSection(amount = 100) {
  return render(<PaymentSection {...baseProps} amount={amount} />);
}

describe("PaymentSection custom tip input", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    // Tip UI is hidden for cashier-only; mock a card plugin so the tip
    // surface is available (auto-select also picks this first method).
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          name: "stripe",
          display_name: "Stripe",
          is_enabled: true,
          business_id: 42,
          plugin_id: 1,
          config: "{}",
        },
      ],
      counter_settlement_ready: true,
    });
  });

  it("renders the custom tip input with a decimal keyboard", async () => {
    renderSection();
    const input = await screen.findByTestId("custom-tip-input");
    expect(input).toHaveAttribute("inputMode", "decimal");
  });

  it("parses comma-decimal tips without truncating cents", async () => {
    renderSection();
    const input = await screen.findByTestId("custom-tip-input");
    fireEvent.change(input, { target: { value: "5,50" } });

    // The payment summary tip line must reflect 5.5, not the truncated 5.
    await waitFor(() => {
      const prices = screen
        .getAllByTestId("price")
        .map((n) => Number(n.textContent));
      expect(prices).toContain(5.5);
    });
  });

  it("clamps negative custom tips to zero", async () => {
    renderSection();
    const input = await screen.findByTestId("custom-tip-input");
    fireEvent.change(input, { target: { value: "-20" } });

    // The grand-total row (label common.total) must equal the bill amount (100):
    // a negative tip would drop the total to 80.
    await waitFor(() => {
      const totalDt = screen.getByText("common.total");
      const totalRow = totalDt.closest("div")!;
      const totalPrice = Number(
        totalRow.querySelector('[data-testid="price"]')!.textContent,
      );
      expect(totalPrice).toBe(100);
    });
  });

  it("re-lights a tip preset after the value round-trips through the input", async () => {
    // amount * 0.15 on $43.33 = 6.4995, where toFixed(2) loses precision.
    renderSection(43.33);
    const input = await screen.findByTestId("custom-tip-input");

    // Click the 15% preset (raw stored 6.4995, displayed "6.50").
    const preset = await screen.findByText("15%");
    fireEvent.click(preset.closest("button")!);

    // Manually edit to the rounded 2-dp value: "6.5" parses to 6.5, which is
    // within a cent of 6.4995 and must keep the preset highlighted.
    fireEvent.change(input, { target: { value: "6.5" } });

    // The 15% preset button should still read as active (brand background).
    await waitFor(() => {
      const btn = screen.getByText("15%").closest("button")!;
      expect(btn.className).toContain("border-brand bg-brand");
    });
  });

  it("uses an explicit pre-tax tip base for presets without changing the charge amount", async () => {
    render(
      <PaymentSection
        {...baseProps}
        amount={100}
        tipBaseAmount={80}
      />,
    );

    const tenPercentPreset = (await screen.findByText("10%")).closest("button")!;
    expect(within(tenPercentPreset).getByText("8")).toBeInTheDocument();

    fireEvent.click(tenPercentPreset);

    await waitFor(() => {
      const tipRow = screen.getByText("bill.tip").closest("div")!;
      expect(
        Number(tipRow.querySelector('[data-testid="price"]')!.textContent),
      ).toBe(8);

      const totalRow = screen.getByText("common.total").closest("div")!;
      expect(
        Number(totalRow.querySelector('[data-testid="price"]')!.textContent),
      ).toBe(108);
    });
  });

  it("hides the tip selector and charges exactly the bill amount when hideTip is set", async () => {
    // Delivery pay page: the driver tip is already baked into bill.total_amount,
    // so a second pay-page tip selector would double-charge gratuity. hideTip
    // suppresses the selector entirely and forces tip=0.
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          name: "stripe",
          display_name: "Stripe",
          is_enabled: true,
          business_id: 42,
          plugin_id: 1,
          config: "{}",
        },
      ],
    });
    (paymentPluginAPI.createPluginPayment as jest.Mock).mockResolvedValue({
      payment_url: "https://provider.example/checkout",
      payment_id: "pay_1",
    });

    render(
      <PaymentSection
        {...baseProps}
        amount={100}
        hideTip
      />,
    );

    // Select a non-cashier (plugin) method — the path that normally renders
    // the tip selector.
    fireEvent.click(await screen.findByText("paymentMethods.card"));

    // No tip UI: no "Add a tip" heading, no presets, no custom-tip input.
    expect(screen.queryByText("bill.addTip")).not.toBeInTheDocument();
    expect(screen.queryByText("10%")).not.toBeInTheDocument();
    expect(screen.queryByText("15%")).not.toBeInTheDocument();
    expect(screen.queryByTestId("custom-tip-input")).not.toBeInTheDocument();

    // The grand-total row must equal the bill amount exactly (no extra tip).
    const totalRow = screen.getByText("common.total").closest("div")!;
    expect(
      Number(totalRow.querySelector('[data-testid="price"]')!.textContent),
    ).toBe(100);

    // Submitting charges exactly the bill amount with tip_amount = 0.
    fireEvent.click(screen.getByText("bill.payNow"));
    await waitFor(() => {
      expect(paymentPluginAPI.createPluginPayment).toHaveBeenCalledWith(
        "B1",
        expect.objectContaining({ amount: 100, tip_amount: 0 }),
      );
    });
  });

  it("lets split-share cashier payments include a tip", async () => {
    const onCashierPayment = jest.fn();
    render(
      <PaymentSection
        {...baseProps}
        amount={100}
        tipBaseAmount={80}
        splitShareId={44}
        onCashierPayment={onCashierPayment}
      />,
    );

    // Wait for the initial plugin auto-selection to settle before overriding
    // it, just as a guest does once the payment choices are ready.
    await screen.findByText("bill.payNow");
    fireEvent.click(screen.getByText("bill.cashier"));
    await screen.findByText("bill.payWithCashier");
    fireEvent.click(screen.getByText("10%"));
    fireEvent.click(screen.getByText("bill.payWithCashier"));

    expect(onCashierPayment).toHaveBeenCalledWith(
      expect.objectContaining({
        tipAmount: 8,
        totalPaid: 108,
      }),
    );
  });
});
