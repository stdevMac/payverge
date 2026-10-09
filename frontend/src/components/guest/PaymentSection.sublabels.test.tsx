/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

// next/dynamic pass-through (same as PaymentSection.test.tsx)
jest.mock("next/dynamic", () => ({
  __esModule: true,
  default: (importFn: () => Promise<{ default: unknown }>) => {
    const React = require("react") as typeof import("react");
    let Resolved: React.ComponentType | null = null;
    importFn().then((m) => {
      Resolved = (m as { default: React.ComponentType }).default;
    });
    const Wrapper = (props: Record<string, unknown>) =>
      Resolved ? React.createElement(Resolved, props) : null;
    Wrapper.displayName = "DynamicWrapper";
    return Wrapper;
  },
}));

import PaymentSection from "@/components/guest/PaymentSection";
import { paymentPluginAPI } from "@/api/plugins";

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
  default: ({ isOpen }: { isOpen: boolean }) => <div data-testid="payment-processor" data-open={String(isOpen)} />,
}));
jest.mock("@/components/payment/CrossChainPayment", () => ({
  __esModule: true,
  default: ({ isOpen }: { isOpen: boolean }) => <div data-testid="cross-chain-payment" data-open={String(isOpen)} />,
}));
jest.mock("@/components/payment/PaymentStatusChecker", () => ({
  __esModule: true,
  default: ({ isOpen }: { isOpen: boolean }) => isOpen ? <div data-testid="status-checker" /> : null,
}));
jest.mock("@nextui-org/react", () => ({
  Button: ({ children, onPress, onClick, isDisabled }: any) => (
    <button type="button" disabled={isDisabled} onClick={() => { onPress?.(); onClick?.(); }}>
      {children}
    </button>
  ),
  Card: ({ children }: any) => <div>{children}</div>,
  CardBody: ({ children }: any) => <div>{children}</div>,
  Input: ({ label, placeholder, value, onChange }: any) => (
    <label><span>{label}</span><input placeholder={placeholder} value={value} onChange={onChange} /></label>
  ),
}));

const defaultProps = {
  billId: 1,
  billToken: "B42",
  businessId: 42,
  amount: 30,
  businessName: "Cafe",
  businessAddress: "0x1234567890123456789012345678901234567890",
  tipAddress: "0xabcdefabcdefabcdefabcdefabcdefabcdefabcd",
  tableCode: "T1",
  onPaymentComplete: jest.fn(),
  onCashierPayment: jest.fn(),
};

describe("PaymentSection sublabels", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [],
      counter_settlement_ready: true,
    });
  });

  it("renders a distinguishing sublabel under the cashier payment tile", async () => {
    render(<PaymentSection {...defaultProps} />);
    await waitFor(() => expect(paymentPluginAPI.getBusinessPaymentPlugins).toHaveBeenCalled());
    // cashier tile carries the payAtCounter sublabel key (t mock returns key as-is)
    expect(await screen.findByText("payment.sublabel.payAtCounter")).toBeInTheDocument();
  });

  it("labels USDC and cross-chain tiles as needing a wallet, not 'no account needed'", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        { name: "usdc_payment", display_name: "USDC", is_enabled: true },
        { name: "cross_chain_payment", display_name: "Any Token", is_enabled: true },
      ],
    });
    render(<PaymentSection {...defaultProps} />);
    await waitFor(() => expect(paymentPluginAPI.getBusinessPaymentPlugins).toHaveBeenCalled());

    // Both crypto rails resolve the cryptoWallet sublabel (t mock echoes the key).
    const walletSublabels = await screen.findAllByText("payment.sublabel.cryptoWallet");
    expect(walletSublabels.length).toBeGreaterThanOrEqual(2);
    // And never the misleading "no account needed" copy for these tiles.
    expect(screen.queryByText("payment.sublabel.noAccount")).not.toBeInTheDocument();
  });

  it("NEW-12: maps known rail slugs to guest-message keys, not backend display_name", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        { name: "usdc_payment", display_name: "USDC Payment", is_enabled: true },
        { name: "cross_chain_payment", display_name: "Any Token Payment", is_enabled: true },
        { name: "paypal", display_name: "PayPal English Only", is_enabled: true },
        { name: "future_rail", display_name: "Future Rail Brand", is_enabled: true },
      ],
    });
    render(<PaymentSection {...defaultProps} />);
    await waitFor(() => expect(paymentPluginAPI.getBusinessPaymentPlugins).toHaveBeenCalled());

    // Known rails resolve via paymentMethods.* guest keys (t mock echoes the key).
    expect(await screen.findByText("paymentMethods.cryptoUsdc")).toBeInTheDocument();
    expect(screen.getByText("paymentMethods.crossChain")).toBeInTheDocument();
    // Brand names stay brand-stable via paymentMethodLabel.
    expect(screen.getByText("PayPal")).toBeInTheDocument();
    // Unknown plugins fall back to backend display_name.
    expect(screen.getByText("Future Rail Brand")).toBeInTheDocument();
    // Backend English-only labels must not leak for known rails.
    expect(screen.queryByText("USDC Payment")).not.toBeInTheDocument();
    expect(screen.queryByText("Any Token Payment")).not.toBeInTheDocument();
    expect(screen.queryByText("PayPal English Only")).not.toBeInTheDocument();
  });

});
