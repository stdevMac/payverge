"use client";

import React, { useState, useEffect, useCallback } from "react";
import dynamic from "next/dynamic";
import toast from "react-hot-toast";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { analyticsApi, type LiveBill } from "@/api/analytics";
import { getBusiness } from "@/api/business";
import { getBill, type BillWithItemsResponse } from "@/api/bills";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { intlLocaleFor } from "@/utils/intlLocale";
import { formatBusinessTime } from "@/utils/businessTime";
import { OperatorBillNumber } from "@/components/business/OperatorBillNumber";

import { Clock, DollarSign, Users, RefreshCw, Eye } from "lucide-react";
import { usePolling } from "@/hooks/usePolling";
import { localizedErrorMessage } from "@/utils/localizedError";
import { btnGhostIcon } from "@/components/ui/buttonStyles";

// Task 8: every live bill — including delivery/counter without a table_code —
// drills into BillDetailsModal so void/refund are reachable.
const BillDetailsModal = dynamic(
  () =>
    import("@/components/business/BillDetailsModal").then(
      (mod) => mod.BillDetailsModal,
    ),
  { ssr: false },
);

const BILL_HISTORY_LIMIT = 50;

interface LiveBillsProps {
  businessId: string;
  // M5b: currency + IANA timezone threaded down from TodayLivePanel (which gets
  // them from the dashboard page) so this component no longer re-fetches
  // getBusiness just to read those two fields — that was a duplicate fetch and a
  // brief "$/USD" flash on every analytics-tab visit. Optional; a standalone
  // render without the props falls back to a single local fetch.
  currency?: string;
  businessTimezone?: string | null;
  country?: string | null;
  // Fix 5 (poll consolidation): when the parent already runs the shared 10s live
  // poller (useLivePulse in TodayLivePanel), it threads the bills + status down
  // so this component does NOT run its own second poller. Absent → standalone
  // mode, where LiveBills runs its own 10s poll.
  bills?: LiveBill[];
  loading?: boolean;
  error?: string | null;
  lastUpdated?: Date;
  capped?: boolean;
  onRefresh?: () => void;
}

// table_id = 0 bills (delivery/counter) come back with an empty table_name —
// fall back to a human label instead of leaving the card's name slot blank.
export function liveBillTableLabel(
  bill: Pick<LiveBill, "table_name" | "counter_id">,
  tString: (key: string) => string,
): string {
  if (bill.table_name) return bill.table_name;
  return bill.counter_id ? tString("counter") : tString("delivery");
}

