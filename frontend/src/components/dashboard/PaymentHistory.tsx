"use client";

import React, { useState, useEffect, useCallback, useRef, useMemo } from "react";
import dynamic from "next/dynamic";
import toast from "react-hot-toast";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { localDateKey } from "@/lib/localDate";
import {
  getPaymentHistoryPage,
  exportPayments,
  type PaymentHistoryItem,
  type PaymentHistoryQuery,
} from "@/api/payments";
import { getBusiness } from "@/api/business";
import { getBill, type BillWithItemsResponse } from "@/api/bills";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { sumDollars } from "@/types/money";
import { getNetwork } from "@/config/network";
import { payerMethodKey, isWalletAddress } from "@/lib/payerLabel";
import {
  methodFilterOptions,
  parseMethodFilter,
  type PaymentMethod,
} from "@/lib/paymentMethod";
import type { Locale } from "@/i18n/config";
import {
  customRangeClientError,
  paymentHistoryListError,
} from "./paymentHistoryRange";
import {
  commitDateRange,
  DATE_RANGE_DEBOUNCE_MS,
} from "./paymentHistoryDateCommit";
import PeriodTabs from "@/components/business/shared/PeriodTabs";
import { EmptyState } from "@/components/ui/EmptyState";
import { Metric } from "@/components/ui/Metric";
import { SkeletonTable } from "@/components/ui/skeletons";
import { OperatorBillNumber } from "@/components/business/OperatorBillNumber";
import {
  tableRoot,
  tableHeaderCell,
  tableBodyCell,
  tableBody,
} from "@/components/business/shared/tableStyles";

import { Input, Select, SelectItem, Button, DatePicker } from "@nextui-org/react";
import { parseDate } from "@internationalized/date";
import {
  Search,
  Download,
  ExternalLink,
  Receipt,
  ChevronLeft,
  ChevronRight,
  Eye,
} from "lucide-react";

// Task 8: drill every payment row into the existing BillDetailsModal (void/
// refund already live there). Dynamic import keeps the history table light.
const BillDetailsModal = dynamic(
  () =>
    import("@/components/business/BillDetailsModal").then(
      (mod) => mod.BillDetailsModal,
    ),
  { ssr: false },
);

const BILL_HISTORY_LIMIT = 50;

// Only confirmed money should land in the revenue/tip tiles. Pending, failed,
// refunded, and reversed rows still appear in the table but must not inflate
// the headline totals.
const isConfirmed = (p: Payment) => p.status === "confirmed";

// Block explorer for the tx-hash deep link. Payment History doesn't carry a
// per-row chain id, so we key off the configured network like the rest of the
// app (base mainnet in prod, base-sepolia otherwise) instead of hardcoding
// mainnet basescan.org.
const explorerBaseUrl = (): string =>
  getNetwork().blockExplorers?.default.url ?? "https://basescan.org";

type Payment = PaymentHistoryItem;

// Server page size. The backend clamps to <=100; 20 keeps the rendered page and
// its DOM small while the range clamp (<=366 days) bounds the window.
const PAGE_SIZE = 20;

// Debounce for the search box so each keystroke doesn't fire a request.
const SEARCH_DEBOUNCE_MS = 350;

// Period presets that map to the backend's period param. "custom" is an
// extension that swaps the preset row for an explicit start/end date picker.
// "yesterday" is shared with analytics (L6-2) so URL period= survives the
// payments sub-tab without silently falling back to month.
type Period =
  | "today"
  | "yesterday"
  | "week"
  | "month"
  | "quarter"
  | "year"
  | "custom";

interface PaymentHistoryProps {
  businessId: string;
  // M5b/finish-the-job: currency + IANA timezone threaded from the dashboard
  // page so this panel no longer fetches getBusiness just to read them. Optional
  // — a standalone render (or a caller without the business in scope) falls back
  // to a single local fetch.
  currency?: string;
  businessTimezone?: string | null;
  country?: string | null;
  // Fix 4: shared analytics period lifted to the Dashboard. When supplied this
  // panel consumes it (and reports changes back) so the period survives panel
  // switches and lands in the URL. Absent → local period state (standalone use).
  period?: Exclude<Period, "custom">;
  onPeriodChange?: (period: Exclude<Period, "custom">) => void;
}

