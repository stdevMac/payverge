"use client";

import React, { useEffect, useState } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Textarea,
  Chip,
  RadioGroup,
  Radio,
} from "@nextui-org/react";
import toast from "react-hot-toast";
import { Ban, Undo2, History } from "lucide-react";
import {
  AlternativeBillPayment,
  BillAuditEntry,
  BillWithItemsResponse,
  Payment,
  getBillAudit,
  refundBillPayment,
  voidBill,
} from "@/api/bills";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { useWithManagerPin } from "@/components/business/managerPin/ManagerPinProvider";
import { CryptoRefundPanel } from "@/components/business/CryptoRefundPanel";
import { randomUUID } from "@/lib/randomUUID";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { formatBusinessDateTime } from "@/utils/businessTime";
import { getApiErrorCode, getSafeApiErrorMessage } from "@/utils/apiError";

// IMP-15: void + refund actions live in a single component embedded inside
// BillDetailsModal. Two distinct dialogs (void / refund) share a single
// reason-text + manager-PIN + Idempotency-Key contract. The audit section
// reads `/inside/bills/:bill_id/audit` (comp_void_audit rows the manager-PIN
// middleware persisted in IMP-14).

interface BillVoidRefundActionsProps {
  bill: BillWithItemsResponse;
  /** Re-fetch the parent bill after a successful action. */
  onRefresh: () => void;
  /** Close the parent BillDetailsModal after a successful void/refund. */
  onCloseParent: () => void;
  /** Called after a successful void (before the modal closes) so the parent
   * list can navigate somewhere the now-voided bill stays visible. */
  onVoided?: () => void;
  /** Currency code (USD / AED / …) — used to format amounts in the picker. */
  currency: string;
  /** Business IANA timezone (e.g. "America/Argentina/Buenos_Aires"); null/invalid
   * falls back to UTC so audit timestamps render in business wall-clock, not the
   * operator device's local time. */
  businessTimezone?: string | null;
}

const REASON_MIN_LENGTH = 3;

/**
 * Void rejections that carry a stable backend code get operator-tier copy the
 * same way floorErrorMessage does for Live View. Without this branch
 * getSafeApiErrorMessage surfaces the handler's raw English at a Spanish
 * manager — the leak b4f3c2f36 already fixed on the Liberar door (#704).
 */
function voidErrorMessage(err: unknown, t: (key: string) => string): string {
  if (getApiErrorCode(err) === "bill_void_kitchen_tickets_live") {
    return t("voidRefund.errors.kitchenTicketsLive");
  }
  return getSafeApiErrorMessage(err, t("voidRefund.errors.voidFailed"));
}

const formatPaymentMethod = (
  payment: Pick<Payment, "payment_method"> | Pick<AlternativeBillPayment, "payment_method">,
  t: (key: string) => string,
): string => {
  const method = (payment.payment_method ?? "").trim();
  if (!method) return t("voidRefund.paymentMethods.unknown");
  if (method === "crypto") return t("voidRefund.paymentMethods.crypto");
  if (method === "cross-chain") return t("voidRefund.paymentMethods.crossChain");
  if (method === "plugin") return t("voidRefund.paymentMethods.plugin");
  return method.charAt(0).toUpperCase() + method.slice(1);
};

type RefundableTender =
  | { kind: "payment"; id: number; payment: Payment }
  | { kind: "alternative"; id: number; payment: AlternativeBillPayment };

const tenderRadioValue = (tender: RefundableTender): string =>
  `${tender.kind}:${tender.id}`;

