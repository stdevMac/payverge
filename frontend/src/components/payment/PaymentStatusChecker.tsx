"use client";

import React, { useState, useEffect, useCallback, useRef } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  Button,
  Spinner,
  Card,
  CardBody,
} from "@nextui-org/react";
import {
  CheckCircle2,
  XCircle,
  Clock,
  AlertCircle,
  RefreshCw,
} from "lucide-react";
import { useGuestTranslation } from "../../i18n/GuestTranslationProvider";
import { paymentPluginAPI } from "../../api/plugins";
import { paymentMethodLabel } from "@/lib/paymentMethodLabels";

const MAX_POLL_DURATION_MS = 5 * 60 * 1000; // 5 minutes
// After the foreground window elapses we keep a slower "backstop" poll running:
// slow crypto confirmations can land minutes later, and abandoning the poll
// would strand the guest on a scary timed-out screen for a payment that still
// succeeds. 15s keeps the late-confirmation path alive without hammering the API.
const BACKSTOP_POLL_MS = 15 * 1000;

interface PaymentStatusCheckerProps {
  isOpen: boolean;
  onClose: () => void;
  billToken: string;
  paymentId: string;
  paymentMethod: string;
  fallbackTotalPaid?: number;
  fallbackTipAmount?: number;
  onPaymentConfirmed: (paymentDetails: {
    totalPaid: number;
    tipAmount: number;
    paymentMethod: string;
    transactionId: string;
    splitShareId?: number;
  }) => void;
  onTimeout?: () => void;
}

type PaymentStatus =
  | "checking"
  | "completed"
  | "failed"
  | "pending"
  | "cancelled"
  | "timed_out";

