"use client";

import React, { useCallback, useState } from "react";
import { Download, Printer } from "lucide-react";
import type { Locale } from "@/i18n/localeRegistry";
import { useProfitLoss } from "@/hooks/accounting/useAccountingQueries";
import { formatMoney } from "@/components/business/accounting/format";
import { exportLocalizedCsv } from "@/utils/exportLocalizedCsv";
import { PNL_CSV_COLUMNS } from "@/utils/csvColumnManifests/pnl";
import { PremiumPanel } from "../premium";
import {
  accountingPanelClass,
  accountingPanelHeaderClass,
  accountingPanelTitleClass,
  accountingSecondaryButtonClass,
  formatCategoryLabel,
} from "./accountingShared";
import PeriodLockCard from "./PeriodLockCard";

export type ReportsTabProps = {
  businessId: string;
  start: string;
  end: string;
  locale: Locale;
  currency: string;
  /** Owner can close/reopen books. */
  canOwn?: boolean;
  t: (key: string, params?: Record<string, string | number>) => string;
};

function Row({
  label,
  amount,
  compare,
  delta,
  currency,
  locale,
  bold,
}: {
  label: string;
  amount: number;
  compare?: number | null;
  delta?: number | null;
  currency: string;
  locale: Locale;
  bold?: boolean;
}) {
  const cls = bold ? "font-semibold text-ink-950" : "text-ink-800";
  return (
    <tr className="border-b border-warm-100 last:border-0">
      <td className={`py-2 pr-3 text-sm ${cls}`}>{label}</td>
      <td className={`py-2 text-right text-sm tabular-nums ${cls}`}>
        {formatMoney(amount, currency, locale)}
      </td>
      {compare != null ? (
        <td className="py-2 text-right text-sm tabular-nums text-ink-600">
          {formatMoney(compare, currency, locale)}
        </td>
      ) : null}
      {delta != null ? (
        <td
          className={`py-2 text-right text-sm tabular-nums ${
            delta < 0
              ? "text-rose-700"
              : delta > 0
                ? "text-emerald-700"
                : "text-ink-500"
          }`}
        >
          {formatMoney(delta, currency, locale)}
        </td>
      ) : null}
    </tr>
  );
}

