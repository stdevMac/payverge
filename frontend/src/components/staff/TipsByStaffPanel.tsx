"use client";

import React, { useCallback, useEffect, useState } from "react";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { analyticsApi, type StaffTipRow } from "@/api/analytics";
import { formatCurrency } from "@/api/currency";
import { intlLocaleFor } from "@/utils/intlLocale";
import PeriodTabs from "@/components/business/shared/PeriodTabs";
import { SkeletonList } from "@/components/ui/skeletons";

type TipsPeriod = "today" | "yesterday" | "week" | "month";

const PERIODS: TipsPeriod[] = ["today", "yesterday", "week", "month"];

interface TipsByStaffPanelProps {
  businessId: string;
  currency?: string;
}

export default function TipsByStaffPanel({
  businessId,
  currency = "USD",
}: TipsByStaffPanelProps) {
  const { locale } = useSimpleLocale();
  const [period, setPeriod] = useState<TipsPeriod>("week");
  const [rows, setRows] = useState<StaffTipRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessDashboard.dashboard.staffManagement.tips.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const load = useCallback(async () => {
    setLoading(true);
    setError(false);
    try {
      const data = await analyticsApi.getTipsByStaff(businessId, period);
      setRows(Array.isArray(data) ? data : []);
    } catch {
      setError(true);
      setRows([]);
    } finally {
      setLoading(false);
    }
  }, [businessId, period]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <section
      className="rounded-2xl border border-warm-200/90 bg-white p-5 shadow-sm shadow-warm-900/5"
      data-testid="tips-by-staff-panel"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-ink-900">{t("title")}</h3>
          <p
            className="mt-0.5 text-[11px] leading-4 text-ink-500"
            data-testid="tips-by-staff-subtitle"
          >
            {t("subtitle")}
          </p>
        </div>
        <PeriodTabs
          ariaLabel={t("title")}
          value={period}
          onChange={setPeriod}
          options={PERIODS.map((p) => ({
            key: p,
            label: t(`periods.${p}`),
          }))}
        />
      </div>

      <div className="mt-4">
        {loading ? (
          <SkeletonList rows={3} ariaLabel={t("title")} />
        ) : error ? (
          <div className="flex flex-col items-center gap-2 rounded-xl border border-rose-200 bg-rose-50 px-4 py-6 text-center">
            <p className="text-sm text-rose-700">{t("error")}</p>
            <button
              type="button"
              onClick={() => void load()}
              className="rounded-lg border border-rose-300 bg-white px-3 py-1.5 text-sm font-medium text-rose-700 hover:bg-rose-100"
            >
              {t("retry")}
            </button>
          </div>
        ) : rows.length === 0 ? (
          <p className="py-6 text-center text-sm text-ink-500">{t("empty")}</p>
        ) : (
          <ul className="divide-y divide-warm-100">
            {rows.map((row) => {
              const name =
                row.staff_name?.trim() || t("unattributed");
              const key = row.staff_id ?? `unattr-${name}`;
              return (
                <li
                  key={key}
                  className="flex items-center justify-between gap-3 py-2.5 text-sm"
                >
                  <div className="min-w-0">
                    <p className="truncate font-medium text-ink-900">{name}</p>
                    <p className="text-xs text-ink-500">
                      {t("bills").replace(
                        "{count}",
                        String(row.bill_count),
                      )}
                    </p>
                  </div>
                  <p className="shrink-0 tabular-nums font-semibold text-ink-900">
                    {formatCurrency(
                      row.total_tips,
                      currency,
                      undefined,
                      intlLocaleFor(locale),
                    )}
                  </p>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </section>
  );
}