export default function PaymentHistory({
  businessId,
  currency: currencyProp,
  businessTimezone: _businessTimezone,
  country: countryProp,
  period: sharedPeriod,
  onPeriodChange,
}: PaymentHistoryProps) {
  const { locale } = useSimpleLocale();

  const tString = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.paymentHistory.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );
  const tDateRange = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dateRange.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  // BillDetailsModal keys live under billManager.* — bridge so void/refund UI
  // reuses the same operator copy as the Bills tab.
  const billTString = useCallback(
    (key: string): string => {
      const result = getTranslation(`billManager.${key}`, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  // Task 8: open BillDetailsModal from a payment row (reachability only).
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [detailsBill, setDetailsBill] = useState<BillWithItemsResponse | null>(
    null,
  );
  const [detailsLoadingId, setDetailsLoadingId] = useState<number | null>(null);

  const openBillDetails = useCallback(
    async (billId: number) => {
      if (!Number.isFinite(billId) || billId <= 0) return;
      setDetailsLoadingId(billId);
      try {
        const bill = await getBill(billId, undefined, BILL_HISTORY_LIMIT);
        setDetailsBill(bill);
        setDetailsOpen(true);
      } catch (err) {
        console.error("PaymentHistory - open bill failed:", err);
        toast.error(tString("viewBillError"));
      } finally {
        setDetailsLoadingId(null);
      }
    },
    [tString],
  );

  const refreshDetailsBill = useCallback(async () => {
    if (!detailsBill?.bill?.id) return;
    try {
      const bill = await getBill(
        detailsBill.bill.id,
        undefined,
        BILL_HISTORY_LIMIT,
      );
      setDetailsBill(bill);
    } catch {
      // Non-fatal — operator can re-open the row.
    }
  }, [detailsBill?.bill?.id]);

  // Business default currency so non-USD operators don't see "$" on every
  // amount. Prefer the threaded prop; fall back to a single local fetch.
  const [fetchedCurrency, setFetchedCurrency] = useState<string>("USD");
  const [fetchedCountry, setFetchedCountry] = useState<string | null>(null);
  const businessCurrency = currencyProp ?? fetchedCurrency;
  const businessCountry =
    countryProp !== undefined ? countryProp : fetchedCountry;
  useEffect(() => {
    let cancelled = false;
    if (currencyProp !== undefined && countryProp !== undefined) return;
    const numericId = Number(businessId);
    if (!Number.isFinite(numericId)) return;
    getBusiness(numericId)
      .then((biz) => {
        if (cancelled) return;
        if (biz?.default_currency) setFetchedCurrency(biz.default_currency);
        setFetchedCountry(biz?.address?.country ?? "");
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [businessId, currencyProp, countryProp]);

  // --- Filter / period state ---------------------------------------------
  // Local period only used when the parent doesn't drive it.
  // Default to all-time so this list matches Bills (no date filter). A month
  // default next to an all-time bills list reads as missing payments.
  const [localPeriod, setLocalPeriod] = useState<Exclude<Period, "custom"> | undefined>(
    undefined,
  );
  const [customActive, setCustomActive] = useState(false);
  const effectivePeriod: Period | undefined = customActive
    ? "custom"
    : sharedPeriod ?? localPeriod;

  const setPeriod = (p: Exclude<Period, "custom">) => {
    setCustomActive(false);
    if (onPeriodChange) onPeriodChange(p);
    else setLocalPeriod(p);
  };

  const toggleCustomRange = () => {
    setCustomActive((v) => !v);
  };

  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<string>("all");
  // Committed method filter — selectedKeys bind to this so the checkmark
  // tracks the filter the query actually uses (finding 62 dropdown state).
  const [methodFilter, setMethodFilter] = useState<string>("all");
  // Methods present for the current business+window (from the server). The
  // dropdown is generated from this list; never hardcoded crypto/manual/stripe.
  const [availableMethods, setAvailableMethods] = useState<string[]>([]);
  // L6-8: raw DatePicker state (every segment keystroke) vs committed range
  // used by buildQuery/fetch — same debounce cadence as search.
  const [dateRangeInput, setDateRangeInput] = useState({ start: "", end: "" });
  const [dateRange, setDateRange] = useState({ start: "", end: "" });
  const [page, setPage] = useState(1);

  // Debounce the search box → committed `search` used in the query.
  useEffect(() => {
    const id = setTimeout(() => setSearch(searchInput.trim()), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(id);
  }, [searchInput]);

  // L6-8: debounce date input → commit only complete ISO dates (drop garbage
  // intermediate years). Do not abort in-flight requests (Session P owns that).
  useEffect(() => {
    const id = setTimeout(() => {
      setDateRange((prev) => commitDateRange(dateRangeInput, prev));
    }, DATE_RANGE_DEBOUNCE_MS);
    return () => clearTimeout(id);
  }, [dateRangeInput]);

  // Any filter change resets to page 1.
  useEffect(() => {
    setPage(1);
  }, [search, statusFilter, methodFilter, effectivePeriod, dateRange.start, dateRange.end]);

  // --- Data ---------------------------------------------------------------
  const [payments, setPayments] = useState<Payment[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Request-id guard so a slow earlier response never overwrites a newer one.
  const requestIdRef = useRef(0);

  const buildQuery = useCallback((): PaymentHistoryQuery => {
    // Only send a method param when it is a canonical Method key. "all" and
    // any stale non-canonical key are omitted so we never request method=stripe.
    const method = parseMethodFilter(methodFilter) ?? undefined;
    const base: PaymentHistoryQuery = {
      q: search || undefined,
      status: statusFilter,
      method: method,
    };
    if (customActive) {
      base.startDate = dateRange.start || undefined;
      base.endDate = dateRange.end || undefined;
    } else {
      base.period = sharedPeriod ?? localPeriod;
    }
    return base;
  }, [
    search,
    statusFilter,
    methodFilter,
    customActive,
    dateRange.start,
    dateRange.end,
    sharedPeriod,
    localPeriod,
  ]);

  const fetchPage = useCallback(async () => {
    const requestId = ++requestIdRef.current;
    try {
      setLoading(true);
      setError(null);
      // L6-9: client start≤end before round-trip; avoid generic server error +
      // stale period chip while Personalizado stays active.
      const clientRangeError = customRangeClientError(
        customActive,
        dateRange.start,
        dateRange.end,
        tDateRange("reversedRange"),
      );
      if (clientRangeError) {
        setError(clientRangeError);
        setPayments([]);
        setTotal(0);
        return;
      }
      const res = await getPaymentHistoryPage(parseInt(businessId, 10), {
        ...buildQuery(),
        page,
        pageSize: PAGE_SIZE,
      });
      if (requestId !== requestIdRef.current) return; // superseded
      setPayments(res.items);
      setTotal(res.total);
      // available_methods is window-scoped (not method-filtered), so every page
      // response refreshes the same dropdown options.
      if (Array.isArray(res.available_methods)) {
        setAvailableMethods(res.available_methods);
      }
    } catch (err) {
      if (requestId !== requestIdRef.current) return;
      console.error("PaymentHistory - fetch failed:", err);
      setError(paymentHistoryListError(err, locale as Locale));
      setPayments([]);
      setTotal(0);
    } finally {
      if (requestId === requestIdRef.current) setLoading(false);
    }
  }, [
    businessId,
    buildQuery,
    page,
    locale,
    customActive,
    dateRange.start,
    dateRange.end,
    tDateRange,
  ]);

  useEffect(() => {
    fetchPage();
  }, [fetchPage]);

  // Drop a committed method that is no longer present for the window so
  // selectedKeys never points at a missing option.
  useEffect(() => {
    if (
      methodFilter !== "all" &&
      availableMethods.length > 0 &&
      !availableMethods.includes(methodFilter)
    ) {
      setMethodFilter("all");
    }
  }, [availableMethods, methodFilter]);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  // --- Formatting ---------------------------------------------------------
  const formatCurrency = (amount: number, currency?: string) =>
    formatCurrencyIntl(amount, currency || businessCurrency, undefined, locale);

  const formatDate = (dateString: string) =>
    new Date(dateString).toLocaleDateString(locale, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });

  const truncateAddress = (address: string) => {
    if (address.length <= 10) return address;
    return `${address.slice(0, 6)}...${address.slice(-4)}`;
  };

  const handleExportPayments = async () => {
    try {
      const blob = await exportPayments(
        parseInt(businessId, 10),
        buildQuery(),
        "csv",
        locale,
      );
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `payments-${businessId}-${localDateKey()}.csv`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (err) {
      console.error("Failed to export payments:", err);
    }
  };

  const periodOptions = useMemo(
    () =>
      (
        ["today", "yesterday", "week", "month", "quarter", "year"] as const
      ).map((k) => ({
        key: k,
        label: tDateRange(`presets.${k}`),
      })),
    [tDateRange],
  );

  const statusOptions = [
    { key: "all", label: tString("statusOptions.all") },
    { key: "confirmed", label: tString("statusOptions.confirmed") },
    { key: "pending", label: tString("statusOptions.pending") },
    { key: "failed", label: tString("statusOptions.failed") },
    { key: "refunded", label: tString("statusOptions.refunded") },
    { key: "reversed", label: tString("statusOptions.reversed") },
  ];

  // Filter options are generated from the server's available_methods for this
  // business+window. A filter that only offers values that exist cannot return
  // "0 of 0" (finding 62).
  const methodOptions = useMemo(
    () =>
      methodFilterOptions(availableMethods, (key) => {
        if (key === "all") return tString("methodOptions.all");
        const label = tString(`methodOptions.${key as PaymentMethod}`);
        // Fall back to the key if a locale is missing a label.
        return label.startsWith("businessDashboard.") ? key : label;
      }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [availableMethods, locale],
  );

  const statusLabel = (status: string): string => {
    const known = ["confirmed", "pending", "failed", "refunded", "reversed"];
    return known.includes(status) ? tString(`statusOptions.${status}`) : status;
  };

  const payerLabel = (payment: Payment): string =>
    tString(`payerMethods.${payerMethodKey(payment)}`);

  const statusBadgeClass = (status: string): string => {
    if (status === "confirmed") return "bg-green-100 text-green-700";
    if (status === "pending") return "bg-yellow-100 text-yellow-700";
    return "bg-red-100 text-red-700";
  };

  const resetFilters = () => {
    setSearchInput("");
    setSearch("");
    setStatusFilter("all");
    setMethodFilter("all");
    setDateRangeInput({ start: "", end: "" });
    setDateRange({ start: "", end: "" });
    setCustomActive(false);
  };

  // Page-scoped confirmed totals. The whole-window revenue/tips already live in
  // the Revenue panel — here we only summarize the visible page (labeled so).
  const pageRevenue = sumDollars(payments.filter(isConfirmed).map((p) => p.amount));
  const pageTips = sumDollars(payments.filter(isConfirmed).map((p) => p.tip_amount));

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <div>
          <h2 className="text-xl font-semibold text-ink-900">{tString("title")}</h2>
          <p className="text-sm text-ink-600">{tString("subtitle")}</p>
        </div>
        <Button
          variant="bordered"
          size="sm"
          radius="full"
          onPress={handleExportPayments}
          startContent={<Download className="w-4 h-4" />}
          className="font-medium text-ink-600"
        >
          {tString("exportData")}
        </Button>
      </div>

      {/* Filters */}
      <div className="rounded-2xl border border-warm-200/80 bg-warm-50/80 p-4 shadow-[0_18px_42px_rgba(46,42,37,0.05)]">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:flex-wrap">
          {/* Period control (shared PeriodTabs) + custom toggle */}
          <div className="flex items-center gap-2">
            {/*
              L6-9: while Personalizado is active, do not mount controlled
              PeriodTabs with a blank/sentinel selectedKey — NextUI Tabs emits
              onSelectionChange and would clear customActive immediately.
              Show inert chips instead; picking a real period re-enables tabs.
            */}
            {customActive ? (
              <div
                className="inline-flex shrink-0 items-center gap-0 rounded-full bg-warm-100 p-0.5 max-w-full overflow-x-auto"
                role="presentation"
                data-testid="payment-history-period-inert"
              >
                {periodOptions.map((opt) => (
                  <button
                    key={opt.key}
                    type="button"
                    className="h-6 px-2 text-[11px] rounded-full text-ink-500 hover:text-ink-800"
                    onClick={() => setPeriod(opt.key)}
                  >
                    {opt.label}
                  </button>
                ))}
              </div>
            ) : (
              <PeriodTabs
                options={periodOptions}
                value={((sharedPeriod ?? localPeriod) || "") as Exclude<Period, "custom">}
                onChange={(k) => setPeriod(k)}
                ariaLabel={tDateRange("presetsAriaLabel")}
                className="shrink-0"
              />
            )}
            <button
              type="button"
              data-testid="payment-history-custom-toggle"
              aria-pressed={customActive}
              onClick={toggleCustomRange}
              className={
                customActive
                  ? "inline-flex h-8 shrink-0 items-center justify-center rounded-full bg-brand px-3 text-xs font-medium text-white"
                  : "inline-flex h-8 shrink-0 items-center justify-center rounded-full px-3 text-xs font-medium text-ink-700 hover:bg-warm-100"
              }
            >
              {tDateRange("presets.custom")}
            </button>
          </div>

          {/* Search */}
          <div className="min-w-0 flex-1">
            <Input
              value={searchInput}
              onValueChange={setSearchInput}
              placeholder={tString("searchPlaceholder")}
              startContent={<Search className="h-4 w-4 text-ink-400" />}
              size="sm"
              isClearable
              onClear={() => setSearchInput("")}
              aria-label={tString("searchPayments")}
              classNames={{
                input: "text-sm",
                inputWrapper:
                  "border border-warm-200/90 bg-white shadow-sm hover:border-brand/35 data-[focus=true]:border-brand",
              }}
            />
          </div>

          {/* Status filter */}
          <div className="w-full lg:w-40">
            <Select
              size="sm"
              variant="bordered"
              aria-label={tString("status")}
              selectedKeys={[statusFilter]}
              onSelectionChange={(keys) =>
                setStatusFilter((Array.from(keys)[0] as string) || "all")
              }
              classNames={{
                trigger:
                  "border border-warm-200/90 bg-white shadow-sm hover:border-brand/35 data-[open=true]:border-brand",
              }}
            >
              {statusOptions.map((o) => (
                <SelectItem key={o.key}>{o.label}</SelectItem>
              ))}
            </Select>
          </div>

          {/* Method filter — selectedKeys bound to committed methodFilter */}
          <div className="w-full lg:w-40">
            <Select
              size="sm"
              variant="bordered"
              aria-label={tString("methodOptions.label")}
              selectionMode="single"
              disallowEmptySelection
              selectedKeys={new Set([methodFilter])}
              onSelectionChange={(keys) => {
                const key = Array.from(keys as Set<string>)[0];
                // Commit immediately — selectedKeys tracks this same state, so
                // the checkmark cannot lag on "All Methods" after a pick.
                setMethodFilter(key || "all");
              }}
              classNames={{
                trigger:
                  "border border-warm-200/90 bg-white shadow-sm hover:border-brand/35 data-[open=true]:border-brand",
              }}
            >
              {methodOptions.map((o) => (
                <SelectItem key={o.key}>{o.label}</SelectItem>
              ))}
            </Select>
          </div>

          <Button
            variant="light"
            size="sm"
            radius="full"
            onPress={resetFilters}
            className="font-medium text-ink-600"
          >
            {tString("clearAll")}
          </Button>
        </div>

        {/* Custom date range — only when the custom preset is active */}
        {customActive && (
          <div className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-end">
            <DatePicker
              aria-label={tDateRange("startDate")}
              label={tDateRange("startDate")}
              data-testid="payment-history-start-date"
              value={
                dateRangeInput.start ? parseDate(dateRangeInput.start) : null
              }
              maxValue={
                dateRangeInput.end ? parseDate(dateRangeInput.end) : undefined
              }
              onChange={(v) =>
                setDateRangeInput((c) => ({
                  ...c,
                  start: v?.toString() ?? "",
                }))
              }
              granularity="day"
              size="sm"
              className="w-full sm:w-44"
            />
            <DatePicker
              aria-label={tDateRange("endDate")}
              label={tDateRange("endDate")}
              data-testid="payment-history-end-date"
              value={dateRangeInput.end ? parseDate(dateRangeInput.end) : null}
              minValue={
                dateRangeInput.start
                  ? parseDate(dateRangeInput.start)
                  : undefined
              }
              onChange={(v) =>
                setDateRangeInput((c) => ({
                  ...c,
                  end: v?.toString() ?? "",
                }))
              }
              granularity="day"
              size="sm"
              className="w-full sm:w-44"
            />
          </div>
        )}
      </div>

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-4">
          <h3 className="text-sm font-semibold text-red-900">{tString("error")}</h3>
          <p className="text-sm text-red-700">{error}</p>
        </div>
      )}

      {/* Summary strip — transactions = server total; money = this page only.
          Metric keeps $0.00 neutral (never success-green) when the page is empty. */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="rounded-2xl border border-warm-200/90 bg-white p-5 shadow-card">
          <Metric
            label={tString("summary.totalTransactions")}
            value={total}
            state="ok"
            format="count"
            size="lg"
            data-testid="payment-history-total"
          />
        </div>
        <div className="rounded-2xl border border-warm-200/90 bg-white p-5 shadow-card">
          {payments.length === 0 ? (
            <Metric
              label={tString("summary.pageRevenue")}
              value={0}
              state="empty"
              reason="—"
              format={(n) => formatCurrency(n)}
              size="lg"
              data-testid="payment-history-page-revenue"
            />
          ) : (
            <Metric
              label={tString("summary.pageRevenue")}
              value={pageRevenue}
              state="ok"
              format={(n) => formatCurrency(n)}
              size="lg"
              data-testid="payment-history-page-revenue"
            />
          )}
        </div>
        <div className="rounded-2xl border border-warm-200/90 bg-white p-5 shadow-card">
          {payments.length === 0 ? (
            <Metric
              label={tString("summary.pageTips")}
              value={0}
              state="empty"
              reason="—"
              format={(n) => formatCurrency(n)}
              size="lg"
              data-testid="payment-history-page-tips"
            />
          ) : (
            <Metric
              label={tString("summary.pageTips")}
              value={pageTips}
              state="ok"
              format={(n) => formatCurrency(n)}
              size="lg"
              data-testid="payment-history-page-tips"
            />
          )}
        </div>
      </div>

      {/* Table */}
      <div className="rounded-2xl border border-warm-200/90 bg-white shadow-card">
        <div className="border-b border-warm-200/80 p-4 flex items-center justify-between">
          <h3 className="text-lg text-ink-900 tracking-wide">
            {tString("paymentRecords")}
          </h3>
          <div className="text-sm text-ink-600">
            {payments.length}{" "}
            {tString("paymentsCount").replace("{total}", total.toString())}
          </div>
        </div>

        {loading && payments.length === 0 ? (
          <div className="p-4">
            <SkeletonTable rows={8} columns={7} />
          </div>
        ) : payments.length === 0 ? (
          <EmptyState
            icon={Receipt}
            title={tString("noPayments")}
            subtitle={tString("subtitle")}
            compact
          />
        ) : (
          <div className="overflow-x-auto">
            <table className={tableRoot}>
              <thead>
                <tr>
                  <th className={tableHeaderCell}>{tString("tableHeaders.bill")}</th>
                  <th className={tableHeaderCell}>{tString("tableHeaders.table")}</th>
                  <th className={tableHeaderCell}>{tString("tableHeaders.amount")}</th>
                  <th className={tableHeaderCell}>{tString("tableHeaders.total")}</th>
                  <th className={tableHeaderCell}>{tString("tableHeaders.payer")}</th>
                  <th className={tableHeaderCell}>{tString("tableHeaders.status")}</th>
                  <th className={tableHeaderCell}>{tString("tableHeaders.date")}</th>
                  <th className={tableHeaderCell}>{tString("tableHeaders.transaction")}</th>
                  <th className={tableHeaderCell}>{tString("tableHeaders.actions")}</th>
                </tr>
              </thead>
              <tbody className={tableBody}>
                {payments.map((payment) => (
                  <tr key={payment.id} className="hover:bg-warm-50">
                    <td className={`${tableBodyCell} font-medium text-ink-900`}>
                      <OperatorBillNumber
                        id={payment.bill_id}
                        bill_number={payment.bill_number}
                        copyLabel={tString("copyBillNumber")}
                        copiedLabel={tString("copiedBillNumber")}
                        className="text-ink-900"
                      />
                    </td>
                    <td className={`${tableBodyCell} text-ink-600`}>
                      {payment.table_name}
                    </td>
                    <td className={tableBodyCell}>
                      <div className="font-semibold text-ink-900">
                        {formatCurrency(payment.amount, payment.currency)}
                      </div>
                      {payment.tip_amount > 0 && (
                        <div className="text-xs text-ink-600">
                          {tString("tipSuffix").replace(
                            "{amount}",
                            formatCurrency(payment.tip_amount, payment.currency),
                          )}
                        </div>
                      )}
                    </td>
                    <td className={`${tableBodyCell} font-semibold text-ink-900`}>
                      {formatCurrency(
                        sumDollars([payment.amount, payment.tip_amount]),
                        payment.currency,
                      )}
                    </td>
                    <td className={tableBodyCell}>
                      <div className="text-sm font-medium text-ink-900">
                        {payerLabel(payment)}
                      </div>
                      {isWalletAddress(payment.payer_address) && (
                        <div
                          className="font-mono text-xs text-ink-500"
                          title={payment.payer_address}
                        >
                          {truncateAddress(payment.payer_address)}
                        </div>
                      )}
                    </td>
                    <td className={tableBodyCell}>
                      <span
                        className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${statusBadgeClass(
                          payment.status,
                        )}`}
                      >
                        {statusLabel(payment.status)}
                      </span>
                    </td>
                    <td className={`${tableBodyCell} text-ink-500`}>
                      {formatDate(payment.created_at)}
                    </td>
                    <td className={tableBodyCell}>
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-xs text-ink-600">
                          {payment.tx_hash
                            ? `${payment.tx_hash.slice(0, 6)}...${payment.tx_hash.slice(-4)}`
                            : tString("notAvailable")}
                        </span>
                        {payment.tx_hash &&
                          /^0x[0-9a-fA-F]{64}$/.test(payment.tx_hash) && (
                          <button
                            onClick={() =>
                              window.open(
                                `${explorerBaseUrl()}/tx/${payment.tx_hash}`,
                                "_blank",
                                "noopener,noreferrer",
                              )
                            }
                            className="text-brand hover:text-brand-dark transition-colors"
                            title={tString("viewOnExplorer")}
                            aria-label={tString("viewOnExplorer")}
                          >
                            <ExternalLink className="w-4 h-4" />
                          </button>
                        )}
                      </div>
                    </td>
                    <td className={tableBodyCell}>
                      <Button
                        size="sm"
                        variant="light"
                        startContent={<Eye className="h-4 w-4" />}
                        isLoading={detailsLoadingId === payment.bill_id}
                        onPress={() => void openBillDetails(payment.bill_id)}
                        aria-label={`${tString("viewBill")} #${payment.bill_id}`}
                        className="font-medium text-ink-600 hover:bg-warm-50 hover:text-ink-900"
                      >
                        {tString("viewBill")}
                      </Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {/* Pagination */}
        {totalPages > 1 && (
          <div className="flex items-center justify-center gap-2 p-4 border-t border-warm-200/80">
            <Button
              size="sm"
              variant="bordered"
              radius="full"
              isDisabled={page <= 1 || loading}
              onPress={() => setPage((p) => Math.max(1, p - 1))}
              startContent={<ChevronLeft className="w-4 h-4" />}
            >
              {tString("pagination.previous")}
            </Button>
            <span className="px-3 text-sm text-ink-600">
              {tString("pagination.pageOf")
                .replace("{current}", page.toString())
                .replace("{total}", totalPages.toString())}
            </span>
            <Button
              size="sm"
              variant="bordered"
              radius="full"
              isDisabled={page >= totalPages || loading}
              onPress={() => setPage((p) => Math.min(totalPages, p + 1))}
              endContent={<ChevronRight className="w-4 h-4" />}
            >
              {tString("pagination.next")}
            </Button>
          </div>
        )}
      </div>

      <BillDetailsModal
        isOpen={detailsOpen}
        onClose={() => {
          setDetailsOpen(false);
          setDetailsBill(null);
        }}
        bill={detailsBill}
        onCloseBill={() => {
          setDetailsOpen(false);
          setDetailsBill(null);
        }}
        onBillUpdated={() => {
          void refreshDetailsBill();
          void fetchPage();
        }}
        businessId={Number(businessId)}
        tString={billTString}
        currency={businessCurrency}
        businessTimezone={_businessTimezone ?? null}
        country={businessCountry}
        mode="operator"
      />
    </div>
  );
}
