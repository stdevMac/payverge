"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";

import React, { useState, useEffect, useCallback } from "react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import { Select, SelectItem } from "@nextui-org/react";
import { TrendingDown, Package } from "lucide-react";
import { analyticsApi, ItemStats } from "@/api/analytics";
import { getBusiness } from "@/api/business";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { intlLocaleFor } from "@/utils/intlLocale";
import PeriodTabs from "@/components/business/shared/PeriodTabs";
import { MetricStat } from "@/components/dashboard/charts/MetricStat";
import { RankedBarList } from "@/components/dashboard/charts/RankedBarList";
import { ANALYTICS_BAR_HUE } from "@/components/dashboard/charts/analyticsTheme";

interface MenuPanelProps {
  businessId: string;
  /** Wave 4: open Menu Builder focused on an item name. */
  onNavigateToTab?: (tab: string) => void;
  /** Fix 7: currency threaded from the dashboard page (no re-fetch). */
  currency?: string;
  /** Fix 4: shared analytics period (controlled) + change reporter. */
  period?: string;
  onPeriodChange?: (period: string) => void;
}

// Canonical period keys — `as const` so the period state and PeriodTabs<K>
// instantiate on the narrow key union instead of string.
const MENU_PERIOD_KEYS = [
  "today",
  "yesterday",
  "week",
  "month",
  "quarter",
] as const;
type MenuPeriodKey = (typeof MENU_PERIOD_KEYS)[number];

const isMenuPeriodKey = (v: string | undefined): v is MenuPeriodKey =>
  !!v && (MENU_PERIOD_KEYS as readonly string[]).includes(v);

