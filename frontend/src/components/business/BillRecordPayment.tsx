"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Button,
  Chip,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Select,
  SelectItem,
  useDisclosure,
} from "@nextui-org/react";
import { Banknote, CheckCircle2, Clock } from "lucide-react";
import toast from "react-hot-toast";
import {
  AlternativeBillPayment,
  Payment,
  isActiveBillStatus,
} from "@/api/bills";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { useBusinessAlternativePayments } from "@/hooks/useAlternativePayments";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { DecimalInput } from "@/components/ui/DecimalInput";
import {
  PaymentMethod,
  amountMicroToDollars,
  asMoneyString,
  formatUSDCAmount,
  normalizeMoneyInput,
  translatePaymentMethodLabel,
} from "@/types/alternativePayments";
import {
  recordPaymentMethodI18nSuffix,
  recordPaymentTenderOptions,
} from "@/lib/tenderOptions";
import { MercadoPagoPointCharge } from "@/components/business/payments/MercadoPagoPointCharge";
import { MercadoPagoQRCharge } from "@/components/business/payments/MercadoPagoQRCharge";
import { cashRegisterApi } from "@/api/cashRegister";
import { asDollars } from "@/types/money";
import {
  alternativePaymentRequestIsExpired,
  formatAlternativePaymentRequestAge,
  formatAlternativePaymentRequestedAt,
} from "@/lib/alternativePaymentExpiry";

const formatMethodLabel = (
  method: string,
  tString: (key: string) => string,
): string => {
  const normalized = method.trim().toLowerCase();
  if (normalized === "crypto") return tString("recordPayment.methods.crypto");
  if (normalized === "cash") return tString("recordPayment.methods.cash");
  if (normalized === "card") return tString("recordPayment.methods.card");
  if (normalized === "venmo") return tString("recordPayment.methods.venmo");
  if (normalized === "other") return tString("recordPayment.methods.other");
  if (!normalized) return tString("recordPayment.methods.unknown");
  return normalized.charAt(0).toUpperCase() + normalized.slice(1);
};

// Translate a PaymentMethod enum label through recordPayment.methods.* so the
// method dropdown + pending rows aren't stuck on the hardcoded English labels
// in PAYMENT_METHOD_OPTIONS (R3-BP-4).
const methodOptionLabel = (
  method: PaymentMethod,
  tString: (key: string) => string,
  locale?: string,
  country?: string | null,
): string => {
  const suffix = recordPaymentMethodI18nSuffix(method, locale, country);
  const translated = tString(`recordPayment.methods.${suffix}`);
  if (translated && translated !== `recordPayment.methods.${suffix}`) {
    return translated;
  }
  return translatePaymentMethodLabel(method, (key) =>
    tString(`recordPayment.methods.${key}`),
  );
};

/** Hide the in-card Record payment on narrow drawers — the sticky footer
 *  already owns that primary. Keep the node in the DOM so the footer
 *  can still `querySelector` + click it (#373). */
const BILL_RECORD_PAYMENT_OPEN_CLASS =
  "rounded-xl font-semibold max-sm:hidden";

export interface BillRecordPaymentProps {
  billId: number;
  billNumber: string;
  billStatus: string;
  currency: string;
  remainingAmount: number;
  payments?: Payment[];
  alternativePayments?: AlternativeBillPayment[];
  onPaymentRecorded: () => void;
  tString: (key: string) => string;
  /** Business id for Point / plugin charge flows. */
  businessId?: number | string;
  /** When false, show pending/ledger read-only (host/kitchen). */
  canRecordPayment?: boolean;
  /** Compact layout for display mode side panel */
  compact?: boolean;
  /** Wave 4: parent can pre-open the record modal (BillCreator "Record payment"). */
  autoOpenRecord?: boolean;
  /** Wave 4: print the bill receipt (BillDetailsModal.handlePrintBill). */
  onPrintReceipt?: () => void | Promise<void>;
  /** Wave 4: request bill close (routes through the parent's confirmation). */
  onRequestCloseBill?: () => void;
  /** Whether the viewer may print (print:bill / owner). */
  canPrint?: boolean;
  /** Business address / fiscal country — used to hide US-only tenders. */
  country?: string | null;
}

