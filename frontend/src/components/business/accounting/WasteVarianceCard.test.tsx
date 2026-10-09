/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import WasteVarianceCard, {
  type WasteVarianceCardProps,
} from "./WasteVarianceCard";
import type { WasteVarianceReport } from "@/api/wasteVariance";
import { asDollars } from "@/types/money";

const formatMoney = (v: number, c: string) => `${c} ${v.toFixed(2)}`;

function makeLabels(): WasteVarianceCardProps["labels"] {
  return {
    title: "Waste & Variance",
    subtitle: "Theoretical vs actual ingredient usage",
    periodSelectorLabel: "Select period",
    periods: { day: "Day", week: "Week", month: "Month" },
    trackedLoss: "Tracked Loss",
    reasons: {
      spoilage: "Spoilage",
      count_shrink: "Count shrink",
      manual: "Manual adjustment",
    },
    table: {
      ingredient: "Ingredient",
      theoretical: "Theoretical",
      actual: "Actual",
      variance: "Variance",
      varianceCost: "Variance $",
    },
    states: {
      empty: "No variance data for this period",
      sparse: "Limited data — positions are directional",
      needsRecipe: "Some ingredients are missing recipe data",
      loading: "Loading waste variance…",
    },
  };
}

function makeReport(
  overrides: Partial<WasteVarianceReport> = {},
): WasteVarianceReport {
  return {
    period: "week",
    tracked_loss_cost: asDollars(45.5),
    total_variance_cost: asDollars(60.0),
    theoretical_usage_cost: asDollars(200.0),
    loss_by_reason: [],
    ingredients: [
      {
        inventory_item_id: 1,
        name: "Tomato",
        unit: "kg",
        cost_per_unit: asDollars(2.5),
        theoretical_usage: 10.0,
        actual_usage: 12.5,
        variance: 2.5,
        variance_cost: asDollars(6.25),
        tracked_loss_cost: asDollars(0),
        has_recipe: true,
      },
    ],
    items_without_recipe: 0,
    sparse: false,
    ...overrides,
  };
}

function renderCard(overrides: Partial<WasteVarianceCardProps> = {}) {
  const onPeriodChange = jest.fn();
  const props: WasteVarianceCardProps = {
    report: makeReport(),
    loading: false,
    currency: "USD",
    selectedPeriod: "week",
    onPeriodChange,
    formatMoney,
    labels: makeLabels(),
    ...overrides,
  };
  const utils = render(<WasteVarianceCard {...props} />);
  return { onPeriodChange, ...utils };
}

