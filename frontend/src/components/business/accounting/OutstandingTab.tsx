"use client";

import React, { useCallback, useEffect, useMemo, useState } from "react";
import dynamic from "next/dynamic";
import toast from "react-hot-toast";
import { Banknote } from "lucide-react";
import type { Locale } from "@/i18n/localeRegistry";
import type { UnpaidBill } from "@/api/accounting";
import { getBill, type BillWithItemsResponse } from "@/api/bills";
import { getTranslation } from "@/i18n/SimpleTranslationProvider";
import { useUnpaidBills } from "@/hooks/accounting/useAccountingQueries";
import { formatDay, formatMoney } from "@/components/business/accounting/format";
import DataTable, { type DataTableColumn } from "../shared/DataTable";
import { PremiumPanel } from "../premium";
import {
  accountingPanelClass,
  accountingPanelHeaderClass,
  accountingPanelTitleClass,
} from "./accountingShared";

const PAGE_SIZE = 25;
const BILL_HISTORY_LIMIT = 50;

// L6-23 / L6-3: row drilldown reuses operator BillDetailsModal.
const BillDetailsModal = dynamic(
  () =>
    import("@/components/business/BillDetailsModal").then(
      (mod) => mod.BillDetailsModal,
    ),
  { ssr: false },
);

export default function OutstandingTab({
  businessId,
  start,
  end,
  locale,
  currency,
  t,
  businessTimezone = null,
  country = null,
}: {
  businessId: string;
  start: string;
  end: string;
  locale: Locale;
  currency: string;
  t: (key: string, params?: Record<string, string | number>) => string;
  businessTimezone?: string | null;
  country?: string | null;
}) {
  const [page, setPage] = useState(1);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [detailsBill, setDetailsBill] = useState<BillWithItemsResponse | null>(
    null,
  );
  const [detailsLoadingId, setDetailsLoadingId] = useState<number | null>(null);

  // New window restarts pagination.
  useEffect(() => {
    setPage(1);
  }, [start, end]);

  const query = useUnpaidBills(businessId, {
    start,
    end,
    page,
    page_size: PAGE_SIZE,
  });
  const bills = query.data?.bills ?? [];
  const total = query.data?.total ?? 0;

  const billTString = useCallback(
    (key: string): string => {
      const result = getTranslation(`billManager.${key}`, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const openBillDetails = useCallback(
    async (billId: number) => {
      if (!Number.isFinite(billId) || billId <= 0) return;
      setDetailsLoadingId(billId);
      try {
        const bill = await getBill(billId, undefined, BILL_HISTORY_LIMIT);
        setDetailsBill(bill);
        setDetailsOpen(true);
      } catch {
        toast.error(t("outstanding.viewError"));
      } finally {
        setDetailsLoadingId(null);
      }
    },
    [t],
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
      void query.refetch();
    } catch {
      // Keep the open modal; list will refresh on next load.
    }
  }, [detailsBill?.bill?.id, query]);

  const columns: DataTableColumn<UnpaidBill>[] = useMemo(
    () => [
      {
        key: "bill",
        header: t("outstanding.bill"),
        render: (b) => (
          <span className="font-medium text-ink-900">#{b.id}</span>
        ),
      },
      {
        key: "table",
        header: t("outstanding.table"),
        hideBelow: "sm",
        render: (b) => (
          <span className="text-ink-600">{b.table_label || "—"}</span>
        ),
      },
      {
        key: "outstanding",
        header: t("outstanding.outstanding"),
        align: "right",
        render: (b) => (
          <span className="tabular-nums text-ink-900">
            {formatMoney(b.outstanding, b.currency || currency, locale)}
          </span>
        ),
      },
      {
        key: "age",
        header: t("outstanding.age"),
        align: "right",
        render: (b) => (
          <span className="rounded-md bg-amber-50 px-1.5 py-0.5 text-xs font-medium text-amber-800">
            {t("outstanding.ageDays", { days: b.age_days })}
          </span>
        ),
      },
    ],
    [t, currency, locale],
  );

  return (
    <div data-testid="outstanding-tab">
      <PremiumPanel className={accountingPanelClass} withTexture={false}>
        <div className={accountingPanelHeaderClass}>
          <div className="min-w-0">
            <h3 className={accountingPanelTitleClass}>
              {t("outstanding.title")}
            </h3>
            {total > 0 ? (
              <p
                className="mt-1 text-xs leading-tight text-warm-600"
                data-testid="outstanding-scope"
              >
                {t("outstanding.asOf", { end: formatDay(end, locale) })}
              </p>
            ) : null}
          </div>
          {total > 0 ? (
            <span className="text-xs tabular-nums text-warm-600">
              {t("outstanding.count", { count: total })}
            </span>
          ) : null}
        </div>
        <div className="px-2 py-3 sm:px-4">
          {query.isError ? (
            <p role="alert" className="px-2 text-sm text-rose-700">
              {t("outstanding.loadError")}
            </p>
          ) : (
            <DataTable<UnpaidBill>
              aria-label={t("outstanding.title")}
              columns={columns}
              rows={bills}
              rowKey={(b) => b.id}
              loading={
                query.isPending || (query.isFetching && bills.length === 0)
              }
              emptyState={{
                icon: Banknote,
                title: t("outstanding.empty"),
              }}
              onRowClick={(b) => {
                void openBillDetails(b.id);
              }}
              pagination={{
                page,
                pageSize: PAGE_SIZE,
                total,
                onPageChange: setPage,
              }}
            />
          )}
          {detailsLoadingId != null ? (
            <p className="sr-only" role="status">
              {t("outstanding.loadingBill")}
            </p>
          ) : null}
        </div>
      </PremiumPanel>

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
        }}
        businessId={Number(businessId)}
        tString={billTString}
        currency={currency}
        businessTimezone={businessTimezone}
        country={country}
        mode="operator"
      />
    </div>
  );
}
