/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { accountingApi, type FoodCostReport } from "@/api/accounting";
import { asDollars } from "@/types/money";
import { useDishRecipeCoverage } from "@/components/business/accounting/useDishRecipeCoverage";
import CostAnalyticsSection from "./CostAnalyticsSection";

jest.mock("@/api/accounting", () => ({
  accountingApi: {
    getFoodCost: jest.fn().mockResolvedValue(null),
  },
}));

jest.mock("@/api/wasteVariance", () => ({
  wasteVarianceApi: { getWasteVariance: jest.fn().mockResolvedValue(null) },
}));

jest.mock("@/api/laborCost", () => ({
  laborCostApi: { getLaborCost: jest.fn().mockResolvedValue(null) },
}));

jest.mock("@/components/business/accounting/useDishRecipeCoverage", () => ({
  useDishRecipeCoverage: jest.fn(() => ({
    data: { mapped: 6, total: 6, ratio: 1, incomplete: false },
    isLoading: false,
  })),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => {
    const map: Record<string, string> = {
      "businessDashboard.accountingDashboard.overview.costHealth": "Cost health",
      "businessDashboard.accountingDashboard.foodCost.cardLabel": "Food cost",
      "businessDashboard.accountingDashboard.foodCost.blendedHint":
        "Ingredient cost ÷ sales, recipe-mapped items",
      "businessDashboard.accountingDashboard.foodCost.dishCoverageHint":
        "Only {mapped} of {total} dishes have recipes — treat this % as incomplete until every dish is mapped.",
      "businessDashboard.accountingDashboard.foodCost.periodSelectorLabel":
        "Food cost period",
      "businessDashboard.accountingDashboard.foodCost.cogsLabel": "Estimated COGS",
      "businessDashboard.accountingDashboard.foodCost.totalRevenueLabel":
        "Total revenue (recipe-mapped)",
      "businessDashboard.accountingDashboard.foodCost.states.loading": "Loading…",
      "businessDashboard.accountingDashboard.foodCost.states.empty":
        "No recipe-mapped sales",
      "businessDashboard.accountingDashboard.foodCost.states.noSales":
        "No recognized sales",
      "businessDashboard.accountingDashboard.foodCost.periods.day": "Day",
      "businessDashboard.accountingDashboard.foodCost.periods.week": "Week",
      "businessDashboard.accountingDashboard.foodCost.periods.month": "Month",
      "businessDashboard.accountingDashboard.foodCost.noRecipes":
        "Map ingredients to menu items to see food cost",
      "businessDashboard.accountingDashboard.foodCost.table.title":
        "Per-dish margin",
      "businessDashboard.accountingDashboard.foodCost.table.item": "Item",
      "businessDashboard.accountingDashboard.foodCost.table.unitCost":
        "Unit cost",
      "businessDashboard.accountingDashboard.foodCost.table.price": "Price",
      "businessDashboard.accountingDashboard.foodCost.table.foodCostPct":
        "Food cost %",
      "businessDashboard.accountingDashboard.foodCost.table.margin": "Margin",
      "businessDashboard.accountingDashboard.foodCost.table.qty": "Sold",
      "businessDashboard.accountingDashboard.states.loadError": "Load error",
      "businessDashboard.accountingDashboard.states.retry": "Retry",
      "businessDashboard.accountingDashboard.laborCost.title": "Labor Cost",
      "businessDashboard.accountingDashboard.laborCost.subtitle": "Labor share",
      "businessDashboard.accountingDashboard.laborCost.periodSelectorLabel":
        "Labor cost period",
      "businessDashboard.accountingDashboard.laborCost.periods.week": "Week",
      "businessDashboard.accountingDashboard.laborCost.periods.month": "Month",
      "businessDashboard.accountingDashboard.laborCost.laborPctLabel":
        "Labor cost",
      "businessDashboard.accountingDashboard.laborCost.primeCostLabel":
        "Prime cost",
      "businessDashboard.accountingDashboard.laborCost.netSalesLabel":
        "Net sales",
      "businessDashboard.accountingDashboard.laborCost.provenanceOne":
        "Based on 1 payroll run",
      "businessDashboard.accountingDashboard.laborCost.provenance":
        "Based on {count} payroll runs",
      "businessDashboard.accountingDashboard.laborCost.target": "Target ≤30%",
      "businessDashboard.accountingDashboard.laborCost.states.empty":
        "Record a payroll run",
      "businessDashboard.accountingDashboard.laborCost.states.noSales":
        "No sales",
      "businessDashboard.accountingDashboard.laborCost.states.loading":
        "Loading…",
      "businessDashboard.accountingDashboard.wasteVariance.title":
        "Waste & Variance",
      "businessDashboard.accountingDashboard.wasteVariance.subtitle":
        "Theoretical vs actual",
      "businessDashboard.accountingDashboard.wasteVariance.periodSelectorLabel":
        "Waste period",
      "businessDashboard.accountingDashboard.wasteVariance.periods.day": "Day",
      "businessDashboard.accountingDashboard.wasteVariance.periods.week": "Week",
      "businessDashboard.accountingDashboard.wasteVariance.periods.month":
        "Month",
      "businessDashboard.accountingDashboard.wasteVariance.trackedLoss":
        "Tracked loss",
      "businessDashboard.accountingDashboard.wasteVariance.reasons.spoilage":
        "Spoilage",
      "businessDashboard.accountingDashboard.wasteVariance.reasons.count_shrink":
        "Count shrink",
      "businessDashboard.accountingDashboard.wasteVariance.reasons.manual":
        "Manual",
      "businessDashboard.accountingDashboard.wasteVariance.inactive": "Inactive",
      "businessDashboard.accountingDashboard.wasteVariance.table.ingredient":
        "Ingredient",
      "businessDashboard.accountingDashboard.wasteVariance.table.theoretical":
        "Theoretical",
      "businessDashboard.accountingDashboard.wasteVariance.table.actual":
        "Actual",
      "businessDashboard.accountingDashboard.wasteVariance.table.variance":
        "Variance",
      "businessDashboard.accountingDashboard.wasteVariance.table.varianceCost":
        "Variance $",
      "businessDashboard.accountingDashboard.wasteVariance.states.empty":
        "No inventory movements",
      "businessDashboard.accountingDashboard.wasteVariance.states.sparse":
        "Sparse",
      "businessDashboard.accountingDashboard.wasteVariance.states.needsRecipe":
        "Needs recipe",
      "businessDashboard.accountingDashboard.wasteVariance.states.loading":
        "Loading…",
    };
    return map[key] ?? key;
  },
}));

