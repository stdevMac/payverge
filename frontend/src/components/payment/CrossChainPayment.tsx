"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";

import React, { useState, useEffect, useCallback, useRef } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Card,
  CardBody,
  Progress,
  Spinner,
  Select,
  SelectItem,
  Chip,
} from "@nextui-org/react";
import {
  Wallet,
  AlertCircle,
  CheckCircle2,
  ArrowRight,
  Zap,
  Clock,
  DollarSign,
  RefreshCw,
  LogOut,
  ArrowLeft,
  X,
} from "lucide-react";

import { Token, Route } from "@lifi/sdk";
import {
  CrossChainWalletProvider,
  useCrossChainWallet,
} from "../../lib/lifi/WalletConnectProvider";
import { useGuestTranslation } from "../../i18n/GuestTranslationProvider";
import {
  usePaymentRoutes,
  useAvailableTokens,
  useAvailableChains,
  formatTokenAmount,
  getChainName,
} from "../../lib/lifi/hooks";
import { getCryptoQuote, processCrossChainPayment } from "../../api/bills";
import { getNetwork } from "../../config/network";
import {
  readCryptoInflight,
  writeCryptoInflight,
  clearCryptoInflight,
} from "../../lib/cryptoPaymentResume";
import { asDollars } from "@/types/money";
import { isUsableGuestSettlementAddress } from "@/lib/isEvmAddress";
import { formatCurrency as formatCurrencyIntl } from "../../api/currency";

interface CrossChainPaymentProps {
  isOpen: boolean;
  onClose: () => void;
  billToken: string;
  totalAmount: number; // Amount in USD
  tipAmount?: number;
  businessName: string;
  businessAddress: string; // Business payment address (receives USDC on Base)
  /** Business default currency (ISO 4217); falls back to USD. */
  currency?: string;
  splitShareId?: number;
  onPaymentComplete?: (paymentDetails: {
    totalPaid: number;
    tipAmount: number;
    paymentMethod: string;
    transactionId: string;
    sourceChain?: string;
    sourceToken?: string;
  }) => void;
}

type PaymentStep =
  | "connect"
  | "select-token"
  | "fetching-routes"
  | "select-route"
  | "executing"
  | "success"
  | "error";

