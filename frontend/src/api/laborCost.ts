import { axiosInstance } from "@/api/tools/instance";
import type { Dollars } from "@/types/money";

/** Server cost-health status (reporting.Compute). */
export type CostHealthStatus = "ok" | "insufficient_data" | "implausible";

interface LaborRunContribution {
  payroll_run_id: number;
  period_start: string;
  period_end: string;
  overlap_fraction: number;
  labor_cost: Dollars;
}

/**
 * Labor-cost + shared-denominator cost health for a window.
 * food/labor/prime percents all divide by recognized net sales; recipe coverage
 * is separate. prime_cost_pct is omitted when status is implausible/insufficient.
 */
export interface LaborCostReport {
  period: string;
  labor_cost: Dollars;
  net_sales: Dollars;
  labor_cost_pct: number; // fraction — may exceed 1.0 when status=implausible
  payroll_run_count: number;
  has_data: boolean;
  contributions: LaborRunContribution[];
  food_cost_pct: number; // fraction — shared recognized-revenue denominator
  /** Present only when status === "ok". */
  prime_cost_pct?: number;
  status?: CostHealthStatus;
  reason?: string;
  recipe_coverage_pct?: number;
  /** Always "labor_accrued_prorated" for this endpoint's labor dollars. */
  labor_basis?: string;
  labor_accrued_prorated?: Dollars;
  payroll_paid_cash_basis?: Dollars;
}

// Actual-basis labor (Slice 4): approved time-clock worked-hours × pay rate.
// The server gates EVERY $ field on the caller's financial:read — non-financial
// managers get worked_hours + labor_cost_pct only, never an amount. Mirrors the
// `basis=actual` branch of backend/internal/handlers/accounting.go:GetLaborCost.
interface LaborActualStaffContribution {
  staff_id: number;
  worked_minutes: number;
  worked_hours: number;
  labor_cost?: Dollars; // financial:read only
}

export interface LaborActualReport {
  period: string;
  basis: "actual";
  labor_cost_pct: number; // fraction 0..1 (always present)
  worked_hours: number; // always present
  has_data: boolean;
  staff_contributions: LaborActualStaffContribution[];
  food_cost_pct: number;
  prime_cost_pct?: number;
  status?: CostHealthStatus;
  reason?: string;
  recipe_coverage_pct?: number;
  labor_basis?: string;
  payroll_basis_labor_cost_pct: number;
  variance: { compared_to: string; labor_cost_pct: number; labor_cost?: Dollars };
  labor_cost?: Dollars; // financial:read only
  net_sales?: Dollars; // financial:read only
  payroll_basis_labor_cost?: Dollars; // financial:read only
  labor_accrued_prorated?: Dollars;
  payroll_paid_cash_basis?: Dollars;
}

export const laborCostApi = {
  getLaborCost: async (
    businessId: string,
    period: "week" | "month" = "week",
    range?: { start: string; end: string },
  ): Promise<LaborCostReport> => {
    const params = range
      ? { start: range.start, end: range.end }
      : { period };
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/accounting/labor-cost",
      { params },
    );
    return response.data.data;
  },

  // Actual-basis report (worked hours from approved time entries). Only call this
  // when the caller can see financials; the server still strips $ regardless.
  getActual: async (
    businessId: string,
    period: "week" | "month" = "week",
  ): Promise<LaborActualReport> => {
    const response = await axiosInstance.get(
      "/inside/businesses/" + businessId + "/accounting/labor-cost",
      { params: { basis: "actual", period } },
    );
    return response.data.data;
  },
};
