"use client";

import { getPublicConfig } from "@/config/publicConfig";
import { getSafeApiErrorMessage } from "@/utils/apiError";
import {
  guestPaymentErrorInfo,
  presentGuestPaymentError,
} from "@/lib/guestPaymentErrors";

import React, { useState, useEffect, useCallback, useRef } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Progress,
  Spinner,
  Chip,
} from "@nextui-org/react";
import {
  Wallet,
  AlertCircle,
  CheckCircle2,
  ArrowLeft,
  X,
  LogOut,
} from "lucide-react";
import { Hash, createPublicClient, http } from "viem";
import { getNetwork } from "../../config/network";
import { USDC_ADDRESSES } from "@/constants/usdc";
import {
  CrossChainWalletProvider,
  useCrossChainWallet,
} from "../../lib/lifi/WalletConnectProvider";
import { getCryptoQuote, updateBillPayment } from "../../api/bills";
import {
  readCryptoInflight,
  writeCryptoInflight,
  clearCryptoInflight,
} from "../../lib/cryptoPaymentResume";
import { asDollars } from "@/types/money";
import { isUsableGuestSettlementAddress } from "@/lib/isEvmAddress";
import { formatUsdcMicrounits } from "@/lib/usdcAmount";
import { formatCurrency as formatCurrencyIntl } from "../../api/currency";
import { useGuestTranslation } from "../../i18n/GuestTranslationProvider";

interface PaymentProcessorProps {
  isOpen: boolean;
  onClose: () => void;
  billToken: string;
  totalAmount: number; // Already includes tip
  tipAmount?: number;
  businessName: string;
  businessAddress: string;
  /** Business default currency (ISO 4217); falls back to USD. */
  currency?: string;
  splitShareId?: number;
  onPaymentComplete?: (paymentDetails: {
    totalPaid: number;
    tipAmount: number;
    paymentMethod: string;
    transactionId: string;
  }) => void;
}

type PaymentStep = "connect" | "ready" | "processing" | "success" | "error";

