/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

// next/dynamic wraps CrossChainPayment after P-1. Make it a synchronous
// pass-through in tests so the mock at "@/components/payment/CrossChainPayment"
// still resolves and the existing assertions hold.
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
  default: ({
    isOpen,
    splitShareId,
  }: {
    isOpen: boolean;
    splitShareId?: number;
  }) => (
    <div
      data-open={String(isOpen)}
      data-split-share-id={splitShareId ?? ""}
      data-testid="payment-processor"
    />
  ),
}));
jest.mock("@/components/payment/CrossChainPayment", () => ({
  __esModule: true,
  default: ({
    isOpen,
    splitShareId,
  }: {
    isOpen: boolean;
    splitShareId?: number;
  }) => (
    <div
      data-open={String(isOpen)}
      data-split-share-id={splitShareId ?? ""}
      data-testid="cross-chain-payment"
    />
  ),
}));
jest.mock("@/components/payment/PaymentStatusChecker", () => ({
  __esModule: true,
  default: ({
    isOpen,
    paymentId,
    onPaymentConfirmed,
  }: {
    isOpen: boolean;
    paymentId: string;
    onPaymentConfirmed: (details: {
      totalPaid: number;
      tipAmount: number;
      paymentMethod: string;
      transactionId: string;
    }) => void;
  }) =>
    isOpen ? (
      <div data-testid="status-checker">
        <span>{paymentId}</span>
        <button
          type="button"
          onClick={() =>
            onPaymentConfirmed({
              totalPaid: 41,
              tipAmount: 4,
              paymentMethod: "paypal",
              transactionId: paymentId,
            })
          }
        >
          confirm-server
        </button>
      </div>
    ) : null,
}));
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
  Card: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  CardBody: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
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
      <input
        placeholder={placeholder}
        value={value}
        onChange={onChange}
      />
    </label>
  ),
}));

