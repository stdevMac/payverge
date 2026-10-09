import { toCostHealthView } from "./useCostHealth";
import type { LaborCostReport } from "@/api/laborCost";
import { asDollars } from "@/types/money";

function report(overrides: Partial<LaborCostReport> = {}): LaborCostReport {
  return {
    period: "week",
    labor_cost: asDollars(300),
    net_sales: asDollars(1000),
    labor_cost_pct: 0.3,
    payroll_run_count: 1,
    has_data: true,
    contributions: [],
    food_cost_pct: 0.2,
    prime_cost_pct: 0.5,
    status: "ok",
    recipe_coverage_pct: 0.4,
    labor_basis: "labor_accrued_prorated",
    ...overrides,
  };
}

describe("toCostHealthView", () => {
  it("maps shared-denominator ok report into display percents", () => {
    const view = toCostHealthView(report({ recipe_coverage_pct: 0.9 }));
    expect(view.status).toBe("ok");
    expect(view.foodPct).toBe(0.2);
    expect(view.laborPct).toBe(0.3);
    expect(view.primePct).toBe(0.5);
    expect(view.recipeCoveragePct).toBe(0.9);
    expect(view.lowCoverage).toBe(false);
  });

  it("flags thin recipe coverage so UI does not present food cost as healthy", () => {
    const view = toCostHealthView(report({ recipe_coverage_pct: 0.4 }));
    expect(view.status).toBe("ok");
    expect(view.recipeCoveragePct).toBe(0.4);
    expect(view.lowCoverage).toBe(true);
  });

  it("nulls percentages for insufficient_data (no green zeros)", () => {
    const view = toCostHealthView(
      report({
        status: "insufficient_data",
        reason: "no_sales",
        net_sales: asDollars(0),
        labor_cost_pct: 0,
        food_cost_pct: 0,
        prime_cost_pct: undefined,
      }),
    );
    expect(view.status).toBe("insufficient_data");
    expect(view.foodPct).toBeNull();
    expect(view.laborPct).toBeNull();
    expect(view.primePct).toBeNull();
  });

  it("keeps implausible labor ratio visible but omits prime", () => {
    const view = toCostHealthView(
      report({
        status: "implausible",
        reason: "labor_exceeds_revenue",
        labor_cost: asDollars(19440),
        net_sales: asDollars(5600),
        labor_cost_pct: 19440 / 5600,
        food_cost_pct: 0,
        prime_cost_pct: undefined,
      }),
    );
    expect(view.status).toBe("implausible");
    expect(view.reason).toBe("labor_exceeds_revenue");
    expect(view.laborPct).toBeCloseTo(3.47, 2);
    expect(view.primePct).toBeNull();
  });

  it("infers implausible from legacy payloads with ratio > 1", () => {
    const view = toCostHealthView(
      report({
        status: undefined,
        labor_cost_pct: 3.29,
        food_cost_pct: 0.17,
        prime_cost_pct: 3.46,
      }),
    );
    expect(view.status).toBe("implausible");
    expect(view.primePct).toBeNull();
  });
});
