"use client";

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useSearchParams } from "next/navigation";
import { DatePicker } from "@nextui-org/react";
import { parseDate } from "@internationalized/date";
import {
  AlertCircle,
  Banknote,
  BarChart3,
  BookOpen,
  FileBarChart,
  Receipt,
  RefreshCw,
} from "lucide-react";
import {
  accountingApi,
  type ManualLedgerEntry,
  type PayrollRun,
} from "@/api/accounting";
import { fiscalApi, type FiscalReceipt } from "@/api/fiscal";
import {
  INVOICE_FAILURE_STATUSES,
  isInvoiceFailure,
} from "../fiscal/invoiceStatus";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import {
  accountingKeys,
  useSummary,
} from "@/hooks/accounting/useAccountingQueries";
import { useQueryClient } from "@tanstack/react-query";
import { AnalyticsSkeleton } from "../AnalyticsSkeleton";
import DashboardLockedTabView from "../DashboardLockedTabView";
import {
  formatDay,
  formatMoney,
  formatPeriod,
} from "@/components/business/accounting/format";
import TimeRangePresets, {
  detectPreset,
  type RangePreset,
} from "./TimeRangePresets";
import { type ActivityItem } from "./RecentActivity";
import { PremiumPanel } from "../premium";
import SegmentedTabs from "../shared/SegmentedTabs";
import DashboardTabShell from "../shared/DashboardTabShell";
import { btnGhostIcon } from "@/components/ui/buttonStyles";
import {
  formatDateInput,
  type AccountingDashboardProps,
  type AccountingDashboardTab,
} from "./accountingShared";
import OverviewTab from "./OverviewTab";
import EntriesTab from "./EntriesTab";
import PayrollTab from "./PayrollTab";
import InvoicesTab from "./InvoicesTab";
import ReportsTab from "./ReportsTab";
import OutstandingTab from "./OutstandingTab";

/** Recent-activity window — full tables are server-paginated in sub-tabs. */
const ACTIVITY_PAGE_SIZE = 10;

