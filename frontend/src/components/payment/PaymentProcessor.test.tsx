/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

import PaymentProcessor from "@/components/payment/PaymentProcessor";

// --- Mock the bills API: real exports preserved, quote/update stubbed. ---
jest.mock("../../api/bills", () => ({
  ...jest.requireActual("../../api/bills"),
  updateBillPayment: jest.fn().mockResolvedValue({ success: true }),
  // Payer-bound quote: 55 USD plus the server-chosen sub-cent offset (4919
  // micro-USDC) that ties the on-chain transfer to this one quote.
  getCryptoQuote: jest.fn().mockResolvedValue({
    quote_id: 9,
    usd_microunits: 55_004_919,
    usd_amount: 55.004919,
    rate: 1,
    settlement_address: "0x3333333333333333333333333333333333333333",
    chain_id: 8453,
    token: "USDC",
    expires_at: Math.floor(Date.now() / 1000) + 600,
    quote_token: "tok.x",
  }),
}));

const updateBillPaymentMock = () =>
  require("../../api/bills").updateBillPayment as jest.Mock;

// --- Mock viem so on-chain receipt polling resolves instantly. The real
// module pulls in TextEncoder and abi encoders the jsdom env lacks; the
// component only needs createPublicClient + http (and the Hash type). ---
const mockWriteContract = jest.fn().mockResolvedValue("0xhash" as const);
// Recorded so CR-4 (confirmations) and CR-5 (RPC transport) can be asserted.
const mockWaitForTransactionReceipt = jest
  .fn()
  .mockResolvedValue({ status: "success" });
const mockHttp = jest.fn(() => ({}));
// The factory is hoisted above the `const` handles, so reference them lazily
// (inside thunks) — eager `http: mockHttp` would dereference before init.
jest.mock("viem", () => ({
  createPublicClient: jest.fn(() => ({
    waitForTransactionReceipt: (
      ...args: Parameters<typeof mockWaitForTransactionReceipt>
    ) => mockWaitForTransactionReceipt(...args),
  })),
  http: (...args: Parameters<typeof mockHttp>) => mockHttp(...args),
  parseUnits: jest.fn(() => BigInt(0)),
}));

// --- Mock the standalone wallet hook with a connected account. ---
// Mutable so the CR-6 test can swap in a walletClient with no `account`.
// Must be `mock`-prefixed to be referenced inside the jest.mock factory.
const mockWalletState = {
  walletClient: {
    account: { address: "0xguest" } as { address: string } | null,
    getChainId: jest.fn().mockResolvedValue(8453),
    writeContract: mockWriteContract,
  } as Record<string, unknown> | null,
};
jest.mock("../../lib/lifi/WalletConnectProvider", () => ({
  CrossChainWalletProvider: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  useCrossChainWallet: () => ({
    address: "0xguest",
    isConnected: true,
    isConnecting: false,
    chainId: 8453,
    walletClient: mockWalletState.walletClient,
    connectInjected: jest.fn(),
    connectWalletConnect: jest.fn(),
    disconnect: jest.fn(),
    switchChain: jest.fn(),
    hasWalletConnect: false,
  }),
}));

// USDC_ADDRESSES lives in @/constants/usdc. Stub the contract the transfer
// targets so the test does not depend on the production Base address.
jest.mock("@/constants/usdc", () => ({
  USDC_ADDRESSES: { 8453: "0xUSDC" } as Record<number, string>,
}));

jest.mock("../../config/network", () => ({
  getNetwork: () => ({
    id: 8453,
    name: "Base",
    blockExplorers: { default: { url: "https://basescan.org" } },
  }),
  getNetworkId: () => 8453,
}));

jest.mock("../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
    currentLanguage: "en",
  }),
}));

jest.mock("../../api/currency", () => ({
  formatCurrency: (value: number) => `$${value}`,
  convertAmount: jest.fn(),
}));

