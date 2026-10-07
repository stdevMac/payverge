"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { Button, Input, Select, SelectItem, Spinner } from "@nextui-org/react";
import { Search } from "lucide-react";
import type { Locale } from "@/i18n/localeRegistry";
import type { IssuableBill, IssueReceiverOverride } from "@/api/fiscal";
import { resolveReceiptType } from "@/api/fiscal";
import {
  useIssuableBills,
  useIssueInvoice,
} from "@/hooks/accounting/useAccountingQueries";
import { useDebouncedValue } from "@/components/admin/primitives";
import { formatMoney } from "@/components/business/accounting/format";
import {
  DATE_TIME_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";
import { operatorBillDisplayNumber } from "@/lib/operatorBillNumber";
import { getApiErrorCode, getApiErrorStatus } from "@/utils/apiError";
import DetailDrawer from "../shared/DetailDrawer";
import {
  accountingPrimaryButtonClass,
  accountingSecondaryButtonClass,
} from "./accountingShared";

export type IssueInvoiceDrawerProps = {
  open: boolean;
  onClose: () => void;
  businessId: string;
  locale: Locale;
  currency: string;
  businessTimezone?: string | null;
  t: (key: string, params?: Record<string, string | number>) => string;
  /** Optional success side-effect (toast / parent banner). */
  onSuccess?: () => void;
  /** Open an already-issued receipt from the picker (dinner-service retrieve path). */
  onViewExisting?: (info: {
    receiptId: number;
    bill: IssuableBill;
  }) => void;
};

const SEARCH_DEBOUNCE_MS = 300;

const DOC_TYPES = [
  { key: "CF", labelKey: "issueInvoice.receiver.docType.cf" },
  { key: "DNI", labelKey: "issueInvoice.receiver.docType.dni" },
  { key: "CUIT", labelKey: "issueInvoice.receiver.docType.cuit" },
  { key: "CUIL", labelKey: "issueInvoice.receiver.docType.cuil" },
] as const;

const TAX_CONDITIONS = [
  {
    key: "consumidor_final",
    labelKey: "issueInvoice.receiver.condition.consumidor_final",
  },
  {
    key: "responsable_inscripto",
    labelKey: "issueInvoice.receiver.condition.responsable_inscripto",
  },
  {
    key: "monotributo",
    labelKey: "issueInvoice.receiver.condition.monotributo",
  },
  { key: "exento", labelKey: "issueInvoice.receiver.condition.exento" },
] as const;

// Backend code (server.ErrCodeFiscalReceiptAlreadyIssued): the bill already has a
// live fiscal receipt. The picker marks those rows and swaps Issue for "View
// invoice", but the manual bill-ID field below bypasses the picker entirely.
const FISCAL_RECEIPT_ALREADY_ISSUED = "fiscal_receipt_already_issued";

function mutationErrorMessage(err: unknown, fallback: string): string {
  if (err && typeof err === "object") {
    const maybeAxios = err as {
      response?: { data?: { error?: string; message?: string } };
      message?: string;
    };
    const data = maybeAxios.response?.data;
    if (data?.error && typeof data.error === "string") return data.error;
    if (data?.message && typeof data.message === "string") return data.message;
    if (typeof maybeAxios.message === "string" && maybeAxios.message) {
      return maybeAxios.message;
    }
  }
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}

function parsePositiveBillId(value: string): number | null {
  const trimmed = value.trim();
  if (!/^[1-9]\d*$/.test(trimmed)) return null;
  const parsed = Number(trimmed);
  return Number.isSafeInteger(parsed) ? parsed : null;
}

function defaultsFromBill(bill: IssuableBill | null): {
  docType: string;
  docNumber: string;
  taxCondition: string;
  name: string;
} {
  if (!bill) {
    return {
      docType: "CF",
      docNumber: "",
      taxCondition: "consumidor_final",
      name: "",
    };
  }
  const rawType = (bill.customer_doc_type || "").toUpperCase();
  const docType =
    rawType === "DNI" || rawType === "CUIT" || rawType === "CUIL"
      ? rawType
      : "CF";
  return {
    docType,
    docNumber: bill.customer_doc_number || "",
    taxCondition: bill.customer_tax_condition || "consumidor_final",
    name: bill.customer_name || "",
  };
}

export default function IssueInvoiceDrawer({
  open,
  onClose,
  businessId,
  locale,
  currency,
  businessTimezone = null,
  t,
  onSuccess,
  onViewExisting,
}: IssueInvoiceDrawerProps) {
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search, SEARCH_DEBOUNCE_MS);
  const [issuedIds, setIssuedIds] = useState<Set<number>>(() => new Set());
  const [issuingBillId, setIssuingBillId] = useState<number | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const [showIssueById, setShowIssueById] = useState(false);
  const [manualBillId, setManualBillId] = useState("");

  // Receiver step: selected bill + overrides.
  const [selectedBill, setSelectedBill] = useState<IssuableBill | null>(null);
  const [manualMode, setManualMode] = useState(false);
  const [docType, setDocType] = useState("CF");
  const [docNumber, setDocNumber] = useState("");
  const [taxCondition, setTaxCondition] = useState("consumidor_final");
  const [customerName, setCustomerName] = useState("");
  const [typePreview, setTypePreview] = useState<{
    receiptType: string;
    letter: string;
  } | null>(null);
  const [letterLoading, setLetterLoading] = useState(false);

  const issueMutation = useIssueInvoice(businessId);
  const { data, isLoading, isFetching, isError, refetch } = useIssuableBills(
    businessId,
    open ? debouncedSearch.trim() : "",
    { enabled: open },
  );

  // Reset local UI state whenever the drawer opens.
  useEffect(() => {
    if (!open) return;
    setSearch("");
    setIssuedIds(new Set());
    setIssuingBillId(null);
    setSuccessMessage(null);
    setErrorMessage(null);
    setShowIssueById(false);
    setManualBillId("");
    setSelectedBill(null);
    setManualMode(false);
    setDocType("CF");
    setDocNumber("");
    setTaxCondition("consumidor_final");
    setCustomerName("");
    setTypePreview(null);
  }, [open]);

  // Auto-dismiss success banner.
  useEffect(() => {
    if (!successMessage) return;
    const timer = window.setTimeout(() => setSuccessMessage(null), 4000);
    return () => window.clearTimeout(timer);
  }, [successMessage]);

  const bills = useMemo(() => {
    const items = data ?? [];
    if (issuedIds.size === 0) return items;
    return items.filter((b) => !issuedIds.has(b.bill_id));
  }, [data, issuedIds]);

  // Live type preview via server ResolveIssuableReceiptTypeForCountry.
  useEffect(() => {
    if (!open || (!selectedBill && !manualMode)) {
      setTypePreview(null);
      return;
    }
    let cancelled = false;
    const timer = window.setTimeout(() => {
      setLetterLoading(true);
      const apiDocType = docType === "CF" ? "" : docType;
      void resolveReceiptType(businessId, {
        doc_type: apiDocType,
        doc_number: docNumber,
        tax_condition: taxCondition,
      })
        .then((res) => {
          if (!cancelled) {
            setTypePreview({
              receiptType: res.receipt_type,
              letter: res.letter || "",
            });
          }
        })
        .catch(() => {
          if (!cancelled) setTypePreview(null);
        })
        .finally(() => {
          if (!cancelled) setLetterLoading(false);
        });
    }, 250);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [
    open,
    selectedBill,
    manualMode,
    docType,
    docNumber,
    taxCondition,
    businessId,
  ]);

  const typePreviewLabel = useMemo(() => {
    if (!typePreview) return null;
    if (typePreview.letter) {
      return t("issueInvoice.letterWillIssue", {
        letter: typePreview.letter,
      });
    }
    const key = `invoices.receiptType.${typePreview.receiptType}`;
    const translated = t(key);
    const typeLabel =
      translated === key
        ? typePreview.receiptType.replace(/_/g, " ")
        : translated;
    return t("issueInvoice.typeWillIssue", { type: typeLabel });
  }, [typePreview, t]);

  const beginIssue = useCallback(
    (bill: IssuableBill | null, isManual: boolean) => {
      setErrorMessage(null);
      setSelectedBill(bill);
      setManualMode(isManual);
      const d = defaultsFromBill(bill);
      setDocType(d.docType);
      setDocNumber(d.docNumber);
      setTaxCondition(d.taxCondition);
      setCustomerName(d.name);
    },
    [],
  );

  const buildReceiver = useCallback((): IssueReceiverOverride => {
    const apiDocType = docType === "CF" ? "CF" : docType;
    return {
      customer_doc_type: apiDocType,
      customer_doc_number: docType === "CF" ? "" : docNumber.trim(),
      customer_tax_condition: taxCondition,
      customer_name: customerName.trim(),
    };
  }, [docType, docNumber, taxCondition, customerName]);

  const handleConfirmIssue = useCallback(() => {
    const billId = selectedBill?.bill_id ?? parsePositiveBillId(manualBillId);
    if (!billId || issuingBillId != null) return;
    setErrorMessage(null);
    setIssuingBillId(billId);
    issueMutation.mutate(
      { billId, receiver: buildReceiver() },
      {
        onSuccess: () => {
          setIssuedIds((prev) => {
            const next = new Set(prev);
            next.add(billId);
            return next;
          });
          setSuccessMessage(t("issueInvoice.success"));
          setIssuingBillId(null);
          setManualBillId("");
          setSelectedBill(null);
          setManualMode(false);
          // R2-5: the receiver fields feed `dirty`, so leaving them populated
          // made a just-issued invoice look like unsaved work and raised a
          // false discard confirm on close. Reset to the same baseline the
          // open-effect uses.
          setDocType("CF");
          setDocNumber("");
          setTaxCondition("consumidor_final");
          setCustomerName("");
          setTypePreview(null);
          onSuccess?.();
        },
        onError: (err) => {
          if (
            getApiErrorCode(err) === FISCAL_RECEIPT_ALREADY_ISSUED ||
            getApiErrorStatus(err) === 409
          ) {
            // #907: this used to come back 202 {"ok":true} and the drawer showed
            // "Invoice issue queued" for a bill that already had a factura.
            setErrorMessage(t("issueInvoice.alreadyIssuedError"));
          } else {
            setErrorMessage(mutationErrorMessage(err, t("issueInvoice.error")));
          }
          setIssuingBillId(null);
        },
      },
    );
  }, [
    selectedBill,
    manualBillId,
    issuingBillId,
    issueMutation,
    buildReceiver,
    onSuccess,
    t,
  ]);

  const parsedManualId = parsePositiveBillId(manualBillId);
  const canIssueManual = parsedManualId != null && issuingBillId == null;
  const showReceiverStep = selectedBill != null || manualMode;
  const showLoading = isLoading || (isFetching && bills.length === 0);

  // L6-18: receiver step / manual path counts as in-progress (dirty).
  const dirty =
    selectedBill != null ||
    manualMode ||
    manualBillId.trim() !== "" ||
    docNumber.trim() !== "" ||
    customerName.trim() !== "";

  return (
    <DetailDrawer
      open={open}
      onClose={onClose}
      title={t("issueInvoice.title")}
      size="md"
      dirty={dirty}
      discardConfirm={{
        title: t("discardConfirm.issueInvoice.title"),
        description: t("discardConfirm.issueInvoice.description"),
        confirmLabel: t("discardConfirm.confirm"),
        cancelLabel: t("discardConfirm.cancel"),
      }}
    >
      <div className="space-y-4" data-testid="issue-invoice-content">
        {successMessage ? (
          <div
            role="status"
            className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800"
          >
            {successMessage}
          </div>
        ) : null}

        {errorMessage ? (
          <div
            role="alert"
            className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700"
            data-testid="issue-invoice-error"
          >
            {errorMessage}
          </div>
        ) : null}

        {showReceiverStep ? (
          <div
            // flex gap, not space-y: sibling margin-top would override the
            // NextUI outside-label headroom and clip the labels.
            className="flex flex-col gap-3"
            data-testid="issue-receiver-form"
          >
            <div className="rounded-xl border border-brand/20 bg-brand/5 px-3 py-2 text-sm text-ink-800">
              <span className="font-medium">
                {t("issueInvoice.typePreview")}:{" "}
              </span>
              {letterLoading ? (
                <Spinner size="sm" />
              ) : (
                <span data-testid="letter-preview">
                  {typePreviewLabel || "—"}
                </span>
              )}
            </div>

            <p className="text-xs font-semibold uppercase tracking-wide text-ink-500">
              {t("issueInvoice.receiver.title")}
            </p>

            <Select
              label={t("issueInvoice.receiver.docTypeLabel")}
              labelPlacement="outside"
              placeholder=" "
              selectedKeys={new Set([docType])}
              onSelectionChange={(keys) => {
                const v = Array.from(keys)[0];
                if (typeof v === "string") setDocType(v);
              }}
              variant="bordered"
              classNames={{
                trigger: "h-10 min-h-10 border-warm-200 bg-white",
                label:
                  "text-xs font-semibold uppercase tracking-wide text-ink-500",
              }}
            >
              {DOC_TYPES.map((d) => (
                <SelectItem key={d.key}>{t(d.labelKey)}</SelectItem>
              ))}
            </Select>

            {docType !== "CF" ? (
              <Input
                type="text"
                label={t("issueInvoice.receiver.docNumberLabel")}
                labelPlacement="outside"
                placeholder=" "
                value={docNumber}
                onValueChange={setDocNumber}
                variant="bordered"
                classNames={{
                  inputWrapper:
                    "h-10 min-h-10 border-warm-200 bg-white data-[hover=true]:border-brand/40",
                  label:
                    "text-xs font-semibold uppercase tracking-wide text-ink-500",
                  input: "text-sm",
                }}
              />
            ) : null}

            <Input
              type="text"
              label={t("issueInvoice.receiver.nameLabel")}
              labelPlacement="outside"
              placeholder=" "
              value={customerName}
              onValueChange={setCustomerName}
              variant="bordered"
              classNames={{
                inputWrapper:
                  "h-10 min-h-10 border-warm-200 bg-white data-[hover=true]:border-brand/40",
                label:
                  "text-xs font-semibold uppercase tracking-wide text-ink-500",
                input: "text-sm",
              }}
            />

            <Select
              label={t("issueInvoice.receiver.conditionLabel")}
              labelPlacement="outside"
              placeholder=" "
              selectedKeys={new Set([taxCondition])}
              onSelectionChange={(keys) => {
                const v = Array.from(keys)[0];
                if (typeof v === "string") setTaxCondition(v);
              }}
              variant="bordered"
              classNames={{
                trigger: "h-10 min-h-10 border-warm-200 bg-white",
                label:
                  "text-xs font-semibold uppercase tracking-wide text-ink-500",
              }}
            >
              {TAX_CONDITIONS.map((c) => (
                <SelectItem key={c.key}>{t(c.labelKey)}</SelectItem>
              ))}
            </Select>

            <div className="flex flex-wrap gap-2 pt-1">
              <Button
                type="button"
                className={accountingPrimaryButtonClass}
                isLoading={issuingBillId != null}
                isDisabled={issuingBillId != null}
                onPress={handleConfirmIssue}
                data-testid="issue-confirm"
              >
                {issuingBillId != null
                  ? t("issueInvoice.issuing")
                  : t("issueInvoice.confirmIssue")}
              </Button>
              <button
                type="button"
                className={accountingSecondaryButtonClass}
                onClick={() => {
                  setSelectedBill(null);
                  setManualMode(false);
                }}
              >
                {t("actions.back")}
              </button>
            </div>
          </div>
        ) : (
          <>
            <Input
              type="search"
              value={search}
              onValueChange={setSearch}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("issueInvoice.searchPlaceholder")}
              aria-label={t("issueInvoice.searchPlaceholder")}
              startContent={
                <Search className="h-4 w-4 text-ink-400" aria-hidden />
              }
              variant="bordered"
              classNames={{
                inputWrapper:
                  "h-10 min-h-10 border-warm-200 bg-white data-[hover=true]:border-brand/40",
                input: "text-sm",
              }}
            />

            <div className="space-y-2" data-testid="issuable-bills-list">
              {showLoading ? (
                <div
                  className="flex items-center justify-center py-10"
                  data-testid="issuable-bills-loading"
                >
                  <Spinner size="sm" color="primary" />
                </div>
              ) : isError ? (
                <div
                  className="rounded-xl border border-dashed border-rose-200 bg-rose-50/60 px-4 py-8 text-center text-sm text-rose-700"
                  data-testid="issuable-bills-error"
                >
                  <p>{t("issueInvoice.loadError")}</p>
                  <button
                    type="button"
                    className="mt-3 text-sm font-medium text-brand-700 underline-offset-2 hover:underline"
                    onClick={() => {
                      void refetch();
                    }}
                  >
                    {t("issueInvoice.retryLoad")}
                  </button>
                </div>
              ) : bills.length === 0 ? (
                <div
                  className="rounded-xl border border-dashed border-warm-200 bg-warm-50/60 px-4 py-8 text-center text-sm text-ink-500"
                  data-testid="issuable-bills-empty"
                >
                  <p>{t("issueInvoice.empty")}</p>
                  <p className="mt-2 text-xs text-ink-400">
                    {t("issueInvoice.emptyHint")}
                  </p>
                </div>
              ) : (
                <ul className="divide-y divide-warm-100 overflow-hidden rounded-xl border border-warm-200/90 bg-white">
                  {bills.map((row) => (
                    <IssuableBillRow
                      key={row.bill_id}
                      bill={row}
                      locale={locale}
                      currency={currency}
                      businessTimezone={businessTimezone}
                      issuing={issuingBillId === row.bill_id}
                      issueDisabled={issuingBillId != null}
                      onIssue={() => beginIssue(row, false)}
                      onViewExisting={
                        onViewExisting
                          ? (receiptId) => {
                              onViewExisting({ receiptId, bill: row });
                              onClose();
                            }
                          : undefined
                      }
                      t={t}
                    />
                  ))}
                </ul>
              )}
            </div>

            <div className="border-t border-warm-200/80 pt-3">
              {!showIssueById ? (
                <button
                  type="button"
                  className="text-sm font-medium text-brand-700 underline-offset-2 hover:underline"
                  onClick={() => setShowIssueById(true)}
                >
                  {t("issueInvoice.issueById")}
                </button>
              ) : (
                <form
                  className="space-y-2"
                  onSubmit={(e) => {
                    e.preventDefault();
                    if (parsedManualId == null) return;
                    beginIssue(null, true);
                    setManualMode(true);
                  }}
                  data-testid="issue-by-id-form"
                >
                  <p className="text-xs text-ink-500">
                    {t("issueInvoice.issueByIdHint")}
                  </p>
                  <div className="flex flex-wrap items-end gap-2">
                    <Input
                      type="text"
                      inputMode="numeric"
                      pattern="[0-9]*"
                      label={t("issueInvoice.billIdLabel")}
                      aria-label={t("issueInvoice.billIdLabel")}
                      placeholder={t("issueInvoice.billIdPlaceholder")}
                      value={manualBillId}
                      onValueChange={setManualBillId}
                      onChange={(e) => setManualBillId(e.target.value)}
                      variant="bordered"
                      labelPlacement="outside"
                      className="min-w-[10rem] flex-1"
                      classNames={{
                        inputWrapper:
                          "h-10 min-h-10 border-warm-200 bg-white data-[hover=true]:border-brand/40",
                        label:
                          "text-xs font-semibold uppercase tracking-wide text-ink-500",
                        input: "text-sm",
                      }}
                    />
                    <Button
                      type="submit"
                      className={accountingPrimaryButtonClass}
                      isDisabled={!canIssueManual}
                    >
                      {t("issueInvoice.continue")}
                    </Button>
                    <button
                      type="button"
                      className={accountingSecondaryButtonClass}
                      onClick={() => {
                        setShowIssueById(false);
                        setManualBillId("");
                      }}
                    >
                      {t("actions.cancel")}
                    </button>
                  </div>
                </form>
              )}
            </div>
          </>
        )}
      </div>
    </DetailDrawer>
  );
}

