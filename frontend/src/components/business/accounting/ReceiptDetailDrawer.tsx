"use client";

import React, { useCallback, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Button,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@nextui-org/react";
import type { Locale } from "@/i18n/localeRegistry";
import {
  listReceiptDelivery,
  type FiscalDeliveryTask,
  type FiscalStatus,
  type ReceiptRow,
} from "@/api/fiscal";
import {
  useCreditNote,
  useResendReceipt,
  useRetryDelivery,
} from "@/hooks/accounting/useAccountingQueries";
import { formatMoney } from "@/components/business/accounting/format";
import {
  DATE_TIME_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";
import { AnimatedNumberText } from "../premium";
import DetailDrawer from "../shared/DetailDrawer";
import StatusBadge, { type StatusTone } from "./StatusBadge";
import {
  accountingPrimaryButtonClass,
  accountingSecondaryButtonClass,
} from "./accountingShared";

export type ReceiptDetailDrawerProps = {
  open: boolean;
  receipt: ReceiptRow | null;
  businessId: string;
  locale: Locale;
  currency: string;
  /** When false, hide Re-send / delivery Retry. */
  canWrite?: boolean;
  /** Owner-gated credit notes (fiscal:credit). Defaults to canWrite. */
  canManageSensitive?: boolean;
  businessTimezone?: string | null;
  onClose: () => void;
  t: (key: string, params?: Record<string, string | number>) => string;
  tWith?: (
    key: string,
    replacements: Record<string, string | number>,
  ) => string;
};

function DefinitionRow({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="grid grid-cols-[7.5rem_1fr] gap-x-3 gap-y-1 text-sm sm:grid-cols-[9rem_1fr]">
      <dt className="text-ink-500">{label}</dt>
      <dd className="min-w-0 text-ink-900">{children}</dd>
    </div>
  );
}

function statusTone(status: FiscalStatus | string): StatusTone {
  if (status === "authorized" || status === "credited") return "success";
  if (status === "pending") return "pending";
  if (status === "failed_retryable") return "warning";
  if (
    status === "failed_permanent" ||
    status === "rejected" ||
    status === "cancelled"
  ) {
    return "danger";
  }
  return "neutral";
}

function statusLabel(
  status: FiscalStatus | string,
  t: (key: string) => string,
): string {
  const key = `invoices.status.${status}`;
  const translated = t(key);
  return translated === key ? status : translated;
}

function deliveryTone(status: string): StatusTone {
  if (status === "succeeded") return "success";
  if (status === "dead") return "danger";
  if (status === "leased" || status === "pending") return "pending";
  return "neutral";
}

/** Map API channel names onto the three UI labels (artifact → pdf). */
function channelLabelKey(channel: string): string {
  const lower = channel.toLowerCase();
  if (lower === "artifact" || lower === "pdf") return "invoices.delivery.pdf";
  if (lower === "email") return "invoices.delivery.email";
  if (lower === "print") return "invoices.delivery.print";
  return channel;
}

/** Mask a customer document number, keeping the last 4 characters. */
function maskCustomerDoc(doc: string | null | undefined): string {
  if (!doc || !doc.trim()) return "—";
  const cleaned = doc.trim();
  if (cleaned.length <= 4) return cleaned;
  const visible = cleaned.slice(-4);
  const maskedLen = Math.min(cleaned.length - 4, 12);
  return `${"*".repeat(maskedLen)}${visible}`;
}

function formatCustomer(receipt: ReceiptRow): string {
  const type = receipt.customer_doc_type?.trim();
  const masked = maskCustomerDoc(receipt.customer_doc_number);
  if (masked === "—") return "—";
  return type ? `${type} ${masked}` : masked;
}

export default function ReceiptDetailDrawer({
  open,
  receipt,
  businessId,
  locale,
  currency,
  canWrite = true,
  canManageSensitive,
  businessTimezone = null,
  onClose,
  t,
  tWith,
}: ReceiptDetailDrawerProps) {
  const sensitive = canManageSensitive ?? canWrite;
  const receiptId = receipt?.id ?? 0;
  const numericBizId = Number.parseInt(String(businessId), 10);

  const resendMutation = useResendReceipt(businessId);
  const creditMutation = useCreditNote(businessId);
  const retryMutation = useRetryDelivery(businessId);

  const [confirmCredit, setConfirmCredit] = useState(false);
  const [creditReason, setCreditReason] = useState("");
  const [creditAmountDollars, setCreditAmountDollars] = useState("");
  const [retryingTaskId, setRetryingTaskId] = useState<number | null>(null);

  const interpolate = useCallback(
    (key: string, replacements: Record<string, string | number>) => {
      if (tWith) return tWith(key, replacements);
      let value = t(key);
      Object.entries(replacements).forEach(([name, replacement]) => {
        value = value.replace(
          new RegExp(`\\{${name}\\}`, "g"),
          String(replacement),
        );
      });
      return value;
    },
    [t, tWith],
  );

  // One listReceiptDelivery request per open (not per table row).
  const deliveryQuery = useQuery<FiscalDeliveryTask[]>({
    queryKey: ["accounting", businessId, "receipt-delivery", receiptId],
    queryFn: () => listReceiptDelivery(numericBizId, receiptId),
    enabled:
      open &&
      receiptId > 0 &&
      Number.isFinite(numericBizId) &&
      numericBizId > 0,
  });

  const deliveryTasks = deliveryQuery.data ?? [];
  const deliveryLoading = deliveryQuery.isLoading || deliveryQuery.isFetching;

  const handleRetry = useCallback(
    (taskId: number) => {
      setRetryingTaskId(taskId);
      retryMutation.mutate(taskId, {
        onSettled: () => {
          setRetryingTaskId(null);
          void deliveryQuery.refetch();
        },
      });
    },
    [retryMutation, deliveryQuery],
  );

  const handleResend = useCallback(() => {
    if (!receipt) return;
    resendMutation.mutate(receipt.id);
  }, [receipt, resendMutation]);

  const handleConfirmCredit = useCallback(() => {
    if (!receipt) return;
    const reason = creditReason.trim();
    if (!reason) return;
    const id = receipt.id;
    const maxDollars = (receipt.total_amount_cents || 0) / 100;
    let amountCents: number | undefined;
    if (creditAmountDollars.trim()) {
      const parsed = Number.parseFloat(creditAmountDollars.replace(",", "."));
      if (
        !Number.isFinite(parsed) ||
        parsed <= 0 ||
        parsed > maxDollars + 1e-9
      ) {
        return;
      }
      amountCents = Math.round(parsed * 100);
    }
    setConfirmCredit(false);
    setCreditReason("");
    setCreditAmountDollars("");
    creditMutation.mutate(
      { receiptId: id, reason, amountCents },
      { onSuccess: () => onClose() },
    );
  }, [receipt, creditMutation, onClose, creditReason, creditAmountDollars]);

  const canResend = canWrite && !!receipt && receipt.status === "authorized";
  const canCredit =
    sensitive &&
    !!receipt &&
    receipt.status === "authorized" &&
    receipt.action !== "credit_note";

  const isActionPending = resendMutation.isPending || creditMutation.isPending;

  const receiptCurrency = receipt?.currency || currency;
  const totalDollars = (receipt?.total_amount_cents || 0) / 100;
  const tipDollars = (receipt?.tip_amount_cents || 0) / 100;

  const canPdf = !!receipt?.pdf_path;

  const footer = useMemo(() => {
    if (!receipt) return null;
    if (!canResend && !canCredit && !canPdf) return null;
    return (
      <div className="flex w-full flex-wrap items-center justify-end gap-2">
        {canPdf ? (
          <>
            <button
              type="button"
              className={accountingSecondaryButtonClass}
              onClick={() => {
                if (receipt.pdf_path) {
                  window.open(receipt.pdf_path, "_blank", "noopener,noreferrer");
                }
              }}
              data-testid="receipt-detail-pdf"
            >
              {t("receiptDetail.actions.downloadPdf")}
            </button>
            <button
              type="button"
              className={accountingSecondaryButtonClass}
              onClick={() => {
                if (!receipt.pdf_path) return;
                const w = window.open(
                  receipt.pdf_path,
                  "_blank",
                  "noopener,noreferrer",
                );
                if (!w) return;
                window.setTimeout(() => {
                  try {
                    w.focus();
                    w.print();
                  } catch {
                    // Cross-origin PDF viewers may block print().
                  }
                }, 500);
              }}
              data-testid="receipt-detail-print"
            >
              {t("receiptDetail.actions.print")}
            </button>
          </>
        ) : null}
        {canResend ? (
          <button
            type="button"
            className={accountingSecondaryButtonClass}
            onClick={handleResend}
            disabled={isActionPending}
            data-testid="receipt-detail-resend"
          >
            {t("receiptDetail.actions.resend")}
          </button>
        ) : null}
        {canCredit ? (
          <button
            type="button"
            className="inline-flex h-9 items-center gap-1.5 rounded-xl border border-rose-200 bg-white px-3 text-sm font-medium text-rose-700 shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:border-rose-300 hover:bg-rose-50 disabled:cursor-not-allowed disabled:opacity-50"
            onClick={() => setConfirmCredit(true)}
            disabled={isActionPending}
            data-testid="receipt-detail-credit"
          >
            {t("receiptDetail.actions.creditNote")}
          </button>
        ) : null}
      </div>
    );
  }, [receipt, canResend, canCredit, canPdf, handleResend, isActionPending, t]);

  const subtitle = receipt
    ? receipt.receipt_number || t("invoices.unnumbered")
    : undefined;

  return (
    <>
      <DetailDrawer
        open={open && !!receipt}
        onClose={onClose}
        title={t("receiptDetail.title")}
        subtitle={subtitle}
        size="md"
        footer={footer}
      >
        {receipt ? (
          <div className="space-y-5" data-testid="receipt-detail-drawer">
            {/* Totals hero */}
            <div className="rounded-2xl border border-warm-200/80 bg-white px-4 py-4 shadow-sm shadow-warm-900/5">
              <p className="text-xs font-medium uppercase tracking-wide text-ink-500">
                {t("receiptDetail.totals")}
              </p>
              <AnimatedNumberText
                value={totalDollars}
                format={(v) => formatMoney(v, receiptCurrency, locale)}
                className="mt-1 block font-title text-display-md tabular-nums text-ink-900"
              />
              {tipDollars > 0 ? (
                <p className="mt-1 text-sm text-ink-500">
                  {t("receiptDetail.tip")}:{" "}
                  <span className="tabular-nums text-ink-700">
                    {formatMoney(tipDollars, receiptCurrency, locale)}
                  </span>
                </p>
              ) : null}
              <div className="mt-2">
                <StatusBadge
                  tone={statusTone(receipt.status)}
                  label={statusLabel(receipt.status, t)}
                  size="sm"
                />
              </div>
            </div>

            {/* Definition list */}
            <dl className="space-y-3 rounded-2xl border border-warm-200/80 bg-white px-4 py-4 shadow-sm shadow-warm-900/5">
              <DefinitionRow label={t("receiptDetail.bill")}>
                #{receipt.bill_id}
              </DefinitionRow>
              <DefinitionRow label={t("receiptDetail.receiptNumber")}>
                {receipt.receipt_number || t("invoices.unnumbered")}
              </DefinitionRow>
              {receipt.auth_code ? (
                <DefinitionRow label={t("receiptDetail.authCode")}>
                  <span className="font-mono text-sm tracking-wide">
                    {receipt.auth_code}
                  </span>
                </DefinitionRow>
              ) : null}
              <DefinitionRow label={t("receiptDetail.issuedAt")}>
                {receipt.issued_at
                  ? formatBusinessDateTime(
                      receipt.issued_at,
                      locale,
                      businessTimezone,
                      DATE_TIME_SHORT,
                    )
                  : "—"}
              </DefinitionRow>
              <DefinitionRow label={t("receiptDetail.customer")}>
                {formatCustomer(receipt)}
              </DefinitionRow>
            </dl>

            {/* Error when failed */}
            {receipt.error_message ? (
              <div
                role="alert"
                data-testid="receipt-detail-error"
                className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800"
              >
                <p className="font-medium">{t("receiptDetail.error")}</p>
                <p className="mt-0.5">{receipt.error_message}</p>
              </div>
            ) : null}

            {/* Delivery channels */}
            <section
              aria-label={t("receiptDetail.delivery.title")}
              data-testid="receipt-detail-delivery"
            >
              <h3 className="mb-2 text-xs font-semibold uppercase tracking-[0.16em] text-ink-500">
                {t("receiptDetail.delivery.title")}
              </h3>
              {deliveryLoading && deliveryTasks.length === 0 ? (
                <div
                  data-testid="receipt-detail-delivery-loading"
                  className="space-y-2 animate-pulse"
                >
                  <div className="h-10 rounded-xl bg-warm-100" />
                  <div className="h-10 rounded-xl bg-warm-100" />
                </div>
              ) : deliveryTasks.length > 0 ? (
                <ul className="space-y-2">
                  {deliveryTasks.map((task) => {
                    const channelKey = channelLabelKey(task.channel);
                    const channelName = t(channelKey);
                    const canRetryChannel = canWrite && task.status === "dead";
                    const isRetrying =
                      retryingTaskId === task.id ||
                      (retryMutation.isPending && retryingTaskId === task.id);
                    return (
                      <li
                        key={task.id}
                        data-testid={`delivery-task-${task.id}`}
                        data-delivery-channel={task.channel}
                        className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-warm-200/80 bg-white px-3 py-2.5 shadow-sm shadow-warm-900/5"
                      >
                        <div className="min-w-0 space-y-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="text-sm font-medium text-ink-900">
                              {channelName === channelKey
                                ? task.channel
                                : channelName}
                            </span>
                            <StatusBadge
                              tone={deliveryTone(task.status)}
                              label={task.status}
                              size="sm"
                            />
                          </div>
                          {task.masked_recipient ? (
                            <p className="text-xs text-ink-500">
                              {task.masked_recipient}
                            </p>
                          ) : null}
                          {task.last_error_category ? (
                            <p className="text-xs text-rose-700">
                              {task.last_error_category}
                            </p>
                          ) : null}
                        </div>
                        {canRetryChannel ? (
                          <button
                            type="button"
                            className={accountingPrimaryButtonClass}
                            disabled={retryMutation.isPending}
                            aria-busy={isRetrying}
                            data-testid={`delivery-retry-${task.id}`}
                            onClick={() => handleRetry(task.id)}
                          >
                            {isRetrying
                              ? t("receiptDetail.delivery.retrying")
                              : t("receiptDetail.delivery.retry")}
                          </button>
                        ) : null}
                      </li>
                    );
                  })}
                </ul>
              ) : (
                // Fallback: show embedded badges from the list row while empty.
                <ul className="space-y-2">
                  {(receipt.delivery ?? []).map((badge) => {
                    const channelKey = channelLabelKey(badge.channel);
                    const channelName = t(channelKey);
                    return (
                      <li
                        key={badge.task_id}
                        data-testid={`delivery-badge-${badge.task_id}`}
                        className="flex items-center justify-between gap-2 rounded-xl border border-warm-200/80 bg-white px-3 py-2.5"
                      >
                        <span className="text-sm font-medium text-ink-900">
                          {channelName === channelKey
                            ? badge.channel
                            : channelName}
                        </span>
                        <StatusBadge
                          tone={deliveryTone(badge.status)}
                          label={badge.status}
                          size="sm"
                        />
                      </li>
                    );
                  })}
                </ul>
              )}
            </section>
          </div>
        ) : null}
      </DetailDrawer>

      <Modal
        isOpen={confirmCredit && !!receipt}
        onOpenChange={(open) => {
          if (!open) {
            setConfirmCredit(false);
            setCreditReason("");
            setCreditAmountDollars("");
          }
        }}
        size="md"
      >
        <ModalContent>
          {(onClose) => {
            const letter =
              (receipt?.country || "").toUpperCase() === "AR"
                ? receiptLetter(receipt?.receipt_type)
                : "";
            const maxDollars = (receipt?.total_amount_cents || 0) / 100;
            const reasonOk = creditReason.trim().length > 0;
            return (
              <>
                <ModalHeader>
                  {t("receiptDetail.confirmCreditTitle")}
                </ModalHeader>
                {/* ModalBody already stacks with its own flex gap; space-y
                    margins would clip the NextUI outside labels below. */}
                <ModalBody>
                  <p className="text-sm text-ink-700">
                    {receipt
                      ? interpolate("receiptDetail.confirmCreditBody", {
                          bill: receipt.bill_id,
                        })
                      : ""}
                  </p>
                  {letter ? (
                    <p
                      className="rounded-lg border border-warm-200 bg-warm-50 px-3 py-2 text-sm text-ink-800"
                      data-testid="credit-nc-letter"
                    >
                      {t("receiptDetail.ncLetter", { letter })}
                    </p>
                  ) : null}
                  <Input
                    type="text"
                    label={t("receiptDetail.creditReason")}
                    labelPlacement="outside"
                    placeholder=" "
                    value={creditReason}
                    onValueChange={setCreditReason}
                    isRequired
                    variant="bordered"
                    data-testid="credit-reason"
                  />
                  <Input
                    type="text"
                    inputMode="decimal"
                    label={t("receiptDetail.creditAmount")}
                    labelPlacement="outside"
                    placeholder=" "
                    description={t("receiptDetail.creditAmountHint", {
                      max: formatMoney(
                        maxDollars,
                        receipt?.currency || currency,
                        locale,
                      ),
                    })}
                    value={creditAmountDollars}
                    onValueChange={setCreditAmountDollars}
                    variant="bordered"
                    data-testid="credit-amount"
                  />
                </ModalBody>
                <ModalFooter>
                  <Button variant="light" onPress={onClose}>
                    {t("actions.cancel")}
                  </Button>
                  <Button
                    color="danger"
                    isDisabled={!reasonOk || creditMutation.isPending}
                    isLoading={creditMutation.isPending}
                    onPress={handleConfirmCredit}
                    data-testid="credit-confirm"
                  >
                    {t("receiptDetail.actions.creditNote")}
                  </Button>
                </ModalFooter>
              </>
            );
          }}
        </ModalContent>
      </Modal>
    </>
  );
}

/** Map AFIP receipt_type to letter A/B/C for NC preview (NC matches original). */
function receiptLetter(receiptType: string | undefined): string {
  if (!receiptType) return "";
  const rt = receiptType.toLowerCase();
  // Only Argentine letter docs — do not treat US "invoice"/"receipt" as A/B/C.
  if (
    rt.includes("factura_a") ||
    rt.includes("nota_de_credito_a") ||
    rt.includes("nota_credito_a")
  ) {
    return "A";
  }
  if (
    rt.includes("factura_b") ||
    rt.includes("nota_de_credito_b") ||
    rt.includes("nota_credito_b")
  ) {
    return "B";
  }
  if (
    rt.includes("factura_c") ||
    rt.includes("nota_de_credito_c") ||
    rt.includes("nota_credito_c")
  ) {
    return "C";
  }
  return "";
}