export default function LiveBills({
  businessId,
  currency: currencyProp,
  businessTimezone: businessTimezoneProp,
  country: countryProp,
  bills: billsProp,
  loading: loadingProp,
  error: errorProp,
  lastUpdated: lastUpdatedProp,
  capped: cappedProp,
  onRefresh,
}: LiveBillsProps) {
  // Translation setup
  const { locale } = useSimpleLocale();

  // Translation helper
  const tString = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.liveBills.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );
  const billTString = useCallback(
    (key: string): string => {
      const result = getTranslation(`billManager.${key}`, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

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
        const details = await getBill(billId, undefined, BILL_HISTORY_LIMIT);
        setDetailsBill(details);
        setDetailsOpen(true);
      } catch (err) {
        console.error("LiveBills - open bill failed:", err);
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
      const details = await getBill(
        detailsBill.bill.id,
        undefined,
        BILL_HISTORY_LIMIT,
      );
      setDetailsBill(details);
    } catch {
      // Non-fatal.
    }
  }, [detailsBill?.bill?.id]);

  // Externally-driven mode: the parent (TodayLivePanel) runs the shared 10s
  // poller and threads the bills/status down. In that mode this component runs
  // NO poller of its own — that was the duplicate poll the audit flagged.
  const externallyDriven = billsProp !== undefined;

  const [localBills, setLocalBills] = useState<LiveBill[]>([]);
  const [localLoading, setLocalLoading] = useState(true);
  const [localError, setLocalError] = useState<string | null>(null);
  const [localLastUpdated, setLocalLastUpdated] = useState<Date>(new Date());

  const bills = externallyDriven ? billsProp! : localBills;
  const loading = externallyDriven ? loadingProp ?? false : localLoading;
  const error = externallyDriven ? errorProp ?? null : localError;
  const lastUpdated = externallyDriven ? lastUpdatedProp ?? new Date() : localLastUpdated;
  // AED operators were seeing "$" on live bills; the business currency/timezone
  // now arrive as props from the dashboard page (M5b). These fetched fallbacks
  // only apply when the props are absent (standalone render).
  const [fetchedCurrency, setFetchedCurrency] = useState<string>("USD");
  const [fetchedTimezone, setFetchedTimezone] = useState<string | null>(null);
  const [fetchedCountry, setFetchedCountry] = useState<string | null>(null);

  // Prefer the props (no fetch); fall back to the locally fetched values only
  // when the parent didn't supply them.
  const businessCurrency = currencyProp ?? fetchedCurrency;
  // Business IANA timezone so live-bill times render in the restaurant's wall
  // clock, never the operator's device timezone. null falls back to UTC.
  const businessTimezone =
    businessTimezoneProp !== undefined ? businessTimezoneProp : fetchedTimezone;
  const businessCountry =
    countryProp !== undefined ? countryProp : fetchedCountry;

  useEffect(() => {
    let cancelled = false;
    if (!businessId) return;
    // Only fetch when the parent didn't thread currency/timezone/country down.
    if (
      currencyProp !== undefined &&
      businessTimezoneProp !== undefined &&
      countryProp !== undefined
    ) {
      return;
    }
    const numericId = Number(businessId);
    if (!Number.isFinite(numericId)) return;
    getBusiness(numericId)
      .then((biz) => {
        if (cancelled) return;
        if (biz?.default_currency) {
          setFetchedCurrency(biz.default_currency);
        }
        setFetchedTimezone(biz?.timezone ?? null);
        setFetchedCountry(biz?.address?.country ?? "");
      })
      .catch(() => {
        // Best-effort — leave USD fallback in place.
      });
    return () => {
      cancelled = true;
    };
  }, [businessId, currencyProp, businessTimezoneProp, countryProp]);

  const fetchLiveBills = useCallback(
    async (isInitialLoad = false) => {
      try {
        // Only show loading state on initial load, not during polling
        if (isInitialLoad) {
          setLocalLoading(true);
        }
        setLocalError(null);
        const data = await analyticsApi.getLiveBills(businessId);
        setLocalBills(Array.isArray(data) ? data : []);
        setLocalLastUpdated(new Date());
      } catch (err) {
        // Raw error stays in the console/Sentry breadcrumb; the banner shows a
        // translated generic message rather than raw axios/sanitized text.
        console.error("LiveBills - API Error:", err);
        setLocalError(localizedErrorMessage(err, locale));
      } finally {
        // Only clear loading state if this was an initial load
        if (isInitialLoad) {
          setLocalLoading(false);
        }
      }
    },
    [businessId, locale],
  );

  // Initial load with loading state — ONLY in standalone mode. When the parent
  // drives the data (shared poller) we skip our own fetch entirely.
  useEffect(() => {
    if (businessId && !externallyDriven) {
      fetchLiveBills(true); // isInitialLoad = true
    }
  }, [businessId, fetchLiveBills, externallyDriven]);

  // Silent polling callback (no loading state)
  const silentFetchLiveBills = useCallback(() => {
    return fetchLiveBills(false); // isInitialLoad = false
  }, [fetchLiveBills]);

  // Manual refresh: delegate to the parent's shared poller when externally
  // driven, otherwise refresh our own data.
  const handleManualRefresh = useCallback(() => {
    if (externallyDriven) {
      onRefresh?.();
      return;
    }
    return fetchLiveBills(true); // isInitialLoad = true
  }, [fetchLiveBills, externallyDriven, onRefresh]);

  // Poll for real-time updates — ONLY in standalone mode. In externally-driven
  // mode the parent's single shared poller is the sole live source, so this
  // component never runs a second overlapping poll cycle.
  const { isPolling: localIsPolling } = usePolling({
    callback: silentFetchLiveBills,
    interval: 10000, // Poll every 10 seconds (6 requests per minute)
    enabled: !!businessId && !externallyDriven,
    immediate: false, // fetchLiveBills is already called in useEffect above
  });
  // The "Live" chip reflects real polling: the shared poller when driven, our
  // own otherwise.
  const isPolling = externallyDriven ? true : localIsPolling;

  // Route money through the operator locale so es/es-AR see comma-grouped
  // amounts ("US$ 37,82"), matching the summary cards — not the en-US default
  // formatCurrencyIntl falls back to when the locale arg is omitted (audit H4).
  const formatCurrency = useCallback(
    (amount: number) =>
      formatCurrencyIntl(amount, businessCurrency, undefined, intlLocaleFor(locale)),
    [businessCurrency, locale],
  );

  const formatTime = (dateString: string) => {
    return formatBusinessTime(dateString, locale, businessTimezone, {
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  const getTimeElapsed = (dateString: string) => {
    const now = new Date();
    const created = new Date(dateString);
    const diffMs = now.getTime() - created.getTime();
    const diffMins = Math.floor(diffMs / 60000);

    if (diffMins < 1) return tString("timeElapsed.justNow");
    if (diffMins < 60) return `${diffMins}${tString("timeElapsed.minutesAgo")}`;

    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return `${diffHours}${tString("timeElapsed.hoursAgo")}`;

    const diffDays = Math.floor(diffHours / 24);
    return `${diffDays}${tString("timeElapsed.daysAgo")}`;
  };

  const getStatusColor = (bill: LiveBill) => {
    const status = bill.status?.toLowerCase();
    if (status === "paid") return "success";
    if (status === "partial") return "warning";
    if (bill.paid_amount >= bill.total_amount) return "success";
    if (bill.paid_amount > 0) return "warning";
    return "default";
  };

  const getStatusText = (bill: LiveBill) => {
    const status = bill.status?.toLowerCase();
    if (status === "paid") return tString("status.paid");
    if (status === "partial") return tString("status.partial");
    if (bill.paid_amount >= bill.total_amount) return tString("status.paid");
    if (bill.paid_amount > 0) return tString("status.partial");
    return tString("status.unpaid");
  };

  const getPaymentProgress = (bill: LiveBill) => {
    return bill.total_amount > 0
      ? (bill.paid_amount / bill.total_amount) * 100
      : 0;
  };

  if (loading && bills.length === 0) {
    return (
      <div className="flex items-center justify-center py-16">
        <div className="text-center">
          <div className="w-16 h-16 bg-warm-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-warm-100">
            <div className="w-8 h-8 border-2 border-warm-300 border-t-ink-900 rounded-full animate-spin"></div>
          </div>
          <p className="text-ink-600 tracking-wide">
            {tString("loading")}
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <div>
          <div className="flex items-center gap-3">
            <h2 className="text-xl text-ink-900 tracking-wide">
              {tString("title")}
            </h2>
            <span className="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-brand/10 text-brand-dark">
              {bills.length}
            </span>
            {isPolling && (
              <span className="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-green-100 text-green-700">
                <div className="w-2 h-2 bg-green-500 rounded-full mr-1 motion-safe:animate-pulse"></div>
                {tString("live")}
              </span>
            )}
          </div>
          <p className="text-ink-600 text-sm">
            {tString("subtitle")}
          </p>
        </div>

        <div className="flex items-center gap-3">
          <span className="text-sm text-ink-600">
            {tString("lastUpdated")}:{" "}
            {formatBusinessTime(lastUpdated, locale, businessTimezone, {
              hour: "2-digit",
              minute: "2-digit",
            })}
          </span>
          <button
            type="button"
            onClick={handleManualRefresh}
            disabled={loading}
            aria-label={tString("refresh")}
            title={tString("refresh")}
            className={`${btnGhostIcon} disabled:opacity-50`}
          >
            <RefreshCw className={`w-4 h-4 ${loading ? "animate-spin" : ""}`} />
          </button>
        </div>
      </div>

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-4">
          <div className="flex items-center gap-3">
            <Clock className="w-5 h-5 text-red-600 flex-shrink-0" />
            <p className="text-red-800 font-medium">
              {tString("error")}: {error}
            </p>
          </div>
        </div>
      )}

      {cappedProp && (
        <div className="rounded-lg border border-warm-200 bg-warm-50 px-4 py-2 text-sm text-ink-600">
          {tString("cappedNotice").replace("{count}", bills.length.toString())}
        </div>
      )}

      {bills.length === 0 ? (
        <div className="text-center py-12">
          <div className="w-16 h-16 bg-warm-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-warm-100">
            <Users className="w-8 h-8 text-ink-400" />
          </div>
          <h3 className="text-lg text-ink-900 tracking-wide mb-2">
            {tString("noActiveBills")}
          </h3>
          <p className="text-ink-600 text-sm">
            {tString("noActiveBillsDesc")}
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {bills.map((bill) => (
            <div
              key={bill.id}
              className="bg-white border border-warm-200 rounded-lg shadow-sm hover:shadow-md transition-all duration-200"
            >
              <div className="border-b border-warm-200 p-4">
                <div className="flex justify-between items-start">
                  <div>
                    <h3 className="font-semibold text-lg text-ink-900">
                      <OperatorBillNumber
                        id={bill.id}
                        bill_number={bill.bill_number}
                        copyLabel={tString("copyBillNumber")}
                        copiedLabel={tString("copiedBillNumber")}
                      />
                    </h3>
                    <div className="flex items-center gap-2 mt-1">
                      <DollarSign className="w-4 h-4 text-ink-600" />
                      <p className="text-sm font-medium text-ink-600">
                        {liveBillTableLabel(bill, tString)}
                      </p>
                    </div>
                  </div>
                  <span
                    className={`inline-flex items-center px-2 py-1 rounded-full text-xs font-medium ${
                      getStatusColor(bill) === "success"
                        ? "bg-green-100 text-green-700"
                        : getStatusColor(bill) === "warning"
                          ? "bg-yellow-100 text-yellow-700"
                          : "bg-warm-100 text-ink-600"
                    }`}
                  >
                    {getStatusText(bill)}
                  </span>
                </div>
              </div>

              <div className="p-4">
                <div className="space-y-4">
                  {/* Amount Details */}
                  <div className="space-y-3">
                    <div className="flex justify-between items-center p-3 bg-warm-50 rounded-lg">
                      <span className="text-sm font-medium text-ink-600">
                        {tString("amounts.totalAmount")}
                      </span>
                      <span className="font-semibold text-lg text-ink-900">
                        {formatCurrency(bill.total_amount)}
                      </span>
                    </div>

                    <div className="flex justify-between items-center p-3 bg-green-50 rounded-lg">
                      <span className="text-sm font-medium text-green-700">
                        {tString("amounts.paidAmount")}
                      </span>
                      <span className="font-semibold text-lg text-green-700">
                        {formatCurrency(bill.paid_amount)}
                      </span>
                    </div>

                    {bill.remaining_amount > 0 && (
                      <div className="flex justify-between items-center">
                        <span className="text-sm text-ink-600">
                          {tString("amounts.remaining")}
                        </span>
                        <span className="font-semibold text-yellow-600">
                          {formatCurrency(bill.remaining_amount)}
                        </span>
                      </div>
                    )}

                    <div className="flex justify-between items-center">
                      <span className="text-sm text-ink-600">
                        {tString("amounts.tips")}
                      </span>
                      <span className="font-semibold text-brand">
                        {formatCurrency(bill.tip_amount)}
                      </span>
                    </div>
                  </div>

                  {/* Payment Progress Bar */}
                  <div className="space-y-1">
                    <div className="flex justify-between text-xs text-ink-600">
                      <span>{tString("paymentProgress")}</span>
                      <span>{Math.round(getPaymentProgress(bill))}%</span>
                    </div>
                    <div className="w-full bg-warm-200 rounded-full h-2">
                      <div
                        className={`h-2 rounded-full transition-all duration-300 ${
                          getPaymentProgress(bill) === 100
                            ? "bg-green-500"
                            : getPaymentProgress(bill) > 0
                              ? "bg-yellow-500"
                              : "bg-warm-300"
                        }`}
                        style={{
                          width: `${Math.min(getPaymentProgress(bill), 100)}%`,
                        }}
                      />
                    </div>
                  </div>

                  {/* Time Info */}
                  <div className="flex items-center justify-between text-xs text-ink-600">
                    <div className="flex items-center gap-1">
                      <Clock className="w-3 h-3" />
                      <span>
                        {tString("created")} {formatTime(bill.created_at)}
                      </span>
                    </div>
                    <span>{getTimeElapsed(bill.created_at)}</span>
                  </div>

                  {/* Task 8: every money row drills into operator BillDetailsModal
                      (including delivery/counter bills with empty table_code). */}
                  <div className="flex gap-2 pt-2">
                    <button
                      type="button"
                      onClick={() => void openBillDetails(bill.id)}
                      disabled={detailsLoadingId === bill.id}
                      aria-label={`${tString("view")} #${bill.id}`}
                      className="flex-1 inline-flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium text-ink-600 bg-warm-50 border border-warm-200 rounded-lg hover:bg-warm-100 disabled:opacity-60"
                    >
                      <Eye className="w-4 h-4" />
                      {tString("view")}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Summary Stats */}
      {bills.length > 0 && (
        <div className="bg-white border border-warm-200 rounded-lg shadow-sm">
          <div className="border-b border-warm-200 p-4">
            <h3 className="text-lg text-ink-900 tracking-wide">
              {tString("quickStats.title")}
            </h3>
          </div>
          <div className="p-4">
            <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
              <div className="text-center">
                <p className="text-2xl font-semibold text-brand">
                  {bills.length}
                </p>
                <p className="text-sm text-ink-600">
                  {tString("quickStats.activeBills")}
                </p>
              </div>

              <div className="text-center">
                <p className="text-2xl font-semibold text-green-600">
                  {formatCurrency(
                    bills.reduce((sum, bill) => sum + bill.total_amount, 0),
                  )}
                </p>
                <p className="text-sm text-ink-600">
                  {tString("quickStats.totalValue")}
                </p>
              </div>

              <div className="text-center">
                <p className="text-2xl font-semibold text-yellow-600">
                  {formatCurrency(
                    bills.reduce((sum, bill) => sum + bill.remaining_amount, 0),
                  )}
                </p>
                <p className="text-sm text-ink-600">
                  {tString("quickStats.outstanding")}
                </p>
              </div>

              <div className="text-center">
                <p className="text-2xl font-semibold text-brand">
                  {formatCurrency(
                    bills.reduce((sum, bill) => sum + bill.tip_amount, 0),
                  )}
                </p>
                <p className="text-sm text-ink-600">
                  {tString("quickStats.tipsCollected")}
                </p>
              </div>
            </div>
          </div>
        </div>
      )}

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
          void handleManualRefresh();
        }}
        businessId={Number(businessId)}
        tString={billTString}
        currency={businessCurrency}
        businessTimezone={businessTimezone}
        country={businessCountry}
        mode="operator"
      />
    </div>
  );
}