function IssuableBillRow({
  bill,
  locale,
  currency,
  businessTimezone,
  issuing,
  issueDisabled,
  onIssue,
  onViewExisting,
  t,
}: {
  bill: IssuableBill;
  locale: Locale;
  currency: string;
  businessTimezone: string | null;
  issuing: boolean;
  issueDisabled: boolean;
  onIssue: () => void;
  onViewExisting?: (receiptId: number) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const amount = Number(bill.total_amount) || 0;
  const closedLabel = bill.closed_at
    ? formatBusinessDateTime(bill.closed_at, locale, businessTimezone, DATE_TIME_SHORT)
    : "—";
  const existingId =
    typeof bill.existing_receipt_id === "number" && bill.existing_receipt_id > 0
      ? bill.existing_receipt_id
      : null;

  return (
    <li
      data-testid={`issuable-bill-${bill.bill_id}`}
      className="flex flex-wrap items-center justify-between gap-3 px-3 py-3 sm:px-4"
    >
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
          <span className="font-semibold text-ink-950">
            {operatorBillDisplayNumber({
              id: bill.bill_id,
              bill_number: bill.bill_number,
            })}
          </span>
          {bill.table_label ? (
            <span className="text-sm text-ink-600">{bill.table_label}</span>
          ) : null}
          {bill.customer_name ? (
            <span className="text-sm text-ink-600">{bill.customer_name}</span>
          ) : null}
        </div>
        <div className="mt-0.5 flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-ink-500">
          <span>
            <span className="sr-only">{t("issueInvoice.closedAt")}: </span>
            {closedLabel}
          </span>
          <span className="tabular-nums font-medium text-ink-800">
            <span className="sr-only">{t("issueInvoice.amount")}: </span>
            {formatMoney(amount, bill.currency || currency, locale)}
          </span>
          {existingId != null ? (
            <span className="font-medium text-brand-700">
              {t("issueInvoice.alreadyIssued")}
            </span>
          ) : null}
        </div>
      </div>
      {existingId != null && onViewExisting ? (
        <Button
          type="button"
          size="sm"
          className={accountingSecondaryButtonClass}
          onPress={() => onViewExisting(existingId)}
          data-testid={`view-existing-invoice-${bill.bill_id}`}
          aria-label={`${t("issueInvoice.viewInvoice")} ${operatorBillDisplayNumber({
            id: bill.bill_id,
            bill_number: bill.bill_number,
          })}`}
        >
          {t("issueInvoice.viewInvoice")}
        </Button>
      ) : (
        <Button
          type="button"
          size="sm"
          className={accountingPrimaryButtonClass}
          isDisabled={issueDisabled && !issuing}
          isLoading={issuing}
          onPress={onIssue}
          aria-label={`${t("issueInvoice.issue")} ${operatorBillDisplayNumber({
            id: bill.bill_id,
            bill_number: bill.bill_number,
          })}`}
        >
          {issuing ? t("issueInvoice.issuing") : t("issueInvoice.issue")}
        </Button>
      )}
    </li>
  );
}