export const BillRecordPayment: React.FC<BillRecordPaymentProps> = ({
  billId,
  billNumber,
  billStatus,
  currency,
  remainingAmount,
  payments = [],
  alternativePayments = [],
  onPaymentRecorded,
  tString,
  businessId,
  canRecordPayment = true,
  compact = false,
  autoOpenRecord = false,
  onPrintReceipt,
  onRequestCloseBill,
  canPrint = false,
  country = null,
}) => {
  const { locale } = useSimpleLocale();
  const billIdStr = String(billId);
  const { pendingPayments, loading, markPayment, rejectPayment, refresh } =
    useBusinessAlternativePayments(billIdStr);

  const { isOpen, onOpen, onClose } = useDisclosure();
  const [amount, setAmount] = useState("");
  const [tipAmount, setTipAmount] = useState("");
  const [note, setNote] = useState("");
  const [selectedMethod, setSelectedMethod] = useState<PaymentMethod>(
    PaymentMethod.CASH,
  );
  const [submitting, setSubmitting] = useState(false);
  const [confirmingId, setConfirmingId] = useState<string | null>(null);
  const [rejectingId, setRejectingId] = useState<string | null>(null);
  const [settledSuccess, setSettledSuccess] = useState(false);
  const [cashSessionOpen, setCashSessionOpen] = useState<boolean | null>(null);
  const [cashSessionLookupFailed, setCashSessionLookupFailed] = useState(false);
  const [suggestedOpeningFloat, setSuggestedOpeningFloat] = useState(
    asDollars(0),
  );
  const [openingDrawer, setOpeningDrawer] = useState(false);

  const isPayable = isActiveBillStatus(billStatus);
  const remaining = Math.max(0, remainingAmount);
  const cashSelected = selectedMethod === PaymentMethod.CASH;
  // null = still loading / unverifiable. Cash must stay blocked until an
  // applicable open drawer is confirmed — do not treat unknown as Unassigned.
  const cashSubmitBlocked = cashSelected && cashSessionOpen !== true;
  const cashNeedsSession =
    cashSelected && (cashSessionOpen === false || businessId == null);
  const cashSessionChecking =
    cashSelected &&
    businessId != null &&
    cashSessionOpen === null &&
    !cashSessionLookupFailed;

  const formatCurrency = useCallback(
    (value: number) => formatCurrencyIntl(value, currency, undefined, locale),
    [currency, locale],
  );

  // Wave 4: parent can pre-open the record modal (BillCreator "Record payment").
  const autoOpenedRef = React.useRef(false);
  useEffect(() => {
    if (
      autoOpenRecord &&
      !autoOpenedRef.current &&
      isPayable &&
      remaining > 0 &&
      canRecordPayment
    ) {
      autoOpenedRef.current = true;
      onOpen();
    }
  }, [autoOpenRecord, isPayable, remaining, canRecordPayment, onOpen]);

  useEffect(() => {
    if (isOpen) {
      setAmount(remaining > 0 ? remaining.toFixed(2) : "");
      setTipAmount("");
      setNote("");
      setSelectedMethod(PaymentMethod.CASH);
      setSettledSuccess(false);
    }
  }, [isOpen, remaining]);

  // Cash recording requires a known open drawer. Unknown (null) stays blocked
  // until GET /current resolves; a lookup failure is an explicit error, not
  // a silent Unassigned-cash path (#374).
  useEffect(() => {
    if (!isOpen || !businessId) {
      setCashSessionOpen(null);
      setCashSessionLookupFailed(false);
      return;
    }
    let cancelled = false;
    setCashSessionOpen(null);
    setCashSessionLookupFailed(false);
    void cashRegisterApi
      .getCurrent(String(businessId))
      .then((res) => {
        if (!cancelled) {
          setCashSessionOpen(res.session?.status === "open");
          setSuggestedOpeningFloat(
            asDollars(
              typeof res.suggested_opening_float === "number"
                ? res.suggested_opening_float
                : 0,
            ),
          );
          setCashSessionLookupFailed(false);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setCashSessionOpen(null);
          setCashSessionLookupFailed(true);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [isOpen, businessId]);

  const confirmedLedger = useMemo(() => {
    const cryptoRows = payments
      .filter((p) => p.status === "confirmed")
      .map((p) => ({
        id: `payment-${p.id}`,
        method: p.payment_method ?? "crypto",
        amount: p.amount,
        tip: p.tip_amount ?? 0,
        at: p.confirmed_at ?? p.created_at,
      }));
    const altRows = alternativePayments
      .filter((p) => p.status === "confirmed")
      .map((p) => ({
        id: `alt-${p.id}`,
        method: p.payment_method,
        amount: p.amount,
        // tip_amount_cents is CENTS (models_json.go); convert to dollars so it
        // renders with the same "+$X tip" affordance as crypto rows (R3-BP-5).
        tip: (p.tip_amount_cents ?? 0) / 100,
        at: p.confirmed_at ?? p.created_at,
      }));
    return [...cryptoRows, ...altRows].sort((a, b) => {
      const aTime = Date.parse(a.at ?? "") || 0;
      const bTime = Date.parse(b.at ?? "") || 0;
      return bTime - aTime;
    });
  }, [payments, alternativePayments]);

  const handleOpenDrawer = async () => {
    if (!businessId || openingDrawer) {
      return;
    }
    setOpeningDrawer(true);
    try {
      await cashRegisterApi.openSession(String(businessId), {
        opening_float: suggestedOpeningFloat,
        opening_note: tString("recordPayment.actions.openDrawer"),
      });
      setCashSessionOpen(true);
      setCashSessionLookupFailed(false);
      toast.success(tString("recordPayment.toasts.drawerOpened"));
    } catch {
      toast.error(tString("recordPayment.errors.openDrawerFailed"));
    } finally {
      setOpeningDrawer(false);
    }
  };

  const handleRecordPayment = async () => {
    // Normalize/validate to a 2-decimal string (accepting comma decimals) so
    // the validation gate agrees with the asMoneyString submit path — before,
    // "10,50"/"10.999"/".50" passed parseFloat then threw a generic failure at
    // submit time (R3-BP-6).
    const normalizedAmount = normalizeMoneyInput(amount);
    if (normalizedAmount === null) {
      toast.error(tString("recordPayment.errors.amountInvalid"));
      return;
    }
    if (Number(normalizedAmount) > remaining + 0.001) {
      toast.error(tString("recordPayment.errors.amountTooHigh"));
      return;
    }

    let normalizedTip: string | null = null;
    if (tipAmount.trim()) {
      normalizedTip = normalizeMoneyInput(tipAmount);
      if (normalizedTip === null) {
        toast.error(tString("recordPayment.errors.tipInvalid"));
        return;
      }
    }

    if (selectedMethod === PaymentMethod.CASH && cashSessionOpen !== true) {
      toast.error(
        tString(
          cashSessionLookupFailed
            ? "recordPayment.errors.cashSessionLookupFailed"
            : "recordPayment.errors.cashSessionRequired",
        ),
      );
      return;
    }

    // Open-drawer lives on the warning, not this submit path.

    setSubmitting(true);
    try {
      const response = await markPayment(
        undefined,
        asMoneyString(normalizedAmount),
        selectedMethod,
        undefined,
        {
          participantName: note.trim() || billNumber,
          tipAmount: normalizedTip ? asMoneyString(normalizedTip) : undefined,
          // No explicit idempotencyKey: the API layer mints an attempt-scoped
          // key and replays it on retry, so a lost response can't double-credit.
        },
      );

      if (response.success) {
        const settles = Number(normalizedAmount) >= remaining - 0.001;
        toast.success(tString("recordPayment.toasts.recorded"));
        onPaymentRecorded();
        void refresh();
        if (settles && (onPrintReceipt || onRequestCloseBill)) {
          setSettledSuccess(true); // keep the modal open on the settled step
        } else {
          onClose();
        }
      } else {
        toast.error(response.message || tString("recordPayment.errors.failed"));
      }
    } catch {
      toast.error(tString("recordPayment.errors.failed"));
    } finally {
      setSubmitting(false);
    }
  };

  const handleConfirmPending = async (
    pendingId: string,
    pendingAmount: string,
  ) => {
    const pending = pendingPayments.find((p) => p.id === pendingId);
    if (pending && alternativePaymentRequestIsExpired(pending)) {
      toast.error(tString("recordPayment.errors.requestExpired"));
      return;
    }
    setConfirmingId(pendingId);
    try {
      const dollars = formatUSDCAmount(pendingAmount);
      const response = await markPayment(
        pending?.participantAddress ?? "guest",
        asMoneyString(dollars),
        pending?.paymentMethod ?? PaymentMethod.CASH,
        pendingId,
      );

      if (response.success) {
        toast.success(tString("recordPayment.toasts.confirmed"));
        onPaymentRecorded();
        void refresh();
      } else {
        toast.error(response.message || tString("recordPayment.errors.failed"));
      }
    } catch {
      toast.error(tString("recordPayment.errors.failed"));
    } finally {
      setConfirmingId(null);
    }
  };

  const handleRejectPending = async (pendingId: string) => {
    setRejectingId(pendingId);
    try {
      const response = await rejectPayment(pendingId);
      if (response.success) {
        toast.success(tString("recordPayment.toasts.rejected"));
        onPaymentRecorded();
        void refresh();
      } else {
        toast.error(tString("recordPayment.errors.rejectFailed"));
      }
    } catch {
      toast.error(tString("recordPayment.errors.rejectFailed"));
    } finally {
      setRejectingId(null);
    }
  };

  if (
    !isPayable &&
    confirmedLedger.length === 0 &&
    pendingPayments.length === 0
  ) {
    return null;
  }

  return (
    <section
      className={`rounded-2xl border border-warm-200/80 bg-white/90 shadow-sm shadow-warm-900/5 ${
        compact ? "p-3" : "p-4"
      }`}
      aria-label={tString("recordPayment.title")}
    >
      <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold uppercase tracking-wider text-ink-950">
            {tString("recordPayment.title")}
          </h3>
          <p className="mt-1 text-sm text-ink-600">
            {isPayable && canRecordPayment
              ? tString("recordPayment.subtitlePayable").replace(
                  "{remaining}",
                  formatCurrency(remaining),
                )
              : isPayable && !canRecordPayment
                ? tString("recordPayment.subtitleReadOnly")
                : tString("recordPayment.subtitleSettled")}
          </p>
        </div>
        {isPayable && remaining > 0 && canRecordPayment ? (
          <div className="flex flex-wrap items-center gap-2">
            <Button
              color="primary"
              size="sm"
              className={BILL_RECORD_PAYMENT_OPEN_CLASS}
              startContent={<Banknote className="h-4 w-4" />}
              onPress={onOpen}
              data-testid="bill-record-payment-open"
            >
              {tString("recordPayment.actions.record")}
            </Button>
            {businessId != null ? (
              <>
                <MercadoPagoPointCharge
                  businessId={businessId}
                  billId={billId}
                  remainingAmount={remaining}
                  currency={currency}
                  canCharge={canRecordPayment}
                  onPaymentSettled={onPaymentRecorded}
                  compact={compact}
                />
                <MercadoPagoQRCharge
                  businessId={businessId}
                  billId={billId}
                  remainingAmount={remaining}
                  currency={currency}
                  canCharge={canRecordPayment}
                  onPaymentSettled={onPaymentRecorded}
                  compact={compact}
                />
              </>
            ) : null}
          </div>
        ) : null}
      </div>

      {pendingPayments.length > 0 ? (
        <div className="mb-4 space-y-2">
          <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-amber-800">
            <Clock className="h-3.5 w-3.5" />
            {tString("recordPayment.pending.title")}
          </div>
          {pendingPayments.map((pending) => {
            const dollars = amountMicroToDollars(pending.amount);
            const expired = alternativePaymentRequestIsExpired(pending);
            const requestedAt = formatAlternativePaymentRequestedAt(
              pending.timestamp,
              locale,
            );
            const age = formatAlternativePaymentRequestAge(pending.timestamp);
            return (
              <div
                key={pending.id}
                className={`flex flex-wrap items-center justify-between gap-2 rounded-xl border px-3 py-2 ${
                  expired
                    ? "border-rose-200 bg-rose-50"
                    : "border-amber-200 bg-amber-50"
                }`}
              >
                <div className="min-w-0">
                  <p className="text-sm font-medium text-ink-900">
                    {methodOptionLabel(
                      pending.paymentMethod,
                      tString,
                      locale,
                      country,
                    )} ·{" "}
                    {formatCurrency(dollars)}
                  </p>
                  <p className="truncate text-xs text-ink-600">
                    {pending.participantName?.trim() ||
                      pending.participantAddress ||
                      tString("recordPayment.pending.guest")}
                  </p>
                  <p className="mt-1 text-xs text-ink-500">
                    <span data-testid="pending-payment-requested-at">
                      {tString("recordPayment.pending.requestedAt").replace(
                        "{time}",
                        requestedAt,
                      )}
                    </span>
                    {" · "}
                    <span data-testid="pending-payment-age">{age}</span>
                  </p>
                  {expired ? (
                    <p
                      className="mt-1 text-xs font-semibold text-rose-700"
                      data-testid="pending-payment-expired"
                    >
                      {tString("recordPayment.pending.expired")}
                    </p>
                  ) : null}
                </div>
                {canRecordPayment ? (
                  <div className="flex items-center gap-2">
                    <Button
                      size="sm"
                      variant="flat"
                      className="rounded-lg font-semibold text-rose-700"
                      isLoading={rejectingId === pending.id}
                      isDisabled={
                        confirmingId !== null ||
                        (rejectingId !== null && rejectingId !== pending.id)
                      }
                      onPress={() => handleRejectPending(pending.id)}
                    >
                      {tString("recordPayment.pending.reject")}
                    </Button>
                    <Button
                      size="sm"
                      color="primary"
                      className="rounded-lg font-semibold"
                      isLoading={confirmingId === pending.id}
                      isDisabled={
                        expired ||
                        rejectingId !== null ||
                        (confirmingId !== null && confirmingId !== pending.id)
                      }
                      onPress={() =>
                        handleConfirmPending(pending.id, pending.amount)
                      }
                    >
                      {tString("recordPayment.pending.confirm")}
                    </Button>
                  </div>
                ) : (
                  <Chip
                    size="sm"
                    variant="flat"
                    className="bg-amber-100 text-amber-800"
                  >
                    {tString("recordPayment.pending.awaitingStaff")}
                  </Chip>
                )}
              </div>
            );
          })}
        </div>
      ) : null}

      {confirmedLedger.length > 0 ? (
        <div className="space-y-2">
          <p className="text-xs font-semibold uppercase tracking-wide text-ink-500">
            {tString("recordPayment.ledger.title")}
          </p>
          <ul className="space-y-1.5">
            {confirmedLedger.map((row) => (
              <li
                key={row.id}
                className="flex items-center justify-between gap-2 rounded-lg bg-warm-50 px-3 py-2 text-sm"
              >
                <span className="text-ink-700">
                  {formatMethodLabel(row.method, tString)}
                </span>
                <span className="font-mono font-semibold tabular-nums text-ink-900">
                  {formatCurrency(row.amount)}
                  {row.tip > 0 ? (
                    <span className="ml-1 text-xs font-normal text-emerald-700">
                      +{formatCurrency(row.tip)}{" "}
                      {tString("recordPayment.ledger.tip")}
                    </span>
                  ) : null}
                </span>
              </li>
            ))}
          </ul>
        </div>
      ) : !loading && isPayable && remaining > 0 ? (
        <p className="text-sm text-ink-500">{tString("recordPayment.empty")}</p>
      ) : null}

      {isPayable && remaining <= 0 && confirmedLedger.length > 0 ? (
        <Chip
          size="sm"
          variant="flat"
          className="mt-3 bg-emerald-50 font-medium text-emerald-700"
          startContent={<CheckCircle2 className="h-3.5 w-3.5" />}
        >
          {tString("recordPayment.status.paid")}
        </Chip>
      ) : null}

      <Modal
        isOpen={isOpen}
        onClose={onClose}
        size="md"
        isDismissable={!submitting}
        isKeyboardDismissDisabled={submitting}
      >
        <ModalContent>
          <ModalHeader className="border-b border-warm-200">
            {tString("recordPayment.modal.title")}
          </ModalHeader>
          <ModalBody className="gap-4 py-5">
            {settledSuccess ? (
              <div
                data-testid="record-payment-settled"
                className="flex flex-col items-center gap-4 py-4 text-center"
              >
                <CheckCircle2
                  className="h-10 w-10 text-emerald-600"
                  aria-hidden="true"
                />
                <p className="text-sm font-semibold text-ink-900">
                  {tString("recordPayment.settled.title")}
                </p>
                <div className="flex flex-wrap justify-center gap-2">
                  {canPrint && onPrintReceipt ? (
                    <Button
                      color="primary"
                      onPress={() => {
                        void onPrintReceipt();
                      }}
                    >
                      {tString("recordPayment.settled.print")}
                    </Button>
                  ) : null}
                  {onRequestCloseBill ? (
                    <Button
                      variant="flat"
                      className="bg-amber-100 font-medium text-amber-800"
                      onPress={() => {
                        setSettledSuccess(false);
                        onClose();
                        onRequestCloseBill();
                      }}
                    >
                      {tString("recordPayment.settled.closeBill")}
                    </Button>
                  ) : null}
                  <Button
                    variant="light"
                    onPress={() => {
                      setSettledSuccess(false);
                      onClose();
                    }}
                  >
                    {tString("recordPayment.settled.done")}
                  </Button>
                </div>
              </div>
            ) : (
              <>
                <p className="text-sm text-ink-600">
                  {tString("recordPayment.modal.description").replace(
                    "{remaining}",
                    formatCurrency(remaining),
                  )}
                </p>

                <DecimalInput
                  label={tString("recordPayment.modal.amount")}
                  min={0}
                  step={0.01}
                  maxFractionDigits={2}
                  value={amount}
                  onValueChange={setAmount}
                  isInvalid={
                    amount.trim() !== "" && normalizeMoneyInput(amount) === null
                  }
                  errorMessage={
                    amount.trim() !== "" && normalizeMoneyInput(amount) === null
                      ? tString("recordPayment.errors.amountInvalid")
                      : undefined
                  }
                  startContent={
                    <span className="text-sm text-ink-500">
                      {formatCurrencyIntl(0, currency)
                        .replace(/[\d.,\s]/g, "")
                        .trim() || "$"}
                    </span>
                  }
                  data-testid="record-payment-amount"
                />

                <DecimalInput
                  label={tString("recordPayment.modal.tip")}
                  min={0}
                  step={0.01}
                  maxFractionDigits={2}
                  value={tipAmount}
                  onValueChange={setTipAmount}
                  isInvalid={
                    tipAmount.trim() !== "" &&
                    normalizeMoneyInput(tipAmount) === null
                  }
                  errorMessage={
                    tipAmount.trim() !== "" &&
                    normalizeMoneyInput(tipAmount) === null
                      ? tString("recordPayment.errors.tipInvalid")
                      : undefined
                  }
                  description={tString("recordPayment.modal.tipHint")}
                  data-testid="record-payment-tip"
                />

                <Select
                  label={tString("recordPayment.modal.method")}
                  selectedKeys={[String(selectedMethod)]}
                  data-testid="record-payment-method"
                  onSelectionChange={(keys) => {
                    const key = Array.from(keys)[0];
                    if (key != null) {
                      setSelectedMethod(Number(key) as PaymentMethod);
                    }
                  }}
                >
                  {recordPaymentTenderOptions(locale, country).map((opt) => {
                    const suffix = recordPaymentMethodI18nSuffix(
                      opt.value,
                      locale,
                      country,
                    );
                    const label = methodOptionLabel(
                      opt.value,
                      tString,
                      locale,
                      country,
                    );
                    return (
                      <SelectItem
                        key={String(opt.value)}
                        textValue={label}
                        data-testid={`record-payment-tender-${suffix}`}
                      >
                        {label}
                      </SelectItem>
                    );
                  })}
                </Select>

                {cashSessionLookupFailed && cashSelected ? (
                  <div
                    className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-900"
                    data-testid="cash-session-error"
                    role="alert"
                  >
                    {tString("recordPayment.errors.cashSessionLookupFailed")}
                  </div>
                ) : cashSessionChecking ? (
                  <div
                    className="rounded-xl border border-warm-200 bg-warm-50 px-3 py-2 text-sm text-ink-700"
                    data-testid="cash-session-checking"
                    role="status"
                  >
                    {tString("recordPayment.warnings.cashSessionChecking")}
                  </div>
                ) : cashNeedsSession ? (
                  <div
                    className="rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900"
                    data-testid="cash-session-warning"
                    role="status"
                  >
                    <p>{tString("recordPayment.warnings.cashSessionRequired")}</p>
                    {businessId != null ? (
                      <Button
                        size="sm"
                        className="mt-2 font-semibold"
                        color="primary"
                        isLoading={openingDrawer}
                        onPress={() => {
                          void handleOpenDrawer();
                        }}
                        data-testid="cash-session-open-drawer"
                      >
                        {tString("recordPayment.actions.openDrawer")}
                      </Button>
                    ) : null}
                  </div>
                ) : null}

                <Input
                  label={tString("recordPayment.modal.note")}
                  placeholder={tString("recordPayment.modal.notePlaceholder")}
                  value={note}
                  onValueChange={setNote}
                  description={tString("recordPayment.modal.noteHint")}
                />
              </>
            )}
          </ModalBody>
          {settledSuccess ? null : (
            <ModalFooter className="border-t border-warm-200">
              <Button variant="light" onPress={onClose}>
                {tString("recordPayment.modal.cancel")}
              </Button>
              <Button
                color="primary"
                className="font-semibold"
                isLoading={submitting}
                isDisabled={cashSubmitBlocked}
                onPress={handleRecordPayment}
              >
                {tString("recordPayment.modal.submit")}
              </Button>
            </ModalFooter>
          )}
        </ModalContent>
      </Modal>
    </section>
  );
};
