import { useQuery } from "@tanstack/react-query";
import {
  laborCostApi,
  type CostHealthStatus,
  type LaborCostReport,
} from "@/api/laborCost";
import type { AccountingBizId, AccountingRange } from "./useAccountingQueries";
import { accountingKeys } from "./useAccountingQueries";

function bizId(businessId: AccountingBizId): string {
  return String(businessId);
}

function isBizEnabled(businessId: AccountingBizId): boolean {
  if (typeof businessId === "number") return businessId > 0;
  const trimmed = businessId.trim();
  return trimmed.length > 0 && trimmed !== "0";
}

const costHealthKeys = {
  report: (biz: AccountingBizId, r: AccountingRange) =>
    [...accountingKeys.root(biz), "cost-health", r] as const,
};

/**
 * Below this share of recognized sales with recipe-mapped costs, food/prime
 * ratios are directional only — never present as a healthy restaurant KPI.
 */
const RECIPE_COVERAGE_TRUST_THRESHOLD = 0.5;

/** Normalized cost-health view for Overview + Analytics strip/cards. */
export type CostHealthView = {
  status: CostHealthStatus;
  reason?: string;
  foodPct: number | null;
  laborPct: number | null;
  primePct: number | null;
  recipeCoveragePct: number | null;
  /** True when recipe coverage is too thin to trust food/prime as healthy. */
  lowCoverage: boolean;
  report: LaborCostReport | null;
};

export function isLowRecipeCoverage(
  recipeCoveragePct: number | null | undefined,
): boolean {
  return (
    recipeCoveragePct != null &&
    Number.isFinite(recipeCoveragePct) &&
    recipeCoveragePct < RECIPE_COVERAGE_TRUST_THRESHOLD
  );
}

/**
 * Maps a labor-cost response into display-safe cost-health fields.
 * Implausible / insufficient never surface as healthy green percentages.
 */
export function toCostHealthView(
  report: LaborCostReport | null | undefined,
): CostHealthView {
  if (!report) {
    return {
      status: "insufficient_data",
      foodPct: null,
      laborPct: null,
      primePct: null,
      recipeCoveragePct: null,
      lowCoverage: false,
      report: null,
    };
  }

  const status: CostHealthStatus = report.status ?? inferLegacyStatus(report);
  const recipeCoveragePct = report.recipe_coverage_pct ?? null;
  const lowCoverage = isLowRecipeCoverage(recipeCoveragePct);

  if (status === "insufficient_data") {
    return {
      status,
      reason: report.reason,
      foodPct: null,
      laborPct: null,
      primePct: null,
      recipeCoveragePct,
      lowCoverage,
      report,
    };
  }

  if (status === "implausible") {
    // Surface the impossible ratio (never clamp) but do not invent a prime%.
    return {
      status,
      reason: report.reason,
      foodPct: Number.isFinite(report.food_cost_pct) ? report.food_cost_pct : null,
      laborPct: Number.isFinite(report.labor_cost_pct)
        ? report.labor_cost_pct
        : null,
      primePct: null,
      recipeCoveragePct,
      lowCoverage,
      report,
    };
  }

  // status === "ok"
  const hasSales = (report.net_sales ?? 0) > 0;
  const hasLabor = report.has_data === true;
  return {
    status,
    reason: report.reason,
    foodPct:
      hasSales && Number.isFinite(report.food_cost_pct)
        ? report.food_cost_pct
        : null,
    laborPct:
      hasLabor && hasSales && Number.isFinite(report.labor_cost_pct)
        ? report.labor_cost_pct
        : null,
    primePct:
      hasSales &&
      report.prime_cost_pct != null &&
      Number.isFinite(report.prime_cost_pct)
        ? report.prime_cost_pct
        : null,
    recipeCoveragePct,
    lowCoverage,
    report,
  };
}

/** Pre-status servers still returned prime = labor + food; treat as ok when sane. */
function inferLegacyStatus(report: LaborCostReport): CostHealthStatus {
  if ((report.net_sales ?? 0) <= 0) return "insufficient_data";
  if (
    report.labor_cost_pct > 1 ||
    report.food_cost_pct > 1 ||
    (report.prime_cost_pct ?? 0) > 1
  ) {
    return "implausible";
  }
  return "ok";
}

/**
 * Shared cost-health query: one labor-cost fetch for the Overview date range
 * (server returns shared-denominator food/labor/prime + status).
 */
export function useCostHealth(
  businessId: AccountingBizId,
  range: AccountingRange,
) {
  return useQuery({
    queryKey: costHealthKeys.report(businessId, range),
    queryFn: async () => {
      const report = await laborCostApi.getLaborCost(bizId(businessId), "week", {
        start: range.start,
        end: range.end,
      });
      return toCostHealthView(report);
    },
    enabled: isBizEnabled(businessId) && Boolean(range.start && range.end),
  });
}