describe("PaymentSection", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.history.replaceState({}, "", "/t/T1/bill?payment=success&method=paypal");
    sessionStorage.setItem(
      "payverge_payment",
      JSON.stringify({
        billId: 1,
        billToken: "B42-opaque",
        paymentId: "pay_123",
        method: "paypal",
        amount: 33.5,
        tipAmount: 3.5,
      }),
    );
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [],
    });
  });

  afterEach(() => {
    window.history.replaceState({}, "", "/t/T1/bill");
    sessionStorage.clear();
  });

  it("does not mark plugin payments complete from the redirect query alone", async () => {
    const onPaymentComplete = jest.fn();

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
        onPaymentComplete={onPaymentComplete}
        onCashierPayment={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(paymentPluginAPI.getBusinessPaymentPlugins).toHaveBeenCalled();
    });

    expect(onPaymentComplete).not.toHaveBeenCalled();
  });

  it("auto-selects the first payment method so the pay CTA is enabled", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          name: "paypal",
          display_name: "PayPal",
          is_enabled: true,
        },
      ],
    });

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

    // Without a method tap, primary CTA must not stay on selectPaymentMethod.
    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: /bill\.selectPaymentMethod/i }),
      ).toBeNull();
      expect(
        screen.getByRole("button", { name: /bill\.payNow/i }),
      ).not.toBeDisabled();
    });
  });

  it("includes the opaque bill number in plugin payment return URLs", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          name: "paypal",
          display_name: "PayPal",
          is_enabled: true,
        },
      ],
    });
    (paymentPluginAPI.createPluginPayment as jest.Mock).mockResolvedValue({
      payment_id: "pay_123",
      status: "pending",
    });

    render(
      <PaymentSection
        billId={1}
        billToken="B42-opaque"
        businessId={42}
        amount={30}
        splitShareId={42}
        businessName="Cafe"
        businessAddress="0x1111111111111111111111111111111111111111"
        tipAddress="0x456"
        tableCode="T1"
        onPaymentComplete={jest.fn()}
        onCashierPayment={jest.fn()}
      />,
    );

    let paypalButton: HTMLButtonElement | null = null;
    await waitFor(() => {
      paypalButton = screen.getByText("PayPal").closest("button");
      expect(paypalButton).not.toBeNull();
    });

    fireEvent.click(paypalButton!);
    fireEvent.click(screen.getByRole("button", { name: /bill\.payNow/i }));

    await waitFor(() => {
      expect(paymentPluginAPI.createPluginPayment).toHaveBeenCalledWith(
        "B42-opaque",
        expect.objectContaining({
          return_url: expect.stringContaining("bill_token=B42-opaque"),
          cancel_url: expect.stringContaining("bill_token=B42-opaque"),
          metadata: expect.objectContaining({ split_share_id: 42 }),
        }),
      );
    });
  });

  it("stores the split share id for redirect plugin recovery", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          name: "paypal",
          display_name: "PayPal",
          is_enabled: true,
        },
      ],
    });
    (paymentPluginAPI.createPluginPayment as jest.Mock).mockResolvedValue({
      payment_id: "pay_split_redirect",
      payment_url: "https://paypal.example/checkout",
      status: "pending",
    });

    render(
      <PaymentSection
        billId={1}
        billToken="B42-opaque"
        businessId={42}
        amount={30}
        splitShareId={42}
        businessName="Cafe"
        businessAddress="0x1111111111111111111111111111111111111111"
        tipAddress="0x456"
        tableCode="T1"
        onPaymentComplete={jest.fn()}
        onCashierPayment={jest.fn()}
      />,
    );

    let paypalButton: HTMLButtonElement | null = null;
    await waitFor(() => {
      paypalButton = screen.getByText("PayPal").closest("button");
      expect(paypalButton).not.toBeNull();
    });

    fireEvent.click(paypalButton!);
    fireEvent.click(screen.getByRole("button", { name: /bill\.payNow/i }));

    await waitFor(() => {
      expect(JSON.parse(sessionStorage.getItem("payverge_payment") || "{}")).toEqual(
        expect.objectContaining({
          paymentId: "pay_split_redirect",
          method: "paypal",
          billToken: "B42-opaque",
          splitShareId: 42,
        }),
      );
    });
  });

  it("passes the held split share to the USDC payment modal", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          name: "usdc_payment",
          display_name: "USDC",
          is_enabled: true,
        },
      ],
    });

    render(
      <PaymentSection
        billId={1}
        billToken="B42-opaque"
        businessId={42}
        amount={30}
        splitShareId={42}
        businessName="Cafe"
        businessAddress="0x1111111111111111111111111111111111111111"
        tipAddress="0x456"
        tableCode="T1"
        onPaymentComplete={jest.fn()}
        onCashierPayment={jest.fn()}
      />,
    );

    let usdcButton: HTMLButtonElement | null = null;
    await waitFor(() => {
      usdcButton = screen.getByText("paymentMethods.cryptoUsdc").closest("button");
      expect(usdcButton).not.toBeNull();
    });

    fireEvent.click(usdcButton!);
    fireEvent.click(screen.getByRole("button", { name: /bill\.payNow/i }));

    const processor = await screen.findByTestId("payment-processor");
    await waitFor(() => {
      expect(processor).toHaveAttribute("data-open", "true");
    });
    expect(processor).toHaveAttribute("data-split-share-id", "42");
  });

  it("passes the held split share to the cross-chain payment modal", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          name: "cross_chain_payment",
          display_name: "Cross-chain",
          is_enabled: true,
        },
      ],
    });

    render(
      <PaymentSection
        billId={1}
        billToken="B42-opaque"
        businessId={42}
        amount={30}
        splitShareId={42}
        businessName="Cafe"
        businessAddress="0x1111111111111111111111111111111111111111"
        tipAddress="0x456"
        tableCode="T1"
        onPaymentComplete={jest.fn()}
        onCashierPayment={jest.fn()}
      />,
    );

    let crossChainButton: HTMLButtonElement | null = null;
    await waitFor(() => {
      crossChainButton = screen.getByText("paymentMethods.crossChain").closest("button");
      expect(crossChainButton).not.toBeNull();
    });

    fireEvent.click(crossChainButton!);
    fireEvent.click(screen.getByRole("button", { name: /bill\.payNow/i }));

    const crossChain = await screen.findByTestId("cross-chain-payment");
    await waitFor(() => {
      expect(crossChain).toHaveAttribute("data-open", "true");
    });
    expect(crossChain).toHaveAttribute("data-split-share-id", "42");
  });

  // Audit C-04: a plugin response with no redirect URL must be confirmed by
  // polling the server (PaymentStatusChecker), never by optimistically
  // reporting the client-computed totals as paid.
  it("routes non-redirect plugin payments through server confirmation", async () => {
    const onPaymentComplete = jest.fn();
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          name: "paypal",
          display_name: "PayPal",
          is_enabled: true,
        },
      ],
    });
    (paymentPluginAPI.createPluginPayment as jest.Mock).mockResolvedValue({
      payment_id: "pay_789",
      status: "completed", // claimed by the create response — NOT trusted
    });

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
        onPaymentComplete={onPaymentComplete}
        onCashierPayment={jest.fn()}
      />,
    );

    let paypalButton: HTMLButtonElement | null = null;
    await waitFor(() => {
      paypalButton = screen.getByText("PayPal").closest("button");
      expect(paypalButton).not.toBeNull();
    });

    fireEvent.click(paypalButton!);
    fireEvent.click(screen.getByRole("button", { name: /bill\.payNow/i }));

    // The checker opens for the created payment; completion is NOT fired
    // off the create response.
    const checker = await screen.findByTestId("status-checker");
    expect(checker).toHaveTextContent("pay_789");
    expect(onPaymentComplete).not.toHaveBeenCalled();

    // Only the server-confirmed amounts complete the payment.
    fireEvent.click(await screen.findByText("confirm-server"));
    expect(onPaymentComplete).toHaveBeenCalledWith({
      totalPaid: 41,
      tipAmount: 4,
      paymentMethod: "paypal",
      transactionId: "pay_789",
    });
  });

  it("hides USDC / Any-Token rails when no settlement address is configured", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        { name: "paypal", display_name: "PayPal", is_enabled: true },
        { name: "usdc_payment", display_name: "USDC", is_enabled: true },
        { name: "cross_chain_payment", display_name: "AnyToken", is_enabled: true },
      ],
    });

    render(
      <PaymentSection
        billId={1}
        billToken="B42-opaque"
        businessId={42}
        amount={30}
        businessName="Cafe"
        businessAddress=""
        tipAddress="0x456"
        tableCode="T1"
        onPaymentComplete={jest.fn()}
        onCashierPayment={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByText("PayPal")).toBeInTheDocument();
    });
    // Crypto-wallet rails cannot settle without a configured payout address,
    // so they must not be offered to the guest.
    expect(screen.queryByText("paymentMethods.cryptoUsdc")).not.toBeInTheDocument();
    expect(screen.queryByText("AnyToken")).not.toBeInTheDocument();
  });

  it("hides USDC / Any-Token rails when the settlement address is malformed", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        { name: "paypal", display_name: "PayPal", is_enabled: true },
        { name: "usdc_payment", display_name: "USDC", is_enabled: true },
        {
          name: "cross_chain_payment",
          display_name: "AnyToken",
          is_enabled: true,
        },
      ],
    });

    render(
      <PaymentSection
        billId={1}
        billToken="B42-opaque"
        businessId={42}
        amount={30}
        businessName="Cafe"
        businessAddress="not-an-address"
        tipAddress="0x456"
        tableCode="T1"
        onPaymentComplete={jest.fn()}
        onCashierPayment={jest.fn()}
      />,
    );

    await waitFor(() => expect(screen.getByText("PayPal")).toBeInTheDocument());
    expect(screen.queryByText("paymentMethods.cryptoUsdc")).not.toBeInTheDocument();
    expect(screen.queryByText("AnyToken")).not.toBeInTheDocument();
  });

  it("hides USDC / Any-Token rails for placeholder settlement addresses", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        { name: "paypal", display_name: "PayPal", is_enabled: true },
        { name: "usdc_payment", display_name: "USDC", is_enabled: true },
        {
          name: "cross_chain_payment",
          display_name: "AnyToken",
          is_enabled: true,
        },
      ],
    });

    render(
      <PaymentSection
        billId={1}
        billToken="B42-opaque"
        businessId={42}
        amount={30}
        businessName="Cafe"
        businessAddress="0x000000000000000000000000000000000000dE01"
        tipAddress="0x456"
        tableCode="T1"
        onPaymentComplete={jest.fn()}
        onCashierPayment={jest.fn()}
      />,
    );

    await waitFor(() => expect(screen.getByText("PayPal")).toBeInTheDocument());
    expect(screen.queryByText("paymentMethods.cryptoUsdc")).not.toBeInTheDocument();
    expect(screen.queryByText("AnyToken")).not.toBeInTheDocument();
  });

  it("orders card/cash tenders before crypto rails", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        { name: "usdc_payment", display_name: "USDC", is_enabled: true },
        { name: "paypal", display_name: "PayPal", is_enabled: true },
      ],
      counter_settlement_ready: true,
    });

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

    await waitFor(() => {
      expect(screen.getByText("paymentMethods.cryptoUsdc")).toBeInTheDocument();
    });

    const labels = screen
      .getAllByText(/PayPal|paymentMethods.cryptoUsdc|bill\.cashier/)
      .map((el) => el.textContent);
    // Card (PayPal) and cash (cashier) lead; USDC crypto rail comes last.
    expect(labels.indexOf("PayPal")).toBeLessThan(
      labels.indexOf("paymentMethods.cryptoUsdc"),
    );
    expect(labels.indexOf("bill.cashier")).toBeLessThan(
      labels.indexOf("paymentMethods.cryptoUsdc"),
    );
  });

  it("hides Amount/Total twin summary for cashier full-bill with no tip", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [],
      counter_settlement_ready: true,
    });

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

    await waitFor(() => {
      expect(screen.getByText("bill.cashier")).toBeInTheDocument();
    });
    // Auto-selects first method (cashier when no plugins).
    await waitFor(() => {
      expect(screen.queryByTestId("payment-amount-summary")).not.toBeInTheDocument();
    });
    expect(screen.queryByText("bill.amount")).not.toBeInTheDocument();
    expect(await screen.findByText("bill.payWithCashier")).toBeInTheDocument();
  });

  it("blocks guest checkout when no rails are live and the drawer is closed", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [],
      counter_settlement_ready: false,
    });

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

    await waitFor(() => {
      expect(screen.getByTestId("guest-no-tender")).toHaveTextContent(
        "payment.noTenderAvailable",
      );
    });
    expect(screen.queryByText("bill.cashier")).not.toBeInTheDocument();
    expect(screen.queryByText("bill.payWithCashier")).not.toBeInTheDocument();
  });

  it("hides cashier when counter_settlement_ready is omitted", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [],
    });

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

    await waitFor(() => {
      expect(screen.getByTestId("guest-no-tender")).toBeInTheDocument();
    });
    expect(screen.queryByText("bill.cashier")).not.toBeInTheDocument();
    expect(screen.queryByText("bill.payWithCashier")).not.toBeInTheDocument();
  });
  // Issue 894: the guest rail list now carries the venue's own counter rail
  // when the drawer is open, so a venue with no processor credentials is not
  // reported as having nothing to tap. It is not a plugin — it must render
  // through the existing localized cashier tile exactly once, never as a raw
  // "Counter Cash" chip that would POST /plugin-payment with that name.
  it("renders the listed house counter rail once, as the localized cashier tile", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          name: "counter_cash",
          category: "payment",
          is_enabled: true,
          settlement: "counter",
        },
      ],
      counter_settlement_ready: true,
    });

    render(
      <PaymentSection
        billId={1}
        billToken="B42-opaque"
        businessId={42}
        amount={185.5}
        businessName="Cafe"
        businessAddress="0x1111111111111111111111111111111111111111"
        tipAddress="0x456"
        tableCode="T1"
        onPaymentComplete={jest.fn()}
        onCashierPayment={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getAllByText("bill.cashier")).toHaveLength(1);
    });
    expect(screen.queryByText("Counter Cash")).not.toBeInTheDocument();
    expect(screen.queryByTestId("guest-no-tender")).not.toBeInTheDocument();
    expect(await screen.findByText("bill.payWithCashier")).toBeInTheDocument();
  });

  // The house rail is a listing, not a second source of truth: a payload that
  // lists it while reporting the drawer closed must not resurrect a cashier
  // tile that cannot settle.
  it("still blocks checkout when the house rail is listed but the drawer is closed", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          name: "counter_cash",
          category: "payment",
          is_enabled: true,
          settlement: "counter",
        },
      ],
      counter_settlement_ready: false,
    });

    render(
      <PaymentSection
        billId={1}
        billToken="B42-opaque"
        businessId={42}
        amount={185.5}
        businessName="Cafe"
        businessAddress="0x1111111111111111111111111111111111111111"
        tipAddress="0x456"
        tableCode="T1"
        onPaymentComplete={jest.fn()}
        onCashierPayment={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId("guest-no-tender")).toBeInTheDocument();
    });
    expect(screen.queryByText("Counter Cash")).not.toBeInTheDocument();
    expect(screen.queryByText("bill.cashier")).not.toBeInTheDocument();
  });

  it("keeps payment amount summary when tip can change total (non-cashier)", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          name: "paypal",
          display_name: "PayPal",
          is_enabled: true,
        },
      ],
    });

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

    await waitFor(() => {
      expect(screen.getByText("PayPal")).toBeInTheDocument();
    });
    // First method may be PayPal or cashier depending on ordering — select PayPal.
    fireEvent.click(screen.getByText("PayPal"));
    await waitFor(() => {
      expect(screen.getByTestId("payment-amount-summary")).toBeInTheDocument();
    });
    expect(await screen.findByText("bill.amount")).toBeInTheDocument();
    expect(await screen.findByText("common.total")).toBeInTheDocument();
  });

  it("renders Mercado Pago as a first-class brand CTA, not a peer chip", async () => {
    (paymentPluginAPI.getBusinessPaymentPlugins as jest.Mock).mockResolvedValue({
      plugins: [
        {
          name: "paypal",
          display_name: "PayPal",
          is_enabled: true,
        },
        {
          name: "mercadopago",
          display_name: "MercadoPago",
          is_enabled: true,
        },
      ],
    });

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

    const brandCta = await screen.findByTestId("mercadopago-brand-cta");
    expect(brandCta).toHaveTextContent("payment.payWithMercadoPago");
    // Brand blue from Mercado Pago press kit.
    expect(brandCta).toHaveStyle({ backgroundColor: "#009EE3" });

    // Auto-selected first; primary pay CTA uses brand label, not bill.payNow.
    // Brand selector + pay button both use the same key — require at least two.
    await waitFor(() => {
      const branded = screen.getAllByRole("radio", {
        name: /payment\.payWithMercadoPago/i,
      });
      expect(branded.length).toBeGreaterThanOrEqual(1);
      expect(
        screen.getByRole("button", { name: /payment\.payWithMercadoPago/i }),
      ).toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /bill\.payNow/i }),
      ).toBeNull();
    });

    // Peer grid still lists other card rails (PayPal) but not MercadoPago as a chip.
    expect(await screen.findByText("PayPal")).toBeInTheDocument();
    // display_name alone must not appear as a peer chip label — brand CTA uses the i18n key.
    expect(screen.queryByText("MercadoPago")).not.toBeInTheDocument();
  });
});