export function AccountingTab({
  businessId,
  initialTab = "overview",
  onTabChange,
  canManagePayroll = true,
  businessTimezone = null,
  country = null,
  fiscalEntry = false,
}: AccountingDashboardProps) {
  const { locale } = useSimpleLocale();
  const searchParams = useSearchParams();
  const {
    hasAccess,
    isSuspended,
    loading: accessLoading,
  } = useBusinessAccess(businessId);

  // Deep-link filter seeds from NeedsAttentionStrip Review links
  // (`?sub=payroll&status=draft`, `?sub=invoices&filter=needs_attention`).
  const deepLinkStatus = searchParams?.get("status") ?? null;
  const deepLinkFilter = searchParams?.get("filter") ?? null;
  const payrollInitialStatus =
    deepLinkStatus === "draft" ||
    deepLinkStatus === "paid" ||
    deepLinkStatus === "void"
      ? deepLinkStatus
      : undefined;
  const invoicesInitialFilter =
    deepLinkFilter === "needs_attention" ||
    deepLinkFilter === "authorized" ||
    deepLinkFilter === "pending"
      ? deepLinkFilter
      : undefined;

  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const result = getTranslation(
        `businessDashboard.accountingDashboard.${key}`,
        locale,
        params,
      );
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  // NEW-15: fiscal sidebar entry reuses AccountingTab on the invoices sub-tab.
  const fiscalPageTitle = useMemo(() => {
    const result = getTranslation("fiscal.dashboard.title", locale);
    return Array.isArray(result) ? result[0] || "Invoices" : (result as string);
  }, [locale]);
  const fiscalPageSubtitle = useMemo(() => {
    const result = getTranslation("fiscal.dashboard.description", locale);
    return Array.isArray(result)
      ? result[0] || ""
      : (result as string);
  }, [locale]);
  // Locale-aware money formatter — mirrors the sibling FiscalDashboard so
  // accounting figures use the operator's number grouping/decimal separators
  // instead of a hardcoded en-US format.
  const fmtMoney = useCallback(
    (value: number, currency: string): string =>
      formatMoney(value, currency, locale),
    [locale],
  );

  const tWith = useCallback(
    (key: string, replacements: Record<string, string | number>): string => {
      let value = t(key);
      Object.entries(replacements).forEach(([name, replacement]) => {
        value = value.replace(
          new RegExp(`\\{${name}\\}`, "g"),
          String(replacement),
        );
      });
      return value;
    },
    [t],
  );

  const today = useMemo(() => new Date(), []);
  const defaultEnd = formatDateInput(today);
  const defaultStart = formatDateInput(
    new Date(today.getTime() - 29 * 24 * 60 * 60 * 1000),
  );

  const [startDate, setStartDate] = useState(defaultStart);
  const [endDate, setEndDate] = useState(defaultEnd);
  const requestedRangeRef = useRef(`${defaultStart}:${defaultEnd}`);
  /** Overview recent-activity only (page 1, small page_size). */
  const [recentEntries, setRecentEntries] = useState<ManualLedgerEntry[]>([]);
  const [payrollRuns, setPayrollRuns] = useState<PayrollRun[]>([]);
  /** Accurate draft-run total for the Payroll tab badge (server count). */
  const [draftPayrollTotal, setDraftPayrollTotal] = useState(0);
  const [receipts, setReceipts] = useState<FiscalReceipt[]>([]);
  /** Server totals for the Invoices tab badge (scoped page_size:1 probes). */
  const [pendingInvoiceTotal, setPendingInvoiceTotal] = useState(0);
  const [failedInvoiceTotal, setFailedInvoiceTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [activeTab, setActiveTab] =
    useState<AccountingDashboardTab>(initialTab);

  // Single owner of accounting/summary for a business+range (shared RQ cache
  // with OverviewTab / NeedsAttentionStrip). Parent no longer raw-getSummary.
  const summaryRange = useMemo(
    () => ({ start: startDate, end: endDate }),
    [startDate, endDate],
  );
  const summaryEnabled =
    !accessLoading && hasAccess && !isSuspended && !(endDate < startDate);
  const summaryQuery = useSummary(
    summaryEnabled ? businessId : 0,
    summaryRange,
  );
  const summary = summaryQuery.data ?? null;
  const queryClient = useQueryClient();

  const reportingCurrency = summary?.currency || "USD";

  const startDateRef = useRef(startDate);
  const endDateRef = useRef(endDate);
  // Guards the core summary/entries/payroll/receipts writes against out-of-order
  // resolution: rapid range changes (Today then YTD) can run two loadData calls
  // concurrently, and if the stale one resolves last it would paint the wrong
  // range's numbers. Only the newest loadData applies its results.
  const loadRequestIdRef = useRef(0);
  useEffect(() => {
    startDateRef.current = startDate;
  }, [startDate]);
  useEffect(() => {
    endDateRef.current = endDate;
  }, [endDate]);

  useEffect(() => {
    setActiveTab(initialTab);
  }, [initialTab]);

  const loadData = useCallback(
    async (isRefresh = false) => {
      const start = startDateRef.current;
      const end = endDateRef.current;
      // Never fetch a backwards reporting window — it returns confident zeros.
      // Mirrors the payroll date validation (endDate < startDate); surface the
      // inline error and bail before touching loading/refreshing state so the
      // dashboard keeps its last-good numbers. The reporting-range inputs also
      // block their own refetch triggers, but this is the source-of-truth guard
      // that also covers post-mutation refetches.
      if (start && end && end < start) {
        setError(t("errors.reportingDateRangeInvalid"));
        return;
      }
      const loadRequestId = (loadRequestIdRef.current += 1);
      if (isRefresh) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }
      setError(null);
      const businessIdNum = Number.parseInt(businessId, 10);
      // Receipts fail independently of activity/badge probes; swallowing them
      // used to paint confident-but-wrong zeros. On failure keep last-known.
      let receiptsFailed = false;
      try {
        const [
          nextEntriesPage,
          nextPayrollPage,
          nextDraftPayrollPage,
          nextReceiptsPage,
          nextPendingProbe,
          nextFailedProbe,
        ] = await Promise.all([
          // Recent-activity feed only — full table is server-paginated via
          // EntriesTab / useEntries; CSV is server export (entriesExportUrl).
          // Sparklines/KPIs/warnings/currency come from useSummary + useTimeseries
          // (single accounting/summary request per range — not duplicated here).
          // Cost analytics (food/labor/waste) live on the Analytics tab.
          accountingApi.listEntries(businessId, {
            start,
            end,
            page: 1,
            page_size: 5,
          }),
          // Activity strip only — full payroll table is server-paginated via
          // PayrollTab / usePayrollRuns (listPayrollRunsPage).
          accountingApi.listPayrollRunsPage(businessId, {
            start,
            end,
            page: 1,
            page_size: ACTIVITY_PAGE_SIZE,
          }),
          // Tab badge uses server total for draft runs (not client-filtered page).
          accountingApi.listPayrollRunsPage(businessId, {
            start,
            end,
            status: "draft",
            page: 1,
            page_size: 1,
          }),
          // Activity strip only — newest receipts in the reporting window.
          // Server-paginated: the legacy unpaginated listReceipts fetched every
          // receipt (with delivery rows) just to slice a handful client-side.
          Number.isFinite(businessIdNum)
            ? fiscalApi
                .listReceiptsPage(businessIdNum, {
                  start,
                  end,
                  page: 1,
                  page_size: ACTIVITY_PAGE_SIZE,
                })
                .catch(() => {
                  receiptsFailed = true;
                  return null;
                })
            : Promise.resolve(null),
          // Invoices tab badge: server totals via page_size:1 probes, scoped to
          // the reporting window like every other number on this page.
          Number.isFinite(businessIdNum)
            ? fiscalApi
                .listReceiptsPage(businessIdNum, {
                  start,
                  end,
                  status: "pending",
                  page: 1,
                  page_size: 1,
                })
                .catch(() => {
                  receiptsFailed = true;
                  return null;
                })
            : Promise.resolve(null),
          Number.isFinite(businessIdNum)
            ? fiscalApi
                .listReceiptsPage(businessIdNum, {
                  start,
                  end,
                  status: INVOICE_FAILURE_STATUSES.join(","),
                  page: 1,
                  page_size: 1,
                })
                .catch(() => {
                  receiptsFailed = true;
                  return null;
                })
            : Promise.resolve(null),
        ]);

        // Only the newest loadData applies activity/badge results — a superseded
        // concurrent load resolving late must not paint a stale range.
        const isCurrent = loadRequestIdRef.current === loadRequestId;
        if (isCurrent) {
          setRecentEntries(nextEntriesPage.entries || []);
          setPayrollRuns(nextPayrollPage.runs || []);
          setDraftPayrollTotal(nextDraftPayrollPage.total ?? 0);
          // Only overwrite receipts when the fetch succeeded; on failure keep the
          // last-known set so the badge does not silently zero.
          if (!receiptsFailed) {
            setReceipts(nextReceiptsPage?.receipts ?? []);
            setPendingInvoiceTotal(nextPendingProbe?.total ?? 0);
            setFailedInvoiceTotal(nextFailedProbe?.total ?? 0);
          }
        }
      } catch (loadError) {
        console.error("Failed to load accounting data:", loadError);
        if (loadRequestIdRef.current === loadRequestId) {
          setError(
            loadError instanceof Error
              ? loadError.message
              : t("errors.loadData"),
          );
        }
      } finally {
        // A superseded load must not clear the loading/refreshing flag the newer
        // load set, or the spinner would drop while the current fetch is pending.
        if (loadRequestIdRef.current === loadRequestId) {
          setLoading(false);
          setRefreshing(false);
        }
      }
    },
    [businessId, t],
  );

  useEffect(() => {
    if (!accessLoading && hasAccess && !isSuspended) {
      void loadData();
    }
  }, [loadData, accessLoading, hasAccess, isSuspended]);

  useEffect(() => {
    // Skip the refetch when the range is inverted (endDate < startDate) — the
    // inline error on the inputs tells the operator to fix it, and fetching a
    // backwards window would paint confident-but-wrong zeros.
    const rangeKey = `${startDate}:${endDate}`;
    if (requestedRangeRef.current === rangeKey) return;
    if (startDate && endDate && endDate < startDate) {
      requestedRangeRef.current = rangeKey;
      return;
    }
    if (accessLoading || !hasAccess || isSuspended || loading) return;
    requestedRangeRef.current = rangeKey;
    void loadData(true);
  }, [
    endDate,
    hasAccess,
    isSuspended,
    loadData,
    loading,
    startDate,
    accessLoading,
  ]);

  const presetLabels = useMemo(
    () => ({
      today: t("dateRange.presets.today"),
      week: t("dateRange.presets.week"),
      month: t("dateRange.presets.month"),
      "30d": t("dateRange.presets.30d"),
      ytd: t("dateRange.presets.ytd"),
      custom: t("dateRange.presets.custom"),
    }),
    [t],
  );

  const activePreset: RangePreset = useMemo(
    () => detectPreset(startDate, endDate),
    [startDate, endDate],
  );

  // Reporting-range order guard — mirrors the payroll date validation
  // (endDate < startDate). Both values are YYYY-MM-DD strings, so the
  // lexicographic comparison is chronological. When inverted we surface an
  // inline error on the inputs and SKIP the refetch below so the dashboard
  // never renders confident zeros for a backwards window.
  const reportingRangeInvalid = useMemo(
    () => Boolean(startDate && endDate && endDate < startDate),
    [startDate, endDate],
  );

  const handlePresetSelect = (
    preset: RangePreset,
    start: string,
    end: string,
  ) => {
    if (preset === "custom") return;
    setStartDate(start);
    setEndDate(end);
  };

  // Recent entries feed the activity strip (Overview hosts RecentActivity).
  const entriesList: ManualLedgerEntry[] = recentEntries;

  const unpaidPayrollCount = draftPayrollTotal;
  // Server totals (scoped probes), not client counts over one fetched page.
  const invoicesBadgeCount = pendingInvoiceTotal + failedInvoiceTotal;
  // Stable H1 — always "Accounting" regardless of sub-tab (Task 14).
  const formatActivityTimestamp = useCallback(
    (iso: string): string => {
      const date = new Date(iso);
      if (Number.isNaN(date.getTime())) return "";
      const diffMs = Date.now() - date.getTime();
      const minutes = Math.max(0, Math.round(diffMs / 60000));
      if (minutes < 1) return t("activity.justNow");
      if (minutes < 60) return tWith("activity.minutesAgo", { n: minutes });
      const hours = Math.round(minutes / 60);
      if (hours < 24) return tWith("activity.hoursAgo", { n: hours });
      const days = Math.round(hours / 24);
      if (days <= 7) return tWith("activity.daysAgo", { n: days });
      return formatDay(iso, locale);
    },
    [t, tWith, locale],
  );

  // Recent activity (last 5 across all sub-domains)
  const activityItems: ActivityItem[] = useMemo(() => {
    const items: ActivityItem[] = [];
    entriesList.forEach((entry) => {
      const occurred = entry.created_at || entry.occurred_at;
      if (!occurred) return;
      const titleKey = entry.voided_at
        ? "activity.entryVoided"
        : entry.entry_type === "income"
          ? "activity.incomeRecorded"
          : entry.entry_type === "expense"
            ? "activity.expenseRecorded"
            : "activity.entryCreated";
      items.push({
        id: `entry-${entry.id}`,
        kind: entry.voided_at ? "entry_voided" : "entry_created",
        title: t(titleKey),
        detail: `${entry.description} · ${fmtMoney(entry.amount, entry.currency || reportingCurrency)}`,
        occurredAt: entry.voided_at || occurred,
      });
    });
    payrollRuns.forEach((run) => {
      // Three-way so a voided run isn't shown as a fresh "Payroll created", and
      // format the amount in the run's stamped currency (history is not
      // re-valued when the business currency changes).
      const kind: ActivityItem["kind"] =
        run.status === "paid"
          ? "payroll_paid"
          : run.status === "void"
            ? "payroll_voided"
            : "payroll_created";
      const title =
        run.status === "paid"
          ? t("activity.payrollPaid")
          : run.status === "void"
            ? t("activity.payrollVoided")
            : t("activity.payrollCreated");
      const occurredAt =
        run.status === "void"
          ? run.voided_at || run.period_end
          : run.paid_at || run.period_end;
      items.push({
        id: `payroll-${run.id}-${run.status}`,
        kind,
        title,
        detail: `${formatPeriod(run.period_start, run.period_end, locale)} · ${fmtMoney(run.net_total, run.currency || reportingCurrency)}`,
        occurredAt,
      });
    });
    receipts.forEach((receipt) => {
      const kind: ActivityItem["kind"] =
        receipt.status === "authorized" || receipt.status === "credited"
          ? "invoice_authorized"
          : isInvoiceFailure(receipt.status)
            ? "invoice_failed"
            : "invoice_pending";
      const titleKey =
        kind === "invoice_authorized"
          ? "activity.invoiceAuthorized"
          : kind === "invoice_failed"
            ? "activity.invoiceFailed"
            : "activity.invoicePending";
      items.push({
        id: `invoice-${receipt.id}`,
        kind,
        title: t(titleKey),
        detail: `#${receipt.bill_id}${receipt.receipt_number ? ` · ${receipt.receipt_number}` : ""}`,
        occurredAt:
          receipt.issued_at || receipt.updated_at || receipt.created_at,
      });
    });
    return items
      .filter((item) => Boolean(item.occurredAt))
      .sort(
        (a, b) =>
          new Date(b.occurredAt).getTime() - new Date(a.occurredAt).getTime(),
      )
      .slice(0, 6);
  }, [
    entriesList,
    payrollRuns,
    receipts,
    t,
    reportingCurrency,
    locale,
    fmtMoney,
  ]);

  return (
    <DashboardTabShell
      width="wide"
      locked={
        !accessLoading && (!hasAccess || isSuspended) ? (
          <DashboardLockedTabView
            title={t("header.title")}
            subtitle={t("header.subtitle")}
            businessId={businessId}
          />
        ) : null
      }
      loading={
        (((loading || summaryQuery.isLoading) && !summary) || accessLoading) ? (
          <AnalyticsSkeleton />
        ) : null
      }
      header={{
        // ?tab=fiscal is a distinct Setup destination (official invoices), not
        // Accounting-with-a-prefilter. Title stays fiscal.dashboard.title
        // ("Invoices"); eyebrow is Setup (not Accounting). Sub-tabs stay hidden.
        // Sidebar rail label remains Fiscal (EN) / Facturas (ES) from #272.
        title: fiscalEntry ? fiscalPageTitle : t("header.title"),
        eyebrow: fiscalEntry ? t("fiscalEntry.eyebrow") : undefined,
        subtitle: fiscalEntry ? fiscalPageSubtitle : t("header.subtitle"),
      }}
    >
      {/* Reporting-range toolbar — one flat row; the tab badges and KPI tiles
          below are the single home for accounting numbers. */}
      <PremiumPanel
        className="flex flex-col gap-3 p-3 lg:flex-row lg:items-end lg:justify-between"
        withTexture={false}
      >
        <div className="max-w-full overflow-x-auto pb-1 lg:pb-0">
          <TimeRangePresets
            active={activePreset}
            onSelect={handlePresetSelect}
            labels={presetLabels}
            ariaLabel={t("dateRange.presetsAriaLabel")}
          />
        </div>
        <div className="grid w-full grid-cols-1 gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end lg:w-auto">
          <DatePicker
            aria-label={t("dateRange.startDate")}
            label={t("dateRange.startDate")}
            value={startDate ? parseDate(startDate) : null}
            maxValue={endDate ? parseDate(endDate) : undefined}
            onChange={(value) => setStartDate(value?.toString() ?? "")}
            granularity="day"
            size="sm"
            className="min-w-0"
            classNames={{
              label:
                "text-xs font-semibold uppercase tracking-[0.16em] text-ink-500",
            }}
          />
          <DatePicker
            aria-label={t("dateRange.endDate")}
            label={t("dateRange.endDate")}
            value={endDate ? parseDate(endDate) : null}
            minValue={startDate ? parseDate(startDate) : undefined}
            onChange={(value) => setEndDate(value?.toString() ?? "")}
            granularity="day"
            size="sm"
            isInvalid={reportingRangeInvalid}
            className="min-w-0"
            classNames={{
              label:
                "text-xs font-semibold uppercase tracking-[0.16em] text-ink-500",
            }}
          />
          <button
            type="button"
            onClick={() => {
              if (reportingRangeInvalid) return;
              // Explicit refresh: revalidate the shared useSummary cache so
              // KPIs/warnings/currency stay in lockstep with activity probes.
              void queryClient.invalidateQueries({
                queryKey: accountingKeys.summary(businessId, {
                  start: startDate,
                  end: endDate,
                }),
              });
              void loadData(true);
            }}
            disabled={refreshing || reportingRangeInvalid}
            aria-label={t("header.refresh")}
            title={t("header.refresh")}
            className={`${btnGhostIcon} self-end disabled:opacity-50`}
          >
            <RefreshCw
              className={`h-4 w-4 ${refreshing ? "animate-spin" : ""}`}
            />
          </button>
          {reportingRangeInvalid && (
            <p
              className="col-span-full text-xs font-medium text-rose-600"
              role="alert"
            >
              {t("errors.reportingDateRangeInvalid")}
            </p>
          )}
        </div>
      </PremiumPanel>

      <div className="space-y-6">
        {error && (
          <div className="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
            {error}
          </div>
        )}

        {summary?.warnings?.length ? (
          <div className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800">
            <div className="font-medium text-amber-900">
              {t("warnings.title")}
            </div>
            <ul className="mt-2 list-disc space-y-1 pl-5">
              {summary.warnings.map((warning) => {
                const scope = warning.params?.scope ?? "";
                const scopeLabel =
                  t(`warnings.scopes.${scope}`) ||
                  warning.params?.entry_type ||
                  scope;
                const details = warning.params?.details;
                const detailsSuffix = details
                  ? t("warnings.detailsSuffix", { details })
                  : "";
                return (
                  <li
                    key={`${warning.code}-${scope}-${warning.params?.count ?? ""}-${details ?? ""}`}
                  >
                    {t(`warnings.fxWarnings.${warning.code}`, {
                      count: warning.params?.count ?? "0",
                      currency: warning.params?.currency ?? "",
                      scope_label: scopeLabel,
                      details_suffix: detailsSuffix,
                    })}
                  </li>
                );
              })}
            </ul>
          </div>
        ) : null}

        {/* Setup → Invoices (?tab=fiscal) is a focused destination — hide the
            Accounting sub-tab strip so operators aren't bounced into Overview. */}
        {!fiscalEntry ? (
          <SegmentedTabs
            activeKey={activeTab}
            onChange={(key) => {
              const nextTab = key as AccountingDashboardTab;
              setActiveTab(nextTab);
              onTabChange?.(nextTab);
            }}
            ariaLabel={t("header.title")}
            className="w-full sm:w-auto"
            size="sm"
            tabs={[
              { key: "overview", label: t("tabs.overview"), icon: BarChart3 },
              { key: "entries", label: t("tabs.entries"), icon: BookOpen },
              {
                key: "payroll",
                label: t("tabs.payroll"),
                icon: Banknote,
                badge: unpaidPayrollCount,
              },
              {
                key: "invoices",
                label: t("tabs.invoices"),
                icon: Receipt,
                badge: invoicesBadgeCount,
              },
              {
                key: "reports",
                label: t("tabs.reports"),
                icon: FileBarChart,
              },
              {
                key: "outstanding",
                label: t("tabs.outstanding"),
                icon: AlertCircle,
              },
            ]}
          />
        ) : null}

        <div className="space-y-6">
          {activeTab === "overview" && (
            <OverviewTab
              businessId={businessId}
              start={startDate}
              end={endDate}
              locale={locale}
              t={t}
              tWith={tWith}
              fmtMoney={fmtMoney}
              activityItems={activityItems}
              formatOccurredAt={formatActivityTimestamp}
              onSubTabChange={(tab) => {
                setActiveTab(tab);
                onTabChange?.(tab);
              }}
            />
          )}

          {activeTab === "entries" && (
            <EntriesTab
              businessId={businessId}
              start={startDate}
              end={endDate}
              locale={locale}
              currency={reportingCurrency}
              canWrite={canManagePayroll}
              t={t}
              tWith={tWith}
            />
          )}

          {activeTab === "payroll" && (
            <PayrollTab
              businessId={businessId}
              start={startDate}
              end={endDate}
              locale={locale}
              currency={reportingCurrency}
              canWrite={canManagePayroll}
              initialStatusFilter={payrollInitialStatus}
              t={t}
              tWith={tWith}
            />
          )}

          {activeTab === "invoices" && (
            <InvoicesTab
              businessId={businessId}
              start={startDate}
              end={endDate}
              locale={locale}
              currency={reportingCurrency}
              canWrite={canManagePayroll}
              canManageSensitive={canManagePayroll}
              businessTimezone={businessTimezone}
              initialFilter={invoicesInitialFilter}
              t={t}
              tWith={tWith}
            />
          )}

          {activeTab === "reports" && (
            <ReportsTab
              businessId={businessId}
              start={startDate}
              end={endDate}
              locale={locale}
              currency={reportingCurrency}
              canOwn={canManagePayroll}
              t={t}
            />
          )}

          {activeTab === "outstanding" && (
            <OutstandingTab
              businessId={businessId}
              start={startDate}
              end={endDate}
              locale={locale}
              currency={reportingCurrency}
              businessTimezone={businessTimezone}
              country={country}
              t={t}
            />
          )}
        </div>
      </div>
    </DashboardTabShell>
  );
}
