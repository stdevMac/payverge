"use client";

import React, { useCallback, useEffect, useRef, useState } from "react";
import {
  accountingApi,
  type FoodCostReport,
} from "@/api/accounting";
import { laborCostApi, type LaborCostReport } from "@/api/laborCost";
import {
  wasteVarianceApi,
  type WasteVarianceReport,
} from "@/api/wasteVariance";
import FoodCostTable from "@/components/business/accounting/FoodCostTable";
import LaborCostCard, {
  type LaborCostPeriod,
} from "@/components/business/accounting/LaborCostCard";
import WasteVarianceCard, {
  type WasteVariancePeriod,
} from "@/components/business/accounting/WasteVarianceCard";
import {
  FoodCostKpiCard,
  foodCostTone,
  type FoodCostPeriod,
} from "@/components/business/accounting/accountingShared";
import { isLowRecipeCoverage } from "@/hooks/accounting/useCostHealth";
import { combineCostHealthCoverage } from "@/components/business/accounting/dishRecipeCoverage";
import { useDishRecipeCoverage } from "@/components/business/accounting/useDishRecipeCoverage";
import { formatMoney } from "@/components/business/accounting/format";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";

export interface CostAnalyticsSectionProps {
  businessId: string;
  currency?: string;
}

/**
 * Cost-intelligence cards (food / labor / waste) live on the Analytics tab.
 * Self-contained fetch + period controls so Accounting Overview only needs a
 * compact Cost health strip that deep-links here.
 */