export default function ReportsTab({
  businessId,
  start,
  end,
  locale,
  currency,
  canOwn = false,
  t,
}: ReportsTabProps) {
  const [compare, setCompare] = useState(true);
  const query = useProfitLoss(businessId, { start, end }, compare);
  const stmt = query.data ?? null;
  const loading = query.isPending || (query.isFetching && !stmt);
  const error = query.isError ? t("reports.loadError") : null;

  const handlePrint = useCallback(() => {
    window.print();
  }, []);

  const cur = stmt?.current;
  const prev = stmt?.previous;
  const delta = stmt?.delta;
  const curCurrency = cur?.currency || currency;

  // L6-25 / S-14: client-side CSV so compare toggle + localized section labels apply.
  const handleExportCsv = () => {
    if (!cur) return;
    const rows: Record<string, unknown>[] = [];
    const push = (section: string, label: string, current: number, previous: number | null) => {
      const d = previous != null ? current - previous : null;
      rows.push({
        section,
        label,
        current: current.toFixed(2),
        previous: previous != null && compare ? previous.toFixed(2) : "",
        delta: d != null && compare ? d.toFixed(2) : "",
      });
    };
    push(t("reports.sections.revenue"), t("reports.lines.revenue"), Number(cur.revenue) || 0, prev ? Number(prev.revenue) || 0 : null);
    push(t("reports.sections.revenue"), t("reports.lines.otherIncome"), Number(cur.other_income) || 0, prev ? Number(prev.other_income) || 0 : null);
    push(t("reports.sections.costs"), t("reports.lines.cogs"), Number(cur.cogs) || 0, prev ? Number(prev.cogs) || 0 : null);
    push(t("reports.sections.costs"), t("reports.lines.labor"), Number(cur.labor) || 0, prev ? Number(prev.labor) || 0 : null);
    push(t("reports.sections.costs"), t("reports.lines.opex"), Number(cur.opex) || 0, prev ? Number(prev.opex) || 0 : null);
    push(t("reports.sections.net"), t("reports.lines.net"), Number(cur.net) || 0, prev ? Number(prev.net) || 0 : null);
    exportLocalizedCsv({
      columns: PNL_CSV_COLUMNS,
      rows,
      filename: `pnl-${start}-${end}.csv`,
      t,
    });
  };

  return (
    <div className="space-y-4" data-testid="reports-tab">
      <PeriodLockCard businessId={businessId} canOwn={canOwn} t={t} />
      <PremiumPanel
        className={`${accountingPanelClass} print:shadow-none print:border-0`}
        withTexture={false}
      >
        <div className={`${accountingPanelHeaderClass} print:hidden`}>
          <h3 className={accountingPanelTitleClass}>{t("reports.title")}</h3>
          <div className="flex flex-wrap items-center gap-2">
            <label className="flex items-center gap-2 text-sm text-ink-600">
              <input
                type="checkbox"
                checked={compare}
                onChange={(e) => setCompare(e.target.checked)}
                className="rounded border-warm-300"
              />
              {t("reports.comparePrev")}
            </label>
            <button
              type="button"
              className={accountingSecondaryButtonClass}
              onClick={handleExportCsv}
              data-testid="pnl-export-csv"
              disabled={!cur}
            >
              <Download className="h-4 w-4" aria-hidden />
              {t("reports.exportCsv")}
            </button>
            <button
              type="button"
              className={accountingSecondaryButtonClass}
              onClick={handlePrint}
              data-testid="pnl-print"
            >
              <Printer className="h-4 w-4" aria-hidden />
              {t("reports.print")}
            </button>
          </div>
        </div>

        <div className="px-4 py-4 sm:px-5" data-testid="pnl-statement">
          <p className="mb-3 text-sm text-ink-500">
            {t("reports.period")}: {start} → {end}
          </p>

          {loading ? (
            <p className="text-sm text-ink-500">{t("reports.loading")}</p>
          ) : error ? (
            <p role="alert" className="text-sm text-rose-700">
              {error}
            </p>
          ) : cur ? (
            <div
              data-testid="pnl-table-scroller"
              role="region"
              tabIndex={0}
              aria-label={t("reports.title")}
              className="min-w-0 max-w-full overflow-x-auto"
            >
            <table className="w-full min-w-[20rem] border-collapse">
              <thead>
                <tr className="border-b border-warm-200 text-left text-xs font-semibold uppercase tracking-wide text-ink-500">
                  <th className="py-2 pr-3">{t("reports.line")}</th>
                  <th className="py-2 text-right">{t("reports.current")}</th>
                  {prev ? (
                    <th className="py-2 text-right">{t("reports.previous")}</th>
                  ) : null}
                  {delta ? (
                    <th className="py-2 text-right">{t("reports.delta")}</th>
                  ) : null}
                </tr>
              </thead>
              <tbody>
                <Row
                  label={t("reports.revenue")}
                  amount={Number(cur.revenue) || 0}
                  compare={prev ? Number(prev.revenue) || 0 : null}
                  delta={delta ? Number(delta.revenue) || 0 : null}
                  currency={curCurrency}
                  locale={locale}
                />
                <Row
                  label={t("reports.otherIncome")}
                  amount={Number(cur.other_income) || 0}
                  compare={prev ? Number(prev.other_income) || 0 : null}
                  delta={delta ? Number(delta.other_income) || 0 : null}
                  currency={curCurrency}
                  locale={locale}
                />
                {(cur.other_income_by_category || []).map((line) => (
                  <Row
                    key={`oi-${line.category}`}
                    label={`  ${formatCategoryLabel(line.category, t)}`}
                    amount={Number(line.amount) || 0}
                    currency={curCurrency}
                    locale={locale}
                  />
                ))}
                <Row
                  label={t("reports.cogs")}
                  amount={Number(cur.cogs) || 0}
                  compare={prev ? Number(prev.cogs) || 0 : null}
                  delta={delta ? Number(delta.cogs) || 0 : null}
                  currency={curCurrency}
                  locale={locale}
                />
                <Row
                  label={t("reports.labor")}
                  amount={Number(cur.labor) || 0}
                  compare={prev ? Number(prev.labor) || 0 : null}
                  delta={delta ? Number(delta.labor) || 0 : null}
                  currency={curCurrency}
                  locale={locale}
                />
                <Row
                  label={t("reports.opex")}
                  amount={Number(cur.opex) || 0}
                  compare={prev ? Number(prev.opex) || 0 : null}
                  delta={delta ? Number(delta.opex) || 0 : null}
                  currency={curCurrency}
                  locale={locale}
                />
                {(cur.opex_by_category || []).map((line) => (
                  <Row
                    key={`ox-${line.category}`}
                    label={`  ${formatCategoryLabel(line.category, t)}`}
                    amount={Number(line.amount) || 0}
                    currency={curCurrency}
                    locale={locale}
                  />
                ))}
                <Row
                  label={t("reports.net")}
                  amount={Number(cur.net) || 0}
                  compare={prev ? Number(prev.net) || 0 : null}
                  delta={delta ? Number(delta.net) || 0 : null}
                  currency={curCurrency}
                  locale={locale}
                  bold
                />
              </tbody>
            </table>
            </div>
          ) : (
            <p className="text-sm text-ink-500">{t("reports.empty")}</p>
          )}
        </div>
      </PremiumPanel>
    </div>
  );
}
