/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

import CrossChainPayment from "@/components/payment/CrossChainPayment";

// --- Mock the bills API: real exports preserved, quote/settle stubbed. ---
jest.mock("../../api/bills", () => ({
  ...jest.requireActual("../../api/bills"),
  processCrossChainPayment: jest.fn().mockResolvedValue({ success: true }),
  getCryptoQuote: jest.fn().mockResolvedValue({
    usd_microunits: 108_000_000,
    usd_amount: 108,
    rate: 1.08,
    settlement_address: "0x3333333333333333333333333333333333333333",
    chain_id: 8453,
    token: "USDC",
    expires_at: Math.floor(Date.now() / 1000) + 600,
    quote_token: "tok.xc",
  }),
}));

// --- Mock the standalone wallet hook: connected account + an executePayment
// that immediately invokes the success callback with a tx hash + route. ---
const mockExecutePayment = jest.fn(
  async (
    route: { id: string },
    onSuccess: (txHash: string, executedRoute: { id: string }) => void,
    // Mirrors the production executePayment signature (onError is the optional
    // third callback). Declared here so per-spec failure-path overrides can
    // supply a 3-arg impl without a signature mismatch.
    _onError?: (error: string) => void,
  ) => {
    onSuccess("0xhash", route);
  },
);
// A single stable wallet object so identity-sensitive effects (the reset
// effect depends on resetPayment) don't re-run every render and bounce the
// step back to select-token.
const mockResetPayment = jest.fn();
const mockWalletValue = {
  address: "0xguest",
  isConnected: true,
  chainId: 8453,
  isConnecting: false,
  connectInjected: jest.fn(),
  connectWalletConnect: jest.fn(),
  disconnect: jest.fn(),
  hasWalletConnect: false,
  paymentProgress: { currentStep: 0, totalSteps: 0, txHash: "" },
  executePayment: mockExecutePayment,
  resetPayment: mockResetPayment,
};
jest.mock("../../lib/lifi/WalletConnectProvider", () => ({
  CrossChainWalletProvider: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  useCrossChainWallet: () => mockWalletValue,
}));

// --- Mock the LI.FI hooks: real module pulls in @lifi/sdk + viem which the
// jsdom env can't load. Provide a controlled token, route, and fetchRoutes. ---
const mockFetchRoutes = jest.fn();
const fakeToken = {
  address: "0xtoken",
  symbol: "WETH",
  decimals: 18,
  name: "Wrapped Ether",
  priceUSD: "3000",
  logoURI: "",
  chainId: 8453,
};
const fakeRoute = { id: "route-1" };
jest.mock("../../lib/lifi/hooks", () => ({
  useAvailableChains: () => ({
    chains: [{ id: 8453, name: "Base" }],
    loading: false,
  }),
  useAvailableTokens: () => ({ tokens: [fakeToken], loading: false }),
  usePaymentRoutes: () => ({
    routes: [
      {
        route: fakeRoute,
        fromToken: { symbol: "WETH", decimals: 18, logoURI: "" },
        fromAmount: "1000000000000000000",
        toAmount: "108000000",
        estimatedTime: 120,
        estimatedGas: "1.50",
        steps: 1,
      },
    ],
    loading: false,
    fetchRoutes: mockFetchRoutes,
  }),
  formatTokenAmount: (amount: string) => amount,
  getChainName: () => "Base",
}));

jest.mock("../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
    currentLanguage: "en",
  }),
}));

jest.mock("../../api/currency", () => ({
  formatCurrency: (value: number) => `$${Number(value).toFixed(2)}`,
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
  Card: ({
    children,
    onPress,
  }: {
    children: React.ReactNode;
    onPress?: () => void;
  }) => (
    <div role="button" onClick={onPress} onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') onPress?.(); }}>
      {children}
    </div>
  ),
  CardBody: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  Progress: () => <div>progress</div>,
  Spinner: () => <div>spinner</div>,
  Chip: ({ children }: { children: React.ReactNode }) => (
    <span>{children}</span>
  ),
  Select: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
  SelectItem: ({ children }: { children: React.ReactNode }) => (
    <div>{children}</div>
  ),
}));