export default function CostAnalyticsSection({
  businessId,
  currency = "USD",
}: CostAnalyticsSectionProps) {
  const { locale } = useSimpleLocale();

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

  const fmtMoney = useCallback(
    (value: number, cur: string): string => formatMoney(value, cur, locale),
    [locale],
  );

  const [foodCost, setFoodCost] = useState<FoodCostReport | null>(null);
  const [foodCostError, setFoodCostError] = useState(false);
  const [foodCostPeriod, setFoodCostPeriod] = useState<FoodCostPeriod>("week");
  const [foodCostLoading, setFoodCostLoading] = useState(false);

  const [wasteVariance, setWasteVariance] =
    useState<WasteVarianceReport | null>(null);
  const [wasteVarianceError, setWasteVarianceError] = useState(false);
  const [wvPeriod, setWvPeriod] = useState<WasteVariancePeriod>("week");
  const [wasteVarianceLoading, setWasteVarianceLoading] = useState(false);

  const [laborCost, setLaborCost] = useState<LaborCostReport | null>(null);
  const [laborCostError, setLaborCostError] = useState(false);
  const [laborPeriod, setLaborPeriod] = useState<LaborCostPeriod>("week");
  const [laborCostLoading, setLaborCostLoading] = useState(false);

  const foodCostPeriodRef = useRef<FoodCostPeriod>(foodCostPeriod);
  const foodCostRequestIdRef = useRef(0);
  const wvPeriodRef = useRef<WasteVariancePeriod>(wvPeriod);
  const wvRequestIdRef = useRef(0);
  const laborPeriodRef = useRef<LaborCostPeriod>(laborPeriod);
  const laborRequestIdRef = useRef(0);

  const blendedFoodCostPct = Math.round(
    (foodCost?.blended_food_cost_pct ?? 0) * 100,
  );
  // Prefer shared-denominator coverage from labor/cost-health; fall back to a
  // coarse mapped-vs-missing signal from the food-cost report itself.
  const recipeCoveragePct =
    laborCost?.recipe_coverage_pct ??
    (foodCost
      ? foodCost.items.length + foodCost.items_without_recipe > 0
        ? foodCost.items.length /
          (foodCost.items.length + foodCost.items_without_recipe)
        : null
      : null);
  const { data: dishCoverage } = useDishRecipeCoverage(businessId);
  const coverage = combineCostHealthCoverage(
    isLowRecipeCoverage(recipeCoveragePct),
    dishCoverage,
  );
  const lowCoverage = coverage.lowCoverage;
  const coveragePctLabel =
    recipeCoveragePct != null && Number.isFinite(recipeCoveragePct)
      ? Math.round(recipeCoveragePct * 100)
      : null;
  // Never paint a sub-30% food cost emerald when coverage is thin (#181/#254).
  const foodTrend = lowCoverage
    ? null
    : foodCostTone((foodCost?.blended_food_cost_pct ?? 0) as number);

  const loadFoodCost = useCallback(
    async (period: FoodCostPeriod) => {
      const requestId = (foodCostRequestIdRef.current += 1);
      setFoodCostLoading(true);
      let failed = false;
      const nextFoodCost = await accountingApi
        .getFoodCost(businessId, period)
        .catch(() => {
          failed = true;
          return null;
        });
      if (
        foodCostRequestIdRef.current === requestId &&
        foodCostPeriodRef.current === period
      ) {
        setFoodCostLoading(false);
        setFoodCostError(failed);
        if (!failed) {
          setFoodCost(
            nextFoodCost
              ? { ...nextFoodCost, items: nextFoodCost.items ?? [] }
              : null,
          );
        }
      }
    },
    [businessId],
  );

  const handleFoodCostPeriodChange = useCallback(
    (period: FoodCostPeriod) => {
      if (foodCostPeriodRef.current === period) return;
      foodCostPeriodRef.current = period;
      setFoodCostPeriod(period);
      void loadFoodCost(period);
    },
    [loadFoodCost],
  );

  const loadWasteVariance = useCallback(
    async (period: WasteVariancePeriod) => {
      const requestId = (wvRequestIdRef.current += 1);
      setWasteVarianceLoading(true);
      let failed = false;
      const nextWasteVariance = await wasteVarianceApi
        .getWasteVariance(businessId, period)
        .catch(() => {
          failed = true;
          return null;
        });
      if (
        wvRequestIdRef.current === requestId &&
        wvPeriodRef.current === period
      ) {
        setWasteVarianceLoading(false);
        setWasteVarianceError(failed);
        if (!failed) {
          setWasteVariance(nextWasteVariance);
        }
      }
    },
    [businessId],
  );

  const handleWvPeriodChange = useCallback(
    (period: WasteVariancePeriod) => {
      if (wvPeriodRef.current === period) return;
      wvPeriodRef.current = period;
      setWvPeriod(period);
      void loadWasteVariance(period);
    },
    [loadWasteVariance],
  );

  const loadLaborCost = useCallback(
    async (period: LaborCostPeriod) => {
      const requestId = (laborRequestIdRef.current += 1);
      setLaborCostLoading(true);
      let failed = false;
      const nextLaborCost = await laborCostApi
        .getLaborCost(businessId, period)
        .catch(() => {
          failed = true;
          return null;
        });
      if (
        laborRequestIdRef.current === requestId &&
        laborPeriodRef.current === period
      ) {
        setLaborCostLoading(false);
        setLaborCostError(failed);
        if (!failed) {
          setLaborCost(nextLaborCost);
        }
      }
    },
    [businessId],
  );

  const handleLaborPeriodChange = useCallback(
    (period: LaborCostPeriod) => {
      if (laborPeriodRef.current === period) return;
      laborPeriodRef.current = period;
      setLaborPeriod(period);
      void loadLaborCost(period);
    },
    [loadLaborCost],
  );

  // Initial load (default week for all three).
  useEffect(() => {
    void loadFoodCost(foodCostPeriodRef.current);
    void loadWasteVariance(wvPeriodRef.current);
    void loadLaborCost(laborPeriodRef.current);
  }, [loadFoodCost, loadWasteVariance, loadLaborCost]);

  return (
    <section className="mt-8 space-y-4" data-testid="cost-analytics-section">
      <div className="flex items-end justify-between gap-3">
        <h2 className="text-lg font-semibold text-ink-950">
          {t("overview.costHealth")}
        </h2>
      </div>

      <div className="grid gap-3 lg:grid-cols-2">
        <FoodCostKpiCard
          label={t("foodCost.cardLabel")}
          value={`${blendedFoodCostPct}%`}
          trend={foodTrend}
          hint={
            coverage.kind === "dish" && dishCoverage
              ? tWith("foodCost.dishCoverageHint", {
                  mapped: dishCoverage.mapped,
                  total: dishCoverage.total,
                })
              : lowCoverage && coveragePctLabel != null
                ? tWith("foodCost.coverageHint", { coverage: coveragePctLabel })
                : t("foodCost.blendedHint")
          }
          estimatedCogs={fmtMoney(foodCost?.estimated_cogs || 0, currency)}
          totalRevenue={fmtMoney(foodCost?.total_revenue || 0, currency)}
          hasMappedData={
            foodCostLoading || foodCostError
              ? true
              : Boolean(foodCost?.has_sales && (foodCost.total_revenue ?? 0) > 0)
          }
          selectedPeriod={foodCostPeriod}
          onPeriodChange={handleFoodCostPeriodChange}
          loading={foodCostLoading}
          error={foodCostError}
          onRetry={() => void loadFoodCost(foodCostPeriod)}
          labels={{
            periodSelector: t("foodCost.periodSelectorLabel"),
            cogs: t("foodCost.cogsLabel"),
            totalRevenue: t("foodCost.totalRevenueLabel"),
            loading: t("foodCost.states.loading"),
            error: t("states.loadError"),
            retry: t("states.retry"),
            empty:
              foodCost && !foodCost.has_sales
                ? t("foodCost.states.noSales")
                : t("foodCost.states.empty"),
            periods: {
              day: t("foodCost.periods.day"),
              week: t("foodCost.periods.week"),
              month: t("foodCost.periods.month"),
            },
          }}
        />
        <LaborCostCard
          report={laborCost}
          currency={currency}
          selectedPeriod={laborPeriod}
          onPeriodChange={handleLaborPeriodChange}
          formatMoney={fmtMoney}
          loading={laborCostLoading}
          error={laborCostError}
          onRetry={() => void loadLaborCost(laborPeriod)}
          labels={{
            title: t("laborCost.title"),
            subtitle: t("laborCost.subtitle"),
            periodSelectorLabel: t("laborCost.periodSelectorLabel"),
            periods: {
              week: t("laborCost.periods.week"),
              month: t("laborCost.periods.month"),
            },
            laborPct: t("laborCost.laborPctLabel"),
            primeCost: t("laborCost.primeCostLabel"),
            netSales: t("laborCost.netSalesLabel"),
            provenance: (count: number) =>
              count === 1
                ? t("laborCost.provenanceOne")
                : tWith("laborCost.provenance", { count }),
            target: t("laborCost.target"),
            states: {
              empty: t("laborCost.states.empty"),
              noSales: t("laborCost.states.noSales"),
              loading: t("laborCost.states.loading"),
              error: t("states.loadError"),
              retry: t("states.retry"),
              implausible: t("laborCost.states.implausible"),
            },
          }}
        />
        <div className="lg:col-span-2">
          <WasteVarianceCard
            report={wasteVariance}
            currency={currency}
            selectedPeriod={wvPeriod}
            onPeriodChange={handleWvPeriodChange}
            formatMoney={fmtMoney}
            loading={wasteVarianceLoading}
            error={wasteVarianceError}
            onRetry={() => void loadWasteVariance(wvPeriod)}
            labels={{
              title: t("wasteVariance.title"),
              subtitle: t("wasteVariance.subtitle"),
              periodSelectorLabel: t("wasteVariance.periodSelectorLabel"),
              periods: {
                day: t("wasteVariance.periods.day"),
                week: t("wasteVariance.periods.week"),
                month: t("wasteVariance.periods.month"),
              },
              trackedLoss: t("wasteVariance.trackedLoss"),
              reasons: {
                spoilage: t("wasteVariance.reasons.spoilage"),
                count_shrink: t("wasteVariance.reasons.count_shrink"),
                manual: t("wasteVariance.reasons.manual"),
              },
              inactive: t("wasteVariance.inactive"),
              table: {
                ingredient: t("wasteVariance.table.ingredient"),
                theoretical: t("wasteVariance.table.theoretical"),
                actual: t("wasteVariance.table.actual"),
                variance: t("wasteVariance.table.variance"),
                varianceCost: t("wasteVariance.table.varianceCost"),
              },
              states: {
                empty: t("wasteVariance.states.empty"),
                sparse: t("wasteVariance.states.sparse"),
                needsRecipe: t("wasteVariance.states.needsRecipe"),
                loading: t("wasteVariance.states.loading"),
                error: t("states.loadError"),
                retry: t("states.retry"),
              },
            }}
          />
        </div>
      </div>

      {foodCost && foodCost.items_missing_cost > 0 ? (
        <p className="mt-2 text-[11px] leading-tight text-amber-700">
          {tWith("foodCost.missingCostNudge", {
            count: foodCost.items_missing_cost,
          })}
        </p>
      ) : null}
      {foodCost && foodCost.items.length === 0 ? (
        <p className="mt-2 text-[11px] leading-tight text-ink-500">
          {t("foodCost.noRecipes")}
        </p>
      ) : null}
      {foodCost ? (
        <FoodCostTable
          items={foodCost.items}
          currency={currency}
          formatMoney={fmtMoney}
          labels={{
            title: t("foodCost.table.title"),
            item: t("foodCost.table.item"),
            unitCost: t("foodCost.table.unitCost"),
            price: t("foodCost.table.price"),
            foodCostPct: t("foodCost.table.foodCostPct"),
            margin: t("foodCost.table.margin"),
            qty: t("foodCost.table.qty"),
          }}
        />
      ) : null}
    </section>
  );
}
