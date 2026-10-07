/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import PaymentSection from "@/components/guest/PaymentSection";
import { paymentPluginAPI } from "@/api/plugins";

jest.mock("@/api/plugins", () => ({
  paymentPluginAPI: {
    getBusinessPaymentPlugins: jest.fn(),
    createPluginPayment: jest.fn(),
    getPluginPaymentStatus: jest.fn(),
  },
}));

const stableT = (key: string) => key;
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: stableT,
    currentLanguage: "en",
  }),
}));

jest.mock("@/components/common/CurrencyConverter", () => ({
  CurrencyPrice: ({ amount }: { amount: number }) => <span>{amount}</span>,
}));

jest.mock("@/components/payment/PaymentProcessor", () => () => null);
jest.mock("@/components/payment/CrossChainPayment", () => () => null);
jest.mock("@/components/payment/PaymentStatusChecker", () => () => null);

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
  }: {
    label?: string;
    placeholder?: string;
    value?: string;
    onChange?: (event: React.ChangeEvent<HTMLInputElement>) => void;
  }) => (
    <label>
      <span>{label}</span>
      <input placeholder={placeholder} value={value} onChange={onChange} />
    </label>
  ),
}));

const baseProps = {
  billId: 1,
  billToken: "B1",
  businessId: 42,
  amount: 100,
  businessName: "Cafe",
  businessAddress: "0x1234567890123456789012345678901234567890",
  tipAddress: "0x456",
  tableCode: "T1",
  onPaymentComplete: jest.fn(),
  onCashierPayment: jest.fn(),
};

describe("PaymentSection selected state", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          name: "paypal",
          display_name: "PayPal",
          is_enabled: true,
        },
      ],
      counter_settlement_ready: true,
    });
  });

  it("exposes exclusive payment-method selection to assistive tech", async () => {
    render(<PaymentSection {...baseProps} />);

    const paypal = await screen.findByRole("radio", { name: /PayPal/i });
    const cashier = screen.getByRole("radio", { name: /bill\.cashier/i });

    expect(paypal).toHaveAttribute("aria-checked", "true");
    expect(cashier).toHaveAttribute("aria-checked", "false");

    fireEvent.click(cashier);
    await waitFor(() => {
      expect(cashier).toHaveAttribute("aria-checked", "true");
      expect(paypal).toHaveAttribute("aria-checked", "false");
    });
  });

  it("exposes exclusive tip selection to assistive tech", async () => {
    render(<PaymentSection {...baseProps} />);

    const noTip = await screen.findByRole("radio", { name: /bill\.noTipButton/i });
    const fifteen = screen.getByRole("radio", { name: /15%/ });

    expect(noTip).toHaveAttribute("aria-checked", "true");
    expect(fifteen).toHaveAttribute("aria-checked", "false");

    fireEvent.click(fifteen);
    await waitFor(() => {
      expect(fifteen).toHaveAttribute("aria-checked", "true");
      expect(noTip).toHaveAttribute("aria-checked", "false");
    });
  });
});