// Inner component that uses the standalone wallet hooks
function PaymentProcessorInner({
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
}: PaymentProcessorProps) {
  const [paymentStep, setPaymentStep] = useState<PaymentStep>("connect");
  const [error, setError] = useState<string>("");
  const [txHash, setTxHash] = useState<Hash | null>(null);
  // True once the USDC transfer is confirmed on-chain. When set, a retry must
  // ONLY re-send the backend notification — never a second writeContract — so a
  // failed backend notify can't make the guest pay twice.
  const [transferConfirmed, setTransferConfirmed] = useState(false);
  // Signed, server-locked USD quote token. Set when handlePayment fetches the
  // quote; forwarded to the backend at settlement so it can verify that the
  // on-chain transfer carries the quote's EXACT amount. Cleared only on modal
  // open (NOT between an attempt and its retry, so a re-notify re-sends the
  // same token).
  const [quoteToken, setQuoteToken] = useState<string | null>(null);
  // The quote's exact micro-USDC (base + a unique sub-cent offset that binds
  // the transfer to this quote). Shown while the wallet prompt is open so the
  // guest sees the same figure their wallet asks them to sign.
  const [quotedMicrounits, setQuotedMicrounits] = useState<number | null>(null);
  const { t, currentLanguage } = useGuestTranslation();

  // Standalone wallet hooks (NOT Dynamic Labs - uses WalletConnect)
  const {
    address,
    isConnected,
    isConnecting,
    chainId,
    walletClient,
    connectInjected,
    connectWalletConnect,
    disconnect,
    switchChain,
    hasWalletConnect,
  } = useCrossChainWallet();

  const formatCurrency = (value: number) =>
    formatCurrencyIntl(value, currency, undefined, currentLanguage || "en");

  const getProgressValue = () => {
    switch (paymentStep) {
      case "connect":
        return 20;
      case "ready":
        return 50;
      case "processing":
        return 75;
      case "success":
        return 100;
      case "error":
        return 0;
      default:
        return 0;
    }
  };

  const tr = (key: string, fallback: string): string => {
    const v = t(`paymentProcessor.step.${key}`) as string | undefined;
    return v && v !== `paymentProcessor.step.${key}` ? v : fallback;
  };

  const guestText = (key: string, fallback: string): string => {
    const value = t(key) as string | undefined;
    return value && value !== key ? value : fallback;
  };

  const getStepTitle = () => {
    switch (paymentStep) {
      case "connect":
        return tr("connectTitle", "Connect Wallet");
      case "ready":
        return tr("readyTitle", "Confirm Payment");
      case "processing":
        return tr("processingTitle", "Processing Payment");
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
        return tr("connectBody", "Connect your wallet to pay with USDC");
      case "ready": {
        const body = tr("readyBody", "");
        return body
          ? body
              .replace("{amount}", formatCurrency(totalAmount))
              .replace("{business}", businessName)
          : `Send ${formatCurrency(totalAmount)} to ${businessName}`;
      }
      case "processing":
        return tr("processingBody", "Processing your payment...");
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
      setPaymentStep("ready");
    }
  }, [isConnected, paymentStep]);

  // Notify the backend of a confirmed on-chain transfer and mark success. This
  // is the only step that should be retried after the transfer confirms — it is
  // idempotent backend-side (Payment.TxHash is uniquely indexed), so re-sending
  // the same hash cannot double-credit or move money again.
  const finalizePayment = useCallback(
    async (hash: Hash, token?: string) => {
      // On the initial attempt the token is passed in directly (state set in the
      // same tick isn't visible to this closure yet). On retry it's omitted and
      // we fall back to the persisted quoteToken — which survives the
      // attempt→retry transition and is only cleared on modal open.
      const settleToken = token ?? quoteToken;
      if (!settleToken) {
        throw new Error(t("paymentProcessor.missingPaymentQuote") as string);
      }
      await updateBillPayment(billToken, {
        transaction_hash: hash,
        amount_paid: asDollars(Math.max(totalAmount - tipAmount, 0)),
        tip_amount: asDollars(tipAmount),
        payment_method: "crypto",
        blockchain_network: getNetwork().name.toLowerCase(),
        quote_token: settleToken,
        ...(splitShareId ? { split_share_id: splitShareId } : {}),
      });

      onPaymentComplete?.({
        totalPaid: totalAmount,
        tipAmount,
        paymentMethod: "usdc_payment",
        transactionId: hash,
      });

      // Backend recorded the payment — the guard has done its job; drop it so a
      // later unrelated bill on the same device can't accidentally rehydrate it.
      clearCryptoInflight();
      setPaymentStep("success");
    },
    [
      billToken,
      totalAmount,
      tipAmount,
      onPaymentComplete,
      quoteToken,
      splitShareId,
      t,
    ],
  );

  // Resume an unconfirmed transfer after a reload: poll the SAME hash to
  // confirmation, then finalize. NEVER calls writeContract — the on-chain
  // transfer already happened pre-reload; re-sending it would double-spend.
  const resumeConfirmation = useCallback(
    async (hash: Hash, token: string) => {
      setPaymentStep("processing");
      setError("");
      try {
        const targetNetwork = getNetwork();
        const publicClient = createPublicClient({
          chain: targetNetwork,
          transport: http(getPublicConfig().rpcUrl),
        });
        const receipt = await publicClient.waitForTransactionReceipt({
          hash,
          confirmations: 3,
        });
        if (receipt.status === "reverted") {
          throw new Error(
            (t("paymentProcessor.transactionReverted") as string) ||
              "Transaction was reverted",
          );
        }
        setTransferConfirmed(true);
        writeCryptoInflight({
          billToken,
          txHash: hash,
          quoteToken: token,
          confirmed: true,
        });
        await finalizePayment(hash, token);
      } catch (err: unknown) {
        setError(
          getSafeApiErrorMessage(
            err,
            t("paymentProcessor.paymentFailed") as string,
          ),
        );
        setPaymentStep("error");
      }
    },
    [billToken, finalizePayment, t],
  );

  // Keep the latest resumeConfirmation in a ref so the reset-on-open effect can
  // call it WITHOUT depending on its identity. resumeConfirmation is recreated
  // every render (it closes over `t`, which GuestTranslationProvider rebuilds
  // each render); listing it as an effect dep would re-run the reset-on-open
  // effect on every render and clobber legitimate state (e.g. an error set by
  // handlePayment, or the rehydrated guard itself).
  const resumeConfirmationRef = useRef(resumeConfirmation);
  resumeConfirmationRef.current = resumeConfirmation;

  // Reset OR rehydrate state when the modal opens. A page refresh mid-confirm
  // unmounts this component and wipes React state; without rehydration the
  // guest would drop to "ready" and could fire a SECOND writeContract. If a
  // bill-scoped in-flight record survives, restore the guard instead of
  // clearing it.
  useEffect(() => {
    if (!isOpen) return;

    const inflight = readCryptoInflight(billToken);
    if (inflight) {
      setError("");
      setQuoteToken(inflight.quoteToken);
      setTxHash(inflight.txHash as Hash);
      setTransferConfirmed(inflight.confirmed);
      if (inflight.confirmed) {
        // Land on the finalize/retry screen; handleRetry re-notifies the same
        // hash. (renderError keys its confirmed UI off transferConfirmed.)
        setPaymentStep("error");
      } else {
        // Re-enter confirmation for the persisted hash — no wallet call.
        void resumeConfirmationRef.current(
          inflight.txHash as Hash,
          inflight.quoteToken,
        );
      }
      return;
    }

    setPaymentStep(isConnected ? "ready" : "connect");
    setError("");
    setTxHash(null);
    setTransferConfirmed(false);
    setQuoteToken(null);
    setQuotedMicrounits(null);
  }, [isOpen, isConnected, billToken]);

  // Handle direct USDC transfer to business
  const handlePayment = useCallback(async () => {
    if (!walletClient || !address) {
      setError(t("paymentProcessor.walletNotConnected"));
      setPaymentStep("error");
      return;
    }

    setPaymentStep("processing");
    setError("");
    setTransferConfirmed(false);
    setQuotedMicrounits(null);

    try {
      // Get the target network from config
      const targetNetwork = getNetwork();
      const targetNetworkId = targetNetwork.id;

      // Switch to the correct network if needed and verify the switch
      if (chainId !== targetNetworkId) {
        await switchChain(targetNetworkId);
        const actualChainId = await walletClient.getChainId();
        if (actualChainId !== targetNetworkId) {
          throw new Error(
            `Chain switch failed: wallet is on chain ${actualChainId}, expected ${targetNetworkId}`,
          );
        }
      }

      // Lock the USD settlement amount server-side (immune to FX drift between
      // quote and transfer). The backend returns the exact USDC micro-units the
      // on-chain transfer must move plus a signed token verifying that amount.
      const quote = await getCryptoQuote(billToken, {
        amount_paid: Math.max(totalAmount - tipAmount, 0),
        tip_amount: tipAmount,
        payment_method: "usdc_payment",
        ...(splitShareId ? { split_share_id: splitShareId } : {}),
      });
      if (
        !quote.quote_token ||
        !Number.isSafeInteger(quote.usd_microunits) ||
        quote.usd_microunits <= 0 ||
        quote.chain_id !== targetNetworkId ||
        quote.token !== "USDC" ||
        !isUsableGuestSettlementAddress(quote.settlement_address)
      ) {
        throw new Error(
          t("paymentProcessor.currencyConversionUnavailable") as string,
        );
      }
      setQuoteToken(quote.quote_token);
      setQuotedMicrounits(quote.usd_microunits);

      // Get USDC contract address based on configured network
      const USDC_ADDRESS = USDC_ADDRESSES[targetNetworkId];
      if (!USDC_ADDRESS) {
        throw new Error(
          t("paymentProcessor.currencyConversionUnavailable") as string,
        );
      }
      // Transfer EXACTLY the locked micro-units (no parseUnits/toFixed
      // rounding). The backend settles only a transfer of exactly this amount:
      // its sub-cent offset is what ties the transfer to this guest's quote,
      // so any other value (even a larger one) is never applied to the bill.
      const amountInWei = BigInt(quote.usd_microunits);

      // Send USDC directly to business address
      // Use targetNetwork directly instead of walletClient.chain (which may be stale after switchChain)
      if (!walletClient.account) {
        throw new Error(
          (t("paymentProcessor.walletAccountUnavailable") as string) ||
            "Wallet account unavailable. Please reconnect your wallet.",
        );
      }
      const hash = await walletClient.writeContract({
        address: USDC_ADDRESS as `0x${string}`,
        abi: [
          {
            name: "transfer",
            type: "function",
            inputs: [
              { name: "to", type: "address" },
              { name: "amount", type: "uint256" },
            ],
            outputs: [{ name: "", type: "bool" }],
          },
        ] as const,
        functionName: "transfer",
        args: [quote.settlement_address as `0x${string}`, amountInWei],
        chain: targetNetwork,
        account: walletClient.account,
      });

      setTxHash(hash);
      // Persist the guard the instant we have a hash: a refresh between here
      // and on-chain confirmation must NOT drop the guest to "ready" (which
      // would fire a second writeContract / double-spend). confirmed:false ->
      // on restore we re-enter waitForTransactionReceipt for THIS hash.
      writeCryptoInflight({
        billToken,
        txHash: hash,
        quoteToken: quote.quote_token,
        confirmed: false,
      });

      // Wait for transaction confirmation
      const publicClient = createPublicClient({
        chain: targetNetwork,
        transport: http(getPublicConfig().rpcUrl),
      });

      const receipt = await publicClient.waitForTransactionReceipt({
        hash,
        confirmations: 3,
      });

      if (receipt.status === "reverted") {
        throw new Error(
          (t("paymentProcessor.transactionReverted") as string) ||
            "Transaction was reverted",
        );
      }

      // The on-chain transfer is final. From here, any failure is a
      // backend-notification failure, which must be retried WITHOUT a second
      // transfer (see handleRetry).
      setTransferConfirmed(true);
      // Upgrade the persisted guard: a refresh from here lands the guest on the
      // finalize/retry screen (NOT "ready"), where retry only re-notifies the
      // same hash.
      writeCryptoInflight({
        billToken,
        txHash: hash,
        quoteToken: quote.quote_token,
        confirmed: true,
      });

      await finalizePayment(hash, quote.quote_token);
    } catch (err: unknown) {
      console.error("Payment failed:", err);
      const paymentError = guestPaymentErrorInfo(err);
      setError(
        paymentError.code
          ? (t(presentGuestPaymentError(paymentError).messageKey) as string)
          : getSafeApiErrorMessage(
              err,
              t("paymentProcessor.paymentFailed") as string,
            ),
      );
      setPaymentStep("error");
    }
  }, [
    walletClient,
    address,
    billToken,
    totalAmount,
    tipAmount,
    chainId,
    switchChain,
    finalizePayment,
    splitShareId,
    t,
  ]);

  // Check if can go back
  const canGoBack = () => {
    return paymentStep === "ready" || paymentStep === "error";
  };

  // Handle going back
  const handleBack = useCallback(() => {
    if (paymentStep === "ready") {
      disconnect();
      setPaymentStep("connect");
    } else if (paymentStep === "error") {
      // If the transfer already confirmed on-chain, returning to "ready" would
      // let the guest pay a second time. Keep them on the finalize screen.
      if (transferConfirmed) return;
      setError("");
      setPaymentStep("ready");
    }
  }, [paymentStep, disconnect, transferConfirmed]);

  // Handle close
  const handleClose = useCallback(() => {
    if (paymentStep === "processing") {
      return; // Can't close during processing
    }
    onClose();
  }, [paymentStep, onClose]);

  // Handle retry
  const handleRetry = useCallback(async () => {
    setError("");
    // The transfer already settled on-chain — only the backend notification
    // failed. Re-send the SAME hash; do NOT restart the wallet flow (that would
    // be a second on-chain payment / double-spend).
    if (transferConfirmed && txHash) {
      setPaymentStep("processing");
      try {
        await finalizePayment(txHash);
      } catch (err: unknown) {
        setError(
          getSafeApiErrorMessage(
            err,
            t("paymentProcessor.paymentFailed") as string,
          ),
        );
        setPaymentStep("error");
      }
      return;
    }
    setPaymentStep("ready");
  }, [transferConfirmed, txHash, finalizePayment, t]);

  // Render connect step
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
            {(t("paymentProcessor.totalAmount") as string) || "Total Amount"}
          </span>
          <span>{formatCurrency(totalAmount)}</span>
        </div>
      </div>

      {/* Wallet connection options */}
      <div className="space-y-3">
        <p className="text-sm font-medium text-gray-700">
          {(t("paymentProcessor.chooseHowToConnect") as string) ||
            "Choose how to connect:"}
        </p>

        <Button
          className="w-full"
          color="primary"
          variant="bordered"
          size="lg"
          isLoading={isConnecting}
          onPress={connectInjected}
          startContent={<Wallet className="w-5 h-5" />}
        >
          {guestText(
            "paymentProcessor.browserWallet",
            "Browser Wallet (MetaMask, etc.)",
          )}
        </Button>

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
            {guestText(
              "paymentProcessor.walletConnect",
              "WalletConnect (300+ wallets)",
            )}
          </Button>
        )}

        {/* IMP-08: exit hatch. Don't trap guests who tapped USDC out of
            curiosity but don't have a wallet. Single tap returns to the
            payment-method picker with the bill state preserved. */}
        <Button
          className="w-full mt-2"
          variant="light"
          color="default"
          size="lg"
          onPress={onClose}
          aria-label={
            (t("paymentProcessor.exitToPaymentMethods") as string) ||
            "Use a different payment method"
          }
        >
          ←{" "}
          {(t("paymentProcessor.exitToPaymentMethods") as string) ||
            "Use a different payment method"}
        </Button>

        <p className="text-xs text-gray-500 text-center mt-1">
          {(t("paymentProcessor.noWalletPrimer") as string) ||
            "Don't have a wallet? Pay by card or cash instead — no account needed."}
        </p>
      </div>
    </div>
  );

  // Render ready step
  const renderReady = () => (
    <div className="text-center space-y-6">
      <div className="mx-auto w-16 h-16 bg-green-100 rounded-full flex items-center justify-center">
        <CheckCircle2 className="w-8 h-8 text-green-600" />
      </div>
      <div className="space-y-2">
        <h3 className="text-lg font-semibold">{getStepTitle()}</h3>
        <p className="text-gray-600">{getStepDescription()}</p>
      </div>

      {address && (
        <div className="bg-gray-50 rounded-lg p-3">
          <p className="text-xs text-gray-500 mb-1">
            {guestText("paymentProcessor.connectedWallet", "Connected Wallet")}
          </p>
          <p className="text-sm font-mono">
            {address.slice(0, 6)}...{address.slice(-4)}
          </p>
        </div>
      )}

      <div className="bg-gray-50 rounded-lg p-4 space-y-2">
        <div className="flex justify-between">
          <span>{guestText("bill.amountToPay", "Amount")}</span>
          <span className="font-semibold">{formatCurrency(totalAmount)}</span>
        </div>
        <div className="flex justify-between text-sm text-gray-500">
          <span>{guestText("paymentProcessor.to", "To")}</span>
          <span>{businessName}</span>
        </div>
        <div className="flex justify-between text-sm text-gray-500">
          <span>{guestText("paymentProcessor.network", "Network")}</span>
          <span>{getNetwork().name}</span>
        </div>
      </div>

      <Button
        color="primary"
        size="lg"
        className="w-full"
        onPress={handlePayment}
      >
        {/* The figure is the business-currency total — the exact USDC the
            wallet transfers comes from the server quote locked on click, so
            labeling this amount "USDC" was wrong (audit CRIT-3). The USDC
            context lives in the method the guest chose + the Network row. */}
        {guestText("paymentProcessor.payUsdc", "Pay {amount}").replace(
          "{amount}",
          formatCurrency(totalAmount),
        )}
      </Button>

      <Button
        variant="light"
        size="sm"
        color="danger"
        startContent={<LogOut className="w-3 h-3" />}
        onPress={() => {
          disconnect();
          setPaymentStep("connect");
        }}
      >
        {guestText("paymentProcessor.disconnectWallet", "Disconnect Wallet")}
      </Button>
    </div>
  );

  // Render processing step
  const renderProcessing = () => (
    <div className="text-center space-y-6">
      <div className="mx-auto w-16 h-16 bg-brand/10 rounded-full flex items-center justify-center">
        <Spinner size="lg" color="primary" />
      </div>
      <div className="space-y-2">
        <h3 className="text-lg font-semibold">{getStepTitle()}</h3>
        <p className="text-gray-600">{getStepDescription()}</p>
      </div>
      <div className="bg-gray-50 rounded-lg p-4 space-y-2">
        <div className="flex justify-between font-semibold">
          <span>{guestText("bill.amountToPay", "Amount")}</span>
          <span>{formatCurrency(totalAmount)}</span>
        </div>
        {quotedMicrounits !== null && (
          <>
            <div className="flex justify-between gap-4 text-sm">
              <span className="text-gray-600">
                {guestText(
                  "paymentProcessor.exactUsdcAmount",
                  "Exact USDC amount",
                )}
              </span>
              <span
                className="font-mono font-semibold tabular-nums"
                data-testid="exact-usdc-amount"
              >
                {`${formatUsdcMicrounits(quotedMicrounits)} USDC`}
              </span>
            </div>
            <p className="text-xs text-gray-500 text-left">
              {guestText(
                "paymentProcessor.exactUsdcHint",
                "The last digits of this amount link your transfer to this bill.",
              )}
            </p>
          </>
        )}
      </div>
      <p className="text-sm text-gray-500">
        {guestText(
          "paymentProcessor.confirmInWallet",
          "Please confirm the transaction in your wallet",
        )}
      </p>
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
        <div className="flex justify-between font-semibold">
          <span>
            {(t("paymentProcessor.totalPaid") as string) || "Total Paid"}
          </span>
          <span>{formatCurrency(totalAmount)}</span>
        </div>
        <div className="flex justify-between text-sm text-gray-500">
          <span>{(t("paymentProcessor.to") as string) || "To"}</span>
          <span>{businessName}</span>
        </div>
      </div>
      {txHash && (
        <div className="bg-gray-50 rounded-lg p-3">
          <p className="text-xs text-gray-500 mb-1">
            {(t("paymentProcessor.txHash") as string) || "Transaction Hash"}
          </p>
          <a
            href={`${getNetwork().blockExplorers?.default.url}/tx/${txHash}`}
            target="_blank"
            rel="noopener noreferrer"
            className="text-xs font-mono text-brand hover:underline break-all"
          >
            {txHash}
          </a>
        </div>
      )}
    </div>
  );

  // Render error step
  const renderError = () => (
    <div className="text-center space-y-6">
      <div
        className={`mx-auto w-16 h-16 rounded-full flex items-center justify-center ${
          transferConfirmed ? "bg-amber-100" : "bg-red-100"
        }`}
      >
        <AlertCircle
          className={`w-8 h-8 ${transferConfirmed ? "text-amber-600" : "text-red-600"}`}
        />
      </div>
      <div className="space-y-2">
        <h3 className="text-lg font-semibold">
          {transferConfirmed
            ? guestText(
                "paymentProcessor.transferConfirmedTitle",
                "Payment received — finalizing",
              )
            : getStepTitle()}
        </h3>
        <p className="text-gray-600">
          {transferConfirmed
            ? guestText(
                "paymentProcessor.transferConfirmedBody",
                "Your USDC transfer is confirmed on-chain. We couldn't reach our server to record it. Tap Retry to finish — this will NOT charge you again.",
              )
            : getStepDescription()}
        </p>
      </div>
      {/* When the transfer is already confirmed, always surface the tx hash so
          the guest has on-chain proof of payment regardless of backend state. */}
      {txHash && (
        <div className="bg-gray-50 rounded-lg p-3">
          <p className="text-xs text-gray-500 mb-1">
            {(t("paymentProcessor.txHash") as string) || "Transaction Hash"}
          </p>
          <a
            href={`${getNetwork().blockExplorers?.default.url}/tx/${txHash}`}
            target="_blank"
            rel="noopener noreferrer"
            className="text-xs font-mono text-brand hover:underline break-all"
          >
            {txHash}
          </a>
        </div>
      )}
      {error && !transferConfirmed && (
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
            {guestText("paymentProcessor.goBack", "Go Back")}
          </Button>
        )}
        <Button color="primary" onPress={handleRetry}>
          {transferConfirmed
            ? guestText("paymentProcessor.retryFinalize", "Retry")
            : guestText("paymentProcessor.tryAgain", "Try Again")}
        </Button>
      </div>

      {/* IMP-08: exit hatch on the error step too. A guest stuck on
          "wrong network" or "insufficient USDC" needs an out — not just
          a Try Again button that'll lead to the same error.
          Hide once the wallet transfer confirmed so guests are not invited
          to abandon a finalized on-chain payment (double-submit risk). */}
      {!transferConfirmed && (
        <Button
          className="w-full mt-2"
          variant="light"
          color="default"
          onPress={onClose}
        >
          ←{" "}
          {(t("paymentProcessor.exitToPaymentMethods") as string) ||
            "Use a different payment method"}
        </Button>
      )}
    </div>
  );

  const renderStepContent = () => {
    switch (paymentStep) {
      case "connect":
        return renderConnect();
      case "ready":
        return renderReady();
      case "processing":
        return renderProcessing();
      case "success":
        return renderSuccess();
      case "error":
        return renderError();
      default:
        return null;
    }
  };

  const shouldShowFooter = () => paymentStep === "success";

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      size="lg"
      isDismissable={paymentStep !== "processing"}
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
                {(t("paymentProcessor.payWithUSDC") as string) ||
                  "Pay with USDC"}
              </h2>
            </div>
            <div className="flex items-center gap-2">
              <Chip size="sm" color="success" variant="flat">
                {getNetwork().name}
              </Chip>
              {paymentStep !== "processing" && (
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
              color="primary"
              onPress={onClose}
              className="w-full"
              size="lg"
            >
              {guestText("paymentProcessor.close", "Close")}
            </Button>
          </ModalFooter>
        )}
      </ModalContent>
    </Modal>
  );
}

// Main component that wraps with the standalone wallet provider
export default function PaymentProcessor(props: PaymentProcessorProps) {
  if (!props.isOpen) return null;

  return (
    <CrossChainWalletProvider>
      <PaymentProcessorInner {...props} />
    </CrossChainWalletProvider>
  );
}