// Lightweight NextUI stand-ins so onPress maps to a real click handler.
jest.mock("@nextui-org/react", () => ({
  Modal: ({
    isOpen,
    children,
  }: {
    isOpen: boolean;
    children: React.ReactNode;
  }) => (isOpen ? <div>{children}</div> : null),
  ModalContent: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  ModalHeader: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  ModalBody: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  ModalFooter: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  Button: ({
    children,
    onPress,
    ...rest
  }: {
    children: React.ReactNode;
    onPress?: () => void;
    [key: string]: unknown;
  }) => (
    <button type="button" onClick={onPress} {...(rest as object)}>
      {children}
    </button>
  ),
  Progress: () => <div>progress</div>,
  Spinner: () => <div>spinner</div>,
  Chip: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
}));

describe("PaymentProcessor (direct USDC)", () => {
  const ORIGINAL_RPC_URL = process.env.NEXT_PUBLIC_RPC_URL;
  beforeEach(() => {
    jest.clearAllMocks();
    sessionStorage.clear();
    mockWriteContract.mockResolvedValue("0xhash" as const);
    mockWaitForTransactionReceipt.mockResolvedValue({ status: "success" });
    mockHttp.mockReturnValue({});
    // Restore a fully-connected wallet with a valid account for each test.
    mockWalletState.walletClient = {
      account: { address: "0xguest" },
      getChainId: jest.fn().mockResolvedValue(8453),
      writeContract: mockWriteContract,
    };
    delete process.env.NEXT_PUBLIC_RPC_URL;
  });
  afterEach(() => {
    if (ORIGINAL_RPC_URL === undefined) {
      delete process.env.NEXT_PUBLIC_RPC_URL;
    } else {
      process.env.NEXT_PUBLIC_RPC_URL = ORIGINAL_RPC_URL;
    }
  });

  it("never submits a crypto transfer when the authoritative quote is unavailable", async () => {
    const { getCryptoQuote } = require("../../api/bills");
    getCryptoQuote.mockRejectedValueOnce({
      response: {
        status: 403,
        data: { code: "business_unavailable" },
      },
      message: "This business is not available right now.",
    });

    render(
      <PaymentProcessor
        isOpen
        onClose={jest.fn()}
        billToken="B-77"
        totalAmount={55}
        tipAmount={5}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="USD"
      />,
    );

    fireEvent.click(await screen.findByText(/Pay \$55\b/i));

    expect(
      await screen.findAllByText("menu.orderingDisabled"),
    ).not.toHaveLength(0);
    expect(mockWriteContract).not.toHaveBeenCalled();
  });

  it("locks a server quote, transfers the exact micro-units, and threads quote_token", async () => {
    const { getCryptoQuote, updateBillPayment } = require("../../api/bills");

    render(
      <PaymentProcessor
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-77"
        totalAmount={55}
        tipAmount={5}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="USD"
        splitShareId={42}
      />,
    );

    // Connected wallet auto-advances to the "ready" step; click Pay.
    const payButton = await screen.findByText(/Pay \$55\b/i);
    // Audit CRIT-3: the pre-quote figure is the business-currency total, so
    // it must NOT be labeled "USDC" (the exact USDC comes from the quote
    // locked on click and is only shown in the wallet's signing prompt).
    expect(payButton.textContent).not.toMatch(/USDC/);
    fireEvent.click(payButton);

    await waitFor(() => {
      expect(getCryptoQuote).toHaveBeenCalledWith(
        "B-77",
        expect.objectContaining({
          amount_paid: expect.any(Number),
          tip_amount: expect.any(Number),
          payment_method: "usdc_payment",
          split_share_id: 42,
        }),
      );
    });

    // The on-chain transfer must move EXACTLY the locked micro-units.
    await waitFor(() => {
      expect(mockWriteContract).toHaveBeenCalledWith(
        expect.objectContaining({
          args: [
            "0x3333333333333333333333333333333333333333",
            BigInt(55_004_919),
          ],
        }),
      );
    });

    // The settlement notification must forward the signed quote token.
    await waitFor(() => {
      expect(updateBillPayment).toHaveBeenCalledWith(
        "B-77",
        expect.objectContaining({ quote_token: "tok.x", split_share_id: 42 }),
      );
    });
  });

  it("shows the exact payer-bound USDC amount while the wallet prompt is open", async () => {
    // Hold the wallet prompt open so the processing step stays on screen.
    let approveInWallet: (hash: `0x${string}`) => void = () => undefined;
    mockWriteContract.mockImplementationOnce(
      () =>
        new Promise<`0x${string}`>((resolve) => {
          approveInWallet = resolve;
        }),
    );

    render(
      <PaymentProcessor
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-77"
        totalAmount={55}
        tipAmount={5}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="USD"
      />,
    );

    fireEvent.click(await screen.findByText(/Pay \$55\b/i));

    // All six decimals: rounding to 55.00 would hide the binding offset and
    // show a figure the wallet is not asking the guest to sign.
    const exact = await screen.findByTestId("exact-usdc-amount");
    expect(exact.textContent).toBe("55.004919 USDC");
    expect(screen.getByText("Exact USDC amount")).toBeInTheDocument();
    expect(
      screen.getByText(
        "The last digits of this amount link your transfer to this bill.",
      ),
    ).toBeInTheDocument();
    expect(mockWriteContract).toHaveBeenCalledTimes(1);
    expect(mockWriteContract.mock.calls[0][0].args[1]).toBe(BigInt(55_004_919));

    approveInWallet("0xhash");
    await waitFor(() => {
      expect(screen.queryByTestId("exact-usdc-amount")).not.toBeInTheDocument();
    });
  });

  it("never transfers when the quoted micro-units are not an exact integer", async () => {
    const { getCryptoQuote } = require("../../api/bills");
    getCryptoQuote.mockResolvedValueOnce({
      quote_id: 9,
      usd_microunits: 55_004_919.5,
      usd_amount: 55.0049195,
      rate: 1,
      settlement_address: "0x3333333333333333333333333333333333333333",
      chain_id: 8453,
      token: "USDC",
      expires_at: Math.floor(Date.now() / 1000) + 600,
      quote_token: "tok.x",
    });

    render(
      <PaymentProcessor
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-77"
        totalAmount={55}
        tipAmount={5}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="USD"
      />,
    );

    fireEvent.click(await screen.findByText(/Pay \$55\b/i));

    await waitFor(() => {
      expect(
        screen.getAllByText("paymentProcessor.currencyConversionUnavailable")
          .length,
      ).toBeGreaterThan(0);
    });
    expect(mockWriteContract).not.toHaveBeenCalled();
  });

  it("surfaces the currencyConversionUnavailable key when the quote is zeroed", async () => {
    const { getCryptoQuote } = require("../../api/bills");
    getCryptoQuote.mockResolvedValueOnce({
      usd_microunits: 0,
      usd_amount: 0,
      rate: 0,
      expires_at: Math.floor(Date.now() / 1000) + 600,
      quote_token: "",
    });

    render(
      <PaymentProcessor
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-77"
        totalAmount={55}
        tipAmount={5}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="USD"
      />,
    );

    const payButton = await screen.findByText(/Pay \$55\b/i);
    fireEvent.click(payButton);

    // The catch block sets the thrown message; with the identity `t` mock the
    // thrown message is the key itself. The error step surfaces it in both the
    // step description and the red error banner, so assert at least one (never
    // the old English literal) is present.
    await waitFor(() => {
      expect(
        screen.getAllByText("paymentProcessor.currencyConversionUnavailable")
          .length,
      ).toBeGreaterThan(0);
    });
  });

  it("persists the in-flight record after writeContract and clears it on success", async () => {
    const resume = require("@/lib/cryptoPaymentResume");
    const { CRYPTO_INFLIGHT_KEY } = resume;
    const writeSpy = jest.spyOn(resume, "writeCryptoInflight");

    render(
      <PaymentProcessor
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-77"
        totalAmount={55}
        tipAmount={5}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="USD"
      />,
    );

    const payButton = await screen.findByText(/Pay \$55\b/i);
    fireEvent.click(payButton);

    // The guard must be persisted the instant we have a hash (confirmed:false),
    // before any backend finalize — proving a refresh mid-confirm keeps the hash.
    await waitFor(() => {
      expect(writeSpy).toHaveBeenCalledWith(
        expect.objectContaining({
          billToken: "B-77",
          txHash: "0xhash",
          quoteToken: "tok.x",
          confirmed: false,
        }),
      );
    });

    // After a successful finalize the key must be cleared.
    await waitFor(() => {
      expect(updateBillPaymentMock()).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(sessionStorage.getItem(CRYPTO_INFLIGHT_KEY)).toBeNull();
    });

    writeSpy.mockRestore();
  });

  it("restores the finalize screen when a confirmed record exists (no second writeContract)", async () => {
    const { writeCryptoInflight } = require("@/lib/cryptoPaymentResume");
    writeCryptoInflight({
      billToken: "B-77",
      txHash: "0xrestored",
      quoteToken: "tok.restored",
      confirmed: true,
    });

    render(
      <PaymentProcessor
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-77"
        totalAmount={55}
        tipAmount={5}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="USD"
      />,
    );

    // Confirmed restore lands on the finalize/retry screen, which exposes a
    // Retry button and the restored hash — NOT the "Pay ... USDC" button. Use
    // the button role so the descriptive "...Tap Retry to finish..." copy in
    // the body paragraph isn't mistaken for the control.
    const retry = await screen.findByRole("button", {
      name: /paymentProcessor\.retryFinalize|Retry/i,
    });
    fireEvent.click(retry);

    // Retry must re-notify the SAME restored hash and NEVER call writeContract.
    await waitFor(() => {
      expect(updateBillPaymentMock()).toHaveBeenCalledWith(
        "B-77",
        expect.objectContaining({
          transaction_hash: "0xrestored",
          quote_token: "tok.restored",
        }),
      );
    });
    expect(mockWriteContract).not.toHaveBeenCalled();
  });

  it("re-enters confirmation for an unconfirmed record without a second writeContract", async () => {
    const { writeCryptoInflight } = require("@/lib/cryptoPaymentResume");
    writeCryptoInflight({
      billToken: "B-77",
      txHash: "0xpending",
      quoteToken: "tok.pending",
      confirmed: false,
    });

    render(
      <PaymentProcessor
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-77"
        totalAmount={55}
        tipAmount={5}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="USD"
      />,
    );

    // The mocked publicClient resolves the receipt instantly, so the resumed
    // confirmation finalizes against the restored hash — without writeContract.
    await waitFor(() => {
      expect(updateBillPaymentMock()).toHaveBeenCalledWith(
        "B-77",
        expect.objectContaining({
          transaction_hash: "0xpending",
          quote_token: "tok.pending",
        }),
      );
    });
    expect(mockWriteContract).not.toHaveBeenCalled();
  });

  it("ignores an in-flight record for a different bill (bill-scope guard)", async () => {
    const { writeCryptoInflight } = require("@/lib/cryptoPaymentResume");
    writeCryptoInflight({
      billToken: "B-OTHER",
      txHash: "0xother",
      quoteToken: "tok.other",
      confirmed: true,
    });

    render(
      <PaymentProcessor
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-77"
        totalAmount={55}
        tipAmount={5}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="USD"
      />,
    );

    // The record is for B-OTHER, so B-77 must start fresh at "ready" with a Pay
    // button — not the finalize/retry screen.
    expect(await screen.findByText(/Pay \$55\b/i)).toBeInTheDocument();
  });

  it("on backend reject after a confirmed transfer: error step, tx hash linked, exactly one writeContract", async () => {
    const { updateBillPayment } = require("../../api/bills");
    updateBillPayment.mockRejectedValueOnce(new Error("backend 503"));

    render(
      <PaymentProcessor
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-77"
        totalAmount={55}
        tipAmount={5}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="USD"
      />,
    );

    fireEvent.click(await screen.findByText(/Pay \$55\b/i));

    // The transfer settled, but the backend notify failed → "finalizing" branch.
    // `guestText` returns the fallback under the identity `t` mock (t(key)===key),
    // so the amber confirmed-title renders its English fallback, not the key.
    await screen.findByText("Payment received — finalizing");

    // The tx hash is rendered as a block-explorer anchor (on-chain proof).
    const txLink = screen.getByText("0xhash").closest("a");
    expect(txLink).not.toBeNull();
    expect(txLink).toHaveAttribute("href", "https://basescan.org/tx/0xhash");

    // CR-2 precondition: the on-chain transfer ran exactly once.
    expect(mockWriteContract).toHaveBeenCalledTimes(1);
  });

  it("CR-2: retry after a confirmed transfer re-finalizes the same hash without a second transfer", async () => {
    const { updateBillPayment } = require("../../api/bills");
    updateBillPayment
      .mockRejectedValueOnce(new Error("backend 503"))
      .mockResolvedValueOnce({ success: true });

    render(
      <PaymentProcessor
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-77"
        totalAmount={55}
        tipAmount={5}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="USD"
      />,
    );

    fireEvent.click(await screen.findByText(/Pay \$55\b/i));

    // First notify failed → confirmed error branch with the Retry button. Under
    // the identity `t` mock `guestText` returns the "Retry" fallback (not the
    // key). Use the button role so the descriptive body copy isn't matched.
    const retryButton = await screen.findByRole("button", {
      name: /paymentProcessor\.retryFinalize|Retry/i,
    });
    fireEvent.click(retryButton);

    // Second notify resolves → success step. "paymentProcessor.totalPaid" is
    // rendered via `t(key) || fallback`; the identity mock returns the truthy
    // key, so the success label is the key itself (only renders in renderSuccess).
    await screen.findByText("paymentProcessor.totalPaid");

    // CR-2: the on-chain transfer ran exactly once across attempt + retry.
    expect(mockWriteContract).toHaveBeenCalledTimes(1);

    // Both notifies carried the SAME confirmed hash (idempotent re-notify).
    expect(updateBillPayment).toHaveBeenCalledTimes(2);
    const firstHash = updateBillPayment.mock.calls[0][1].transaction_hash;
    const secondHash = updateBillPayment.mock.calls[1][1].transaction_hash;
    expect(firstHash).toBe("0xhash");
    expect(secondHash).toBe("0xhash");
  });

  it("CR-6: undefined walletClient.account shows a user-facing error and never transfers", async () => {
    // Drop the account but keep the rest of the client wired.
    mockWalletState.walletClient = {
      account: null,
      getChainId: jest.fn().mockResolvedValue(8453),
      writeContract: mockWriteContract,
    };

    render(
      <PaymentProcessor
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-77"
        totalAmount={55}
        tipAmount={5}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="USD"
      />,
    );

    fireEvent.click(await screen.findByText(/Pay \$55\b/i));

    // The missing-account guard fires after getCryptoQuote but before
    // writeContract; the thrown message is `t(key) || fallback`, and the
    // identity mock returns the truthy key. The error step surfaces it in both
    // the step description and the red error banner, so at least one renders
    // (and crucially, never an unhandled crash).
    await waitFor(() => {
      expect(
        screen.getAllByText("paymentProcessor.walletAccountUnavailable").length,
      ).toBeGreaterThan(0);
    });

    // No on-chain transfer was ever attempted.
    expect(mockWriteContract).not.toHaveBeenCalled();
  });

  it("CR-5/CR-4: receipt client uses NEXT_PUBLIC_RPC_URL transport and 3 confirmations", async () => {
    process.env.NEXT_PUBLIC_RPC_URL = "https://rpc.example.test";

    render(
      <PaymentProcessor
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-77"
        totalAmount={55}
        tipAmount={5}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="USD"
      />,
    );

    fireEvent.click(await screen.findByText(/Pay \$55\b/i));

    // Success step confirms the receipt-polling block executed.
    await screen.findByText("paymentProcessor.totalPaid");

    // CR-5: transport built from the configured RPC URL.
    expect(mockHttp).toHaveBeenCalledWith("https://rpc.example.test");

    // CR-4: receipt awaited with 3 confirmations.
    expect(mockWaitForTransactionReceipt).toHaveBeenCalledWith(
      expect.objectContaining({ hash: "0xhash", confirmations: 3 }),
    );
  });
});
