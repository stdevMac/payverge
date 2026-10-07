"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Button,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Select,
  SelectItem,
  Spinner,
  useDisclosure,
} from "@nextui-org/react";
import { useQuery } from "@tanstack/react-query";
import { CreditCard, AlertTriangle, CheckCircle2, Loader2 } from "lucide-react";
import toast from "react-hot-toast";
import axios from "axios";
import {
  mercadoPagoPointAPI,
  paymentPluginAPI,
  type MercadoPagoTerminal,
} from "@/api/plugins";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { PLUGIN } from "@/constants/plugins";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { DecimalInput } from "@/components/ui/DecimalInput";
import {
  normalizeMoneyInput,
} from "@/types/alternativePayments";
import { asDollars, dollarsToCents } from "@/types/money";

const TERMINAL_STORAGE_PREFIX = "payverge_mp_terminal_";

const TERMINAL_ORDER_STATUSES = new Set([
  "completed",
  "cancelled",
  "expired",
  "failed",
]);

function terminalStorageKey(businessId: string): string {
  return `${TERMINAL_STORAGE_PREFIX}${businessId}`;
}

function readStoredTerminalId(businessId: string): string {
  if (typeof window === "undefined") return "";
  try {
    return localStorage.getItem(terminalStorageKey(businessId)) ?? "";
  } catch {
    return "";
  }
}

function storeTerminalId(businessId: string, terminalId: string): void {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(terminalStorageKey(businessId), terminalId);
  } catch {
    // ignore quota / private mode
  }
}

function isTerminalStatus(status: string | undefined): boolean {
  if (!status) return false;
  return TERMINAL_ORDER_STATUSES.has(status);
}

export interface MercadoPagoPointChargeProps {
  businessId: number | string;
  billId: number;
  remainingAmount: number;
  currency: string;
  /** When false, hide charge actions (host/kitchen read-only). */
  canCharge?: boolean;
  onPaymentSettled?: () => void;
  compact?: boolean;
}

