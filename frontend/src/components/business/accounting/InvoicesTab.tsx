"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import { AlertTriangle, Download, FileText, Plus, Search } from "lucide-react";
import { Input } from "@nextui-org/react";
import type { Locale } from "@/i18n/localeRegistry";
import {
  getSettings,
  listReceiptsPage,
  type FiscalStatus,
  type ReceiptDeliveryBadge,
  type ReceiptRow,
  type ReceiptsListParams,
} from "@/api/fiscal";
import { exportLocalizedCsv } from "@/utils/exportLocalizedCsv";
import { RECEIPTS_CSV_COLUMNS } from "@/utils/csvColumnManifests/receipts";
import {
  useReceipts,
  useResendReceipt,
} from "@/hooks/accounting/useAccountingQueries";
import { useDebouncedValue } from "@/components/admin/primitives";
import { formatMoney } from "@/components/business/accounting/format";
import {
  DATE_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";
import { PremiumPanel } from "../premium";
import DataTable, { type DataTableColumn } from "../shared/DataTable";
import RowActionsMenu, { type RowActionItem } from "../shared/RowActionsMenu";
import SegmentedTabs from "../shared/SegmentedTabs";
import FiscalDashboard from "../fiscal/FiscalDashboard";
import {
  displayReceiptType,
  isArgentinaLetterType,
  receiptTypeFilterOptions,
  receiptTypeLabelKey,
} from "../fiscal/receiptTypes";
import IssueInvoiceDrawer from "./IssueInvoiceDrawer";
import ReceiptDetailDrawer from "./ReceiptDetailDrawer";
import StatusBadge, { type StatusTone } from "./StatusBadge";
import {
  accountingPanelClass,
  accountingPanelHeaderClass,
  accountingPanelTitleClass,
  accountingPrimaryButtonClass,
  accountingSecondaryButtonClass,
} from "./accountingShared";

const SEARCH_DEBOUNCE_MS = 300;

const PAGE_SIZE = 20;
/** L6-20: CSV export walks the API with a big page size until exhausted. */
const EXPORT_PAGE_SIZE = 200;

const DELIVERY_CHANNELS = ["pdf", "email", "print"] as const;
type DeliveryChannelKey = (typeof DELIVERY_CHANNELS)[number];

type InvoiceFilter = "all" | "authorized" | "pending" | "needs_attention";

export type InvoicesTabProps = {
  businessId: string;
  start: string;
  end: string;
  locale: Locale;
  currency: string;
  /** When false, hide Issue / resend / credit / retry write controls. */
  canWrite?: boolean;
  /** Owner-gated credit notes (fiscal:credit). Defaults to canWrite. */
  canManageSensitive?: boolean;
  businessTimezone?: string | null;
  /** Deep-link seed from `?filter=needs_attention|authorized|pending` (Task 26). */
  initialFilter?: InvoiceFilter;
  t: (key: string, params?: Record<string, string | number>) => string;
  tWith?: (
    key: string,
    replacements: Record<string, string | number>,
  ) => string;
};

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

/** Map API delivery channel names onto the three UI dots (artifact → pdf). */
function normalizeChannel(channel: string): DeliveryChannelKey | null {
  const lower = channel.toLowerCase();
  if (lower === "artifact" || lower === "pdf") return "pdf";
  if (lower === "email") return "email";
  if (lower === "print") return "print";
  return null;
}

function badgeByChannel(
  badges: ReceiptDeliveryBadge[],
): Partial<Record<DeliveryChannelKey, ReceiptDeliveryBadge>> {
  const map: Partial<Record<DeliveryChannelKey, ReceiptDeliveryBadge>> = {};
  for (const badge of badges) {
    const key = normalizeChannel(badge.channel);
    if (key) map[key] = badge;
  }
  return map;
}

function dotToneClass(status: string | undefined): string {
  if (!status) return "bg-warm-300";
  if (status === "succeeded") return "bg-emerald-500";
  if (status === "dead") return "bg-rose-500";
  // pending / leased / unknown in-flight states
  return "bg-amber-400";
}

function DeliveryDotCluster({
  delivery,
  t,
  receiptId,
}: {
  delivery: ReceiptDeliveryBadge[];
  t: (key: string, params?: Record<string, string | number>) => string;
  receiptId: number;
}) {
  const byChannel = badgeByChannel(delivery);

  const parts = DELIVERY_CHANNELS.map((channel) => {
    const badge = byChannel[channel];
    const channelLabel = t(`invoices.delivery.${channel}`);
    const statusLabelText = badge
      ? badge.status
      : t("invoices.delivery.missing");
    const tip = t("invoices.delivery.tooltip", {
      channel: channelLabel,
      status: statusLabelText,
    });
    return { channel, badge, tip };
  });

  const ariaLabel = parts.map((p) => p.tip).join("; ");

  return (
    <span
      className="inline-flex items-center gap-1"
      role="img"
      aria-label={ariaLabel}
      data-testid={`delivery-dots-${receiptId}`}
      title={ariaLabel}
    >
      {parts.map(({ channel, badge, tip }) => (
        <span
          key={channel}
          data-delivery-channel={channel}
          title={tip}
          className={`inline-block h-2 w-2 rounded-full ${dotToneClass(badge?.status)}`}
        />
      ))}
    </span>
  );
}

function resolveInitialInvoiceFilter(
  value: InvoiceFilter | undefined,
): InvoiceFilter {
  if (
    value === "all" ||
    value === "authorized" ||
    value === "pending" ||
    value === "needs_attention"
  ) {
    return value;
  }
  return "all";
}

/** Compact A/B/C / NC-A chip — Argentina letter types only. */
function LetterChip({
  receiptType,
  t,
}: {
  receiptType: string;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  if (!isArgentinaLetterType(receiptType)) return null;
  const rt = (receiptType || "").toLowerCase();
  let letter = "";
  let isNc = false;
  if (rt.includes("credito") || rt.includes("credit") || rt.startsWith("nc")) {
    isNc = true;
  }
  if (rt.includes("factura_a") || rt.endsWith("_a") || rt === "a") letter = "A";
  else if (rt.includes("factura_b") || rt.endsWith("_b") || rt === "b")
    letter = "B";
  else if (rt.includes("factura_c") || rt.endsWith("_c") || rt === "c")
    letter = "C";
  if (!letter) return null;
  const label = isNc
    ? t("invoices.letter.nc", { letter })
    : t("invoices.letter.factura", { letter });
  return (
    <span
      data-testid="receipt-letter-chip"
      className="inline-flex items-center rounded-md bg-brand/10 px-1.5 py-0.5 text-[10px] font-bold tracking-wide text-brand-800"
    >
      {label}
    </span>
  );
}

/** Single locale-aware type label — never chip + raw slug together (#209). */
function ReceiptTypeLabel({
  receiptType,
  country,
  t,
}: {
  receiptType: string;
  country?: string | null;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const displayed = displayReceiptType(country, receiptType);
  if (isArgentinaLetterType(displayed)) {
    return <LetterChip receiptType={displayed} t={t} />;
  }
  const key = receiptTypeLabelKey(displayed);
  const translated = t(key);
  const label =
    translated === key
      ? (displayed || "").replace(/_/g, " ")
      : translated;
  if (!label) return null;
  return (
    <span
      data-testid="receipt-type-label"
      className="inline-flex items-center rounded-md bg-brand/10 px-1.5 py-0.5 text-[10px] font-bold tracking-wide text-brand-800"
    >
      {label}
    </span>
  );
}

export default function InvoicesTab({
  businessId,
  start,
  end,
  locale,
  currency,
  canWrite = true,
  canManageSensitive,
  businessTimezone = null,
  initialFilter,
  t,
  tWith,
}: InvoicesTabProps) {
  const sensitive = canManageSensitive ?? canWrite;
  const numericBizId = Number.parseInt(businessId, 10);

  const [filter, setFilter] = useState<InvoiceFilter>(() =>
    resolveInitialInvoiceFilter(initialFilter),
  );
  const [receiptTypeFilter, setReceiptTypeFilter] = useState("");
  const [search, setSearch] = useState("");
  const debouncedSearch = useDebouncedValue(search, SEARCH_DEBOUNCE_MS);
  const [page, setPage] = useState(1);
  const [selectedReceiptId, setSelectedReceiptId] = useState<number | null>(
    null,
  );
  /** Keep the clicked row so the detail drawer still opens if the page refetches. */
  const [selectedReceiptCache, setSelectedReceiptCache] =
    useState<ReceiptRow | null>(null);
  const [issueDrawerOpen, setIssueDrawerOpen] = useState(false);
  const [fiscalCountry, setFiscalCountry] = useState<string>("");

  useEffect(() => {
    setPage(1);
  }, [filter, start, end, receiptTypeFilter, debouncedSearch]);

  // Drive the type filter from fiscal settings country (not hardcoded AR letters).
  useEffect(() => {
    if (!Number.isFinite(numericBizId) || numericBizId <= 0) return;
    let cancelled = false;
    void getSettings(numericBizId)
      .then((settings) => {
        if (!cancelled) setFiscalCountry(settings?.country || "");
      })
      .catch(() => {
        if (!cancelled) setFiscalCountry("");
      });
    return () => {
      cancelled = true;
    };
  }, [numericBizId]);

  const typeFilterOptions = useMemo(
    () => receiptTypeFilterOptions(fiscalCountry),
    [fiscalCountry],
  );

  // Drop a stale AR letter filter if the venue is not Argentina.
  useEffect(() => {
    if (!receiptTypeFilter) return;
    const allowed = new Set(typeFilterOptions.map((o) => o.key));
    if (!allowed.has(receiptTypeFilter)) {
      setReceiptTypeFilter("");
    }
  }, [typeFilterOptions, receiptTypeFilter]);

  // Active filters without pagination — shared by the on-screen list and the
  // full-range CSV export (L6-20).
  const filterParams = useMemo(() => {
    const base: {
      start: string;
      end: string;
      needs_attention?: true;
      status?: string;
      receipt_type?: string;
      q?: string;
    } = {
      start,
      end,
    };
    if (filter === "needs_attention") {
      base.needs_attention = true;
    } else if (filter === "authorized" || filter === "pending") {
      base.status = filter;
    }
    if (receiptTypeFilter) {
      base.receipt_type = receiptTypeFilter;
    }
    const q = debouncedSearch.trim();
    if (q) base.q = q;
    return base;
  }, [start, end, filter, receiptTypeFilter, debouncedSearch]);

  const listParams = useMemo(
    () => ({
      ...filterParams,
      page,
      page_size: PAGE_SIZE,
    }),
    [filterParams, page],
  );

  const { data, isLoading, isFetching, isError, refetch } = useReceipts(
    businessId,
    listParams,
  );

  // Lightweight envelope probe for the Needs-attention chip badge count.
  const attentionProbe = useReceipts(businessId, {
    start,
    end,
    needs_attention: true,
    page: 1,
    page_size: 1,
  });
  const attentionCount = attentionProbe.data?.total ?? 0;

  const resendMutation = useResendReceipt(businessId);

  const receipts = useMemo(() => data?.receipts ?? [], [data?.receipts]);
  const total = data?.total ?? 0;

  const [exporting, setExporting] = useState(false);

  // L6-20 / S-14: client-side CSV honors receipt_type + needs_attention filters
  // (server export ignored them) AND covers the ENTIRE date range — it walks
  // every API page for the active filters instead of dumping the on-screen page.
  const handleExportCsv = useCallback(async () => {
    if (exporting) return;
    setExporting(true);
    try {
      const all: ReceiptRow[] = [];
      let exportPage = 1;
      for (;;) {
        const params: ReceiptsListParams = {
          ...filterParams,
          page: exportPage,
          page_size: EXPORT_PAGE_SIZE,
        };
        const pageData = await listReceiptsPage(businessId, params);
        const rows = pageData.receipts ?? [];
        all.push(...rows);
        const totalRows = pageData.total ?? all.length;
        if (rows.length === 0 || all.length >= totalRows) break;
        exportPage += 1;
      }

      const filename = `invoices-${start}-${end}.csv`;
      exportLocalizedCsv({
        columns: RECEIPTS_CSV_COLUMNS,
        rows: all.map((r) => ({
          bill_id: r.bill_id,
          receipt_type: displayReceiptType(
            r.country || fiscalCountry,
            r.receipt_type,
          ),
          receipt_number: r.receipt_number || "",
          amount: (r.total_amount_cents / 100).toFixed(2),
          tip: ((r.tip_amount_cents || 0) / 100).toFixed(2),
          currency: r.currency,
          status: r.status,
          needs_attention: r.needs_attention ? "yes" : "no",
          issued_at: r.issued_at || "",
          error: r.error_message || "",
        })),
        filename,
        t,
      });
    } catch (error) {
      // Never download a partially fetched range as if it were complete.
      console.error("Invoices CSV export failed", error);
    } finally {
      setExporting(false);
    }
  }, [exporting, filterParams, businessId, start, end, t, fiscalCountry]);

  const openDetail = useCallback((row: ReceiptRow) => {
    setSelectedReceiptCache(row);
    setSelectedReceiptId(row.id);
  }, []);

  const openPdf = useCallback((row: ReceiptRow) => {
    if (!row.pdf_path) return;
    window.open(row.pdf_path, "_blank", "noopener,noreferrer");
  }, []);

  const printPdf = useCallback((row: ReceiptRow) => {
    if (!row.pdf_path) return;
    const w = window.open(row.pdf_path, "_blank", "noopener,noreferrer");
    if (!w) return;
    const tryPrint = () => {
      try {
        w.focus();
        w.print();
      } catch {
        // Cross-origin PDF viewers may block print(); opening the tab is enough.
      }
    };
    // Give the PDF viewer a moment to load before invoking print.
    window.setTimeout(tryPrint, 500);
  }, []);

  const handleRowAction = useCallback(
    (key: string, row: ReceiptRow) => {
      if (key === "view") {
        openDetail(row);
        return;
      }
      if (key === "pdf" && row.pdf_path) {
        openPdf(row);
        return;
      }
      if (key === "print" && row.pdf_path) {
        printPdf(row);
        return;
      }
      if (key === "resend" && canWrite && row.status === "authorized") {
        resendMutation.mutate(row.id);
        return;
      }
      if (key === "retry" && canWrite && row.needs_attention) {
        // Task 23 owns per-channel delivery retry in the detail drawer.
        openDetail(row);
        return;
      }
      if (
        key === "credit" &&
        sensitive &&
        row.status === "authorized" &&
        row.action !== "credit_note"
      ) {
        // Credit note requires reason (+ optional amount) in ReceiptDetailDrawer.
        openDetail(row);
      }
    },
    [canWrite, sensitive, resendMutation, openDetail, openPdf, printPdf],
  );

  const menuItemsFor = useCallback(
    (row: ReceiptRow): RowActionItem[] => {
      const items: RowActionItem[] = [
        { key: "view", label: t("invoices.menu.view") },
      ];
      if (row.pdf_path) {
        items.push({ key: "pdf", label: t("invoices.menu.downloadPdf") });
        items.push({ key: "print", label: t("invoices.menu.print") });
      }
      if (canWrite && row.status === "authorized") {
        items.push({ key: "resend", label: t("invoices.menu.resend") });
      }
      if (canWrite && row.needs_attention) {
        items.push({ key: "retry", label: t("invoices.menu.retry") });
      }
      if (
        sensitive &&
        row.status === "authorized" &&
        row.action !== "credit_note"
      ) {
        items.push({
          key: "credit",
          label: t("invoices.menu.creditNote"),
          tone: "danger",
        });
      }
      return items;
    },
    [canWrite, sensitive, t],
  );

  const columns: DataTableColumn<ReceiptRow>[] = useMemo(
    () => [
      {
        key: "bill",
        header: t("invoices.table.bill"),
        render: (r) => (
          <div className="min-w-0">
            <div className="font-semibold text-ink-950">#{r.bill_id}</div>
            {r.issued_at ? (
              <div className="text-xs font-normal text-ink-500">
                {formatBusinessDateTime(r.issued_at, locale, businessTimezone, DATE_SHORT)}
              </div>
            ) : null}
          </div>
        ),
      },
      {
        key: "guest",
        header: t("invoices.table.guestTable"),
        render: (r) => {
          const guest = (r.customer_name || "").trim();
          const table = (r.table_label || "").trim();
          if (!guest && !table) {
            return <span className="text-ink-400">—</span>;
          }
          return (
            <div className="min-w-0" data-testid={`invoice-guest-table-${r.id}`}>
              {table ? (
                <div className="font-medium text-ink-900">{table}</div>
              ) : null}
              {guest ? (
                <div className="truncate text-xs text-ink-500">{guest}</div>
              ) : null}
            </div>
          );
        },
      },
      {
        key: "invoice",
        header: t("invoices.table.invoice"),
        hideBelow: "sm",
        render: (r) => (
          <div className="min-w-0">
            <div className="text-ink-900">
              {r.receipt_number || t("invoices.unnumbered")}
            </div>
            <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
              <ReceiptTypeLabel
                receiptType={r.receipt_type}
                country={r.country || fiscalCountry}
                t={t}
              />
            </div>
          </div>
        ),
      },
      {
        key: "amount",
        header: t("invoices.table.amount"),
        align: "right",
        render: (r) => (
          <span className="tabular-nums text-ink-950">
            {formatMoney(
              (r.total_amount_cents || 0) / 100,
              r.currency || currency,
              locale,
            )}
          </span>
        ),
      },
      {
        key: "status",
        header: t("invoices.table.status"),
        render: (r) => (
          <StatusBadge
            tone={statusTone(r.status)}
            label={statusLabel(r.status, t)}
            size="sm"
          />
        ),
      },
      {
        key: "delivery",
        header: t("invoices.table.delivery"),
        render: (r) =>
          r.provider === "demo" ? (
            <span
              className="inline-flex rounded-md bg-warm-100 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-ink-600"
              data-testid="demo-receipt-chip"
              title={t("invoices.delivery.demoTooltip")}
            >
              {t("invoices.delivery.demo")}
            </span>
          ) : (
            <DeliveryDotCluster
              delivery={r.delivery ?? []}
              t={t}
              receiptId={r.id}
            />
          ),
      },
    ],
    [t, locale, currency, businessTimezone, fiscalCountry],
  );

  const filterTabs = useMemo(
    () => [
      { key: "all", label: t("invoices.filters.all") },
      { key: "authorized", label: t("invoices.filters.authorized") },
      { key: "pending", label: t("invoices.filters.pending") },
      {
        key: "needs_attention",
        label: t("invoices.filters.needsAttention"),
        badge: attentionCount > 0 ? attentionCount : undefined,
      },
    ],
    [t, attentionCount],
  );

  const selectedReceipt = useMemo(() => {
    if (selectedReceiptId == null) return null;
    return (
      receipts.find((r) => r.id === selectedReceiptId) ??
      (selectedReceiptCache?.id === selectedReceiptId
        ? selectedReceiptCache
        : null)
    );
  }, [receipts, selectedReceiptId, selectedReceiptCache]);

  const closeDetail = useCallback(() => {
    setSelectedReceiptId(null);
    setSelectedReceiptCache(null);
  }, []);

  return (
    <div className="space-y-4">
      <FiscalDashboard
        businessId={
          Number.isFinite(numericBizId) && numericBizId > 0
            ? numericBizId
            : 0
        }
        businessTimezone={businessTimezone}
        canManageSensitive={sensitive}
        variant="setup-only"
      />

      <PremiumPanel className={accountingPanelClass} withTexture={false}>
        <div className={accountingPanelHeaderClass}>
          <h3 className={accountingPanelTitleClass}>{t("invoices.title")}</h3>
          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              className={accountingSecondaryButtonClass}
              onClick={() => void handleExportCsv()}
              disabled={exporting}
              aria-busy={exporting}
              data-testid="invoices-export-csv"
            >
              <Download className="h-4 w-4" aria-hidden />
              {t("invoices.exportCsv")}
            </button>
            {canWrite ? (
              <button
                type="button"
                className={accountingPrimaryButtonClass}
                onClick={() => setIssueDrawerOpen(true)}
                data-testid="invoices-issue-open"
              >
                <Plus className="h-4 w-4" aria-hidden />
                {t("invoices.issueInvoice")}
              </button>
            ) : null}
          </div>
        </div>

        <div
          className="border-b border-warm-200/80 px-4 py-3 sm:px-5"
          data-testid="invoice-status-filters"
        >
          <div className="flex flex-wrap items-center gap-3">
            <SegmentedTabs
              tabs={filterTabs}
              activeKey={filter}
              onChange={(key) => setFilter(key as InvoiceFilter)}
              size="sm"
              ariaLabel={t("invoices.filters.status")}
            />
            <label className="flex items-center gap-2 text-xs text-ink-600">
              <span className="font-medium">{t("invoices.receiptType.label")}</span>
              <select
                data-testid="receipt-type-filter"
                className="h-8 rounded-lg border border-warm-200 bg-white px-2 text-sm text-ink-900"
                value={receiptTypeFilter}
                onChange={(e) => setReceiptTypeFilter(e.target.value)}
                aria-label={t("invoices.receiptType.label")}
              >
                {typeFilterOptions.map((opt) => (
                  <option key={opt.key || "all"} value={opt.key}>
                    {t(opt.labelKey)}
                  </option>
                ))}
              </select>
            </label>
            <Input
              type="search"
              value={search}
              onValueChange={setSearch}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("invoices.searchPlaceholder")}
              aria-label={t("invoices.searchPlaceholder")}
              data-testid="invoices-search"
              startContent={
                <Search className="h-4 w-4 text-ink-400" aria-hidden />
              }
              variant="bordered"
              className="w-full max-w-xs"
              classNames={{
                inputWrapper:
                  "h-8 min-h-8 border-warm-200 bg-white data-[hover=true]:border-brand/40",
                input: "text-sm",
              }}
            />
          </div>
          {attentionCount > 0 && filter !== "needs_attention" ? (
            <span className="sr-only" data-testid="attention-count">
              {attentionCount}
            </span>
          ) : null}
        </div>

        <div className="flex flex-wrap items-center gap-3 px-4 pb-3 text-xs text-ink-500 print:hidden" data-testid="delivery-legend">
          <span className="font-medium text-ink-600">{t("invoices.delivery.legend")}:</span>
          <span className="inline-flex items-center gap-1"><span className="inline-block h-2 w-2 rounded-full bg-emerald-500" /> {t("invoices.delivery.legendDelivered")}</span>
          <span className="inline-flex items-center gap-1"><span className="inline-block h-2 w-2 rounded-full bg-amber-400" /> {t("invoices.delivery.legendPending")}</span>
          <span className="inline-flex items-center gap-1"><span className="inline-block h-2 w-2 rounded-full bg-rose-500" /> {t("invoices.delivery.legendFailed")}</span>
          <span className="inline-flex items-center gap-1"><span className="inline-block h-2 w-2 rounded-full bg-warm-300" /> {t("invoices.delivery.legendMissing")}</span>
        </div>

        <div className="px-2 py-3 sm:px-4">
          {isError ? (
            <div className="mb-3">
              <button
                type="button"
                className={accountingSecondaryButtonClass}
                onClick={() => void refetch()}
                data-testid="invoices-retry"
              >
                {t("invoices.retry")}
              </button>
            </div>
          ) : null}
          <DataTable<ReceiptRow>
            aria-label={t("invoices.title")}
            columns={columns}
            rows={receipts}
            rowKey={(row) => row.id}
            loading={
              isLoading || exporting || (isFetching && receipts.length === 0)
            }
            emptyState={
              isError
                ? {
                    icon: AlertTriangle,
                    title: t("invoices.loadError.title"),
                    subtitle: t("invoices.loadError.subtitle"),
                  }
                : {
                    icon: FileText,
                    title: t("invoices.empty.title"),
                    subtitle: t("invoices.empty.subtitle"),
                  }
            }
            pagination={{
              page,
              pageSize: PAGE_SIZE,
              total,
              onPageChange: setPage,
            }}
            onRowClick={openDetail}
            rowActions={(row) => {
              const items = menuItemsFor(row);
              if (items.length === 0) return null;
              return (
                <RowActionsMenu
                  aria-label={t("invoices.rowActionsAria", {
                    number: String(row.receipt_number || row.bill_id),
                  })}
                  data-testid={`invoice-actions-${row.id}`}
                  items={items}
                  placement="bottom-end"
                  onAction={(key) => handleRowAction(key, row)}
                />
              );
            }}
          />
        </div>
      </PremiumPanel>

      <ReceiptDetailDrawer
        open={selectedReceiptId != null && selectedReceipt != null}
        receipt={selectedReceipt}
        businessId={businessId}
        locale={locale}
        currency={currency}
        canWrite={canWrite}
        canManageSensitive={sensitive}
        businessTimezone={businessTimezone}
        onClose={closeDetail}
        t={t}
        tWith={tWith}
      />

      <IssueInvoiceDrawer
        open={issueDrawerOpen}
        onClose={() => setIssueDrawerOpen(false)}
        businessId={businessId}
        locale={locale}
        currency={currency}
        businessTimezone={businessTimezone}
        t={t}
        onViewExisting={({ receiptId, bill }) => {
          setIssueDrawerOpen(false);
          setSelectedReceiptId(receiptId);
          const fromList = receipts.find((r) => r.id === receiptId) ?? null;
          if (fromList) {
            setSelectedReceiptCache(fromList);
            return;
          }
          // Stub enough of the row for the detail drawer when the receipt is
          // outside the current list page / filters (common on demo data).
          setSelectedReceiptCache({
            id: receiptId,
            business_id: numericBizId || 0,
            settings_id: 0,
            bill_id: bill.bill_id,
            payment_id: null,
            alternative_payment_id: null,
            country: "",
            provider: "",
            action: "issue_receipt",
            receipt_type: "",
            receipt_number: null,
            provider_receipt_id: null,
            auth_code: null,
            auth_expires_at: null,
            qr_payload: null,
            qr_image_path: null,
            pdf_path: null,
            customer_doc_type: bill.customer_doc_type || null,
            customer_doc_number: bill.customer_doc_number || null,
            customer_name: bill.customer_name || null,
            total_amount_cents: Math.round((Number(bill.total_amount) || 0) * 100),
            tip_amount_cents: 0,
            currency: bill.currency || currency,
            status: "authorized",
            error_code: null,
            error_message: null,
            issued_at: bill.closed_at || null,
            created_at: bill.closed_at || new Date().toISOString(),
            updated_at: bill.closed_at || new Date().toISOString(),
            delivery: [],
            needs_attention: false,
            table_label: bill.table_label,
          });
        }}
      />

    </div>
  );
}