export const BillVoidRefundActions: React.FC<BillVoidRefundActionsProps> = ({
  bill,
  onRefresh,
  onCloseParent,
  onVoided,
  currency,
  businessTimezone = null,
}) => {
  const { withManagerPin } = useWithManagerPin();
  const { locale } = useSimpleLocale();

  // Operator-tier translation helper (en/es). Strings live under
  // billManager.voidRefund.* alongside the rest of the bill UI.
  const t = React.useCallback(
    (key: string): string => {
      const result = getTranslation(`billManager.${key}`, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  // Operator-locale-aware number grouping (Audit A-04).
  const formatCurrency = (amount: number) =>
    formatCurrencyIntl(amount, currency, undefined, locale);

  const [voidOpen, setVoidOpen] = useState(false);
  const [voidReason, setVoidReason] = useState("");
  const [voidSubmitting, setVoidSubmitting] = useState(false);

  const [refundOpen, setRefundOpen] = useState(false);
  const [refundReason, setRefundReason] = useState("");
  const [refundPaymentId, setRefundPaymentId] = useState<string>("");
  const [refundSubmitting, setRefundSubmitting] = useState(false);

  const [auditLoading, setAuditLoading] = useState(false);
  const [auditEntries, setAuditEntries] = useState<BillAuditEntry[]>([]);

  const billRow = bill.bill;
  const billStatus = billRow.status;
  const refundablePayments: Payment[] = (billRow.payments ?? []).filter(
    (p) => p.status === "confirmed",
  );
  const refundableAlternativePayments: AlternativeBillPayment[] = (
    billRow.alternative_payments ?? []
  ).filter((p) => p.status === "confirmed");
  const refundableTenders: RefundableTender[] = [
    ...refundablePayments.map((payment) => ({
      kind: "payment" as const,
      id: payment.id,
      payment,
    })),
    ...refundableAlternativePayments.map((payment) => ({
      kind: "alternative" as const,
      id: payment.id,
      payment,
    })),
  ];

  // Voiding makes sense before any money has landed. Once paid/closed/voided
  // the operator must refund individual payments instead.
  const canVoid = billStatus === "open" && billRow.paid_amount === 0;
  const canRefund = refundableTenders.length > 0;

  const loadAudit = React.useCallback(async () => {
    setAuditLoading(true);
    try {
      const data = await getBillAudit(billRow.id);
      setAuditEntries(data.entries ?? []);
    } catch (error) {
      // Non-fatal — the operator can still void/refund without the log.
      console.error("Failed to load bill audit log:", error);
    } finally {
      setAuditLoading(false);
    }
  }, [billRow.id]);

  useEffect(() => {
    void loadAudit();
  }, [loadAudit]);

  const openVoid = () => {
    setVoidReason("");
    setVoidOpen(true);
  };

  const openRefund = () => {
    setRefundReason("");
    setRefundPaymentId(
      refundableTenders[0] ? tenderRadioValue(refundableTenders[0]) : "",
    );
    setRefundOpen(true);
  };

  const handleVoidSubmit = async () => {
    const reason = voidReason.trim();
    if (reason.length < REASON_MIN_LENGTH) {
      toast.error(t("voidRefund.errors.reasonTooShort"));
      return;
    }
    const idempotencyKey = randomUUID();
    setVoidSubmitting(true);
    try {
      await withManagerPin(
        (pin) =>
          voidBill(
            billRow.id,
            { reason },
            { pin, idempotencyKey },
          ),
        {
          title: t("voidRefund.pin.confirmVoidTitle"),
          description: t("voidRefund.pin.voidDescription").replace(
            "{billNumber}",
            billRow.bill_number,
          ),
        },
      );
      toast.success(t("voidRefund.toasts.voided"));
      setVoidOpen(false);
      onRefresh();
      void loadAudit();
      // Land the operator on a list that still shows the voided bill before
      // the modal unmounts (the bill just left the Active tab).
      onVoided?.();
      onCloseParent();
    } catch (error) {
      const message = error instanceof Error ? error.message : "";
      if (message === "PIN entry cancelled") {
        return;
      }
      toast.error(voidErrorMessage(error, t));
    } finally {
      setVoidSubmitting(false);
    }
  };

  const handleRefundSubmit = async () => {
    const reason = refundReason.trim();
    if (reason.length < REASON_MIN_LENGTH) {
      toast.error(t("voidRefund.errors.reasonTooShort"));
      return;
    }
    const [refundKind, refundID] = refundPaymentId.split(":");
    const numericPaymentId = Number(refundID);
    if (!Number.isFinite(numericPaymentId) || numericPaymentId <= 0) {
      toast.error(t("voidRefund.errors.selectPayment"));
      return;
    }
    if (refundKind !== "payment" && refundKind !== "alternative") {
      toast.error(t("voidRefund.errors.selectPayment"));
      return;
    }
    const idempotencyKey = randomUUID();
    setRefundSubmitting(true);
    try {
      await withManagerPin(
        (pin) =>
          refundBillPayment(
            billRow.id,
            refundKind === "alternative"
              ? { alternative_payment_id: numericPaymentId, reason }
              : { payment_id: numericPaymentId, reason },
            { pin, idempotencyKey },
          ),
        {
          title: t("voidRefund.pin.confirmRefundTitle"),
          description: t("voidRefund.pin.refundDescription").replace(
            "{billNumber}",
            billRow.bill_number,
          ),
        },
      );
      toast.success(t("voidRefund.toasts.refunded"));
      setRefundOpen(false);
      onRefresh();
      void loadAudit();
    } catch (error) {
      const message = error instanceof Error ? error.message : "";
      if (message === "PIN entry cancelled") {
        return;
      }
      toast.error(
        getSafeApiErrorMessage(error, t("voidRefund.errors.refundFailed")),
      );
    } finally {
      setRefundSubmitting(false);
    }
  };

  const renderAuditAmount = (entry: BillAuditEntry) => {
    if (entry.amount_cents === undefined || entry.amount_cents === null) {
      return null;
    }
    return (
      <span className="text-xs font-medium text-ink-700">
        {formatCurrency(entry.amount_cents / 100)}
      </span>
    );
  };

  const renderAuditActionLabel = (entry: BillAuditEntry) => {
    const action = entry.action || "";
    const color =
      action === "refund" ? "warning" : action === "void" ? "danger" : "default";
    const label =
      action === "refund"
        ? t("voidRefund.audit.refund")
        : action === "void"
          ? t("voidRefund.audit.void")
          : action || t("voidRefund.audit.action");
    return (
      <Chip size="sm" variant="flat" color={color}>
        {label}
      </Chip>
    );
  };

  const cryptoPayments = refundablePayments.filter(
    (p) =>
      p.payment_method === "crypto" || p.payment_method === "cross-chain",
  );

  return (
    <>
      {/* Action buttons live inline above the modal footer. Hidden when the
          bill is in a state that disallows the action (clearer than greyed-
          out buttons that confuse operators on busy services). */}
      {(canVoid || canRefund) && (
        <div className="flex flex-wrap gap-2">
          {canVoid && (
            <Button
              size="sm"
              variant="flat"
              startContent={<Ban className="w-4 h-4" />}
              onPress={openVoid}
              className="bg-rose-50 font-semibold text-rose-700 hover:bg-rose-100"
            >
              {t("voidRefund.actions.voidBill")}
            </Button>
          )}
          {canRefund && (
            <Button
              size="sm"
              variant="flat"
              startContent={<Undo2 className="w-4 h-4" />}
              onPress={openRefund}
              className="bg-amber-50 font-semibold text-amber-700 hover:bg-amber-100"
            >
              {t("voidRefund.actions.refundPayment")}
            </Button>
          )}
        </div>
      )}

      {/* Wave 4: noncustodial on-chain crypto refund lifecycle (honest states). */}
      {cryptoPayments.map((payment) => (
        <CryptoRefundPanel
          key={`crypto-refund-${payment.id}`}
          businessId={billRow.business_id}
          billId={billRow.id}
          payment={payment}
          onRefresh={onRefresh}
        />
      ))}

      {/* Audit log surface — IMP-15 acceptance criterion. Only renders when
          we have rows so the modal stays clean for fresh bills. */}
      {(auditEntries.length > 0 || auditLoading) && (
        <div className="space-y-3 rounded-2xl border border-warm-200 bg-white p-4 shadow-sm shadow-warm-900/5">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 bg-amber-50 rounded-xl flex items-center justify-center">
              <History className="w-5 h-5 text-amber-700" />
            </div>
            <div>
              <h3 className="text-sm font-semibold uppercase tracking-wider text-ink-950">
                {t("voidRefund.audit.title")}
              </h3>
              <p className="text-sm text-ink-600">
                {t("voidRefund.audit.subtitle")}
              </p>
            </div>
          </div>

          {auditLoading ? (
            <p className="text-sm text-ink-500">{t("voidRefund.audit.loading")}</p>
          ) : auditEntries.length === 0 ? (
            <p className="text-sm text-ink-500">{t("voidRefund.audit.empty")}</p>
          ) : (
            <div className="space-y-2">
              {auditEntries.map((entry) => (
                <div
                  key={entry.id}
                  className="flex items-start justify-between gap-3 rounded-2xl border border-warm-200/80 bg-warm-50/50 px-3 py-2"
                >
                  <div className="min-w-0 space-y-1">
                    <div className="flex items-center gap-2">
                      {renderAuditActionLabel(entry)}
                      <span className="text-xs uppercase tracking-wide text-ink-500">
                        {entry.target_type}
                      </span>
                      {!entry.pin_present && (
                        <Chip size="sm" variant="flat" color="default">
                          {t("voidRefund.audit.noPin")}
                        </Chip>
                      )}
                    </div>
                    {entry.reason && (
                      <p className="text-sm text-ink-600">{entry.reason}</p>
                    )}
                  </div>
                  <div className="flex flex-col items-end gap-1 text-right">
                    {renderAuditAmount(entry)}
                    <span className="whitespace-nowrap text-xs text-ink-500">
                      {formatBusinessDateTime(
                        entry.created_at,
                        locale,
                        businessTimezone,
                      )}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Void confirmation modal */}
      <Modal
        isOpen={voidOpen}
        onClose={() => setVoidOpen(false)}
        size="md"
      >
        <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
          <ModalHeader className="border-b border-warm-200/80 bg-warm-50/70 px-6 py-5 text-lg font-semibold text-ink-950">
            {t("voidRefund.voidModal.title").replace(
              "{billNumber}",
              billRow.bill_number,
            )}
          </ModalHeader>
          <ModalBody className="space-y-3 px-6 py-5">
            <p className="text-sm text-ink-600">
              {t("voidRefund.voidModal.warning")}
            </p>
            <Textarea
              label={t("voidRefund.reasonLabel")}
              placeholder={t("voidRefund.voidModal.reasonPlaceholder")}
              value={voidReason}
              onValueChange={setVoidReason}
              minRows={3}
              isRequired
            />
            <p className="text-xs text-ink-500">
              {t("voidRefund.minChars")}
            </p>
          </ModalBody>
          <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
            <Button
              variant="light"
              onPress={() => setVoidOpen(false)}
              isDisabled={voidSubmitting}
            >
              {t("voidRefund.cancel")}
            </Button>
            <Button
              onPress={handleVoidSubmit}
              isLoading={voidSubmitting}
              isDisabled={voidReason.trim().length < REASON_MIN_LENGTH}
              className="bg-rose-600 font-semibold text-white shadow-sm shadow-rose-200 hover:bg-rose-700"
            >
              {t("voidRefund.actions.voidBill")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Refund confirmation modal — pick a payment row + reason */}
      <Modal
        isOpen={refundOpen}
        onClose={() => setRefundOpen(false)}
        size="md"
      >
        <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
          <ModalHeader className="border-b border-warm-200/80 bg-warm-50/70 px-6 py-5 text-lg font-semibold text-ink-950">{t("voidRefund.actions.refundPayment")}</ModalHeader>
          <ModalBody className="space-y-4 px-6 py-5">
            <p className="text-sm text-ink-600">
              {t("voidRefund.refundModal.explanation")}
            </p>

            {refundableTenders.length === 0 ? (
              <p className="text-sm text-ink-500">
                {t("voidRefund.refundModal.noPayments")}
              </p>
            ) : (
              <RadioGroup
                label={t("voidRefund.refundModal.choosePayment")}
                value={refundPaymentId}
                onValueChange={setRefundPaymentId}
              >
                {refundableTenders.map((tender) => {
                  const payment = tender.payment;
                  // Crypto Payment tips arrive as DOLLARS (tip_amount); alt
                  // tenders arrive as CENTS (tip_amount_cents, models_json.go).
                  // Surfacing the alt tip here makes the picker show the true
                  // cash returned, matching what the backend reverses (R3-BP-5,
                  // ties to R3-BP-1).
                  const tipAmount =
                    tender.kind === "payment"
                      ? tender.payment.tip_amount ?? 0
                      : (tender.payment.tip_amount_cents ?? 0) / 100;
                  const totalDollars = payment.amount + tipAmount;
                  return (
                    <Radio key={tenderRadioValue(tender)} value={tenderRadioValue(tender)}>
                      <div className="flex items-center gap-3">
                        <span className="font-medium">
                          {formatPaymentMethod(payment, t)}
                        </span>
                        <span className="text-sm text-ink-600">
                          {formatCurrency(totalDollars)}
                        </span>
                        {tipAmount > 0 && (
                          <span className="text-xs text-ink-500">
                            {t("voidRefund.refundModal.inclTip").replace(
                              "{amount}",
                              formatCurrency(tipAmount),
                            )}
                          </span>
                        )}
                      </div>
                    </Radio>
                  );
                })}
              </RadioGroup>
            )}

            <Textarea
              label={t("voidRefund.reasonLabel")}
              placeholder={t("voidRefund.refundModal.reasonPlaceholder")}
              value={refundReason}
              onValueChange={setRefundReason}
              minRows={3}
              isRequired
            />
            <p className="text-xs text-ink-500">
              {t("voidRefund.minChars")}
            </p>
          </ModalBody>
          <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
            <Button
              variant="light"
              onPress={() => setRefundOpen(false)}
              isDisabled={refundSubmitting}
            >
              {t("voidRefund.cancel")}
            </Button>
            <Button
              onPress={handleRefundSubmit}
              isLoading={refundSubmitting}
              isDisabled={
                !refundPaymentId ||
                refundReason.trim().length < REASON_MIN_LENGTH
              }
              className="bg-amber-500 font-semibold text-white shadow-sm shadow-amber-200 hover:bg-amber-600"
            >
              {t("voidRefund.actions.refundPayment")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </>
  );
};