const foodCostReport = (
  overrides: Partial<FoodCostReport> = {},
): FoodCostReport => ({
  period: "week",
  items: [],
  blended_food_cost_pct: 0.32,
  estimated_cogs: asDollars(123.45),
  total_revenue: asDollars(456.78),
  items_missing_cost: 0,
  items_without_recipe: 0,
  // Default factory report carries revenue, so the window honestly had sales.
  has_sales: true,
  ...overrides,
});

const useDishRecipeCoverageMock = useDishRecipeCoverage as jest.Mock;

describe("CostAnalyticsSection", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useDishRecipeCoverageMock.mockReturnValue({
      data: { mapped: 6, total: 6, ratio: 1, incomplete: false },
      isLoading: false,
    });
  });

  it("loads the default week report and surfaces COGS and revenue as dollars", async () => {
    (accountingApi.getFoodCost as jest.Mock).mockResolvedValue(
      foodCostReport(),
    );

    render(<CostAnalyticsSection businessId="42" currency="USD" />);

    expect(await screen.findByText("Estimated COGS")).toBeTruthy();
    expect(screen.getByText("Total revenue (recipe-mapped)")).toBeTruthy();
    expect(screen.getByText("$123.45")).toBeTruthy();
    expect(screen.getByText("$456.78")).toBeTruthy();
    expect(accountingApi.getFoodCost).toHaveBeenCalledWith("42", "week");
    expect(screen.getByTestId("cost-analytics-section")).toBeInTheDocument();
    expect(screen.getByText("Cost health")).toBeInTheDocument();
  });

  // PV-LIVE-20260720-005: zero recipe-mapped revenue must not paint proud 0%.
  it("shows an empty state instead of 0% when recipe-mapped revenue is zero", async () => {
    (accountingApi.getFoodCost as jest.Mock).mockResolvedValue(
      foodCostReport({
        blended_food_cost_pct: 0,
        estimated_cogs: asDollars(0),
        total_revenue: asDollars(0),
        has_sales: true,
      }),
    );

    render(<CostAnalyticsSection businessId="42" currency="USD" />);

    expect(await screen.findByTestId("food-cost-empty")).toBeTruthy();
    expect(screen.getByText("No recipe-mapped sales")).toBeTruthy();
    expect(screen.queryByText("0%")).toBeNull();
  });

  it("names a quiet week as no sales instead of missing recipes", async () => {
    (accountingApi.getFoodCost as jest.Mock).mockResolvedValue(
      foodCostReport({
        blended_food_cost_pct: 0,
        estimated_cogs: asDollars(0),
        total_revenue: asDollars(0),
        has_sales: false,
      }),
    );

    render(<CostAnalyticsSection businessId="42" currency="USD" />);

    expect(await screen.findByTestId("food-cost-empty")).toBeTruthy();
    expect(screen.getByText("No recognized sales")).toBeTruthy();
    expect(screen.queryByText("No recipe-mapped sales")).toBeNull();
    expect(screen.queryByText("0%")).toBeNull();
  });

  it("survives a food-cost report whose items array is null (cold-start business)", async () => {
    (accountingApi.getFoodCost as jest.Mock).mockResolvedValue({
      ...foodCostReport(),
      items: null,
    } as unknown as FoodCostReport);

    render(<CostAnalyticsSection businessId="42" currency="USD" />);

    expect(await screen.findByText("Estimated COGS")).toBeTruthy();
    expect(screen.getByText("$123.45")).toBeTruthy();
  });

  it("refetches only the food-cost report when the period changes", async () => {
    (accountingApi.getFoodCost as jest.Mock).mockImplementation(
      (_businessId: string, period: "day" | "week" | "month") =>
        Promise.resolve(
          foodCostReport({
            period,
            estimated_cogs: asDollars(period === "month" ? 300 : 123.45),
            total_revenue: asDollars(period === "month" ? 1000 : 456.78),
          }),
        ),
    );

    render(<CostAnalyticsSection businessId="42" currency="USD" />);

    expect(await screen.findByText("$123.45")).toBeTruthy();
    fireEvent.click(screen.getByRole("tab", { name: "Month" }));

    await waitFor(() => {
      expect(accountingApi.getFoodCost).toHaveBeenCalledWith("42", "month");
    });
    expect(await screen.findByText("$300.00")).toBeTruthy();
    expect(screen.getByText("$1,000.00")).toBeTruthy();
  });

  it("caveats a proud food-cost % when only half the menu has recipes", async () => {
    (accountingApi.getFoodCost as jest.Mock).mockResolvedValue(
      foodCostReport({ blended_food_cost_pct: 0.1 }),
    );
    (useDishRecipeCoverage as jest.Mock).mockReturnValue({
      data: { mapped: 3, total: 6, ratio: 0.5, incomplete: true },
      isLoading: false,
    });

    render(<CostAnalyticsSection businessId="42" currency="USD" />);

    expect(
      await screen.findByText(
        "Only 3 of 6 dishes have recipes — treat this % as incomplete until every dish is mapped.",
      ),
    ).toBeTruthy();
  });
});
