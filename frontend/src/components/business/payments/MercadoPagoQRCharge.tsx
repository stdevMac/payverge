"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Button,
  Chip,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  useDisclosure,
} from "@nextui-org/react";
import { useQuery } from "@tanstack/react-query";
import {
  QrCode,
  AlertTriangle,
  CheckCircle2,
  Loader2,
  Clock,
} from "lucide-react";
import toast from "react-hot-toast";
import axios from "axios";
import {
  mercadoPagoQRAPI,
  paymentPluginAPI,
  type MercadoPagoQRChargeResponse,
} from "@/api/plugins";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { PLUGIN } from "@/constants/plugins";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { DecimalInput } from "@/components/ui/DecimalInput";
import { normalizeMoneyInput } from "@/types/alternativePayments";
import { asDollars, dollarsToCents } from "@/types/money";

const TERMINAL_ORDER_STATUSES = new Set([
  "completed",
  "cancelled",
  "expired",
  "failed",
]);

function isTerminalStatus(status: string | undefined): boolean {
  if (!status) return false;
  return TERMINAL_ORDER_STATUSES.has(status);
}

function formatCountdown(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const m = Math.floor(s / 60);
  const r = s % 60;
  return `${m}:${r.toString().padStart(2, "0")}`;
}

export interface MercadoPagoQRChargeProps {
  businessId: number | string;
  billId: number;
  remainingAmount: number;
  currency: string;
  /** When false, hide charge actions (host/kitchen read-only). */
  canCharge?: boolean;
  onPaymentSettled?: () => void;
  compact?: boolean;
}