// Inner component that uses the standalone wallet hooks
function CrossChainPaymentInner({
  isOpen,
  onClose,
  billToken,
  totalAmount,
  tipAmount = 0,
  businessName,
  businessAddress: _businessAddress,
  currency = "USD",
  splitShareId,
  onPaymentComplete,
}: CrossChainPaymentProps) {
  const [paymentStep, setPaymentStep] = useState<PaymentStep>("connect");
  const [error, setError] = useState<string>("");
  const { t, currentLanguage } = useGuestTranslation();
  const [selectedChainId, setSelectedChainId] = useState<number | null>(null);
  const [selectedToken, setSelectedToken] = useState<Token | null>(null);
  const [selectedRoute, setSelectedRoute] = useState<Route | null>(null);
  const [quoteToken, setQuoteToken] = useState<string | null>(null);
  // The locked USD settlement amount (quote.usd_microunits / 1e6). The route
  // screen settles on-chain to this USD figure, so it — not the business-
  // currency total — is what should appear next to the "USDC" label. (Audit
  // D-04.)
  const [quoteUsdAmount, setQuoteUsdAmount] = useState<number | null>(null);
  // True once the LI.FI route has settled USDC on Base. When set, a retry must
  // ONLY re-send the backend notification — never a second executeRoute — so a
  // failed notify can't double-settle. Mirrors PaymentProcessor.transferConfirmed.
  const [transferConfirmed, setTransferConfirmed] = useState(false);
  // The FINAL Base settlement hash from a confirmed route. Retained across the
  // attempt→retry transition so a re-notify re-sends the identical hash (the
  // backend keys idempotency on TxHash uniqueness).
  const [finalTxHash, setFinalTxHash] = useState<string | null>(null);
  // The executed route id, echoed back to the backend for traceability on retry.
  const [settledRouteId, setSettledRouteId] = useState<string | null>(null);
  // The RESOLVED source identity (token symbol + chain name) the first notify
  // recorded. Retained so a same-mount retry re-sends the IDENTICAL values the
  // backend already keyed the payment on — never re-deriving from live state
  // that could have drifted (e.g. the guest switched chains). The remount-resume
  // path reads these from the persisted CryptoInflight record instead.
  const [settledSourceToken, setSettledSourceToken] = useState<string | null>(
    null,
  );
  const [settledSourceChain, setSettledSourceChain] = useState<string | null>(
    null,
  );

  // Standalone wallet hooks (NOT Dynamic Labs - uses WalletConnect)
  const {
    address,
    isConnected,
    chainId: connectedChainId,
    isConnecting,
    connectInjected,
    connectWalletConnect,
    disconnect,
    hasWalletConnect,
    // Payment execution from context (uses the standalone wallet)
    paymentProgress: progress,
    executePayment,
    resetPayment,
  } = useCrossChainWallet();

  // LI.FI hooks (for fetching data only - not for payment execution)
  const { chains, loading: chainsLoading } = useAvailableChains();
  const { tokens, loading: tokensLoading } = useAvailableTokens(
    selectedChainId || connectedChainId,
  );
  const { routes, loading: routesLoading, fetchRoutes } = usePaymentRoutes();

  const formatCurrency = (value: number) =>
    formatCurrencyIntl(value, currency, undefined, currentLanguage || "en");

  const getProgressValue = () => {
    switch (paymentStep) {
      case "connect":
        return 10;
      case "select-token":
        return 30;
      case "fetching-routes":
        return 50;
      case "select-route":
        return 60;
      case "executing":
        return 80;
      case "success":
        return 100;
      case "error":
        return 0;
      default:
        return 0;
    }
  };

  const tr = (key: string, fallback: string): string => {
    const v = t(`crossChain.step.${key}`) as string | undefined;
    return v && v !== `crossChain.step.${key}` ? v : fallback;
  };

  const guestText = (key: string, fallback: string): string => {
    const value = t(key) as string | undefined;
    return value && value !== key ? value : fallback;
  };

  // Renders a translated template around a styled JSX node: splits the
  // template on its placeholder and slots the node in. Keeps locale word
  // order intact (the placeholder can sit anywhere in the sentence).
  const renderTemplate = (
    template: string,
    placeholder: string,
    node: React.ReactNode,
  ): React.ReactNode => {
    const [before, after = ""] = template.split(placeholder);
    return (
      <>
        {before}
        {node}
        {after}
      </>
    );
  };

  const getStepTitle = () => {
    switch (paymentStep) {
      case "connect":
        return tr("connectTitle", "Connect Wallet");
      case "select-token":
        return tr("selectTokenTitle", "Select Payment Token");
      case "fetching-routes":
        return tr("fetchingRoutesTitle", "Finding Best Routes");
      case "select-route":
        return tr("selectRouteTitle", "Select Payment Route");
      case "executing":
        return tr("executingTitle", "Processing Payment");
      case "success":
        return tr("successTitle", "Payment Complete");
      case "error":
        return tr("errorTitle", "Payment Failed");
      default:
        return tr("defaultTitle", "Payment");
    }
  };

  const getStepDescription = () => {
    switch (paymentStep) {
      case "connect":
        return tr(
          "connectBody",
          "Connect your wallet to pay with any token from any chain",
        );
      case "select-token":
        return tr("selectTokenBody", "Choose which token you want to pay with");
      case "fetching-routes":
        return tr(
          "fetchingRoutesBody",
          "Finding the best route to convert your tokens to USDC",
        );
      case "select-route":
        return tr("selectRouteBody", "Select your preferred payment route");
      case "executing": {
        const body = tr("executingBody", "");
        return body
          ? body.replace("{business}", businessName)
          : `Processing your payment to ${businessName}`;
      }
      case "success":
        return tr(
          "successBody",
          "Your payment has been successfully processed",
        );
      case "error":
        return error || tr("errorBody", "Payment failed. Please try again.");
      default:
        return "";
    }
  };

  // Auto-advance when wallet connects
  useEffect(() => {
    if (isConnected && paymentStep === "connect") {
      setSelectedChainId(connectedChainId ?? null);
      setPaymentStep("select-token");
    }
  }, [isConnected, paymentStep, connectedChainId]);

  // Reset state ONLY on the modal's false→true open transition. This effect
  // still lists isConnected/connectedChainId as deps (they're read to seed the
  // initial step/chain), but the wasOpenRef guard stops the body from re-running
  // on mid-session changes. LiFi bridges switch the wallet's active chain as an
  // expected part of executeRoute (WalletConnectProvider setLiFiWalletClient),
  // and the previous unguarded body re-fired on every such switch — wiping the
  // in-flight route/quote/txHash and throwing the guest back to token selection
  // while their funds were bridging. The connect→select-token advance is handled
  // by the separate auto-advance effect above, so this only needs to fire once.
  const wasOpenRef = useRef(false);
  useEffect(() => {
    if (isOpen && !wasOpenRef.current) {
      setPaymentStep(isConnected ? "select-token" : "connect");
      setError("");
      setSelectedToken(null);
      setSelectedRoute(null);
      setQuoteToken(null);
      setQuoteUsdAmount(null);
      setTransferConfirmed(false);
      setFinalTxHash(null);
      setSettledRouteId(null);
      setSettledSourceToken(null);
      setSettledSourceChain(null);
      setSelectedChainId(connectedChainId ?? null);
      resetPayment();
    }
    wasOpenRef.current = isOpen;
  }, [isOpen, isConnected, connectedChainId, resetPayment]);

  // Handle token selection and fetch routes
  const handleTokenSelect = useCallback(
    async (token: Token) => {
      setSelectedToken(token);
      setPaymentStep("fetching-routes");
      setError("");

      try {
        // Lock the USD settlement amount server-side; route by exactly it.
        const quote = await getCryptoQuote(billToken, {
          amount_paid: Math.max(totalAmount - tipAmount, 0),
          tip_amount: tipAmount,
          payment_method: "cross_chain_payment",
          ...(splitShareId ? { split_share_id: splitShareId } : {}),
        });
        if (
          !quote.quote_token ||
          quote.usd_microunits <= 0 ||
          quote.chain_id !== 8453 ||
          quote.token !== "USDC" ||
          !isUsableGuestSettlementAddress(quote.settlement_address)
        ) {
          throw new Error(
            t("paymentProcessor.currencyConversionUnavailable") as string,
          );
        }
        setQuoteToken(quote.quote_token);
        const usdAmount = quote.usd_microunits / 1_000_000;
        setQuoteUsdAmount(usdAmount);

        // Pass token decimals and price for accurate amount calculation
        const fetchedRoutes = await fetchRoutes({
          fromChainId: selectedChainId || connectedChainId || 1,
          fromTokenAddress: token.address,
          fromTokenDecimals: token.decimals,
          fromTokenPriceUsd: token.priceUSD
            ? parseFloat(token.priceUSD)
            : undefined,
          toAddress: quote.settlement_address,
          amountUsd: usdAmount,
          isTestnet: false, // LI.FI only supports mainnet
        });

        if (fetchedRoutes.length > 0) {
          setPaymentStep("select-route");
        } else {
          setError(
            (t("paymentProcessor.noRoutesFound") as string) ||
              "Failed to find payment routes",
          );
          setPaymentStep("select-token");
        }
      } catch (err: unknown) {
        console.error("Failed to fetch routes:", err);
        setError(getSafeApiErrorMessage(err, t("paymentProcessor.noRoutesFound") as string));
        setPaymentStep("select-token");
      }
    },
    [
      billToken,
      selectedChainId,
      connectedChainId,
      totalAmount,
      tipAmount,
      fetchRoutes,
      splitShareId,
      t,
    ],
  );

  // Notify the backend of a confirmed Base settlement and mark success. This is
  // the ONLY step retried after the route settles — it is idempotent backend-side
  // (Payment.TxHash is uniquely indexed), so re-sending the same hash cannot
  // double-credit or move money again. `token`/`routeId` are passed on the first
  // call (state set this tick isn't visible yet) and fall back to state on retry.
  //
  // `srcToken`/`srcChain` are the RESOLVED source identity (symbol + chain name)
  // the first notify recorded. They MUST be replayed verbatim on a remount-resume:
  // the backend's idempotency match (PaymentMatchesConfirmedInput) compares
  // SourceToken/SourceChain EXACTLY, so a resume that fell back to "UNKNOWN" or a
  // fallback chain (selectedToken is null and the wallet may not be reconnected on
  // a fresh mount) would 409 against the payment that already settled on-chain —
  // stranding the guest on the error step for a bill that is already paid. The
  // same-mount path leaves these undefined and derives from live state, which is
  // still populated there.
  const finalizeSettlement = useCallback(
    async (
      hash: string,
      token?: string,
      routeId?: string,
      srcToken?: string,
      srcChain?: string,
    ) => {
      const settleToken = token ?? quoteToken;
      if (!settleToken) {
        throw new Error(
          t("paymentProcessor.currencyConversionUnavailable") as string,
        );
      }
      const sourceToken = srcToken ?? selectedToken?.symbol ?? "UNKNOWN";
      const sourceChain =
        srcChain ?? getChainName(selectedChainId || connectedChainId || 1);
      await processCrossChainPayment(billToken, {
        transaction_hash: hash,
        amount_paid: asDollars(Math.max(totalAmount - tipAmount, 0)),
        tip_amount: asDollars(tipAmount),
        source_chain: sourceChain,
        source_token: sourceToken,
        lifi_route_id: routeId ?? settledRouteId ?? undefined,
        quote_token: settleToken,
        ...(splitShareId ? { split_share_id: splitShareId } : {}),
      });

      onPaymentComplete?.({
        totalPaid: totalAmount,
        tipAmount,
        paymentMethod: "cross_chain_payment",
        transactionId: hash,
        sourceChain,
        sourceToken: srcToken ?? selectedToken?.symbol,
      });

      // Backend recorded the settlement — the resume guard has done its job;
      // drop it so a later unrelated open of the same device can't rehydrate it.
      clearCryptoInflight();
      setPaymentStep("success");
    },
    [
      billToken,
      totalAmount,
      tipAmount,
      selectedChainId,
      connectedChainId,
      selectedToken,
      quoteToken,
      settledRouteId,
      onPaymentComplete,
      splitShareId,
      t,
    ],
  );

  // Keep the latest finalizeSettlement in a ref so the mount/restore effect can
  // call it WITHOUT depending on its identity. finalizeSettlement is recreated
  // every render (it closes over `t`, which GuestTranslationProvider rebuilds
  // each render); listing it as an effect dep would re-run the restore effect
  // and risk a second notify mid-flight.
  const finalizeSettlementRef = useRef(finalizeSettlement);
  finalizeSettlementRef.current = finalizeSettlement;
  const tRef = useRef(t);
  tRef.current = t;

  // Resume an in-flight settlement after a refresh/remount. If a previous
  // session confirmed the Base transfer but the notify never landed, the route
  // must NOT be re-executed — re-send the persisted FINAL hash exactly once.
  const resumeAttempted = useRef(false);
  useEffect(() => {
    if (!isOpen || resumeAttempted.current) return;
    const saved = readCryptoInflight(billToken);
    if (!saved || !saved.confirmed || !saved.txHash) return;
    resumeAttempted.current = true;

    setTransferConfirmed(true);
    setFinalTxHash(saved.txHash);
    setQuoteToken(saved.quoteToken);
    setSettledRouteId(saved.lifiRouteId ?? null);
    setSettledSourceToken(saved.sourceToken ?? null);
    setSettledSourceChain(saved.sourceChain ?? null);
    setPaymentStep("executing");

    void (async () => {
      try {
        // Replay the IDENTICAL source identity the first notify recorded so the
        // backend's exact-match idempotency check passes; without these the
        // resume would send source_token="UNKNOWN" + a fallback chain and 409.
        await finalizeSettlementRef.current(
          saved.txHash,
          saved.quoteToken,
          saved.lifiRouteId,
          saved.sourceToken,
          saved.sourceChain,
        );
      } catch (err: unknown) {
        setError(getSafeApiErrorMessage(err, tRef.current("paymentProcessor.paymentFailed") as string));
        setPaymentStep("error");
      }
    })();
    // `t` is intentionally read via tRef (not a dep): GuestTranslationProvider
    // rebuilds `t` every render, and depending on it would re-run this effect on
    // every render and double-notify mid-flight. Mirrors PaymentProcessor.
  }, [isOpen, billToken]);

  // Handle route selection and execute payment
  const handleRouteSelect = useCallback(
    async (route: Route) => {
      setSelectedRoute(route);
      setPaymentStep("executing");
      setError("");

      // Token-select and route-select are separate user-driven steps, so the
      // quote locked in handleTokenSelect is settled into state by now. Guard
      // rather than silently settle with an empty token.
      if (!quoteToken) {
        setError(t("paymentProcessor.currencyConversionUnavailable") as string);
        setPaymentStep("select-token");
        return;
      }

      try {
        await executePayment(
          route,
          async (txHash, executedRoute) => {
            // The Base settlement is final. From here, any failure is a notify
            // failure, retried WITHOUT a second executeRoute.
            setTransferConfirmed(true);
            setFinalTxHash(txHash);
            setSettledRouteId(executedRoute.id);
            // Snapshot the RESOLVED source identity NOW, while selectedToken /
            // selectedChainId are still populated. The backend keys idempotency
            // on these exact strings; a later remount has no live token/chain,
            // so they must be persisted and replayed verbatim or the resume
            // notify 409s the already-settled payment.
            const srcChainId = selectedChainId || connectedChainId || 1;
            const sourceToken = selectedToken?.symbol || "UNKNOWN";
            const sourceChain = getChainName(srcChainId);
            setSettledSourceToken(sourceToken);
            setSettledSourceChain(sourceChain);
            // Persist BEFORE notifying: if the notify (or the whole tab) dies, a
            // remount must still find the FINAL Base hash AND the original source
            // identity to resume from. We store them in the shared CryptoInflight
            // record so the resume reproduces the IDENTICAL ConfirmedPaymentInput.
            writeCryptoInflight({
              billToken,
              txHash,
              quoteToken,
              lifiRouteId: executedRoute.id,
              confirmed: true,
              sourceToken,
              sourceChain,
              sourceChainId: srcChainId,
            });
            // executePayment does not await this callback, so a notify failure
            // here can't bubble to the outer try/catch. Catch it locally so the
            // guest lands on the error step (where retry only re-notifies).
            try {
              await finalizeSettlement(
                txHash,
                quoteToken,
                executedRoute.id,
                sourceToken,
                sourceChain,
              );
            } catch (err: unknown) {
              const message = getSafeApiErrorMessage(err, t("paymentProcessor.paymentFailed") as string);
              setError(
                message || (t("paymentProcessor.paymentFailed") as string),
              );
              setPaymentStep("error");
            }
          },
          (errorMsg) => {
            setError(errorMsg);
            setPaymentStep("error");
          },
        );
      } catch (err: unknown) {
        console.error("Payment execution failed:", err);
        setError(getSafeApiErrorMessage(err, t("paymentProcessor.paymentFailed") as string));
        setPaymentStep("error");
      }
    },
    [
      billToken,
      quoteToken,
      selectedToken,
      selectedChainId,
      connectedChainId,
      executePayment,
      finalizeSettlement,
      t,
    ],
  );

  const handleClose = useCallback(() => {
    // Only block closing during payment execution
    if (paymentStep === "executing") {
      return;
    }
    onClose();
  }, [paymentStep, onClose]);

  // Handle going back to previous step
  const handleBack = useCallback(() => {
    switch (paymentStep) {
      case "select-token":
        // Can't go back from token selection if connected
        break;
      case "fetching-routes":
        // Cancel route fetching and go back to token selection
        setPaymentStep("select-token");
        setSelectedToken(null);
        setError("");
        break;
      case "select-route":
        // Go back to token selection
        setPaymentStep("select-token");
        setSelectedToken(null);
        setError("");
        break;
      case "error":
        // If the route already settled on-chain, returning to select-token
        // would let the guest pay a SECOND time. Keep them on the finalize
        // screen; only the notify may be retried.
        if (transferConfirmed) break;
        setPaymentStep("select-token");
        setSelectedToken(null);
        setSelectedRoute(null);
        setError("");
        resetPayment();
        break;
      default:
        break;
    }
  }, [paymentStep, transferConfirmed, resetPayment]);

  // Check if back button should be shown
  const canGoBack = () => {
    return ["fetching-routes", "select-route", "error"].includes(paymentStep);
  };

  const handleRetry = useCallback(async () => {
    setError("");
    // Already settled on Base — only the notify failed. Re-send the SAME hash;
    // do NOT restart the route (that would be a second settlement).
    if (transferConfirmed && finalTxHash) {
      setPaymentStep("executing");
      try {
        // Forward the snapshotted source identity (set at confirm time) so the
        // retry reproduces the EXACT ConfirmedPaymentInput the backend recorded,
        // even if live token/chain state has since drifted. quoteToken falls back
        // to state inside finalizeSettlement.
        await finalizeSettlement(
          finalTxHash,
          undefined,
          settledRouteId ?? undefined,
          settledSourceToken ?? undefined,
          settledSourceChain ?? undefined,
        );
      } catch (err: unknown) {
        setError(getSafeApiErrorMessage(err, t("paymentProcessor.paymentFailed") as string));
        setPaymentStep("error");
      }
      return;
    }
    setSelectedToken(null);
    setSelectedRoute(null);
    resetPayment();
    setPaymentStep("select-token");
  }, [
    transferConfirmed,
    finalTxHash,
    settledRouteId,
    settledSourceToken,
    settledSourceChain,
    finalizeSettlement,
    resetPayment,
    t,
  ]);

  // Render connect wallet step
  const renderConnect = () => (
    <div className="text-center space-y-6">
      <div className="mx-auto w-16 h-16 bg-brand/10 rounded-full flex items-center justify-center">
        <Wallet className="w-8 h-8 text-brand" />
      </div>
      <div className="space-y-2">
        <h3 className="text-lg font-semibold">{getStepTitle()}</h3>
        <p className="text-gray-600">{getStepDescription()}</p>
      </div>
      <div className="bg-gray-50 rounded-lg p-4">
        <div className="flex justify-between font-semibold">
          <span>
            {(t("crossChain.totalAmount") as string) || "Total Amount"}
          </span>
          <span>{formatCurrency(totalAmount)}</span>
        </div>
      </div>
      {/* Wallet connection options */}
      <div className="space-y-3">
        <p className="text-sm font-medium text-gray-700">
          {(t("crossChain.chooseHowToConnect") as string) ||
            "Choose how to connect:"}
        </p>

        {/* Injected wallet (MetaMask, etc.) */}
        <Button
          className="w-full"
          color="primary"
          variant="bordered"
          size="lg"
          isLoading={isConnecting}
          onPress={connectInjected}
          startContent={<Wallet className="w-5 h-5" />}
        >
          {(t("crossChain.browserWallet") as string) ||
            "Browser Wallet (MetaMask, etc.)"}
        </Button>

        {/* WalletConnect */}
        {hasWalletConnect && (
          <Button
            className="w-full"
            color="primary"
            size="lg"
            isLoading={isConnecting}
            onPress={connectWalletConnect}
            startContent={
              <svg className="w-5 h-5" viewBox="0 0 24 24" fill="currentColor">
                <path d="M4.913 7.519c3.915-3.831 10.26-3.831 14.174 0l.471.461a.483.483 0 0 1 0 .694l-1.611 1.577a.252.252 0 0 1-.354 0l-.649-.635c-2.73-2.673-7.157-2.673-9.887 0l-.695.68a.252.252 0 0 1-.354 0L4.397 8.72a.483.483 0 0 1 0-.694l.516-.507zm17.506 3.263 1.434 1.404a.483.483 0 0 1 0 .694l-6.466 6.331a.505.505 0 0 1-.708 0l-4.588-4.493a.126.126 0 0 0-.177 0l-4.588 4.493a.505.505 0 0 1-.708 0L.152 12.88a.483.483 0 0 1 0-.694l1.434-1.404a.505.505 0 0 1 .708 0l4.588 4.493a.126.126 0 0 0 .177 0l4.588-4.493a.505.505 0 0 1 .708 0l4.588 4.493a.126.126 0 0 0 .177 0l4.588-4.493a.505.505 0 0 1 .708 0z" />
              </svg>
            }
          >
            {(t("crossChain.walletConnect") as string) ||
              "WalletConnect (300+ wallets)"}
          </Button>
        )}
      </div>

      <div className="text-sm text-gray-500 space-y-1">
        <p className="font-medium">
          {(t("crossChain.payWithAnyTokenTagline") as string) ||
            "Pay with any token from any chain"}
        </p>
        <p>
          {(t("crossChain.autoConvertNotice") as string) ||
            "Your payment will be automatically converted to USDC on Base"}
        </p>
      </div>
    </div>
  );

  // Render token selection step
  const renderTokenSelection = () => {
    const popularTokens = tokens.slice(0, 10);

    return (
      <div className="space-y-4">
        <div className="text-center mb-4">
          <div className="mx-auto w-12 h-12 bg-brand/10 rounded-full flex items-center justify-center mb-3">
            <DollarSign className="w-6 h-6 text-brand" />
          </div>
          <p className="text-sm text-gray-600">
            {renderTemplate(
              guestText(
                "crossChain.payAmountWithAnyToken",
                "Pay {amount} with any token",
              ),
              "{amount}",
              <span className="font-semibold">
                {formatCurrency(totalAmount)}
              </span>,
            )}
          </p>
          {address && (
            <p className="text-xs text-gray-400 mt-1">
              {guestText(
                "crossChain.connectedWallet",
                "Connected: {address}",
              ).replace(
                "{address}",
                `${address.slice(0, 6)}...${address.slice(-4)}`,
              )}
            </p>
          )}
        </div>

        {/* Surface guards (e.g. a missing/zeroed quote) that bounce back to
            token selection — otherwise the error state is set but never shown. */}
        {error && (
          <div className="bg-red-50 border border-red-200 rounded-lg p-3">
            <p className="text-sm text-red-700">{error}</p>
          </div>
        )}

        {/* Chain selector */}
        <Select
          label={(t("crossChain.selectChainLabel") as string) || "Select Chain"}
          placeholder={
            (t("crossChain.selectChainPlaceholder") as string) ||
            "Choose a blockchain"
          }
          selectedKeys={selectedChainId ? [selectedChainId.toString()] : []}
          onChange={(e) => {
            const chainId = parseInt(e.target.value);
            setSelectedChainId(chainId);
            setSelectedToken(null);
          }}
          isLoading={chainsLoading}
        >
          {chains.map((chain) => (
            <SelectItem key={chain.id.toString()} value={chain.id.toString()}>
              {chain.name}
            </SelectItem>
          ))}
        </Select>

        {/* Token list */}
        <div className="space-y-2 max-h-64 overflow-y-auto">
          <p className="text-sm font-medium text-gray-700">
            {tr("selectTokenTitle", "Select Token")}
          </p>
          {tokensLoading ? (
            <div className="flex justify-center py-4">
              <Spinner size="sm" />
            </div>
          ) : (
            <div className="grid grid-cols-2 gap-2">
              {popularTokens.map((token) => (
                <Card
                  key={token.address}
                  isPressable
                  className={`cursor-pointer transition-all ${
                    selectedToken?.address === token.address
                      ? "border-2 border-brand"
                      : "border border-gray-200 hover:border-brand/30"
                  }`}
                  onPress={() => handleTokenSelect(token)}
                >
                  <CardBody className="p-3">
                    <div className="flex items-center gap-2">
                      {token.logoURI && (
                        // eslint-disable-next-line @next/next/no-img-element
                        <img
                          src={token.logoURI}
                          alt={token.symbol}
                          className="w-6 h-6 rounded-full"
                        />
                      )}
                      <div>
                        <p className="font-medium text-sm">{token.symbol}</p>
                        <p className="text-xs text-gray-500 truncate max-w-[80px]">
                          {token.name}
                        </p>
                      </div>
                    </div>
                  </CardBody>
                </Card>
              ))}
            </div>
          )}
        </div>

        {/* Disconnect button */}
        {address && (
          <div className="text-center">
            <Button
              variant="light"
              size="sm"
              color="danger"
              startContent={<LogOut className="w-3 h-3" />}
              onPress={disconnect}
            >
              {guestText(
                "paymentProcessor.disconnectWallet",
                "Disconnect Wallet",
              )}
            </Button>
          </div>
        )}
      </div>
    );
  };

  // Render route selection step
  const renderRouteSelection = () => (
    <div className="space-y-4">
      <div className="text-center mb-4">
        <div className="mx-auto w-12 h-12 bg-brand/10 rounded-full flex items-center justify-center mb-3">
          <Zap className="w-6 h-6 text-brand" />
        </div>
        <p className="text-sm text-gray-600">
          {/* Count-after-label phrasing avoids per-locale plural rules. */}
          {guestText(
            "crossChain.routesFound",
            "Routes found for your payment: {n}",
          ).replace("{n}", String(routes.length))}
        </p>
        <p className="text-xs text-gray-500 mt-1">
          {/* Settlement is the locked USD quote, so show that figure next to
              "USDC". Mislabeling the business-currency total (e.g. "AED 425.25
              USDC") overstated what a non-USD business receives. (Audit D-04.)
              Falls back to the business-currency total only if the USD quote
              isn't available (e.g. a resume path). */}
          {renderTemplate(
            guestText(
              "crossChain.businessReceivesAtLeast",
              "Business will receive at least {amount}",
            ),
            "{amount}",
            <span className="font-semibold">
              {quoteUsdAmount != null
                ? `${quoteUsdAmount.toFixed(2)} USDC`
                : formatCurrency(totalAmount)}
            </span>,
          )}
        </p>
      </div>

      <div className="space-y-3 max-h-72 overflow-y-auto">
        {routes.map((paymentRoute, index) => (
          <Card
            key={index}
            isPressable
            className={`cursor-pointer transition-all ${
              selectedRoute === paymentRoute.route
                ? "border-2 border-brand"
                : "border border-gray-200 hover:border-brand/30"
            }`}
            onPress={() => handleRouteSelect(paymentRoute.route)}
          >
            <CardBody className="p-4">
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-2">
                  {paymentRoute.fromToken.logoURI && (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img
                      src={paymentRoute.fromToken.logoURI}
                      alt={paymentRoute.fromToken.symbol}
                      className="w-6 h-6 rounded-full"
                    />
                  )}
                  <span className="font-medium">
                    {formatTokenAmount(
                      paymentRoute.fromAmount,
                      paymentRoute.fromToken.decimals,
                    )}{" "}
                    {paymentRoute.fromToken.symbol}
                  </span>
                  <ArrowRight className="w-4 h-4 text-gray-400" />
                  <span className="font-medium text-green-600">
                    {formatTokenAmount(paymentRoute.toAmount, 6)} USDC
                  </span>
                </div>
                {index === 0 && (
                  <Chip size="sm" color="success" variant="flat">
                    {guestText("crossChain.bestRoute", "Best")}
                  </Chip>
                )}
              </div>

              <div className="flex items-center gap-4 text-xs text-gray-500">
                <div className="flex items-center gap-1">
                  <Clock className="w-3 h-3" />
                  <span>
                    {guestText("crossChain.minutesEstimate", "~{n} min").replace(
                      "{n}",
                      String(Math.ceil(paymentRoute.estimatedTime / 60)),
                    )}
                  </span>
                </div>
                <div className="flex items-center gap-1">
                  <DollarSign className="w-3 h-3" />
                  <span>
                    {/* Gas estimates are always USD figures from LI.FI.
                        Format the amount with the guest locale (no hardcoded $). */}
                    {guestText("crossChain.gasCost", "Gas: {amount}").replace(
                      "{amount}",
                      formatCurrencyIntl(
                        parseFloat(paymentRoute.estimatedGas),
                        "USD",
                        undefined,
                        currentLanguage || "en",
                      ),
                    )}
                  </span>
                </div>
                <div className="flex items-center gap-1">
                  <Zap className="w-3 h-3" />
                  <span>
                    {guestText("crossChain.stepsCount", "Steps: {n}").replace(
                      "{n}",
                      String(paymentRoute.steps),
                    )}
                  </span>
                </div>
              </div>
            </CardBody>
          </Card>
        ))}
      </div>

      <div className="flex gap-2">
        <Button
          variant="bordered"
          startContent={<ArrowLeft className="w-4 h-4" />}
          onPress={handleBack}
        >
          {guestText("common.back", "Back")}
        </Button>
        <Button
          variant="light"
          startContent={<RefreshCw className="w-4 h-4" />}
          onPress={() => selectedToken && handleTokenSelect(selectedToken)}
          isDisabled={routesLoading}
          className="flex-1"
        >
          {guestText("crossChain.refreshRoutes", "Refresh Routes")}
        </Button>
      </div>
    </div>
  );

  // Render executing step
  const renderExecuting = () => (
    <div className="text-center space-y-6">
      <div className="mx-auto w-16 h-16 bg-brand/10 rounded-full flex items-center justify-center">
        <Spinner size="lg" color="primary" />
      </div>
      <div className="space-y-2">
        <h3 className="text-lg font-semibold">{getStepTitle()}</h3>
        <p className="text-gray-600">{getStepDescription()}</p>
      </div>

      {progress.totalSteps > 0 && (
        <div className="space-y-2">
          <Progress
            value={(progress.currentStep / progress.totalSteps) * 100}
            color="primary"
            className="max-w-md mx-auto"
          />
          <p className="text-sm text-gray-500">
            {guestText("crossChain.stepProgress", "Step {current} of {total}")
              .replace("{current}", String(progress.currentStep))
              .replace("{total}", String(progress.totalSteps))}
          </p>
        </div>
      )}

      {progress.txHash && (
        <div className="bg-gray-50 rounded-lg p-3">
          <p className="text-xs text-gray-500 mb-1">
            {guestText("bill.transactionId", "Transaction Hash")}
          </p>
          <p className="text-xs font-mono break-all">{progress.txHash}</p>
        </div>
      )}

      <div className="bg-gray-50 rounded-lg p-4">
        <div className="flex justify-between">
          <span>{guestText("bill.amountToPay", "Amount")}</span>
          <span className="font-semibold">{formatCurrency(totalAmount)}</span>
        </div>
      </div>
    </div>
  );

  // Render success step
  const renderSuccess = () => (
    <div className="text-center space-y-6">
      <div className="mx-auto w-16 h-16 bg-green-100 rounded-full flex items-center justify-center">
        <CheckCircle2 className="w-8 h-8 text-green-600" />
      </div>
      <div className="space-y-2">
        <h3 className="text-lg font-semibold">{getStepTitle()}</h3>
        <p className="text-gray-600">{getStepDescription()}</p>
      </div>

      <div className="bg-gray-50 rounded-lg p-4 space-y-2">
        <div className="flex justify-between">
          <span>{(t("crossChain.totalPaid") as string) || "Total Paid"}</span>
          <span className="font-semibold">{formatCurrency(totalAmount)}</span>
        </div>
        {selectedToken && (
          <div className="flex justify-between text-sm text-gray-500">
            <span>{(t("crossChain.paidWith") as string) || "Paid With"}</span>
            <span>
              {selectedToken.symbol} on{" "}
              {getChainName(selectedChainId || connectedChainId || 1)}
            </span>
          </div>
        )}
        <div className="flex justify-between text-sm text-gray-500">
          <span>{(t("crossChain.settledAs") as string) || "Settled As"}</span>
          <span>{guestText("crossChain.usdcOnBase", "USDC on Base")}</span>
        </div>
      </div>

      {progress.txHash && (
        <div className="bg-gray-50 rounded-lg p-3">
          <p className="text-xs text-gray-500 mb-1">
            {(t("crossChain.txHash") as string) || "Transaction Hash"}
          </p>
          <a
            href={`${getNetwork().blockExplorers?.default.url}/tx/${progress.txHash}`}
            target="_blank"
            rel="noopener noreferrer"
            className="text-xs font-mono text-brand hover:underline break-all"
          >
            {progress.txHash}
          </a>
        </div>
      )}
    </div>
  );

  // Render error step
  const renderError = () => (
    <div className="text-center space-y-6">
      <div className="mx-auto w-16 h-16 bg-red-100 rounded-full flex items-center justify-center">
        <AlertCircle className="w-8 h-8 text-red-600" />
      </div>
      <div className="space-y-2">
        <h3 className="text-lg font-semibold">{getStepTitle()}</h3>
        <p className="text-gray-600">{getStepDescription()}</p>
      </div>
      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-3">
          <p className="text-sm text-red-700">{error}</p>
        </div>
      )}
      <div className="flex gap-3 justify-center">
        {!transferConfirmed && (
          <Button
            variant="bordered"
            onPress={handleBack}
            startContent={<ArrowLeft className="w-4 h-4" />}
          >
            {guestText("crossChain.tryDifferentToken", "Try Different Token")}
          </Button>
        )}
        <Button color="primary" onPress={handleRetry}>
          {guestText("paymentProcessor.tryAgain", "Try Again")}
        </Button>
      </div>
    </div>
  );

  const renderStepContent = () => {
    switch (paymentStep) {
      case "connect":
        return renderConnect();
      case "select-token":
        return renderTokenSelection();
      case "fetching-routes":
        return (
          <div className="text-center space-y-6">
            <div className="mx-auto w-16 h-16 bg-brand/10 rounded-full flex items-center justify-center">
              <Spinner size="lg" color="secondary" />
            </div>
            <div className="space-y-2">
              <h3 className="text-lg font-semibold">{getStepTitle()}</h3>
              <p className="text-gray-600">{getStepDescription()}</p>
            </div>
            {selectedToken && (
              <div className="bg-gray-50 rounded-lg p-3">
                <p className="text-sm text-gray-600">
                  {renderTemplate(
                    guestText(
                      "crossChain.findingRoutesFor",
                      "Finding routes for {token} → USDC",
                    ),
                    "{token}",
                    <span className="font-semibold">
                      {selectedToken.symbol}
                    </span>,
                  )}
                </p>
              </div>
            )}
            <Button
              variant="light"
              color="danger"
              startContent={<X className="w-4 h-4" />}
              onPress={handleBack}
            >
              {guestText("common.cancel", "Cancel")}
            </Button>
          </div>
        );
      case "select-route":
        return renderRouteSelection();
      case "executing":
        return renderExecuting();
      case "success":
        return renderSuccess();
      case "error":
        return renderError();
      default:
        return null;
    }
  };

  const shouldShowFooter = () =>
    paymentStep === "success" || paymentStep === "error";

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      size="lg"
      isDismissable={paymentStep !== "executing"}
      hideCloseButton={true}
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              {canGoBack() && (
                <Button
                  isIconOnly
                  variant="light"
                  size="sm"
                  onPress={handleBack}
                  aria-label={
                    (t("paymentProcessor.goBackAria") as string) || "Go back"
                  }
                >
                  <ArrowLeft className="w-4 h-4" />
                </Button>
              )}
              <h2 className="text-xl font-semibold">
                {(t("crossChain.heading") as string) || "Pay with Any Token"}
              </h2>
            </div>
            <div className="flex items-center gap-2">
              <Chip size="sm" color="primary" variant="flat">
                {(t("crossChain.crossChainChip") as string) || "Cross-Chain"}
              </Chip>
              {paymentStep !== "executing" && (
                <Button
                  isIconOnly
                  variant="light"
                  size="sm"
                  onPress={handleClose}
                  aria-label={
                    (t("paymentProcessor.closeAria") as string) || "Close"
                  }
                >
                  <X className="w-4 h-4" />
                </Button>
              )}
            </div>
          </div>
          <Progress
            value={getProgressValue()}
            className="max-w-md"
            color="primary"
            size="sm"
          />
        </ModalHeader>
        <ModalBody className="pb-6">{renderStepContent()}</ModalBody>
        {shouldShowFooter() && (
          <ModalFooter>
            <Button
              color={paymentStep === "success" ? "primary" : "default"}
              onPress={onClose}
              className="w-full"
              size="lg"
            >
              {guestText("common.close", "Close")}
            </Button>
          </ModalFooter>
        )}
      </ModalContent>
    </Modal>
  );
}

// Main component that wraps with the standalone wallet provider
export default function CrossChainPayment(props: CrossChainPaymentProps) {
  if (!props.isOpen) return null;

  return (
    <CrossChainWalletProvider>
      <CrossChainPaymentInner {...props} />
    </CrossChainWalletProvider>
  );
}
