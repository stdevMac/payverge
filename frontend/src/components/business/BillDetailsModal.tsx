"use client";

import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Chip,
  Divider,
} from "@nextui-org/react";
import { DollarSign, ChefHat, Plus, History } from "lucide-react";
import toast from "react-hot-toast";
import {
  BillHistoryEvent,
  BillItem,
  BillWithItemsResponse,
  isActiveBillStatus,
  parseBillItems,
  setBillFiscalCustomer,
} from "../../api/bills";
import {
  FiscalIdentityFields,
  fiscalIdentityToPayload,
  type FiscalIdentityValue,
  type FiscalDocTypeChoice,
  type FiscalTaxConditionChoice,
} from "../common/FiscalIdentityFields";
import { shouldShowFiscalIdentityFields } from "@/lib/fiscalIdentityAvailability";
import {
  SplittingAPI,
  type SplitShare,
  type SplitState,
} from "@/api/splitting";
import { formatCurrency as formatCurrencyIntl } from "../../api/currency";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { useSSEEvents, type SSEEvent } from "@/hooks/useSSEEvents";
import { BillItemEditor } from "./BillItemEditor";
import { createBillPrintJob } from "@/api/print";
import { BillVoidRefundActions } from "./BillVoidRefundActions";
import { BillRecordPayment } from "./BillRecordPayment";
import { OperatorBillNumber } from "./OperatorBillNumber";
import { StatusChip } from "../ui/StatusChip";
import {
  DATE_SHORT,
  DATE_TIME_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";
import { getBillLocationLabel } from "@/lib/tableLabel";
import { hasPerm } from "@/constants/permissions";
import { useStaffPermissionsContext } from "@/contexts/StaffPermissionsContext";
import { useAuth } from "@/providers/HybridAuthProvider";
import { formatPreparedUnits } from "@/utils/formatPreparedUnits";
import {
  buildBundleGroupKeys,
  getBillItemType as getSharedBillItemType,
  isDiscountBillLine,
  isOperatorFoodBillLine,
  resolveBundleGroupKey,
} from "@/lib/billBundleGrouping";
import { displayBillHistoryActor } from "@/lib/billHistoryActor";

interface BillDetailsModalProps {
  isOpen: boolean;
  onClose: () => void;
  bill: BillWithItemsResponse | null;
  onCloseBill: (billId: number) => void;
  onBillUpdated: () => void;
  businessId: number;
  tString: (key: string) => string;
  /** Business default currency for line items, totals, payments shown in
   * this modal. Was hardcoded to "$"; AED business saw mismatched
   * currency between the bills list and the bill detail modal.
   */
  currency?: string;
  /** When false, payment section is read-only (host/kitchen). */
  canRecordPayment?: boolean;
  businessTimezone?: string | null;
  /** Wave 4: pre-open the record-payment modal (BillCreator success step). */
  autoOpenRecordPayment?: boolean;
  /** Called after a successful void so the parent can land the operator on a
   * list that still shows the (now voided) bill. */
  onBillVoided?: () => void;
  /** Re-fetch this bill with its FULL history (the modal loads only the newest
   * N events by default). Enables the "show older" affordance. */
  onLoadFullHistory?: (billId: number) => void | Promise<void>;
  /**
   * L6-3 / L6-23: operator review mode (PaymentHistory, Outstanding, LiveBills).
   * Uses operator-voiced fiscal copy and gates fiscal write on open bills only.
   */
  mode?: "default" | "operator";
  /** Business address ISO country. AFIP identity form is hidden unless AR. */
  country?: string | null;
  /** Fiscal-settings country when the client already loaded settings. */
  fiscalCountry?: string | null;
}

interface BillSplitOperatorSummaryProps {
  billId: number;
  businessId: number;
  billNumber: string;
  billToken?: string;
  currency: string;
  locale: string;
  tString: (key: string) => string;
}

const getBillItemType = getSharedBillItemType;

/** Footer action row — stacked on narrow drawers so labels never clip. */
export const BILL_DETAILS_FOOTER_ACTIONS_CLASS =
  "flex w-full min-w-0 flex-col-reverse items-stretch gap-2 sm:flex-row sm:flex-wrap sm:items-center sm:justify-end";

const BILL_DETAILS_FOOTER_BUTTON_CLASS =
  "h-auto min-h-10 w-full shrink-0 justify-center overflow-visible whitespace-nowrap rounded-xl sm:w-auto";

function parseBillItemSnapshot(raw: unknown): BillItem[] | null {
  const parsed = parseBillItems(raw);
  return parsed.length > 0 ? parsed : null;
}

function BillSplitOperatorSummary({
  billId,
  businessId,
  billNumber,
  billToken,
  currency,
  locale,
  tString,
}: BillSplitOperatorSummaryProps) {
  const [splitState, setSplitState] = useState<SplitState | null>(null);
  const mountedRef = useRef(false);
  const splitStateRequestRef = useRef(0);
  const normalizedBillToken = billToken?.trim() ?? "";

  const refreshSplitState = useCallback(() => {
    const requestId = splitStateRequestRef.current + 1;
    splitStateRequestRef.current = requestId;
    if (!normalizedBillToken) {
      if (mountedRef.current) setSplitState(null);
      return Promise.resolve();
    }
    return SplittingAPI.getSplitState(normalizedBillToken)
      .then((state) => {
        if (mountedRef.current && requestId === splitStateRequestRef.current) {
          setSplitState(state);
        }
      })
      .catch(() => {
        if (mountedRef.current && requestId === splitStateRequestRef.current) {
          setSplitState(null);
        }
      });
  }, [normalizedBillToken]);

  useEffect(() => {
    mountedRef.current = true;
    setSplitState(null);
    void refreshSplitState();

    return () => {
      mountedRef.current = false;
      splitStateRequestRef.current += 1;
    };
  }, [billNumber, refreshSplitState]);

  const handleSSEEvent = useCallback(
    (event: SSEEvent) => {
      if (event.type !== "bill.split.updated") return;
      const state = event.data as Partial<SplitState>;
      if (state.bill_number !== billNumber) return;
      // The operator business stream carries a slimmed split frame (counts +
      // status, no per-share amounts/tenders — those are financial:read). Refetch
      // the full split state over REST rather than rendering money from the frame.
      void refreshSplitState();
    },
    [billNumber, refreshSplitState],
  );

  useSSEEvents({
    businessId,
    enabled: Boolean(businessId && billNumber && normalizedBillToken),
    onEvent: handleSSEEvent,
    onReconnect: () => {
      void refreshSplitState();
    },
  });

  if (!splitState || splitState.shares.length === 0) {
    return null;
  }

  const formatCurrency = (amount: number) =>
    formatCurrencyIntl(amount, currency, undefined, locale);
  const total = Math.max(splitState.total_amount, 0);
  const paidPercent =
    total > 0
      ? Math.min(100, Math.max(0, (splitState.paid_amount / total) * 100))
      : 0;
  const heldPercent =
    total > 0
      ? Math.min(
          Math.max(100 - paidPercent, 0),
          Math.max(0, (splitState.held_amount / total) * 100),
        )
      : 0;

  const statusClass = (status: SplitShare["status"]) => {
    switch (status) {
      case "settled":
        return "border-emerald-200 bg-emerald-50 text-emerald-700";
      case "held":
        return "border-amber-200 bg-amber-50 text-amber-800";
      case "failed":
        return "border-rose-200 bg-rose-50 text-rose-700";
      default:
        return "border-warm-200 bg-warm-50 text-warm-700";
    }
  };

  const shareName = (share: SplitShare) =>
    share.display_name?.trim() ||
    tString("splitSummary.guestFallback").replace("{id}", String(share.id));

  return (
    <section
      aria-label={tString("splitSummary.title")}
      className="rounded-2xl border border-warm-200/80 bg-white/90 p-4 shadow-sm shadow-warm-900/5"
    >
      <div className="mb-3 flex items-start justify-between gap-4">
        <div>
          <h3 className="text-sm font-semibold uppercase tracking-wider text-ink-950">
            {tString("splitSummary.title")}
          </h3>
          <p className="mt-1 text-sm text-ink-600">
            {tString("splitSummary.subtitle")}
          </p>
        </div>
        <div className="flex flex-shrink-0 items-center gap-2">
          <span className="rounded-full border border-brand/20 bg-brand/10 px-3 py-1 text-xs font-semibold text-brand-dark">
            {splitState.shares.length} {tString("splitSummary.shares")}
          </span>
          <a
            href={`/business/${businessId}/bills/${billId}/alternative-payments`}
            className="rounded-full border border-warm-300 px-3 py-1 text-xs font-semibold text-ink-800 transition-colors hover:border-brand hover:text-brand"
          >
            {tString("splitSummary.managePayments")}
          </a>
        </div>
      </div>

      <div className="mb-4 h-2 overflow-hidden rounded-full bg-warm-100">
        <div className="flex h-full w-full">
          <div
            className="h-full bg-emerald-500"
            style={{ width: `${paidPercent}%` }}
          />
          <div
            className="h-full bg-amber-400"
            style={{ width: `${heldPercent}%` }}
          />
        </div>
      </div>

      <dl className="grid grid-cols-1 gap-2 sm:grid-cols-3">
        <div className="rounded-xl bg-warm-50 px-3 py-2">
          <dt className="text-xs font-medium text-warm-700">
            {tString("splitSummary.paidOfTotal")}
          </dt>
          <dd className="mt-1 font-mono text-sm font-semibold tabular-nums text-ink-950">
            {formatCurrency(splitState.paid_amount)} /{" "}
            {formatCurrency(splitState.total_amount)}
          </dd>
        </div>
        <div className="rounded-xl bg-amber-50 px-3 py-2">
          <dt className="text-xs font-medium text-amber-800">
            {tString("splitSummary.held")}
          </dt>
          <dd className="mt-1 font-mono text-sm font-semibold tabular-nums text-amber-900">
            {formatCurrency(splitState.held_amount)}
          </dd>
        </div>
        <div className="rounded-xl bg-emerald-50 px-3 py-2">
          <dt className="text-xs font-medium text-emerald-800">
            {tString("splitSummary.available")}
          </dt>
          <dd className="mt-1 font-mono text-sm font-semibold tabular-nums text-emerald-900">
            {formatCurrency(splitState.available_amount)}
          </dd>
        </div>
      </dl>

      <ul className="mt-4 divide-y divide-warm-200 rounded-2xl border border-warm-200 bg-white/80">
        {splitState.shares.map((share) => (
          <li
            key={share.id}
            className="flex items-center justify-between gap-3 px-3 py-2"
          >
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-ink-950">
                {shareName(share)}
              </p>
              <p className="mt-0.5 text-xs text-warm-600">
                {tString(`splitSummary.mode.${share.mode}`)}
                {share.tender ? ` · ${share.tender}` : ""}
              </p>
            </div>
            <div className="flex flex-shrink-0 items-center gap-2">
              <span
                className={`rounded-full border px-2 py-0.5 text-xs font-semibold ${statusClass(share.status)}`}
              >
                {tString(`splitSummary.status.${share.status}`)}
              </span>
              <span className="font-mono text-sm font-semibold tabular-nums text-ink-950">
                {formatCurrency(share.amount)}
              </span>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}

export const BillDetailsModal: React.FC<BillDetailsModalProps> = ({
  isOpen,
  onClose,
  bill,
  onCloseBill,
  onBillUpdated,
  businessId,
  tString,
  currency = "USD",
  canRecordPayment = true,
  businessTimezone = null,
  autoOpenRecordPayment = false,
  onBillVoided,
  onLoadFullHistory,
  mode = "default",
  country,
  fiscalCountry,
}) => {
  const showFiscalIdentity = shouldShowFiscalIdentityFields({
    country,
    fiscalCountry,
  });
  const [isEditing, setIsEditing] = useState(false);
  const [printing, setPrinting] = useState(false);
  const [loadingFullHistory, setLoadingFullHistory] = useState(false);
  const { locale } = useSimpleLocale();
  const { isStaffUser } = useAuth();
  const { permissions: staffPermissions } = useStaffPermissionsContext();
  const canPrint = !isStaffUser || hasPerm(staffPermissions, "print:bill");

  // Fiscal customer identity (AFIP/ARCA). Seeded from the bill and saved
  // explicitly via the dedicated endpoint so the fiscal worker emits the
  // correct factura. Optional — empty clears to NULL.
  const billDocType = (bill?.bill.fiscal_customer_doc_type ??
    "") as FiscalDocTypeChoice;
  const billTaxCondition = (bill?.bill.fiscal_customer_tax_condition ??
    "") as FiscalTaxConditionChoice;
  const [fiscalIdentity, setFiscalIdentity] = useState<FiscalIdentityValue>({
    docType: billDocType,
    docNumber: bill?.bill.fiscal_customer_doc_number ?? "",
    taxCondition: billTaxCondition,
    name: bill?.bill.fiscal_customer_name ?? "",
    email: bill?.bill.fiscal_customer_email ?? "",
  });
  const [fiscalValid, setFiscalValid] = useState(true);
  const [savingFiscal, setSavingFiscal] = useState(false);
  const billId = bill?.bill.id;
  const billHasFiscalIdentity = Boolean(
    bill?.bill.fiscal_customer_doc_number ||
    bill?.bill.fiscal_customer_tax_condition ||
    bill?.bill.fiscal_customer_name ||
    bill?.bill.fiscal_customer_email,
  );

  // Re-seed when a different bill is opened.
  useEffect(() => {
    setFiscalIdentity({
      docType: (bill?.bill.fiscal_customer_doc_type ??
        "") as FiscalDocTypeChoice,
      docNumber: bill?.bill.fiscal_customer_doc_number ?? "",
      taxCondition: (bill?.bill.fiscal_customer_tax_condition ??
        "") as FiscalTaxConditionChoice,
      name: bill?.bill.fiscal_customer_name ?? "",
      email: bill?.bill.fiscal_customer_email ?? "",
    });
    setFiscalValid(true);
  }, [
    billId,
    bill?.bill.fiscal_customer_doc_type,
    bill?.bill.fiscal_customer_doc_number,
    bill?.bill.fiscal_customer_tax_condition,
    bill?.bill.fiscal_customer_name,
    bill?.bill.fiscal_customer_email,
  ]);

  const handleSaveFiscalIdentity = useCallback(async () => {
    if (billId == null || !fiscalValid) return;
    setSavingFiscal(true);
    try {
      await setBillFiscalCustomer(
        billId,
        fiscalIdentityToPayload(fiscalIdentity),
      );
      toast.success(tString("fiscalCustomer.saved"));
      onBillUpdated();
    } catch {
      // Avoid surfacing the doc number (PII) in error output.
      toast.error(tString("fiscalCustomer.saveError"));
    } finally {
      setSavingFiscal(false);
    }
  }, [billId, fiscalValid, fiscalIdentity, tString, onBillUpdated]);

  // Operator mode swaps guest-voiced fiscal toggle/hint for operator copy.
  // Must stay above the early return (rules of hooks).
  const fiscalT = useCallback(
    (key: string) => {
      if (mode === "operator") {
        if (key === "fiscalCustomer.toggle") {
          return tString("fiscalCustomer.operatorToggle");
        }
        if (key === "fiscalCustomer.hint") {
          return tString("fiscalCustomer.operatorHint");
        }
      }
      return tString(key);
    },
    [mode, tString],
  );

  async function handlePrintBill() {
    if (!bill) return;
    setPrinting(true);
    try {
      const job = await createBillPrintJob(businessId, bill.bill.id, locale);
      if (job.payload_html) {
        toast.success(tString("print.queued"));
      } else {
        toast.error(tString("print.noPrinter"));
      }
    } catch (e) {
      toast.error(
        tString("print.failed").replace("{message}", (e as Error).message),
      );
    } finally {
      setPrinting(false);
    }
  }

  useEffect(() => {
    if (!isOpen) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      setIsEditing(false);
      onClose();
    };
    window.addEventListener("keydown", onKeyDown, true);
    return () => window.removeEventListener("keydown", onKeyDown, true);
  }, [isOpen, onClose]);

  if (!bill) return null;

  const formatCurrency = (amount: number) =>
    formatCurrencyIntl(amount, currency, undefined, locale);
  const formatDate = (dateString: string) =>
    formatBusinessDateTime(dateString, locale, businessTimezone, DATE_SHORT);
  const formatDateTime = (dateString: string) =>
    formatBusinessDateTime(
      dateString,
      locale,
      businessTimezone,
      DATE_TIME_SHORT,
    );

  const getItemType = getBillItemType;

  const getItemTypeBadge = (item: BillItem) => {
    const itemType = getItemType(item);
    if (itemType === "bundle")
      return { label: tString("itemTypes.bundle"), color: "primary" as const };
    if (itemType === "bundle_item")
      return {
        label: tString("itemTypes.bundleItem"),
        color: "secondary" as const,
      };
    if (itemType === "discount")
      return {
        label: tString("itemTypes.discount"),
        color: "success" as const,
      };
    return { label: tString("itemTypes.item"), color: "default" as const };
  };

  const billItems = bill.items || [];
  // Operator-facing header count = sellable LINES (menu + bundle parents),
  // not summed quantities. A Tacos 1x + Iced Tea 2x check is 2 items.
  const historyEntries = bill.history || [];
  // When the server bounded the history list, history_total exceeds what we
  // hold — offer a "show older" affordance instead of loading everything.
  const historyTotal = bill.history_total ?? historyEntries.length;
  const hasOlderHistory = historyTotal > historyEntries.length;
  const discountItems = billItems.filter(isDiscountBillLine);
  // Discounts render in their own green band below the items (discountItems);
  // exclude them from the main list too (only bundle_item was excluded before)
  // so a discount isn't shown twice. Matches the guest bill, which renders an
  // applied-discount once. Bill arithmetic is unaffected (subtotal already nets
  // the discount once server-side).
  const visibleItems = billItems.filter(isOperatorFoodBillLine);
  const groupingSource = parseBillItemSnapshot(bill.bill.items) || billItems;
  const bundleGroupKeys = buildBundleGroupKeys(groupingSource);
  const getBundleGroupKey = (item: BillItem) =>
    resolveBundleGroupKey(item, bundleGroupKeys);
  const bundleChildrenByParent = billItems
    .filter((item: BillItem) => getItemType(item) === "bundle_item")
    .reduce((acc: Record<string, BillItem[]>, item: BillItem) => {
      const key = getBundleGroupKey(item);
      if (!key) return acc;
      if (!acc[key]) acc[key] = [];
      acc[key].push(item);
      return acc;
    }, {});

  const isEditableBill = bill.bill.status === "open";
  // L6-3: fiscal write only while the bill is open. Operator mode (closed-bill
  // drilldowns) still shows identity read-only when present.
  const canWriteFiscal = isEditableBill;
  const isPayableBill = isActiveBillStatus(bill.bill.status);
  const remainingAmount = bill.bill.total_amount - bill.bill.paid_amount;

  const getNumericDetail = (entry: BillHistoryEvent, key: string) => {
    const value = entry.details?.[key];
    if (typeof value === "number") return value;
    if (typeof value === "string") {
      const parsed = Number(value);
      return Number.isFinite(parsed) ? parsed : null;
    }
    return null;
  };

  const getHistoryCopy = (entry: BillHistoryEvent) => {
    const withReason = (body: string) =>
      entry.reason?.trim()
        ? `${body} ${tString("history.reasonLabel").replace(
            "{reason}",
            entry.reason.trim(),
          )}`
        : body;

    switch (entry.event_type) {
      case "bill.created":
        return {
          title: tString("history.events.billCreated"),
          body: tString("history.events.billCreatedDescription")
            .replace(
              "{count}",
              String(getNumericDetail(entry, "items_count") ?? 0),
            )
            .replace(
              "{total}",
              formatCurrency(getNumericDetail(entry, "total_amount") ?? 0),
            ),
        };
      case "bill.updated":
        return {
          title: tString("history.events.billUpdated"),
          body: tString("history.events.billUpdatedDescription")
            .replace(
              "{before}",
              String(getNumericDetail(entry, "items_before") ?? 0),
            )
            .replace(
              "{after}",
              String(getNumericDetail(entry, "items_after") ?? 0),
            ),
        };
      case "bill.closed":
        return {
          title: tString("history.events.billClosed"),
          body: tString("history.events.billClosedDescription").replace(
            "{total}",
            formatCurrency(
              getNumericDetail(entry, "total_amount") ?? bill.bill.total_amount,
            ),
          ),
        };
      case "bill_item.added":
        return {
          title: tString("history.events.itemAdded").replace(
            "{item}",
            entry.item_name || tString("unknownItem"),
          ),
          body: tString("history.events.itemAddedDescription")
            .replace(
              "{quantity}",
              String(getNumericDetail(entry, "quantity") ?? 0),
            )
            .replace(
              "{subtotal}",
              formatCurrency(getNumericDetail(entry, "subtotal") ?? 0),
            ),
        };
      case "bill_item.removed":
        return {
          title: tString("history.events.itemRemoved").replace(
            "{item}",
            entry.item_name || tString("unknownItem"),
          ),
          body: tString("history.events.itemRemovedDescription")
            .replace(
              "{quantity}",
              String(getNumericDetail(entry, "quantity") ?? 0),
            )
            .replace(
              "{subtotal}",
              formatCurrency(getNumericDetail(entry, "subtotal") ?? 0),
            ),
        };
      case "bill_item.voided":
        return {
          title: tString("history.events.itemVoided").replace(
            "{item}",
            entry.item_name || tString("unknownItem"),
          ),
          body: withReason(
            tString("history.events.itemVoidedDescription")
              .replace(
                "{quantity}",
                String(getNumericDetail(entry, "quantity") ?? 0),
              )
              .replace(
                "{subtotal}",
                formatCurrency(getNumericDetail(entry, "subtotal") ?? 0),
              ),
          ),
        };
      case "bill_item.quantity_updated":
        return {
          title: tString("history.events.itemQuantityUpdated").replace(
            "{item}",
            entry.item_name || tString("unknownItem"),
          ),
          body: tString("history.events.itemQuantityUpdatedDescription")
            .replace(
              "{before}",
              String(getNumericDetail(entry, "quantity_before") ?? 0),
            )
            .replace(
              "{after}",
              String(getNumericDetail(entry, "quantity_after") ?? 0),
            ),
        };
      case "order.approved":
        return {
          title: tString("history.events.orderApproved").replace(
            "{orderNumber}",
            entry.order_number || "",
          ),
          // Guest-checkout orders are billed atomically at checkout, so the
          // backend logs items_added: 0 + already_billed on approval.
          body:
            entry.details?.already_billed === true
              ? tString("history.events.orderApprovedAlreadyBilledDescription")
              : tString("history.events.orderApprovedDescription").replace(
                  "{count}",
                  String(getNumericDetail(entry, "items_added") ?? 0),
                ),
        };
      case "order.cancelled":
        return {
          title: tString("history.events.orderCancelled").replace(
            "{orderNumber}",
            entry.order_number || "",
          ),
          body:
            entry.reason?.trim() ||
            tString("history.events.orderCancelledDescription"),
        };
      default:
        return {
          title: entry.event_type,
          body: "",
        };
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onOpenChange={(open) => {
        if (!open) {
          setIsEditing(false);
          onClose();
        }
      }}
      onClose={() => {
        setIsEditing(false);
        onClose();
      }}
      isDismissable
      isKeyboardDismissDisabled={false}
      size={isEditing ? "5xl" : "2xl"}
      scrollBehavior="inside"
      classNames={{
        base: "max-h-[90vh] rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15",
        body: "py-6",
      }}
    >
      <ModalContent>
        {/* Header area — bill number, status, metadata */}
        <ModalHeader className="flex flex-col gap-1 pb-4 border-b border-warm-200 bg-warm-50/70">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-brand/10 rounded-2xl flex items-center justify-center border border-brand/20 shadow-sm shadow-brand/10">
                <DollarSign className="w-5 h-5 text-brand" />
              </div>
              <div>
                <div className="flex items-center gap-2">
                  <h2 className="text-lg font-semibold text-ink-950">
                    <OperatorBillNumber
                      id={bill.bill.id}
                      bill_number={bill.bill.bill_number}
                      label={tString("billNumber")}
                      copyLabel={tString("copyBillNumber")}
                      copiedLabel={tString("copiedBillNumber")}
                    />
                  </h2>
                  <StatusChip
                    kind="bill"
                    status={bill.bill.status}
                    labelOverride={tString(`billStatuses.${bill.bill.status}`)}
                  />
                </div>
                <div className="flex items-center gap-3 text-sm text-warm-700 mt-0.5">
                  <span>
                    {getBillLocationLabel(bill.bill, {
                      table: tString("table"),
                      counter: tString("counter"),
                      delivery: tString("delivery"),
                    })}
                  </span>
                  <span>&bull;</span>
                  <span>{formatDate(bill.bill.created_at)}</span>
                </div>
              </div>
            </div>
          </div>
          {bill.bill.notes && (
            <div className="mt-3 bg-brand/10 border border-brand/20 rounded-2xl p-3 shadow-sm shadow-brand/10">
              <p className="text-sm text-brand-dark">
                <span className="font-medium">{tString("orderNotes")}:</span>{" "}
                <span className="whitespace-pre-wrap">{bill.bill.notes}</span>
              </p>
            </div>
          )}
        </ModalHeader>

        <ModalBody className="space-y-4 bg-gradient-to-br from-white via-warm-50/45 to-brand/5">
          {/* Items + Summary (single section) */}
          <div className="bg-white/90 rounded-2xl border border-warm-200/80 p-4 space-y-4 shadow-sm shadow-warm-900/5">
            {/* Items header */}
            <div className="flex items-center justify-between">
              <h3 className="text-sm font-semibold text-ink-950 uppercase tracking-wider">
                {formatPreparedUnits(visibleItems.length, tString)}
              </h3>
              {isEditableBill && isEditing && (
                <Button
                  size="sm"
                  variant="flat"
                  color="primary"
                  onPress={() => setIsEditing(false)}
                  className="rounded-xl bg-brand/10 font-semibold text-brand-dark"
                >
                  {tString("buttons.doneEditing")}
                </Button>
              )}
            </div>

            {/* Item rows (always visible) */}
            {billItems.length === 0 && !isEditing ? (
              <div className="text-center py-6">
                <ChefHat className="w-10 h-10 mx-auto text-warm-500 mb-3" />
                <p className="text-sm text-ink-700">{tString("noItems")}</p>
                <p className="text-xs text-warm-500">
                  {tString("noItemsDescription")}
                </p>
              </div>
            ) : !isEditing ? (
              <div className="space-y-2">
                {visibleItems.map((item: BillItem, index: number) => {
                  const badge = getItemTypeBadge(item);
                  const parentKey = getBundleGroupKey(item);
                  const bundleChildren = parentKey
                    ? bundleChildrenByParent[parentKey] || []
                    : [];

                  return (
                    <div key={index} className="space-y-1">
                      <div className="flex justify-between items-center gap-3">
                        <div className="flex items-center gap-2 min-w-0">
                          <span className="text-sm text-ink-950 font-medium truncate">
                            {item.name || tString("unknownItem")}
                          </span>
                          {getItemType(item) !== "menu_item" && (
                            <Chip
                              size="sm"
                              variant="flat"
                              color={badge.color}
                              className="flex-shrink-0"
                            >
                              {badge.label}
                            </Chip>
                          )}
                        </div>
                        <div className="flex items-center gap-3 text-sm flex-shrink-0 whitespace-nowrap">
                          <span className="text-warm-700">
                            {item.quantity || 1} &times;{" "}
                            {formatCurrency(item.price)}
                          </span>
                          <span className="font-semibold text-ink-950 text-right">
                            {formatCurrency(item.subtotal)}
                          </span>
                        </div>
                      </div>

                      {bundleChildren.length > 0 && (
                        <div className="pl-4 border-l-2 border-brand/10 space-y-0.5">
                          {bundleChildren.map(
                            (child: BillItem, childIndex: number) => (
                              <div
                                key={`${index}-child-${childIndex}`}
                                className="flex justify-between text-xs text-warm-700"
                              >
                                <span>
                                  {child.quantity || 1} &times; {child.name}
                                </span>
                                <span>
                                  {/* Bundle components are priced on the parent —
                                      $0.00 reads like a pricing bug to operators. */}
                                  {(child.subtotal ?? 0) === 0
                                    ? tString("bundleIncluded")
                                    : formatCurrency(child.subtotal)}
                                </span>
                              </div>
                            ),
                          )}
                        </div>
                      )}
                    </div>
                  );
                })}

                {discountItems.length > 0 && (
                  <div
                    className="pt-2 space-y-1"
                    data-testid="bill-discount-band"
                  >
                    {discountItems.map((discount: BillItem, index: number) => {
                      const orderId = discount.order_id ?? discount.orderId;
                      return (
                        <div
                          key={discount.id || `discount-${index}`}
                          className="flex justify-between gap-3 text-sm text-emerald-700 bg-emerald-50 rounded-xl px-2 py-1"
                          data-testid="bill-discount-line"
                        >
                          <div className="min-w-0">
                            <span className="block truncate">
                              {discount.name || tString("discountLabel")}
                            </span>
                            {orderId ? (
                              <span className="block text-xs text-emerald-800/80">
                                {tString("discountOrderAttribution").replace(
                                  "{orderId}",
                                  String(orderId),
                                )}
                              </span>
                            ) : null}
                          </div>
                          <span className="shrink-0 font-medium tabular-nums">
                            {formatCurrency(
                              discount.subtotal || discount.price,
                            )}
                          </span>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            ) : null}

            {/* Edit / Add items (inline editor) */}
            {isEditing && isEditableBill && (
              <div className="bg-white rounded-2xl border border-brand/20 p-4 shadow-sm shadow-brand/10">
                <BillItemEditor
                  billId={bill.bill.id}
                  businessId={businessId}
                  currentItems={billItems}
                  onItemsChanged={onBillUpdated}
                  tString={tString}
                  currency={currency}
                  locale={locale}
                />
              </div>
            )}

            {/* Prominent Edit / Add button for open bills (when not editing) */}
            {isEditableBill && !isEditing && (
              <button
                onClick={() => setIsEditing(true)}
                className="w-full flex items-center justify-center gap-2 py-3 rounded-2xl border-2 border-dashed border-brand/30 bg-brand/10 hover:bg-brand/15 hover:border-brand transition text-brand-dark font-semibold text-sm"
              >
                <Plus className="w-4 h-4" />
                {tString("buttons.editItems")}
              </button>
            )}

            {/* Summary totals (always visible when items exist) */}
            {billItems.length > 0 && !isEditing && (
              <>
                <Divider />
                <div className="space-y-2">
                  <div className="flex justify-between text-sm">
                    <span className="text-warm-700">{tString("subtotal")}</span>
                    <span className="font-medium">
                      {formatCurrency(bill.bill.subtotal)}
                    </span>
                  </div>
                  <div className="flex justify-between text-sm">
                    <span className="text-warm-700">{tString("tax")}</span>
                    <span className="font-medium">
                      {formatCurrency(bill.bill.tax_amount)}
                    </span>
                  </div>
                  {bill.bill.service_fee_amount > 0 && (
                    <div className="flex justify-between text-sm">
                      <span className="text-warm-700">
                        {tString("serviceFee")}
                      </span>
                      <span className="font-medium">
                        {formatCurrency(bill.bill.service_fee_amount)}
                      </span>
                    </div>
                  )}
                  {(bill.bill.loyalty_discount ?? 0) > 0 && (
                    <div className="flex justify-between text-sm">
                      <span className="text-emerald-700">
                        {tString("loyaltyDiscount")}
                      </span>
                      <span className="font-medium text-emerald-700">
                        -{formatCurrency(bill.bill.loyalty_discount ?? 0)}
                      </span>
                    </div>
                  )}
                  <Divider />
                  <div className="flex justify-between text-base font-semibold">
                    <span>{tString("total")}</span>
                    <span>{formatCurrency(bill.bill.total_amount)}</span>
                  </div>

                  {bill.bill.paid_amount > 0 && (
                    <>
                      <div className="flex justify-between text-sm text-emerald-700">
                        <span>{tString("paid")}</span>
                        <span className="font-medium">
                          {formatCurrency(bill.bill.paid_amount)}
                        </span>
                      </div>
                      {bill.bill.tip_amount > 0 && (
                        <div className="flex justify-between text-sm text-emerald-700">
                          <span>{tString("tips")}</span>
                          <span className="font-medium">
                            {formatCurrency(bill.bill.tip_amount)}
                          </span>
                        </div>
                      )}
                      {(() => {
                        if (isPayableBill && remainingAmount > 0) {
                          return (
                            <div className="flex justify-between text-sm text-amber-700">
                              <span>{tString("remaining")}</span>
                              <span className="font-medium">
                                {formatCurrency(remainingAmount)}
                              </span>
                            </div>
                          );
                        } else if (remainingAmount < 0) {
                          return (
                            <div className="flex justify-between text-sm text-emerald-700">
                              <span>{tString("overpaid")}</span>
                              <span className="font-medium">
                                {formatCurrency(Math.abs(remainingAmount))}
                              </span>
                            </div>
                          );
                        }
                        return null;
                      })()}
                    </>
                  )}
                </div>
              </>
            )}
          </div>

          {billId != null && isPayableBill ? (
            <BillRecordPayment
              billId={billId}
              billNumber={bill.bill.bill_number}
              billStatus={bill.bill.status}
              currency={currency}
              remainingAmount={remainingAmount}
              payments={bill.bill.payments}
              alternativePayments={bill.bill.alternative_payments}
              onPaymentRecorded={onBillUpdated}
              tString={tString}
              businessId={businessId}
              canRecordPayment={canRecordPayment}
              autoOpenRecord={autoOpenRecordPayment}
              canPrint={canPrint}
              onPrintReceipt={handlePrintBill}
              onRequestCloseBill={
                isEditableBill ? () => onCloseBill(bill.bill.id) : undefined
              }
              country={fiscalCountry ?? country}
            />
          ) : null}

          {/* Fiscal customer identity (AFIP/ARCA). AR-only; fail closed when
              country/settings are unknown (#548). L6-3: write gated on open
              bills; operator mode uses operator-voiced toggle/hint. */}
          {showFiscalIdentity ? (
            <div data-testid="bill-fiscal-identity">
              <FiscalIdentityFields
                value={fiscalIdentity}
                onChange={(next, valid) => {
                  setFiscalIdentity(next);
                  setFiscalValid(valid);
                }}
                t={fiscalT}
                variant="operator"
                defaultExpanded={billHasFiscalIdentity}
                readOnly={!canWriteFiscal}
                actions={
                  canWriteFiscal ? (
                    <Button
                      size="sm"
                      color="primary"
                      variant="flat"
                      isDisabled={!fiscalValid}
                      isLoading={savingFiscal}
                      onPress={handleSaveFiscalIdentity}
                      className="rounded-xl bg-brand/10 font-semibold text-brand-dark"
                    >
                      {tString("fiscalCustomer.save")}
                    </Button>
                  ) : null
                }
              />
            </div>
          ) : null}

          <BillSplitOperatorSummary
            billId={bill.bill.id}
            businessId={businessId}
            billNumber={bill.bill.bill_number}
            billToken={bill.bill.public_token}
            currency={currency}
            locale={locale}
            tString={tString}
          />

          <div className="bg-white/90 border border-warm-200/80 rounded-2xl p-4 space-y-4 shadow-sm shadow-warm-900/5">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 bg-brand/10 rounded-2xl flex items-center justify-center border border-brand/20 shadow-sm shadow-brand/10">
                <History className="w-5 h-5 text-brand" />
              </div>
              <div>
                <h3 className="text-sm font-semibold text-ink-950 uppercase tracking-wider">
                  {tString("history.title")}
                </h3>
                <p className="text-sm text-ink-600">
                  {tString("history.subtitle")}
                </p>
              </div>
            </div>

            {historyEntries.length ? (
              <div className="space-y-3">
                {historyEntries.map((entry) => {
                  const copy = getHistoryCopy(entry);
                  const actorLabel = displayBillHistoryActor(entry.actor);
                  return (
                    <div
                      key={entry.id}
                      className="rounded-2xl border border-warm-200/80 bg-warm-50/60 px-4 py-3"
                    >
                      <div className="flex items-start justify-between gap-4">
                        <div>
                          <p className="font-semibold text-ink-950">
                            {copy.title}
                          </p>
                          {copy.body ? (
                            <p className="mt-1 text-sm text-ink-700">
                              {copy.body}
                            </p>
                          ) : null}
                          {actorLabel ? (
                            <p className="mt-2 text-xs text-warm-500">
                              {tString("history.byLabel").replace(
                                "{actor}",
                                actorLabel,
                              )}
                            </p>
                          ) : null}
                        </div>
                        <p className="text-xs text-warm-700 whitespace-nowrap">
                          {formatDateTime(entry.created_at)}
                        </p>
                      </div>
                    </div>
                  );
                })}
              </div>
            ) : (
              <p className="text-sm text-ink-700">{tString("history.empty")}</p>
            )}

            {hasOlderHistory && onLoadFullHistory ? (
              <div className="flex items-center justify-between gap-3 pt-1">
                <span className="text-xs text-ink-500">
                  {tString("history.showingCount")
                    .replace("{shown}", String(historyEntries.length))
                    .replace("{total}", String(historyTotal))}
                </span>
                <Button
                  size="sm"
                  variant="light"
                  isLoading={loadingFullHistory}
                  onPress={async () => {
                    setLoadingFullHistory(true);
                    try {
                      await onLoadFullHistory(bill.bill.id);
                    } finally {
                      setLoadingFullHistory(false);
                    }
                  }}
                  className="rounded-lg font-medium text-brand"
                >
                  {tString("history.showOlder")}
                </Button>
              </div>
            ) : null}
          </div>

          {/* IMP-15: void + refund actions and the comp_void_audit log feed.
              The component decides for itself which buttons to render based
              on bill status + payment rows, so we always mount it. */}
          <BillVoidRefundActions
            bill={bill}
            currency={currency}
            businessTimezone={businessTimezone}
            onRefresh={onBillUpdated}
            onCloseParent={() => {
              setIsEditing(false);
              onClose();
            }}
            onVoided={onBillVoided}
          />
        </ModalBody>

        <ModalFooter className="block min-w-0 overflow-visible border-t border-warm-200 bg-white px-4 py-3 sm:px-6 sm:py-4">
          {/* Related to issue 368: a single NextUI footer row clips
              "Close without payment" / "Record payment" at 390. Stack on
              narrow widths so every label stays fully readable. */}
          <div
            data-testid="bill-details-footer"
            className={BILL_DETAILS_FOOTER_ACTIONS_CLASS}
          >
            <Button
              variant="light"
              className={BILL_DETAILS_FOOTER_BUTTON_CLASS}
              onPress={() => {
                setIsEditing(false);
                onClose();
              }}
            >
              {tString("buttons.close")}
            </Button>
            {canPrint && (
              <Button
                onPress={handlePrintBill}
                isLoading={printing}
                className={`${BILL_DETAILS_FOOTER_BUTTON_CLASS} font-semibold`}
                variant="bordered"
              >
                {tString("buttons.printBill")}
              </Button>
            )}
            {isEditableBill && (
              <Button
                variant="light"
                className={`${BILL_DETAILS_FOOTER_BUTTON_CLASS} text-ink-600`}
                onPress={() => {
                  onCloseBill(bill.bill.id);
                }}
              >
                {bill.bill.paid_amount === 0
                  ? tString("buttons.closeWithoutPayment")
                  : tString("buttons.closeBill")}
              </Button>
            )}
            {isPayableBill && canRecordPayment && remainingAmount > 0 && (
              <Button
                color="primary"
                className={`${BILL_DETAILS_FOOTER_BUTTON_CLASS} bg-brand font-semibold text-white`}
                onPress={() => {
                  // Prefer the in-card Record payment control when present; the
                  // footer primary is the rush-path affordance (#99 / #104).
                  const trigger = document.querySelector<HTMLButtonElement>(
                    '[data-testid="bill-record-payment-open"]',
                  );
                  trigger?.click();
                }}
              >
                {tString("recordPayment.actions.record")}
              </Button>
            )}
          </div>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
};