export const MercadoPagoQRCharge: React.FC<MercadoPagoQRChargeProps> = ({
  businessId,
  billId,
  remainingAmount,
  currency,
  canCharge = true,
  onPaymentSettled,
}) => {
  const { locale } = useSimpleLocale();
  const businessIdStr = String(businessId);
  const remaining = Math.max(0, remainingAmount);

  const t = useCallback(
    (key: string, params?: Record<string, string | number>) => {
      const result = getTranslation(`mercadoPagoQR.${key}`, locale, params);
      return typeof result === "string" ? result : key;
    },
    [locale],
  );

  const formatCurrency = useCallback(
    (value: number) => formatCurrencyIntl(value, currency, undefined, locale),
    [currency, locale],
  );

  const { isOpen, onOpen, onClose } = useDisclosure();
  const [amount, setAmount] = useState("");
  const [orderId, setOrderId] = useState<string | null>(null);
  const [qrPngBase64, setQrPngBase64] = useState<string | null>(null);
  const [expiresAt, setExpiresAt] = useState<string | null>(null);
  const [chargedAmount, setChargedAmount] = useState<number | null>(null);
  const [charging, setCharging] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [phase, setPhase] = useState<"form" | "polling" | "done">("form");
  const [forceCancelled, setForceCancelled] = useState(false);
  /** 409 recovery without a local QR image — charge is on another device/session. */
  const [otherDevicePending, setOtherDevicePending] = useState(false);
  const [nowMs, setNowMs] = useState(() => Date.now());

  const enabledQuery = useQuery({
    queryKey: ["business-payment-plugins", businessIdStr, "mercadopago-qr"],
    queryFn: async () => {
      const res = await paymentPluginAPI.getBusinessPaymentPlugins(
        Number(businessId),
      );
      return res.plugins ?? [];
    },
    enabled: canCharge && remaining > 0 && Boolean(businessId),
    staleTime: 60_000,
  });

  const mercadoPagoEnabled = useMemo(() => {
    const plugins = enabledQuery.data ?? [];
    return plugins.some(
      (p) => p.name === PLUGIN.mercadopago && p.is_active !== false,
    );
  }, [enabledQuery.data]);

  // Prefill amount when the modal opens (form reset only if not mid-poll).
  useEffect(() => {
    if (!isOpen) return;
    if (phase === "polling") return;
    setAmount(remaining > 0 ? remaining.toFixed(2) : "");
    if (phase === "form") {
      setOrderId(null);
      setQrPngBase64(null);
      setExpiresAt(null);
      setChargedAmount(null);
      setForceCancelled(false);
      setOtherDevicePending(false);
      setCharging(false);
      setCancelling(false);
    }
  }, [isOpen, remaining, phase]);

  // Countdown tick while showing a live QR.
  useEffect(() => {
    if (!isOpen || !expiresAt || phase !== "polling") return;
    setNowMs(Date.now());
    const id = window.setInterval(() => setNowMs(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [isOpen, expiresAt, phase]);

  const secondsLeft = useMemo(() => {
    if (!expiresAt) return 0;
    const exp = Date.parse(expiresAt);
    if (Number.isNaN(exp)) return 0;
    return Math.max(0, Math.floor((exp - nowMs) / 1000));
  }, [expiresAt, nowMs]);

  const isExpiredLocally =
    Boolean(expiresAt) && secondsLeft <= 0 && phase === "polling";

  const orderStatusQuery = useQuery({
    queryKey: ["mercadopago-qr-order", businessIdStr, orderId],
    queryFn: () =>
      mercadoPagoQRAPI.orderStatus(businessIdStr, orderId as string),
    enabled:
      Boolean(orderId) && phase === "polling" && isOpen && !isExpiredLocally,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (isTerminalStatus(status)) return false;
      return 3000;
    },
  });

  const liveStatus = isExpiredLocally
    ? "expired"
    : (orderStatusQuery.data?.status ?? "pending");

  // P6d: 409 recovery poll — adopt PNG from status when MP still has qr_data.
  useEffect(() => {
    if (phase !== "polling") return;
    const png = orderStatusQuery.data?.qr_png_base64;
    if (png && !qrPngBase64) {
      setQrPngBase64(png);
      setOtherDevicePending(false);
    }
  }, [orderStatusQuery.data?.qr_png_base64, phase, qrPngBase64]);

  useEffect(() => {
    if (phase !== "polling") return;
    if (liveStatus === "completed") {
      setPhase("done");
      toast.success(t("toasts.completed"));
      onPaymentSettled?.();
    } else if (
      liveStatus === "cancelled" ||
      liveStatus === "expired" ||
      liveStatus === "failed"
    ) {
      setPhase("done");
    }
  }, [liveStatus, phase, onPaymentSettled, t]);

  const handleClose = () => {
    onClose();
  };

  const startCharge = async (opts?: { regenerate?: boolean }) => {
    const normalized = normalizeMoneyInput(amount);
    if (normalized === null) {
      toast.error(t("errors.amountInvalid"));
      return;
    }
    const dollars = Number(normalized);
    if (dollars > remaining + 0.001) {
      toast.error(t("errors.amountTooHigh"));
      return;
    }

    const amountCents = dollarsToCents(asDollars(dollars));
    setCharging(true);
    try {
      // Best-effort cancel of a previous order so regenerate is not blocked by 409.
      if (opts?.regenerate && orderId) {
        try {
          await mercadoPagoQRAPI.cancelOrder(businessIdStr, orderId);
        } catch {
          // Continue — server may already have expired the tracker.
        }
      }

      const res: MercadoPagoQRChargeResponse = await mercadoPagoQRAPI.charge(
        businessIdStr,
        billId,
        { amount_cents: amountCents as number },
      );
      setOrderId(res.order_id);
      setQrPngBase64(res.qr_png_base64);
      setExpiresAt(res.expires_at);
      setChargedAmount(dollars);
      setForceCancelled(false);
      setPhase("polling");
      toast.success(t("toasts.charged"));
    } catch (err) {
      let message = t("toasts.chargeFailed");
      if (axios.isAxiosError(err)) {
        const status = err.response?.status;
        const data = err.response?.data as
          | { error?: string; order_id?: string; code?: string }
          | undefined;
        if (status === 409 && data?.order_id) {
          // Resume polling/cancel for an existing pending order (mirror Point).
          // No PNG yet — poll may supply qr_png_base64; until then show other-device.
          setOrderId(data.order_id);
          setQrPngBase64(null);
          setOtherDevicePending(true);
          setPhase("polling");
          toast.error(t("errors.pendingConflict"));
          setCharging(false);
          return;
        }
        if (typeof data?.error === "string" && data.error) {
          message = data.error;
        }
      }
      toast.error(message);
    } finally {
      setCharging(false);
    }
  };

  const handleCancelOrder = async () => {
    if (!orderId) return;
    setCancelling(true);
    try {
      await mercadoPagoQRAPI.cancelOrder(businessIdStr, orderId);
      toast.success(t("toasts.cancelled"));
      setForceCancelled(true);
      setPhase("done");
    } catch {
      toast.error(t("toasts.cancelFailed"));
    } finally {
      setCancelling(false);
    }
  };

  const displayStatus = forceCancelled ? "cancelled" : liveStatus;

  const handleRetry = () => {
    setOrderId(null);
    setQrPngBase64(null);
    setExpiresAt(null);
    setChargedAmount(null);
    setForceCancelled(false);
    setPhase("form");
  };

  const handleRegenerate = () => {
    void startCharge({ regenerate: true });
  };

  if (!canCharge || remaining <= 0 || !mercadoPagoEnabled) {
    return null;
  }

  const statusMessage = (() => {
    switch (displayStatus) {
      case "completed":
        return t("status.completed");
      case "cancelled":
        return t("status.cancelled");
      case "expired":
        return t("status.expired");
      case "failed":
        return t("status.failed");
      default:
        return t("status.pending");
    }
  })();

  const showQr =
    phase === "polling" && qrPngBase64 && displayStatus === "pending";

  return (
    <>
      <Button
        size="sm"
        variant="flat"
        className="rounded-xl font-semibold text-brand-dark bg-brand/10"
        startContent={<QrCode className="h-4 w-4" />}
        onPress={onOpen}
        data-testid="mp-qr-charge-open"
      >
        {t("chargeButton")}
      </Button>

      <Modal isOpen={isOpen} onClose={handleClose} size="md">
        <ModalContent>
          <ModalHeader className="border-b border-warm-200">
            {t("modalTitle")}
          </ModalHeader>
          <ModalBody className="gap-4 py-5">
            {phase === "form" ? (
              <DecimalInput
                label={t("amountLabel")}
                min={0}
                step={0.01}
                maxFractionDigits={2}
                value={amount}
                onValueChange={setAmount}
                description={t("amountHint", {
                  remaining: formatCurrency(remaining),
                })}
                isInvalid={
                  amount.trim() !== "" && normalizeMoneyInput(amount) === null
                }
                errorMessage={
                  amount.trim() !== "" && normalizeMoneyInput(amount) === null
                    ? t("errors.amountInvalid")
                    : undefined
                }
                startContent={
                  <span className="text-sm text-ink-500">
                    {formatCurrencyIntl(0, currency)
                      .replace(/[\d.,\s]/g, "")
                      .trim() || "$"}
                  </span>
                }
                data-testid="mp-qr-amount"
              />
            ) : null}

            {phase === "polling" || phase === "done" ? (
              <div
                className="flex flex-col items-center gap-3 py-2 text-center"
                data-testid="mp-qr-status"
              >
                <Chip
                  size="sm"
                  variant="flat"
                  className="bg-brand/10 font-semibold text-brand-dark"
                >
                  {t("brandChip")}
                </Chip>

                {chargedAmount != null ? (
                  <p className="text-lg font-semibold tabular-nums text-ink-950">
                    {formatCurrency(chargedAmount)}
                  </p>
                ) : null}

                {showQr ? (
                  <>
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={`data:image/png;base64,${qrPngBase64}`}
                      alt={t("scanHint")}
                      className="h-auto w-full max-w-xs rounded-xl border border-warm-200 bg-white p-3"
                      data-testid="mp-qr-image"
                    />
                    <p className="text-sm text-ink-600">{t("scanHint")}</p>
                    <div className="flex items-center gap-1.5 text-xs font-medium text-ink-500">
                      <Clock className="h-3.5 w-3.5" aria-hidden />
                      {t("countdown", { time: formatCountdown(secondsLeft) })}
                    </div>
                  </>
                ) : null}

                {phase === "polling" &&
                otherDevicePending &&
                !showQr &&
                displayStatus === "pending" ? (
                  <p
                    className="text-sm text-ink-600"
                    data-testid="mp-qr-other-device"
                  >
                    {t("errors.otherDevice")}
                  </p>
                ) : null}

                {displayStatus === "completed" ? (
                  <CheckCircle2
                    className="h-10 w-10 text-emerald-600"
                    aria-hidden
                  />
                ) : displayStatus === "pending" && !showQr ? (
                  <Loader2
                    className="h-10 w-10 animate-spin text-brand"
                    aria-hidden
                  />
                ) : displayStatus !== "pending" ? (
                  <AlertTriangle
                    className="h-10 w-10 text-amber-600"
                    aria-hidden
                  />
                ) : null}

                <p className="text-sm font-medium text-ink-900">
                  {statusMessage}
                </p>
                {displayStatus === "expired" ? (
                  <p className="text-sm text-ink-600">{t("expired")}</p>
                ) : null}
                {orderId ? (
                  <p className="font-mono text-xs text-ink-500">{orderId}</p>
                ) : null}
              </div>
            ) : null}
          </ModalBody>
          <ModalFooter className="border-t border-warm-200">
            {phase === "form" ? (
              <>
                <Button variant="light" onPress={handleClose}>
                  {t("cancel")}
                </Button>
                <Button
                  color="primary"
                  className="font-semibold"
                  isLoading={charging}
                  isDisabled={charging}
                  onPress={() => {
                    void startCharge();
                  }}
                  data-testid="mp-qr-confirm"
                >
                  {t("confirmCharge")}
                </Button>
              </>
            ) : null}

            {phase === "polling" ? (
              <>
                {isExpiredLocally ? (
                  <Button
                    color="primary"
                    className="font-semibold"
                    isLoading={charging}
                    onPress={handleRegenerate}
                    data-testid="mp-qr-regenerate"
                  >
                    {t("regenerate")}
                  </Button>
                ) : (
                  <Button
                    variant="flat"
                    className="font-semibold text-rose-700"
                    isLoading={cancelling}
                    onPress={() => {
                      void handleCancelOrder();
                    }}
                    data-testid="mp-qr-cancel-order"
                  >
                    {t("cancelCharge")}
                  </Button>
                )}
                <Button variant="light" onPress={handleClose}>
                  {t("cancel")}
                </Button>
              </>
            ) : null}

            {phase === "done" ? (
              <>
                {displayStatus !== "completed" ? (
                  displayStatus === "expired" ? (
                    <Button
                      color="primary"
                      className="font-semibold"
                      isLoading={charging}
                      onPress={handleRegenerate}
                      data-testid="mp-qr-regenerate"
                    >
                      {t("regenerate")}
                    </Button>
                  ) : (
                    <Button
                      variant="flat"
                      className="font-semibold"
                      onPress={handleRetry}
                      data-testid="mp-qr-retry"
                    >
                      {t("retry")}
                    </Button>
                  )
                ) : null}
                <Button
                  color={displayStatus === "completed" ? "primary" : "default"}
                  variant={displayStatus === "completed" ? "solid" : "light"}
                  className="font-semibold"
                  onPress={handleClose}
                >
                  {t("done")}
                </Button>
              </>
            ) : null}
          </ModalFooter>
        </ModalContent>
      </Modal>
    </>
  );
};