export default function PaymentStatusChecker({
  isOpen,
  onClose,
  billToken,
  paymentId,
  paymentMethod,
  fallbackTotalPaid = 0,
  fallbackTipAmount = 0,
  onPaymentConfirmed,
  onTimeout,
}: PaymentStatusCheckerProps) {
  const { t } = useGuestTranslation();
  const [status, setStatus] = useState<PaymentStatus>("checking");
  const [error, setError] = useState<string>("");
  const [checkCount, setCheckCount] = useState(0);
  const [isManualRefresh, setIsManualRefresh] = useState(false);
  const startTimeRef = useRef<number>(Date.now());
  const confirmedRef = useRef(false);

  // Localized lookup with an English fallback. Defined before checkPaymentStatus
  // so error fallbacks set there can be localized too. Keyed on t (which already
  // drives checkPaymentStatus), so it introduces no new render churn.
  const tr = useCallback(
    (key: string, fallback: string): string => {
      const v = t(`paymentStatus.${key}`) as string | undefined;
      return v && v !== `paymentStatus.${key}` ? v : fallback;
    },
    [t],
  );

  // Check payment status
  const checkPaymentStatus = useCallback(
    async (isManual = false) => {
      if (isManual) {
        setIsManualRefresh(true);
      }

      try {
        const response = await paymentPluginAPI.getPluginPaymentStatus(
          billToken,
          paymentId,
          paymentMethod,
        );

        switch (response.status.toLowerCase()) {
          case "completed":
          case "success":
          case "paid":
            if (!confirmedRef.current) {
              // Resolve the paid amount in priority: explicit response →
              // pre-redirect fallback. If BOTH are missing or zero-valued,
              // treat this as a hard failure — rendering "Paid $0" for what
              // might be a $45 bill is worse than asking the user to contact
              // staff for reconciliation.
              // Prefer total_amount (the fiat figure) over amount — for
              // stablecoin plugins `amount` is the on-chain token decimal
              // (e.g. USDT to transfer), not the money the bill records.
              const metadataAmount = Number(
                response.metadata?.total_amount ?? response.metadata?.amount,
              );
              const rawTotalPaid = Number.isFinite(metadataAmount) && metadataAmount > 0
                ? metadataAmount
                : fallbackTotalPaid;

              const metadataTip = Number(
                response.metadata?.tip_amount ?? response.metadata?.tip,
              );
              const rawTipAmount = Number.isFinite(metadataTip)
                ? metadataTip
                : fallbackTipAmount;
              const metadataSplitShareId = Number(
                response.metadata?.split_share_id ?? response.metadata?.splitShareId,
              );
              const splitShareId =
                Number.isFinite(metadataSplitShareId) && metadataSplitShareId > 0
                  ? metadataSplitShareId
                  : undefined;

              if (!Number.isFinite(rawTotalPaid) || rawTotalPaid <= 0) {
                setStatus("failed");
                setError(
                  tr(
                    "failedNoAmount",
                    "Payment reported as complete but no amount was returned. Please contact staff to confirm the transaction.",
                  ),
                );
                break;
              }

              confirmedRef.current = true;
              setStatus("completed");
              onPaymentConfirmed({
                totalPaid: rawTotalPaid,
                tipAmount: rawTipAmount,
                paymentMethod,
                transactionId: paymentId,
                ...(splitShareId ? { splitShareId } : {}),
              });
            } else {
              setStatus("completed");
            }
            break;

          case "failed":
          case "error":
          case "declined":
            setStatus("failed");
            setError(
              response.metadata?.error_message ||
                tr("failedGeneric", "Payment failed"),
            );
            break;

          case "cancelled":
          case "canceled":
            setStatus("cancelled");
            break;

          case "pending":
          case "processing":
          default:
            // Never downgrade a timed-out session back to "pending": the
            // backstop poll keeps running under the reassuring timed-out
            // screen, and only a terminal "completed"/"failed" should move it.
            setStatus((prev) => (prev === "timed_out" ? "timed_out" : "pending"));
            break;
        }
      } catch (error) {
        console.error("Error checking payment status:", error);
        if (checkCount > 5) {
          setStatus("failed");
          setError(t("paymentStatus.verifyError"));
        }
      } finally {
        if (isManual) {
          setIsManualRefresh(false);
        }
      }
    },
    [
      billToken,
      paymentId,
      paymentMethod,
      checkCount,
      fallbackTotalPaid,
      fallbackTipAmount,
      onPaymentConfirmed,
      t,
      tr,
    ],
  );

  // Reset the start time whenever a new polling session begins
  useEffect(() => {
    if (isOpen && paymentId) {
      startTimeRef.current = Date.now();
      confirmedRef.current = false;
    }
  }, [isOpen, paymentId]);

  // Auto-check payment status. Two cadences share one effect:
  //  - foreground (checking/pending): every 3s until the 5-minute window ends,
  //    at which point we flip to "timed_out";
  //  - backstop (timed_out): every 15s so a late confirmation still resolves.
  // Terminal states (completed/failed/cancelled) stop polling entirely.
  useEffect(() => {
    if (!isOpen || !paymentId) return;

    const foreground = status === "checking" || status === "pending";
    const backstop = status === "timed_out";
    if (!foreground && !backstop) return;

    const cadence = backstop ? BACKSTOP_POLL_MS : 3000;
    const interval = setInterval(() => {
      if (foreground && Date.now() - startTimeRef.current >= MAX_POLL_DURATION_MS) {
        clearInterval(interval);
        setStatus("timed_out");
        onTimeout?.();
        return;
      }
      setCheckCount((prev) => prev + 1);
      void checkPaymentStatus();
    }, cadence);

    // Initial check
    void checkPaymentStatus();

    return () => clearInterval(interval);
  }, [isOpen, paymentId, status, checkPaymentStatus, onTimeout]);

  const getStatusIcon = () => {
    switch (status) {
      case "completed":
        return <CheckCircle2 className="w-16 h-16 text-green-500" />;
      case "failed":
        return <XCircle className="w-16 h-16 text-red-500" />;
      case "cancelled":
        return <XCircle className="w-16 h-16 text-orange-500" />;
      case "pending":
        return <Clock className="w-16 h-16 text-brand" />;
      case "timed_out":
        return <AlertCircle className="w-16 h-16 text-yellow-500" />;
      default:
        return <Spinner size="lg" className="w-16 h-16" />;
    }
  };

  const getStatusTitle = () => {
    switch (status) {
      case "completed":
        return t("payment.paymentSuccess");
      case "failed":
        return t("payment.paymentError");
      case "cancelled":
        return tr("cancelledTitle", "Payment Cancelled");
      case "pending":
        return tr("pendingTitle", "Payment Processing");
      case "timed_out":
        return tr("timedOutTitle", "Verification Timed Out");
      default:
        return tr("checkingTitle", "Checking Payment Status");
    }
  };

  const getStatusMessage = () => {
    const methodLabel = paymentMethodLabel(paymentMethod, t);
    const interp = (key: string, fallback: string) =>
      tr(key, fallback).replace("{method}", methodLabel);
    switch (status) {
      case "completed":
        return interp(
          "completedBody",
          `Your ${methodLabel} payment has been confirmed successfully.`,
        );
      case "failed":
        return (
          error ||
          interp(
            "failedBody",
            `Your ${methodLabel} payment could not be processed.`,
          )
        );
      case "cancelled":
        return interp(
          "cancelledBody",
          `Your ${methodLabel} payment was cancelled.`,
        );
      case "pending":
        return interp(
          "pendingBody",
          `Your ${methodLabel} payment is being processed. This may take a few minutes.`,
        );
      case "timed_out":
        return interp(
          "timedOutBody",
          `Your ${methodLabel} payment is taking longer than usual to confirm. We're still checking in the background and your bill will update automatically once it clears. You can safely close this window — if it doesn't confirm shortly, please check with staff.`,
        );
      default:
        return interp(
          "checkingBody",
          `Verifying your ${methodLabel} payment status...`,
        );
    }
  };

  const getStatusColor = () => {
    switch (status) {
      case "completed":
        return "bg-green-50 border-green-200";
      case "failed":
        return "bg-red-50 border-red-200";
      case "cancelled":
        return "bg-orange-50 border-orange-200";
      case "pending":
        return "bg-brand/10 border-brand/20";
      case "timed_out":
        return "bg-yellow-50 border-yellow-200";
      default:
        return "bg-warm-50 border-warm-200";
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={status === "completed" ? onClose : undefined}
      isDismissable={status !== "checking"}
      hideCloseButton={status === "checking"}
      size="lg"
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <h2 className="text-xl font-medium">
            {(t("paymentStatus.title") as string) || "Payment Status"}
          </h2>
        </ModalHeader>

        <ModalBody className="pb-6">
          <Card className={`border-2 ${getStatusColor()}`}>
            <CardBody className="p-8 text-center">
              <div className="flex justify-center mb-4">{getStatusIcon()}</div>

              <h3 className="text-xl font-semibold mb-2">{getStatusTitle()}</h3>

              <p className="text-ink-600 mb-6">{getStatusMessage()}</p>

              {/* Payment Details */}
              <div className="bg-warm-50 border border-warm-200 rounded-lg p-4 mb-4">
                <div className="flex justify-between items-center text-sm">
                  <span className="text-ink-600">
                    {(t("paymentStatus.paymentMethod") as string) || "Payment Method"}:
                  </span>
                  <span className="font-medium text-ink-900">
                    {paymentMethodLabel(paymentMethod, t) || paymentMethod}
                  </span>
                </div>
                <div className="flex justify-between items-center text-sm mt-2">
                  <span className="text-ink-600">
                    {(t("paymentStatus.paymentId") as string) || "Payment ID"}:
                  </span>
                  <span className="font-mono text-xs text-ink-900">{paymentId}</span>
                </div>
              </div>

              {/* Action Buttons */}
              <div className="flex gap-3 justify-center">
                {status === "completed" && (
                  <Button color="success" onPress={onClose} size="lg">
                    {(t("paymentStatus.continue") as string) || "Continue"}
                  </Button>
                )}

                {status === "failed" && (
                  <>
                    <Button variant="light" onPress={onClose}>
                      {(t("paymentStatus.close") as string) || "Close"}
                    </Button>
                    <Button
                      color="primary"
                      onPress={() => window.location.reload()}
                    >
                      {(t("paymentStatus.tryAgain") as string) || "Try Again"}
                    </Button>
                  </>
                )}

                {status === "cancelled" && (
                  <Button color="warning" onPress={onClose}>
                    {(t("paymentStatus.close") as string) || "Close"}
                  </Button>
                )}

                {status === "timed_out" && (
                  <Button color="warning" onPress={onClose}>
                    {(t("paymentStatus.close") as string) || "Close"}
                  </Button>
                )}

                {(status === "pending" ||
                  status === "checking" ||
                  status === "timed_out") && (
                  <Button
                    variant="light"
                    onPress={() => checkPaymentStatus(true)}
                    isLoading={isManualRefresh}
                    startContent={
                      !isManualRefresh ? (
                        <RefreshCw className="w-4 h-4" />
                      ) : undefined
                    }
                  >
                    {(t("paymentStatus.refreshStatus") as string) || "Refresh Status"}
                  </Button>
                )}
              </div>

              {/* Help Text */}
              {status === "pending" && checkCount > 10 && (
                <div className="mt-4 p-3 bg-yellow-50 border border-yellow-200 rounded-lg">
                  <div className="flex items-start gap-2">
                    <AlertCircle className="w-5 h-5 text-yellow-600 mt-0.5" />
                    <div className="text-sm text-yellow-800">
                      <p className="font-medium">
                        {tr(
                          "takingLongerTitle",
                          "Taking longer than expected?",
                        )}
                      </p>
                      <p>
                        {tr(
                          "takingLongerBody",
                          "Some payment methods can take several minutes to process. You can safely close this window and return later.",
                        )}
                      </p>
                    </div>
                  </div>
                </div>
              )}
            </CardBody>
          </Card>
        </ModalBody>
      </ModalContent>
    </Modal>
  );
}