export const MercadoPagoPointCharge: React.FC<MercadoPagoPointChargeProps> = ({
  businessId,
  billId,
  remainingAmount,
  currency,
  canCharge = true,
  onPaymentSettled,
  compact = false,
}) => {
  const { locale } = useSimpleLocale();
  const businessIdStr = String(businessId);
  const remaining = Math.max(0, remainingAmount);

  const t = useCallback(
    (key: string, params?: Record<string, string | number>) => {
      const result = getTranslation(`mercadoPagoPoint.${key}`, locale, params);
      return typeof result === "string" ? result : key;
    },
    [locale],
  );

  const formatCurrency = useCallback(
    (value: number) => formatCurrencyIntl(value, currency, undefined, locale),
    [currency, locale],
  );

  const { isOpen, onOpen, onClose } = useDisclosure();
  const [selectedTerminalId, setSelectedTerminalId] = useState("");
  const [amount, setAmount] = useState("");
  const [orderId, setOrderId] = useState<string | null>(null);
  const [charging, setCharging] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [activatingPdv, setActivatingPdv] = useState(false);
  const [phase, setPhase] = useState<"form" | "polling" | "done">("form");
  /** Local override after staff cancels (query may lag one tick). */
  const [forceCancelled, setForceCancelled] = useState(false);

  const enabledQuery = useQuery({
    queryKey: ["business-payment-plugins", businessIdStr, "mercadopago-point"],
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
      (p) =>
        p.name === PLUGIN.mercadopago &&
        (p.is_active !== false),
    );
  }, [enabledQuery.data]);

  const terminalsQuery = useQuery({
    queryKey: ["mercadopago-terminals", businessIdStr],
    queryFn: () => mercadoPagoPointAPI.listTerminals(businessIdStr),
    enabled: isOpen && mercadoPagoEnabled,
    staleTime: 30_000,
  });

  const terminals: MercadoPagoTerminal[] = useMemo(
    () => terminalsQuery.data?.terminals ?? [],
    [terminalsQuery.data?.terminals],
  );

  const selectedTerminal = useMemo(
    () => terminals.find((tRow) => tRow.id === selectedTerminalId),
    [terminals, selectedTerminalId],
  );

  const needsPdv =
    Boolean(selectedTerminal) &&
    (selectedTerminal?.operating_mode ?? "").toUpperCase() !== "PDV";

  // Prefill amount + restore last terminal when the modal opens.
  useEffect(() => {
    if (!isOpen) return;
    setAmount(remaining > 0 ? remaining.toFixed(2) : "");
    setOrderId(null);
    setPhase("form");
    setCharging(false);
    setCancelling(false);
    setForceCancelled(false);
    const stored = readStoredTerminalId(businessIdStr);
    setSelectedTerminalId(stored);
  }, [isOpen, remaining, businessIdStr]);

  // When terminals load, keep a stored selection if still present; else first.
  useEffect(() => {
    if (!isOpen || terminals.length === 0) return;
    setSelectedTerminalId((prev) => {
      if (prev && terminals.some((tRow) => tRow.id === prev)) return prev;
      const stored = readStoredTerminalId(businessIdStr);
      if (stored && terminals.some((tRow) => tRow.id === stored)) return stored;
      return terminals[0]?.id ?? "";
    });
  }, [isOpen, terminals, businessIdStr]);

  const orderStatusQuery = useQuery({
    queryKey: ["mercadopago-point-order", businessIdStr, orderId],
    queryFn: () =>
      mercadoPagoPointAPI.orderStatus(businessIdStr, orderId as string),
    enabled: Boolean(orderId) && phase === "polling" && isOpen,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (isTerminalStatus(status)) return false;
      return 3000;
    },
  });

  const liveStatus = orderStatusQuery.data?.status ?? "pending";

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
    // Allow closing while polling — order continues server-side / on terminal.
    onClose();
  };

  const handleCharge = async () => {
    if (!selectedTerminalId) {
      toast.error(t("errors.selectTerminal"));
      return;
    }
    if (needsPdv) {
      return;
    }
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
      storeTerminalId(businessIdStr, selectedTerminalId);
      const res = await mercadoPagoPointAPI.charge(businessIdStr, billId, {
        terminal_id: selectedTerminalId,
        amount_cents: amountCents as number,
      });
      setOrderId(res.order_id);
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
          // Resume polling an existing pending order.
          setOrderId(data.order_id);
          setPhase("polling");
          toast.error(t("errors.pendingConflict"));
          setCharging(false);
          return;
        }
        if (status === 409) {
          message = t("errors.terminalBusy");
        } else if (typeof data?.error === "string" && data.error) {
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
      await mercadoPagoPointAPI.cancelOrder(businessIdStr, orderId);
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

  const handleActivatePdv = async () => {
    if (!selectedTerminalId) return;
    setActivatingPdv(true);
    try {
      await mercadoPagoPointAPI.setTerminalMode(
        businessIdStr,
        selectedTerminalId,
        "PDV",
      );
      toast.success(t("pdvActivated"));
      await terminalsQuery.refetch();
    } catch {
      toast.error(t("pdvActivateFailed"));
    } finally {
      setActivatingPdv(false);
    }
  };

  const handleRetry = () => {
    setOrderId(null);
    setForceCancelled(false);
    setPhase("form");
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

  return (
    <>
      <Button
        size="sm"
        variant="flat"
        className={`rounded-xl font-semibold text-brand-dark bg-brand/10 ${
          compact ? "" : ""
        }`}
        startContent={<CreditCard className="h-4 w-4" />}
        onPress={onOpen}
        data-testid="mp-point-charge-open"
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
              <>
                {terminalsQuery.isLoading ? (
                  <div className="flex items-center gap-2 text-sm text-ink-600">
                    <Spinner size="sm" />
                    {t("loadingTerminals")}
                  </div>
                ) : terminalsQuery.isError ? (
                  <p className="text-sm text-rose-700">{t("terminalsError")}</p>
                ) : terminals.length === 0 ? (
                  <p className="text-sm text-ink-600">{t("noTerminals")}</p>
                ) : (
                  <Select
                    label={t("terminalLabel")}
                    placeholder={t("terminalPlaceholder")}
                    selectedKeys={
                      selectedTerminalId
                        ? new Set([selectedTerminalId])
                        : new Set()
                    }
                    onSelectionChange={(keys) => {
                      const key = Array.from(keys)[0];
                      if (typeof key === "string") {
                        setSelectedTerminalId(key);
                        storeTerminalId(businessIdStr, key);
                      }
                    }}
                    data-testid="mp-point-terminal-select"
                  >
                    {terminals.map((term) => (
                      <SelectItem
                        key={term.id}
                        textValue={
                          term.external_pos_id
                            ? `${term.external_pos_id} (${term.id})`
                            : term.id
                        }
                      >
                        {term.external_pos_id
                          ? `${term.external_pos_id}`
                          : term.id}
                        {term.operating_mode
                          ? ` · ${term.operating_mode}`
                          : ""}
                      </SelectItem>
                    ))}
                  </Select>
                )}

                {needsPdv ? (
                  <div
                    className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-3 text-sm text-amber-900"
                    data-testid="mp-point-pdv-callout"
                  >
                    <div className="mb-1 flex items-center gap-2 font-semibold">
                      <AlertTriangle className="h-4 w-4 shrink-0" />
                      {t("pdvCalloutTitle")}
                    </div>
                    <p className="mb-3 text-amber-800">{t("pdvCalloutBody")}</p>
                    <Button
                      size="sm"
                      color="warning"
                      variant="flat"
                      className="font-semibold"
                      isLoading={activatingPdv}
                      onPress={() => {
                        void handleActivatePdv();
                      }}
                    >
                      {activatingPdv ? t("pdvActivating") : t("pdvActivate")}
                    </Button>
                  </div>
                ) : null}

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
                  data-testid="mp-point-amount"
                />
              </>
            ) : null}

            {phase === "polling" || phase === "done" ? (
              <div
                className="flex flex-col items-center gap-3 py-4 text-center"
                data-testid="mp-point-status"
              >
                {displayStatus === "completed" ? (
                  <CheckCircle2
                    className="h-10 w-10 text-emerald-600"
                    aria-hidden
                  />
                ) : displayStatus === "pending" ? (
                  <Loader2
                    className="h-10 w-10 animate-spin text-brand"
                    aria-hidden
                  />
                ) : (
                  <AlertTriangle
                    className="h-10 w-10 text-amber-600"
                    aria-hidden
                  />
                )}
                <p className="text-sm font-medium text-ink-900">
                  {statusMessage}
                </p>
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
                  isDisabled={
                    charging ||
                    needsPdv ||
                    !selectedTerminalId ||
                    terminals.length === 0 ||
                    terminalsQuery.isLoading
                  }
                  onPress={() => {
                    void handleCharge();
                  }}
                  data-testid="mp-point-confirm"
                >
                  {t("confirmCharge")}
                </Button>
              </>
            ) : null}

            {phase === "polling" ? (
              <>
                <Button variant="light" onPress={handleClose}>
                  {t("cancel")}
                </Button>
                <Button
                  variant="flat"
                  className="font-semibold text-rose-700"
                  isLoading={cancelling}
                  onPress={() => {
                    void handleCancelOrder();
                  }}
                  data-testid="mp-point-cancel-order"
                >
                  {t("cancelCharge")}
                </Button>
              </>
            ) : null}

            {phase === "done" ? (
              <>
                {displayStatus !== "completed" ? (
                  <Button
                    variant="flat"
                    className="font-semibold"
                    onPress={handleRetry}
                    data-testid="mp-point-retry"
                  >
                    {t("retry")}
                  </Button>
                ) : null}
                <Button
                  color="primary"
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