export default function MenuPanel({
  businessId,
  onNavigateToTab,
  currency: currencyProp,
  period: periodProp,
  onPeriodChange,
}: MenuPanelProps) {
  // Translation setup — ported verbatim from ItemAnalytics.tsx
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const tString = (key: string): string => {
    const fullKey = `businessDashboard.dashboard.itemAnalytics.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  // Business currency — prefer the threaded prop (Fix 7); only fetch when absent.
  const [fetchedCurrency, setFetchedCurrency] = useState<string>("USD");
  const businessCurrency = currencyProp ?? fetchedCurrency;

  useEffect(() => {
    let cancelled = false;
    if (!businessId || currencyProp !== undefined) return;
    const numericId = Number(businessId);
    if (!Number.isFinite(numericId)) return;
    getBusiness(numericId)
      .then((biz) => {
        if (!cancelled && biz?.default_currency) {
          setFetchedCurrency(biz.default_currency);
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [businessId, currencyProp]);

  const formatCurrency = (amount: number) =>
    formatCurrencyIntl(amount, businessCurrency, undefined, intlLocaleFor(locale));

  // Items state + fetch — ported verbatim from ItemAnalytics.tsx
  const [items, setItems] = useState<ItemStats[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [localPeriod, setLocalPeriod] = useState<MenuPeriodKey>("week");
  const period: MenuPeriodKey = isMenuPeriodKey(periodProp) ? periodProp : localPeriod;
  const setPeriod = (key: MenuPeriodKey) => {
    setLocalPeriod(key);
    onPeriodChange?.(key);
  };
  const [sortBy, setSortBy] = useState("total_sold");

  // Fix 10: the item-detail list rendered EVERY item as a full card below the
  // top-10 — a menu with hundreds of items painted hundreds of cards unbounded.
  // Cap the rendered detail cards and reveal more on demand. (The backend now
  // also bounds item analytics at 100; this bounds the DOM further.)
  const ITEM_DETAIL_PAGE = 20;
  const [visibleCount, setVisibleCount] = useState(ITEM_DETAIL_PAGE);
  // Reset the reveal when the data set changes (period/sort switch).
  useEffect(() => {
    setVisibleCount(ITEM_DETAIL_PAGE);
  }, [period, sortBy]);

  const fetchItemAnalytics = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await analyticsApi.getItemAnalytics(businessId, period);
      setItems(data || []);
    } catch (err) {
      console.error("MenuPanel - API Error:", err);
      setError(getSafeApiErrorMessage(err, "An error occurred"));
    } finally {
      setLoading(false);
    }
  }, [businessId, period]);

  useEffect(() => {
    fetchItemAnalytics();
  }, [fetchItemAnalytics]);

  // sortedItems + maxValue — ported verbatim from ItemAnalytics.tsx
  const sortedItems = [...items].sort((a, b) => {
    switch (sortBy) {
      case "total_sold":
        return (b.total_sold || 0) - (a.total_sold || 0);
      case "revenue":
        return (b.revenue || 0) - (a.revenue || 0);
      case "bills_featured":
        return (b.bills_featured || 0) - (a.bills_featured || 0);
      case "avg_price":
        return (b.avg_price || 0) - (a.avg_price || 0);
      default:
        return 0;
    }
  });

  const maxValue = Math.max(
    0,
    ...items.map((item) => {
      switch (sortBy) {
        case "total_sold":
          return item.total_sold || 0;
        case "revenue":
          return item.revenue || 0;
        case "bills_featured":
          return item.bills_featured || 0;
        case "avg_price":
          return item.avg_price || 0;
        default:
          return 0;
      }
    }),
  );

  // categoryStats + topCategories — ported verbatim from ItemAnalytics.tsx
  const categoryStats = items.reduce(
    (acc, item) => {
      if (!acc[item.category]) {
        acc[item.category] = { total_sold: 0, revenue: 0, item_count: 0 };
      }
      acc[item.category].total_sold += item.total_sold || 0;
      acc[item.category].revenue += item.revenue || 0;
      acc[item.category].item_count += 1;
      return acc;
    },
    {} as Record<string, { total_sold: number; revenue: number; item_count: number }>,
  );

  const topCategories = Object.entries(categoryStats)
    .sort(([, a], [, b]) => b.revenue - a.revenue)
    .slice(0, 5);

  // periods (keys from the canonical tuple, labels from `periods.<key>`
  // i18n entries) + sortOptions ported verbatim from ItemAnalytics.tsx.
  const periods = MENU_PERIOD_KEYS.map((key) => ({
    key,
    label: tString(`periods.${key}`),
  }));

  const sortOptions = [
    { key: "total_sold", label: tString("sortOptions.quantitySold") },
    { key: "revenue", label: tString("sortOptions.revenue") },
    { key: "bills_featured", label: tString("sortOptions.billsFeatured") },
    { key: "avg_price", label: tString("sortOptions.averagePrice") },
  ];

  // Loading / error early returns — ported verbatim from ItemAnalytics.tsx
  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <div className="text-center">
          <div className="w-16 h-16 bg-warm-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-warm-100">
            <div className="w-8 h-8 border-2 border-warm-300 border-t-ink-900 rounded-full animate-spin"></div>
          </div>
          <p className="text-ink-600 tracking-wide">{tString("loading")}</p>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 rounded-lg p-4">
        <div className="flex items-center gap-3">
          <TrendingDown className="w-5 h-5 text-red-600 flex-shrink-0" />
          <p className="text-red-800 font-medium">
            {tString("error")}: {error}
          </p>
        </div>
      </div>
    );
  }

  // Value extractor for current sort dimension
  const getSortValue = (item: ItemStats): number => {
    switch (sortBy) {
      case "total_sold": return item.total_sold || 0;
      case "revenue": return item.revenue || 0;
      case "bills_featured": return item.bills_featured || 0;
      case "avg_price": return item.avg_price || 0;
      default: return 0;
    }
  };

  const formatForSort = (n: number) =>
    sortBy === "revenue" || sortBy === "avg_price"
      ? formatCurrency(n)
      : n.toLocaleString(intlLocaleFor(locale));

  // Top-items and category RankedBarList data
  const topItems = sortedItems.slice(0, 10).map((it) => ({
    label: it.item_name,
    value: getSortValue(it),
    sublabel: it.category,
  }));

  const categoryItems = topCategories.map(([name, stats]) => ({
    label: name,
    value: stats.revenue,
    sublabel: `${stats.item_count}`,
  }));

  const topItemsHeading = tString("topItems.heading");
  const categoryRevenueTitle = tString("categoryRevenue.title");

  return (
    <div className="space-y-6">
      {/* Header with canonical PeriodTabs + sort Select */}
      <div className="flex flex-col sm:flex-row sm:justify-between sm:items-center gap-4">
        <div>
          <h2 className="text-xl font-semibold text-ink-900">
            {tString("title")}
          </h2>
          <p className="text-sm text-ink-600">{tString("subtitle")}</p>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <PeriodTabs
            options={periods}
            value={period}
            onChange={setPeriod}
            ariaLabel={tString("period")}
          />
          <Select
            size="sm"
            variant="bordered"
            aria-label={tString("sortBy")}
            selectedKeys={[sortBy]}
            onSelectionChange={(keys) =>
              setSortBy(Array.from(keys)[0] as string)
            }
            className="w-full sm:w-48"
          >
            {sortOptions.map((opt) => (
              <SelectItem key={opt.key} value={opt.key}>
                {opt.label}
              </SelectItem>
            ))}
          </Select>
        </div>
      </div>

      {items.length === 0 ? (
        <div className="text-center py-12">
          <div className="w-16 h-16 bg-warm-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-warm-100">
            <Package className="w-8 h-8 text-ink-400" />
          </div>
          <h3 className="text-lg text-ink-900 tracking-wide mb-2">
            {tString("noItemData")}
          </h3>
          <p className="text-ink-600 text-sm">{tString("noSalesData")}</p>
        </div>
      ) : (
        <>
          {/* Summary metric stats */}
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <MetricStat
              label={tString("totalItems")}
              value={items.length}
            />
            <MetricStat
              label={tString("totalRevenue")}
              value={formatCurrency(
                items.reduce((s, i) => s + (i.revenue || 0), 0),
              )}
            />
            <MetricStat
              label={tString("itemsSold")}
              value={items.reduce((s, i) => s + (i.total_sold || 0), 0)}
            />
          </div>

          {/* Ranked bar lists: top items + category revenue */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
            <section className="rounded-2xl border border-warm-200 bg-white p-4">
              <h3 className="mb-3 text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
                {topItemsHeading}
              </h3>
              <RankedBarList
                items={topItems}
                formatValue={formatForSort}
                ariaLabel={topItemsHeading}
                emptyLabel={tString("noData")}
              />
            </section>
            <section className="rounded-2xl border border-warm-200 bg-white p-4">
              <h3 className="mb-3 text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
                {categoryRevenueTitle}
              </h3>
              <RankedBarList
                items={categoryItems}
                formatValue={(n) => formatCurrency(n)}
                ariaLabel={categoryRevenueTitle}
                emptyLabel={tString("noData")}
              />
            </section>
          </div>

          {/* Item detail table — rank-coloring removed; single-hue zero-baseline bar */}
          <div className="bg-white border border-warm-200 rounded-lg shadow-sm">
            <div className="border-b border-warm-200 p-4">
              <div className="flex justify-between items-center">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 bg-warm-50 rounded-xl flex items-center justify-center border border-warm-100">
                    <Package className="w-5 h-5 text-ink-600" />
                  </div>
                  <h3 className="text-lg text-ink-900 tracking-wide">
                    {tString("itemPerformance")}
                  </h3>
                </div>
                <div className="text-sm text-ink-600">
                  {items.length} {tString("totalItems")}
                </div>
              </div>
            </div>
            <div className="p-4">
              <div className="space-y-4">
                {sortedItems.slice(0, visibleCount).map((item) => {
                  const currentValue = getSortValue(item);
                  const progressValue =
                    maxValue > 0 ? (currentValue / maxValue) * 100 : 0;

                  return (
                    <div
                      key={item.item_id || item.item_name}
                      className="p-4 bg-warm-50 rounded-lg hover:bg-warm-100 transition-colors"
                    >
                      <div className="flex items-center justify-between mb-3">
                        <div>
                          <h4 className="font-semibold text-ink-900">
                            {item.item_name}
                          </h4>
                          <p className="text-sm text-ink-600">
                            {item.category}
                          </p>
                          {onNavigateToTab ? (
                            <button
                              type="button"
                              onClick={() =>
                                onNavigateToTab(
                                  `menu?menuSearch=${encodeURIComponent(item.item_name)}`,
                                )
                              }
                              className="mt-1 rounded text-xs font-medium text-brand underline decoration-brand/40 underline-offset-2 hover:text-brand-dark focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
                            >
                              {tString("viewInMenuBuilder")}
                            </button>
                          ) : null}
                        </div>

                        <div className="text-right">
                          <p className="font-semibold text-lg text-ink-900">
                            {formatForSort(currentValue)}
                          </p>
                          <p className="text-xs text-ink-600 uppercase tracking-wide">
                            {sortOptions.find((opt) => opt.key === sortBy)?.label}
                          </p>
                        </div>
                      </div>

                      {/* Single-hue zero-baseline bar — no rank-coloring */}
                      <div className="w-full bg-warm-100 rounded-full h-2 mb-3 overflow-hidden">
                        <div
                          className="h-2 rounded-full transition-all duration-300"
                          style={{
                            width: `${progressValue}%`,
                            backgroundColor: ANALYTICS_BAR_HUE,
                            opacity: 0.5,
                          }}
                        />
                      </div>

                      {/* Per-item Sold / Revenue / Avg / Bills grid */}
                      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
                        <div>
                          <p className="text-ink-600 text-xs uppercase tracking-wide">
                            {tString("metrics.sold")}
                          </p>
                          <p className="font-semibold text-ink-900">
                            {item.total_sold || 0}
                          </p>
                        </div>
                        <div>
                          <p className="text-ink-600 text-xs uppercase tracking-wide">
                            {tString("metrics.revenue")}
                          </p>
                          <p className="font-semibold text-ink-900">
                            {formatCurrency(item.revenue || 0)}
                          </p>
                        </div>
                        <div>
                          <p className="text-ink-600 text-xs uppercase tracking-wide">
                            {tString("metrics.avgPrice")}
                          </p>
                          <p className="font-semibold text-ink-900">
                            {formatCurrency(item.avg_price || 0)}
                          </p>
                        </div>
                        <div>
                          <p className="text-ink-600 text-xs uppercase tracking-wide">
                            {tString("metrics.billsFeatured")}
                          </p>
                          <p className="font-semibold text-ink-900">
                            {item.bills_featured || 0}
                          </p>
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
              {sortedItems.length > visibleCount && (
                <div className="mt-4 flex justify-center">
                  <button
                    type="button"
                    onClick={() =>
                      setVisibleCount((c) => c + ITEM_DETAIL_PAGE)
                    }
                    className="inline-flex items-center gap-2 rounded-lg border border-warm-200 bg-white px-4 py-2 text-sm font-medium text-ink-600 hover:bg-warm-50"
                  >
                    {tString("showMore").replace(
                      "{count}",
                      Math.min(
                        ITEM_DETAIL_PAGE,
                        sortedItems.length - visibleCount,
                      ).toString(),
                    )}
                  </button>
                </div>
              )}
            </div>
          </div>
        </>
      )}
    </div>
  );
}
