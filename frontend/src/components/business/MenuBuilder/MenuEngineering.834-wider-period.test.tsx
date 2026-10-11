/** @jest-environment jsdom */
/**
 * #834 — Menu Engineering opens on `week`. On business 86, live on
 * sha-0024205b6c01 (2026-08-22):
 *
 *   menu-engineering?period=week  → has_sales false, 3 dishes, all qty_sold 0
 *   menu-engineering?period=month → has_sales true,  median_qty_sold 28
 *   analytics/sales?period=week   → total_revenue 0        (Aug 17 → now)
 *   analytics/sales?period=month  → total_revenue 11809.32 (Aug 1 → now)
 *
 * The ISO week really is empty, so "No recognized sales in this period" is not
 * false — but it strands an owner whose Accounting tab reads $18,237.98, and
 * "try a wider period" makes them guess which one. When a wider window is
 * confirmed to have sales, the empty state must name it and offer the switch.
 */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import MenuEngineeringMatrix, {
  type MenuEngineeringMatrixProps,
} from "./MenuEngineeringMatrix";
import type { MenuEngineeringReport } from "@/api/menuEngineering";
import { asDollars } from "@/types/money";

function labels(): MenuEngineeringMatrixProps["labels"] {
  return {
    subtitle: "How dishes perform by margin and popularity",
    periods: { day: "Day", week: "Week", month: "Month" },
    axes: { margin: "Margin", popularity: "Popularity" },
    quadrants: {
      star: { label: "Crowd favorites", action: "Keep featuring" },
      plowhorse: { label: "Popular, thin margin", action: "Review price" },
      puzzle: { label: "Hidden gems", action: "Worth promoting" },
      dog: { label: "Slow movers", action: "Rework or remove" },
    },
    cards: { revenueShare: "of revenue", dishes: "dishes" },
    table: {
      title: "Dishes",
      item: "Dish",
      quadrant: "Group",
      foodCostPct: "Food cost %",
      qty: "Sold",
      price: "Price",
      margin: "Margin",
      suggestion: "Suggestion",
    },
    states: {
      empty: "No dish cost or sales data for this period yet",
      emptyMissingCost:
        "Sales are recorded, but your dishes are missing plate costs",
      emptyNoSales: "No recognized sales in this period — try a wider period",
      emptyNoSalesWider:
        "No recognized sales in this period — {period} has sales",
      showWiderPeriod: "Show {period}",
      sparse: "Positions are directional",
      needsCost: "Dishes are missing cost data",
      loading: "Loading…",
    },
    suggest: "Consider",
  };
}

/** Business 86's live `week` payload: costed dishes, every one unsold. */
function weekReport(over: Partial<MenuEngineeringReport> = {}): MenuEngineeringReport {
  return {
    period: "week",
    median_food_cost_pct: 0,
    median_qty_sold: 0,
    dishes: [
      {
        menu_item_id: "demo-steak",
        menu_item_name: "Steak Plate",
        food_cost_pct: 0,
        qty_sold: 0,
        avg_price: asDollars(0),
        unit_cost: asDollars(0),
        margin_per_unit: asDollars(0),
        quadrant: "star",
        action: "protect",
        suggested_price: asDollars(0),
      },
    ],
    rollups: [],
    items_needing_cost: 0,
    sparse: true,
    has_sales: false,
    ...over,
  };
}

function mount(over: Partial<MenuEngineeringMatrixProps> = {}) {
  const onPeriodChange = jest.fn();
  render(
    <MenuEngineeringMatrix
      report={weekReport()}
      loading={false}
      currency="USD"
      period="week"
      onPeriodChange={onPeriodChange}
      formatMoney={(v, c) => `${c} ${Number(v).toFixed(2)}`}
      labels={labels()}
      {...over}
    />,
  );
  return { onPeriodChange };
}

describe("#834 empty state points at the period that has the sales", () => {
  it("names the wider period instead of shrugging", () => {
    mount({ widerPeriodWithSales: "month" });
    expect(
      screen.getByText("No recognized sales in this period — Month has sales"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/try a wider period/),
    ).not.toBeInTheDocument();
  });

  it("switches to that period in one tap", () => {
    const { onPeriodChange } = mount({ widerPeriodWithSales: "month" });
    fireEvent.click(screen.getByTestId("menu-engineering-show-wider-period"));
    expect(onPeriodChange).toHaveBeenCalledWith("month");
  });

  it("keeps the plain copy when no wider period was confirmed", () => {
    mount({ widerPeriodWithSales: null });
    expect(
      screen.getByText("No recognized sales in this period — try a wider period"),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId("menu-engineering-show-wider-period"),
    ).not.toBeInTheDocument();
  });

  it("never offers the period already selected", () => {
    mount({ period: "month", widerPeriodWithSales: "month" });
    expect(
      screen.queryByTestId("menu-engineering-show-wider-period"),
    ).not.toBeInTheDocument();
  });

  it("offers the wider period on the no-dishes empty state too", () => {
    const { onPeriodChange } = mount({
      report: weekReport({ dishes: [], items_needing_cost: 0 }),
      widerPeriodWithSales: "month",
    });
    expect(screen.getByTestId("menu-engineering-empty")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("menu-engineering-show-wider-period"));
    expect(onPeriodChange).toHaveBeenCalledWith("month");
  });

  it("never replaces the missing-plate-cost copy — that window HAS sales", () => {
    // has_sales true means the blocker is costs, not sales. Even if a stale
    // wider-period offer were still in state, the copy must not flip to a
    // no-sales claim.
    mount({
      report: weekReport({ dishes: [], items_needing_cost: 3, has_sales: true }),
      widerPeriodWithSales: "month",
    });
    expect(
      screen.getByText(
        "Sales are recorded, but your dishes are missing plate costs",
      ),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId("menu-engineering-show-wider-period"),
    ).not.toBeInTheDocument();
  });
});