describe("WasteVarianceCard", () => {
  it("renders the tracked-loss headline with formatMoney", () => {
    renderCard();
    expect(screen.getByText("Tracked Loss")).toBeInTheDocument();
    // tracked_loss_cost = 45.5 → formatMoney → "USD 45.50"
    expect(screen.getByText("USD 45.50")).toBeInTheDocument();
  });

  it("renders a has_recipe:true ingredient with positive variance in rose", () => {
    renderCard();
    const row = screen.getByTestId("wv-row-1");
    expect(row).toBeInTheDocument();
    // The variance cell: variance=2.5 → rose-600
    const varianceCell = row.querySelectorAll("td")[3];
    expect(varianceCell?.className).toContain("text-rose-600");
    expect(varianceCell?.textContent).toContain("2.50");
  });

  it("renders a has_recipe:false ingredient with — for theoretical, variance, variance cost", () => {
    renderCard({
      report: makeReport({
        ingredients: [
          {
            inventory_item_id: 2,
            name: "Basil",
            unit: "g",
            cost_per_unit: asDollars(0.05),
            theoretical_usage: 0,
            actual_usage: 50,
            variance: 0,
            variance_cost: asDollars(0),
            tracked_loss_cost: asDollars(0),
            has_recipe: false,
          },
        ],
      }),
    });

    const row = screen.getByTestId("wv-row-2");
    const cells = row.querySelectorAll("td");
    // td[1] = theoretical → "—"
    expect(cells[1]?.textContent).toBe("—");
    // td[3] = variance → "—"
    expect(cells[3]?.textContent).toBe("—");
    // td[4] = variance cost → "—"
    expect(cells[4]?.textContent).toBe("—");
    // td[2] = actual → shows actual_usage with unit
    expect(cells[2]?.textContent).toContain("50.00");
  });

  it("renders reason breakdown rows with labels.reasons[reason]", () => {
    renderCard({
      report: makeReport({
        loss_by_reason: [
          { reason: "spoilage", cost: asDollars(20.0) },
          { reason: "count_shrink", cost: asDollars(10.0) },
        ],
      }),
    });

    expect(screen.getByText("Spoilage")).toBeInTheDocument();
    expect(screen.getByText("USD 20.00")).toBeInTheDocument();
    expect(screen.getByText("Count shrink")).toBeInTheDocument();
    expect(screen.getByText("USD 10.00")).toBeInTheDocument();
  });

  it("shows needsRecipe callout when items_without_recipe > 0", () => {
    renderCard({
      report: makeReport({ items_without_recipe: 3 }),
    });
    expect(
      screen.getByText("Some ingredients are missing recipe data"),
    ).toBeInTheDocument();
  });

  it("shows sparse hint when report.sparse === true", () => {
    renderCard({
      report: makeReport({ sparse: true }),
    });
    expect(
      screen.getByText("Limited data — positions are directional"),
    ).toBeInTheDocument();
  });

  it("shows empty state when ingredients = []", () => {
    renderCard({
      report: makeReport({ ingredients: [] }),
    });
    expect(
      screen.getByText("No variance data for this period"),
    ).toBeInTheDocument();
  });

  it("shows loading placeholder when loading=true", () => {
    renderCard({ loading: true, report: null });
    expect(
      screen.getByLabelText("Loading waste variance…"),
    ).toBeInTheDocument();
    // No table or headline rendered
    expect(screen.queryByText("Tracked Loss")).not.toBeInTheDocument();
  });

  it("returns null when report is null and not loading", () => {
    const { container } = renderCard({ report: null, loading: false });
    // Only the outer wrapper + header might render; tracked loss should not.
    // Actually our component returns null entirely when !report && !loading.
    expect(container.firstChild).toBeNull();
  });

  it("drives onPeriodChange when a period tab is clicked", () => {
    const { onPeriodChange } = renderCard();
    fireEvent.click(screen.getByRole("tab", { name: "Month" }));
    expect(onPeriodChange).toHaveBeenCalledWith("month");
  });

  it("renders a has_recipe:true ingredient with negative variance in emerald", () => {
    renderCard({
      report: makeReport({
        ingredients: [
          {
            inventory_item_id: 3,
            name: "Cheese",
            unit: "kg",
            cost_per_unit: asDollars(8.0),
            theoretical_usage: 5.0,
            actual_usage: 3.5,
            variance: -1.5,
            variance_cost: asDollars(-12.0),
            tracked_loss_cost: asDollars(0),
            has_recipe: true,
          },
        ],
      }),
    });

    const row = screen.getByTestId("wv-row-3");
    const varianceCell = row.querySelectorAll("td")[3];
    expect(varianceCell?.className).toContain("text-emerald-600");
    expect(varianceCell?.textContent).toContain("-1.50");
  });

  // Live-review regression: color must follow the ROUNDED display value. A
  // raw variance of 0.0004 renders "0.00 kg" — painting that rose reads as
  // an alarm on zero loss.
  it("renders a variance that displays as 0.00 in neutral ink, not rose", () => {
    renderCard({
      report: makeReport({
        ingredients: [
          {
            inventory_item_id: 4,
            name: "Premium Beef",
            unit: "kg",
            cost_per_unit: asDollars(30.0),
            theoretical_usage: 10.0,
            actual_usage: 10.0004,
            variance: 0.0004,
            variance_cost: asDollars(0.001),
            tracked_loss_cost: asDollars(0),
            has_recipe: true,
          },
        ],
      }),
    });

    const cells = screen.getByTestId("wv-row-4").querySelectorAll("td");
    expect(cells[3]?.textContent).toContain("0.00");
    expect(cells[3]?.className).not.toContain("text-rose-600");
    expect(cells[3]?.className).toContain("text-ink-700");
    expect(cells[4]?.className).not.toContain("text-rose-600");
  });

  // Live-review regression: the tracked-loss headline was hardcoded rose —
  // "$0.00" of loss must not render as an alarm.
  it("renders a $0.00 tracked loss in neutral ink, and a real loss in rose", () => {
    renderCard({ report: makeReport({ tracked_loss_cost: asDollars(0) }) });
    expect(screen.getByText("USD 0.00").className).not.toContain(
      "text-rose-600",
    );

    renderCard({ report: makeReport({ tracked_loss_cost: asDollars(45.5) }) });
    expect(screen.getByText("USD 45.50").className).toContain("text-rose-600");
  });

  // R3-AC-9: deliberately-included inactive ingredients must be visually
  // distinguishable from live ones.
  it("tags an inactive ingredient with the inactive chip", () => {
    const labels = makeLabels();
    labels.inactive = "Inactive";
    renderCard({
      labels,
      report: makeReport({
        ingredients: [
          {
            inventory_item_id: 9,
            name: "Discontinued Sauce",
            unit: "L",
            cost_per_unit: asDollars(3),
            theoretical_usage: 1,
            actual_usage: 2,
            variance: 1,
            variance_cost: asDollars(3),
            tracked_loss_cost: asDollars(0),
            has_recipe: true,
            is_active: false,
          },
        ],
      }),
    });
    expect(screen.getByText("Inactive")).toBeInTheDocument();
  });

  // R3-AC: a swallowed fetch used to make the whole card vanish. The error
  // state must render an inline retry instead.
  it("renders an inline retry on error and calls onRetry", () => {
    const onRetry = jest.fn();
    const labels = makeLabels();
    labels.states.error = "Couldn't load waste variance";
    labels.states.retry = "Retry";
    renderCard({ error: true, onRetry, labels });
    expect(
      screen.getByText("Couldn't load waste variance"),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByText("Retry"));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});