describe("CrossChainPayment (LI.FI cross-chain)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    window.sessionStorage.clear();
    // Restore the wallet's active chain (a test below mutates it to simulate a
    // mid-bridge chain hop).
    mockWalletValue.chainId = 8453;
    mockFetchRoutes.mockResolvedValue([fakeRoute]);
    mockExecutePayment.mockImplementation(
      async (
        route: { id: string },
        onSuccess: (txHash: string, executedRoute: { id: string }) => void,
      ) => {
        onSuccess("0xhash", route);
      },
    );
  });

  it("locks a server quote, routes by the locked USD, and threads quote_token", async () => {
    const {
      getCryptoQuote,
      processCrossChainPayment,
    } = require("../../api/bills");

    render(
      <CrossChainPayment
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-99"
        totalAmount={100}
        tipAmount={8}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="EUR"
        splitShareId={42}
      />,
    );

    // Connected wallet auto-advances to token selection; pick the token.
    const tokenCard = await screen.findByText("WETH");
    fireEvent.click(tokenCard);

    // The quote must be fetched with split amount_paid / tip_amount.
    await waitFor(() => {
      expect(getCryptoQuote).toHaveBeenCalledWith(
        "B-99",
        expect.objectContaining({
          amount_paid: expect.any(Number),
          tip_amount: expect.any(Number),
          payment_method: "cross_chain_payment",
          split_share_id: 42,
        }),
      );
    });

    // LI.FI must route by the locked USD (108_000_000 / 1e6 === 108).
    await waitFor(() => {
      expect(mockFetchRoutes).toHaveBeenCalledWith(
        expect.objectContaining({
          amountUsd: 108,
          toAddress: "0x3333333333333333333333333333333333333333",
        }),
      );
    });

    // The route appears (showing the locked 108000000 micro-USDC output);
    // clicking the "Best" route card executes + settles.
    const bestChip = await screen.findByText("Best");
    fireEvent.click(bestChip);

    // The settlement notification must forward the signed quote token.
    await waitFor(() => {
      expect(processCrossChainPayment).toHaveBeenCalledWith(
        "B-99",
        expect.objectContaining({ quote_token: "tok.xc", split_share_id: 42 }),
      );
    });
  });

  it("does not reset in-flight route state when the wallet switches chains mid-bridge", async () => {
    const props = {
      isOpen: true,
      onClose: jest.fn(),
      billToken: "B-99",
      totalAmount: 100,
      tipAmount: 8,
      businessName: "Cafe",
      businessAddress: "0xbiz",
      currency: "EUR",
    } as const;
    const { rerender } = render(<CrossChainPayment {...props} />);

    // Drive to the route screen (past token selection).
    fireEvent.click(await screen.findByText("WETH"));
    expect(await screen.findByText("Best")).toBeInTheDocument();
    const resetCallsBefore = mockResetPayment.mock.calls.length;

    // LI.FI switches the wallet's active chain as an expected part of executeRoute.
    // The reset effect must NOT re-fire and wipe the selected token/route/quote.
    mockWalletValue.chainId = 42161;
    rerender(<CrossChainPayment {...props} />);

    // Still on the route screen — not bounced back to token selection — and the
    // wallet-context reset did not run again.
    expect(screen.getByText("Best")).toBeInTheDocument();
    expect(mockResetPayment.mock.calls.length).toBe(resetCallsBefore);
  });

  it("labels the route settlement minimum in USD (locked quote), not the business-currency total (D-04)", async () => {
    render(
      <CrossChainPayment
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-99"
        totalAmount={100}
        tipAmount={8}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="EUR"
      />,
    );

    // Pick the token → locks the quote (usd_microunits 108_000_000 → 108 USD)
    // and advances to the route screen.
    fireEvent.click(await screen.findByText("WETH"));

    // The "receive at least" line must show the locked USD figure as USDC, not
    // the business-currency total mislabeled with a USDC suffix ("$108 USDC").
    expect(await screen.findByText("108.00 USDC")).toBeInTheDocument();
    // The old mislabel (business-currency value via formatCurrency, $108, with a
    // trailing "USDC") must not be what's shown for the settlement amount.
    expect(screen.queryByText("$108 USDC")).toBeNull();
  });

  // Audit C-02: the route screen's strings are localized templates — the
  // placeholders must be interpolated, never leaked raw to the guest.
  it("interpolates the localized route-screen templates (no raw placeholders)", async () => {
    const { container } = render(
      <CrossChainPayment
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-99"
        totalAmount={100}
        tipAmount={8}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="EUR"
      />,
    );

    fireEvent.click(await screen.findByText("WETH"));

    // English fallbacks with the counts substituted (1 route, 2 min, 1 step).
    expect(
      await screen.findByText("Routes found for your payment: 1"),
    ).toBeInTheDocument();
    expect(screen.getByText("~2 min")).toBeInTheDocument();
    expect(screen.getByText("Steps: 1")).toBeInTheDocument();
    expect(screen.getByText("Gas: $1.50")).toBeInTheDocument();
    expect(screen.getByText("Refresh Routes")).toBeInTheDocument();

    // No template placeholder may leak into the rendered screen.
    expect(container.textContent).not.toMatch(/\{(n|amount|token|address|current|total)\}/);
  });

  it("retry after a notify failure re-notifies the same hash and never re-executes the route", async () => {
    const { processCrossChainPayment } = require("../../api/bills");

    // First notify fails (backend unreachable); second succeeds.
    processCrossChainPayment
      .mockRejectedValueOnce(new Error("network down"))
      .mockResolvedValueOnce({ success: true });

    render(
      <CrossChainPayment
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-99"
        totalAmount={100}
        tipAmount={8}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="EUR"
      />,
    );

    fireEvent.click(await screen.findByText("WETH"));
    fireEvent.click(await screen.findByText("Best"));

    // executePayment ran exactly once and the first notify failed → error step.
    await waitFor(() =>
      expect(processCrossChainPayment).toHaveBeenCalledTimes(1),
    );
    expect(mockExecutePayment).toHaveBeenCalledTimes(1);

    // Retry: must re-send the SAME hash, NOT run the route again.
    fireEvent.click(await screen.findByText("Try Again"));

    await waitFor(() =>
      expect(processCrossChainPayment).toHaveBeenCalledTimes(2),
    );
    // The guard's whole point: no second settlement.
    expect(mockExecutePayment).toHaveBeenCalledTimes(1);
    // Both notifies carried the identical final hash.
    expect(processCrossChainPayment.mock.calls[0][1].transaction_hash).toBe(
      "0xhash",
    );
    expect(processCrossChainPayment.mock.calls[1][1].transaction_hash).toBe(
      "0xhash",
    );
  });

  it("hides 'Try Different Token' once the transfer is confirmed", async () => {
    const { processCrossChainPayment } = require("../../api/bills");
    processCrossChainPayment.mockRejectedValueOnce(new Error("network down"));

    render(
      <CrossChainPayment
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-99"
        totalAmount={100}
        businessName="Cafe"
        businessAddress="0xbiz"
      />,
    );

    fireEvent.click(await screen.findByText("WETH"));
    fireEvent.click(await screen.findByText("Best"));

    await screen.findByText("Try Again");
    // Settled → the re-executable "Try Different Token" path must be gone.
    expect(screen.queryByText("Try Different Token")).toBeNull();
  });

  it("on remount with a persisted confirmed payment, re-notifies the same hash without re-executing", async () => {
    const { processCrossChainPayment } = require("../../api/bills");
    const { writeCryptoInflight } = require("../../lib/cryptoPaymentResume");

    // Simulate: route settled on Base last session, notify never landed. The
    // shared CryptoInflight record carries the FINAL Base hash in `txHash`.
    writeCryptoInflight({
      billToken: "B-99",
      txHash: "0xbase",
      quoteToken: "tok.xc",
      lifiRouteId: "route-1",
      confirmed: true,
      sourceToken: "WETH",
      sourceChain: "Polygon",
      sourceChainId: 137,
    });
    processCrossChainPayment.mockResolvedValueOnce({ success: true });

    render(
      <CrossChainPayment
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-99"
        totalAmount={100}
        tipAmount={8}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="EUR"
      />,
    );

    // The resume path notifies the persisted hash on mount — no token/route pick.
    await waitFor(() =>
      expect(processCrossChainPayment).toHaveBeenCalledWith(
        "B-99",
        expect.objectContaining({
          transaction_hash: "0xbase",
          quote_token: "tok.xc",
        }),
      ),
    );
    // Critically: the route was NEVER re-executed.
    expect(mockExecutePayment).not.toHaveBeenCalled();
  });

  it("resume re-notify carries the SAME source_token/source_chain the first notify recorded (never UNKNOWN/fallback)", async () => {
    const { processCrossChainPayment } = require("../../api/bills");
    const { writeCryptoInflight } = require("../../lib/cryptoPaymentResume");

    // The first (pre-refresh) notify recorded a NON-default source: a Polygon
    // WETH payment. On a fresh remount selectedToken is null and the wallet is
    // not necessarily on Polygon, so deriving from state would yield
    // source_token="UNKNOWN" and a fallback source_chain ("Base" via the mock,
    // or Ethereum id 1 in prod). The backend's PaymentMatchesConfirmedInput
    // compares SourceChain/SourceToken EXACTLY, so a mismatch 409s the bill that
    // already settled on-chain. The resume MUST replay the persisted values.
    writeCryptoInflight({
      billToken: "B-99",
      txHash: "0xbase",
      quoteToken: "tok.xc",
      lifiRouteId: "route-1",
      confirmed: true,
      sourceToken: "WETH",
      sourceChain: "Polygon",
      sourceChainId: 137,
    });
    processCrossChainPayment.mockResolvedValueOnce({ success: true });

    render(
      <CrossChainPayment
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-99"
        totalAmount={100}
        tipAmount={8}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="EUR"
      />,
    );

    await waitFor(() =>
      expect(processCrossChainPayment).toHaveBeenCalledTimes(1),
    );
    const payload = processCrossChainPayment.mock.calls[0][1];
    expect(payload.transaction_hash).toBe("0xbase");
    // The load-bearing assertion: identical source identity to the original.
    expect(payload.source_token).toBe("WETH");
    expect(payload.source_token).not.toBe("UNKNOWN");
    expect(payload.source_chain).toBe("Polygon");
  });

  it("clears the persisted record on successful settlement", async () => {
    const { processCrossChainPayment } = require("../../api/bills");
    const { readCryptoInflight } = require("../../lib/cryptoPaymentResume");
    processCrossChainPayment.mockResolvedValue({ success: true });

    render(
      <CrossChainPayment
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-99"
        totalAmount={100}
        businessName="Cafe"
        businessAddress="0xbiz"
      />,
    );

    fireEvent.click(await screen.findByText("WETH"));
    fireEvent.click(await screen.findByText("Best"));

    await waitFor(() =>
      expect(processCrossChainPayment).toHaveBeenCalledTimes(1),
    );
    // Settled successfully → resume record must be gone.
    await waitFor(() => expect(readCryptoInflight("B-99")).toBeNull());
  });

  it("surfaces the currencyConversionUnavailable key when no quote is locked", async () => {
    const { getCryptoQuote } = require("../../api/bills");
    // Zeroed micro-units → handleTokenSelect's guard throws and quoteToken
    // stays null, so the surfaced error must be the translated key, not a
    // hardcoded English literal.
    getCryptoQuote.mockResolvedValueOnce({
      usd_microunits: 0,
      usd_amount: 0,
      rate: 0,
      expires_at: Math.floor(Date.now() / 1000) + 600,
      quote_token: "",
    });

    render(
      <CrossChainPayment
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-99"
        totalAmount={100}
        tipAmount={8}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="EUR"
      />,
    );

    const tokenCard = await screen.findByText("WETH");
    fireEvent.click(tokenCard);

    // The identity `t` mock returns the key verbatim; assert the key surfaces
    // (never the old "Currency conversion unavailable" literal).
    await waitFor(() => {
      expect(
        screen.getByText("paymentProcessor.currencyConversionUnavailable"),
      ).toBeInTheDocument();
    });
  });

  it("lands on the error step and never settles when execution fails via onError", async () => {
    const { processCrossChainPayment } = require("../../api/bills");

    // Execution reports failure through onError (the unsuccessful-settlement
    // path in WalletConnectProvider). The component must move to the error
    // step and must NOT notify the backend of a settlement that never happened.
    mockExecutePayment.mockImplementationOnce(
      async (
        _route: { id: string },
        _onSuccess: (txHash: string, executedRoute: { id: string }) => void,
        onError?: (error: string) => void,
      ) => {
        onError?.("route execution reverted");
      },
    );

    render(
      <CrossChainPayment
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-99"
        totalAmount={100}
        tipAmount={8}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="EUR"
      />,
    );

    fireEvent.click(await screen.findByText("WETH"));
    fireEvent.click(await screen.findByText("Best"));

    // Error step rendered: its retry/back actions are present.
    expect(await screen.findByText("Try Again")).toBeInTheDocument();
    // The onError message surfaces on the error step. The component renders it
    // in both the step description and the red error box, so assert presence
    // via getAllByText (at least one match) rather than a unique getByText.
    expect(
      screen.getAllByText("route execution reverted").length,
    ).toBeGreaterThan(0);

    // Success step is NEVER entered for a failed execution.
    expect(screen.queryByText("crossChain.step.successTitle")).toBeNull();
    expect(screen.queryByText("crossChain.totalPaid")).toBeNull();

    // A failed route must NOT be notified to the backend as settled.
    expect(processCrossChainPayment).not.toHaveBeenCalled();
    expect(mockExecutePayment).toHaveBeenCalledTimes(1);
  });

  it("catches a thrown executePayment rejection and lands on the error step", async () => {
    const { processCrossChainPayment } = require("../../api/bills");

    // executePayment rejects outright (wallet/provider blew up) rather than
    // routing through onError. The component's try/catch backstop must still
    // surface the error step and surface the thrown message.
    mockExecutePayment.mockImplementationOnce(async () => {
      throw new Error("wallet provider crashed");
    });

    render(
      <CrossChainPayment
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-99"
        totalAmount={100}
        tipAmount={8}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="EUR"
      />,
    );

    fireEvent.click(await screen.findByText("WETH"));
    fireEvent.click(await screen.findByText("Best"));

    expect(await screen.findByText("Try Again")).toBeInTheDocument();
    // The thrown message surfaces on the error step (rendered in both the step
    // description and the red error box); assert via getAllByText.
    expect(
      screen.getAllByText("wallet provider crashed").length,
    ).toBeGreaterThan(0);

    // Still no success, still no settlement notification.
    expect(screen.queryByText("crossChain.step.successTitle")).toBeNull();
    expect(processCrossChainPayment).not.toHaveBeenCalled();
  });

  it("handleRetry after a non-settled failure resets to select-token without re-locking a stale quote or notifying", async () => {
    const {
      getCryptoQuote,
      processCrossChainPayment,
    } = require("../../api/bills");

    // First execution fails via onError (route never settled →
    // transferConfirmed stays false).
    mockExecutePayment.mockImplementationOnce(
      async (
        _route: { id: string },
        _onSuccess: (txHash: string, executedRoute: { id: string }) => void,
        onError?: (error: string) => void,
      ) => {
        onError?.("route execution reverted");
      },
    );

    render(
      <CrossChainPayment
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-99"
        totalAmount={100}
        tipAmount={8}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="EUR"
      />,
    );

    // Pick token (locks quote #1), pick route, execution fails → error step.
    fireEvent.click(await screen.findByText("WETH"));
    await waitFor(() => expect(getCryptoQuote).toHaveBeenCalledTimes(1));
    fireEvent.click(await screen.findByText("Best"));
    await screen.findByText("Try Again");

    // Retry from the NON-settled error: must drop back to the token picker.
    fireEvent.click(screen.getByText("Try Again"));

    // We are back on select-token: the token card is rendered again and the
    // error-step retry button is gone.
    expect(await screen.findByText("WETH")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Try Again")).toBeNull());

    // handleRetry itself must NOT re-fetch a quote — the stale quote #1 is
    // discarded and a fresh one is only locked when the guest re-picks a
    // token. So the count stays at 1 until a new token selection.
    expect(getCryptoQuote).toHaveBeenCalledTimes(1);

    // And it must NEVER notify the backend of a settlement: the route failed.
    expect(processCrossChainPayment).not.toHaveBeenCalled();

    // The reset path ran resetPayment (wallet-context reset).
    expect(mockResetPayment).toHaveBeenCalled();
  });

  it("threads quote_token into processCrossChainPayment on the failure→retry→success path", async () => {
    const {
      getCryptoQuote,
      processCrossChainPayment,
    } = require("../../api/bills");

    // First route execution fails; the default beforeEach impl (onSuccess)
    // applies to the retry attempt, so the second route click settles.
    mockExecutePayment.mockImplementationOnce(
      async (
        _route: { id: string },
        _onSuccess: (txHash: string, executedRoute: { id: string }) => void,
        onError?: (error: string) => void,
      ) => {
        onError?.("route execution reverted");
      },
    );

    render(
      <CrossChainPayment
        isOpen={true}
        onClose={jest.fn()}
        billToken="B-99"
        totalAmount={100}
        tipAmount={8}
        businessName="Cafe"
        businessAddress="0xbiz"
        currency="EUR"
      />,
    );

    // Attempt 1: pick token, pick route → fails.
    fireEvent.click(await screen.findByText("WETH"));
    fireEvent.click(await screen.findByText("Best"));
    await screen.findByText("Try Again");

    // Retry resets to select-token (non-settled branch).
    fireEvent.click(screen.getByText("Try Again"));

    // Attempt 2: re-pick token (locks a FRESH quote) and re-select route.
    fireEvent.click(await screen.findByText("WETH"));
    // Two locks total: one per token pick (attempt 1 + attempt 2).
    await waitFor(() => expect(getCryptoQuote).toHaveBeenCalledTimes(2));
    fireEvent.click(await screen.findByText("Best"));

    // The recovered settlement must still forward the signed quote token.
    await waitFor(() =>
      expect(processCrossChainPayment).toHaveBeenCalledWith(
        "B-99",
        expect.objectContaining({ quote_token: "tok.xc" }),
      ),
    );
    // Exactly one settlement notify — the failed first attempt never notified.
    expect(processCrossChainPayment).toHaveBeenCalledTimes(1);
    // Two executions: the failed one and the recovered one.
    expect(mockExecutePayment).toHaveBeenCalledTimes(2);
  });
});
