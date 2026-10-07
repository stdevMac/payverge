/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, within } from "@testing-library/react";
import MenuEngineeringMatrix, {
  type MenuEngineeringMatrixProps,
} from "./MenuEngineeringMatrix";
import type { MenuEngineeringReport } from "@/api/menuEngineering";
import { asDollars } from "@/types/money";

// Simple, assertable money formatter: `USD 12.50`. Crucially this receives the
// RAW dollar float (never cents) so we can prove the component does not ×100.
const formatMoney = jest.fn(
  (value: number, currency: string) => `${currency} ${value.toFixed(2)}`,
);

function makeLabels(): MenuEngineeringMatrixProps["labels"] {
  return {
    subtitle: "How dishes perform by margin and popularity",
    periods: { day: "Day", week: "Week", month: "Month" },
    axes: { margin: "Margin", popularity: "Popularity" },
    quadrants: {
      star: { label: "Crowd favorites", action: "Keep featuring these" },
      plowhorse: {
        label: "Popular, thin margin",
        action: "Review price or recipe cost",
      },
      puzzle: { label: "Hidden gems", action: "Worth promoting" },
      dog: { label: "Slow movers", action: "Rework or remove" },
    },
    cards: { revenueShare: "of revenue", dishes: "dishes" },
    table: {
      title: "Dish breakdown",
      item: "Item",
      quadrant: "Class",
      foodCostPct: "Food cost %",
      qty: "Qty",
      price: "Price",
      margin: "Unit margin",
      suggestion: "Suggestion",
    },
    states: {
      empty: "No dish data yet for this period",
      emptyMissingCost:
        "Sales are recorded, but dishes are missing plate costs — add costs to group them",
      emptyNoSales:
        "No recognized sales in this period — try a wider period",
      emptyNoSalesWider:
        "No recognized sales in this period — {period} has sales",
      showWiderPeriod: "Show {period}",
      sparse: "Limited data — positions are directional",
      needsCost: "Some dishes are missing cost data",
      loading: "Loading menu engineering…",
    },
    suggest: "Consider",
  };
}

function makeReport(
  overrides: Partial<MenuEngineeringReport> = {},
): MenuEngineeringReport {
  return {
    period: "week",
    median_food_cost_pct: 0.3,
    median_qty_sold: 50,
    dishes: [
      {
        menu_item_id: "star-1",
        menu_item_name: "Margherita",
        food_cost_pct: 0.25,
        qty_sold: 120,
        avg_price: asDollars(12.5),
        unit_cost: asDollars(3.13),
        margin_per_unit: asDollars(9.37),
        quadrant: "star",
        action: "protect",
        suggested_price: asDollars(0),
      },
      {
        menu_item_id: "plow-1",
        menu_item_name: "House Fries",
        food_cost_pct: 0.4,
        qty_sold: 200,
        avg_price: asDollars(5),
        unit_cost: asDollars(2),
        margin_per_unit: asDollars(3),
        quadrant: "plowhorse",
        action: "reprice_up",
        suggested_price: asDollars(6),
      },
      {
        menu_item_id: "puzz-1",
        menu_item_name: "Truffle Risotto",
        food_cost_pct: 0.22,
        qty_sold: 12,
        avg_price: asDollars(24),
        unit_cost: asDollars(5.28),
        margin_per_unit: asDollars(18.72),
        quadrant: "puzzle",
        action: "promote",
        suggested_price: asDollars(0),
      },
      {
        menu_item_id: "dog-1",
        menu_item_name: "Cold Soup",
        food_cost_pct: 0.45,
        qty_sold: 8,
        avg_price: asDollars(9),
        unit_cost: asDollars(4.05),
        margin_per_unit: asDollars(4.95),
        quadrant: "dog",
        action: "cut",
        suggested_price: asDollars(0),
      },
    ],
    rollups: [
      { quadrant: "star", count: 1, revenue_share: 0.45 },
      { quadrant: "plowhorse", count: 1, revenue_share: 0.3 },
      { quadrant: "puzzle", count: 1, revenue_share: 0.15 },
      { quadrant: "dog", count: 1, revenue_share: 0.1 },
    ],
    items_needing_cost: 0,
    sparse: false,
    has_sales: true,
    ...overrides,
  };
}

function renderMatrix(
  overrides: Partial<MenuEngineeringMatrixProps> = {},
): {
  onPeriodChange: jest.Mock;
} & ReturnType<typeof render> {
  const onPeriodChange = jest.fn();
  const props: MenuEngineeringMatrixProps = {
    report: makeReport(),
    loading: false,
    currency: "USD",
    period: "week",
    onPeriodChange,
    formatMoney: formatMoney as MenuEngineeringMatrixProps["formatMoney"],
    labels: makeLabels(),
    ...overrides,
  };
  const utils = render(<MenuEngineeringMatrix {...props} />);
  return { onPeriodChange, ...utils };
}

describe("MenuEngineeringMatrix", () => {
  beforeEach(() => {
    formatMoney.mockClear();
  });

  it("plots one dot per dish with the right data-quadrant and quadrant colors", () => {
    renderMatrix();

    const starDot = screen.getByTestId("dish-dot-star-1");
    const plowDot = screen.getByTestId("dish-dot-plow-1");
    const puzzDot = screen.getByTestId("dish-dot-puzz-1");
    const dogDot = screen.getByTestId("dish-dot-dog-1");

    expect(starDot).toHaveAttribute("data-quadrant", "star");
    expect(plowDot).toHaveAttribute("data-quadrant", "plowhorse");
    expect(puzzDot).toHaveAttribute("data-quadrant", "puzzle");
    expect(dogDot).toHaveAttribute("data-quadrant", "dog");

    // Star = emerald single source of truth drives the dot fill.
    expect(starDot.getAttribute("class")).toContain("fill-emerald-500");
    expect(dogDot.getAttribute("class")).toContain("fill-rose-500");
  });

  it("renders all four quadrant cards with count and revenue share", () => {
    renderMatrix();

    (["star", "plowhorse", "puzzle", "dog"] as const).forEach((q) => {
      expect(screen.getByTestId(`quadrant-card-${q}`)).toBeInTheDocument();
    });

    const starCard = screen.getByTestId("quadrant-card-star");
    expect(within(starCard).getByText("1")).toBeInTheDocument();
    // Math.round(0.45 * 100) = 45
    expect(within(starCard).getByText(/45%/)).toBeInTheDocument();

    const dogCard = screen.getByTestId("quadrant-card-dog");
    // Math.round(0.1 * 100) = 10
    expect(within(dogCard).getByText(/10%/)).toBeInTheDocument();
  });

  it("renders the dish table with names and quadrant badges", () => {
    renderMatrix();

    const table = screen.getByTestId("menu-engineering-table");
    expect(within(table).getByText("Margherita")).toBeInTheDocument();
    expect(within(table).getByText("House Fries")).toBeInTheDocument();
    expect(within(table).getByText("Truffle Risotto")).toBeInTheDocument();
    expect(within(table).getByText("Cold Soup")).toBeInTheDocument();

    // Quadrant badge text (label) scoped to the table.
    expect(within(table).getByText("Crowd favorites")).toBeInTheDocument();
    expect(within(table).getByText("Popular, thin margin")).toBeInTheDocument();
  });

  it("formats price/margin via formatMoney with the raw dollar amount (not ×100)", () => {
    renderMatrix();

    // Raw dollar floats, never cents.
    expect(formatMoney).toHaveBeenCalledWith(12.5, "USD");
    expect(formatMoney).toHaveBeenCalledWith(9.37, "USD");
    expect(formatMoney).not.toHaveBeenCalledWith(1250, "USD");

    const table = screen.getByTestId("menu-engineering-table");
    expect(within(table).getByText("USD 12.50")).toBeInTheDocument();
    expect(within(table).getByText("USD 9.37")).toBeInTheDocument();
  });

  it("shows the empty state and no dots when there are no dishes", () => {
    const labels = makeLabels();
    const { container } = renderMatrix({
      report: makeReport({ dishes: [], rollups: [] }),
    });

    expect(screen.getByText(labels.states.empty)).toBeInTheDocument();
    expect(
      container.querySelector('[data-testid^="dish-dot-"]'),
    ).toBeNull();
    expect(screen.queryByTestId("menu-engineering-table")).toBeNull();
  });

  it("names missing plate costs (not missing sales) when sales exist but no dish has a cost (#834)", () => {
    // A venue with billed income but zero costed dishes gets dishes: [] and
    // items_needing_cost > 0 from the classifier. The blocker is plate costs —
    // telling the operator to "record some sales" is a lie.
    const labels = makeLabels();
    renderMatrix({
      report: makeReport({
        dishes: [],
        rollups: [],
        items_needing_cost: 7,
        has_sales: true,
      }),
    });

    expect(
      screen.getByText(labels.states.emptyMissingCost),
    ).toBeInTheDocument();
    expect(screen.queryByText(labels.states.empty)).toBeNull();
  });

  it("does not claim sales exist when items need costs but the window has zero sales (#834 O2)", () => {
    // A brand-new venue can have 7 uncosted items and NO sales at all. The
    // missing-cost copy asserts "sales are recorded" — showing it here would
    // be a lie. With has_sales false the neutral empty copy must win.
    const labels = makeLabels();
    renderMatrix({
      report: makeReport({
        dishes: [],
        rollups: [],
        items_needing_cost: 7,
        has_sales: false,
      }),
    });

    expect(screen.getByText(labels.states.empty)).toBeInTheDocument();
    expect(screen.queryByText(labels.states.emptyMissingCost)).toBeNull();
  });

  it("shows the loading state and no scatter while loading", () => {
    const labels = makeLabels();
    const { container } = renderMatrix({ loading: true });

    expect(screen.getByText(labels.states.loading)).toBeInTheDocument();
    expect(
      container.querySelector('[data-testid^="dish-dot-"]'),
    ).toBeNull();
  });

  it("surfaces sparse and needs-cost hints", () => {
    const labels = makeLabels();
    renderMatrix({
      report: makeReport({ sparse: true, items_needing_cost: 2 }),
    });

    expect(screen.getByText(labels.states.sparse)).toBeInTheDocument();
    expect(
      screen.getByText(new RegExp(labels.states.needsCost)),
    ).toBeInTheDocument();
  });

  it("pins the inverted-Y (margin) and X (popularity) scatter orientation", () => {
    // The geometry in xOf/yOf is the riskiest code: an accidental axis swap or
    // sign flip would keep every other test green while silently mirroring the
    // chart. This reads the rendered SVG coordinates directly to lock the
    // mapping.
    renderMatrix({
      report: makeReport({
        dishes: [
          {
            menu_item_id: "high-margin",
            menu_item_name: "High Margin",
            food_cost_pct: 0.15, // low cost = high margin -> should plot near TOP
            qty_sold: 150, // also more popular -> should plot to the RIGHT
            avg_price: asDollars(20),
            unit_cost: asDollars(3),
            margin_per_unit: asDollars(17),
            quadrant: "star",
            action: "protect",
            suggested_price: asDollars(0),
          },
          {
            menu_item_id: "low-margin",
            menu_item_name: "Low Margin",
            food_cost_pct: 0.55, // high cost = low margin -> should plot near BOTTOM
            qty_sold: 10, // less popular -> should plot to the LEFT
            avg_price: asDollars(8),
            unit_cost: asDollars(4.4),
            margin_per_unit: asDollars(3.6),
            quadrant: "dog",
            action: "cut",
            suggested_price: asDollars(0),
          },
        ],
      }),
    });

    const highMarginCy = Number(
      screen.getByTestId("dish-dot-high-margin").getAttribute("cy"),
    );
    const lowMarginCy = Number(
      screen.getByTestId("dish-dot-low-margin").getAttribute("cy"),
    );
    // Inverted-Y: smaller SVG y = higher on screen = higher margin (lower cost).
    expect(highMarginCy).toBeLessThan(lowMarginCy);

    const popularCx = Number(
      screen.getByTestId("dish-dot-high-margin").getAttribute("cx"),
    );
    const nicheCx = Number(
      screen.getByTestId("dish-dot-low-margin").getAttribute("cx"),
    );
    // X = popularity: more qty_sold plots further right (greater cx).
    expect(popularCx).toBeGreaterThan(nicheCx);
  });

  it("drives onPeriodChange when a period tab is clicked", () => {
    const { onPeriodChange } = renderMatrix();

    fireEvent.click(screen.getByRole("tab", { name: "Month" }));

    expect(onPeriodChange).toHaveBeenCalledWith("month");
  });

  it("shows a read-only price suggestion (never a one-click reprice button)", () => {
    renderMatrix();

    // Plowhorse with suggested_price 6 → suggestion text only.
    const suggest = screen.getByTestId("suggest-plow-1");
    expect(suggest).toBeInTheDocument();
    expect(suggest).toHaveTextContent("Consider USD 6.00");
    expect(suggest.tagName).not.toBe("BUTTON");

    // No one-click apply affordance remains.
    expect(screen.queryByTestId("propose-plow-1")).toBeNull();
    expect(screen.queryByRole("button", { name: /Propose|Consider/i })).toBeNull();

    // Dishes without a suggestion show an em dash, not a button.
    expect(screen.queryByTestId("suggest-star-1")).toBeNull();
  });

  it("keeps quadrant summary cards from overflowing (min-w-0 + clamp)", () => {
    renderMatrix();
    const card = screen.getByTestId("quadrant-card-plowhorse");
    expect(card.className).toContain("min-w-0");
    expect(card.className).toContain("overflow-hidden");
    const action = within(card).getByText("Review price or recipe cost");
    expect(action.className).toContain("line-clamp-3");
  });

  it("shows an honest empty state when every dish has qty_sold 0 (zero sales)", () => {
    const labels = makeLabels();
    const { container } = renderMatrix({
      report: makeReport({
        median_qty_sold: 0,
        dishes: [
          {
            menu_item_id: "zero-1",
            menu_item_name: "Unsold Pasta",
            food_cost_pct: 0.3,
            qty_sold: 0,
            avg_price: asDollars(12),
            unit_cost: asDollars(3.6),
            margin_per_unit: asDollars(8.4),
            quadrant: "star",
            action: "protect",
            suggested_price: asDollars(0),
          },
          {
            menu_item_id: "zero-2",
            menu_item_name: "Unsold Soup",
            food_cost_pct: 0.4,
            qty_sold: 0,
            avg_price: asDollars(8),
            unit_cost: asDollars(3.2),
            margin_per_unit: asDollars(4.8),
            quadrant: "dog",
            action: "cut",
            suggested_price: asDollars(0),
          },
        ],
        rollups: [
          { quadrant: "star", count: 1, revenue_share: 0 },
          { quadrant: "dog", count: 1, revenue_share: 0 },
        ],
      }),
    });

    // Honest empty — not a matrix of stars at 0,00. Copy must name the real
    // condition (no sales in the window), not "add plate costs" (#834).
    expect(screen.getByText(labels.states.emptyNoSales)).toBeInTheDocument();
    expect(screen.queryByText(labels.states.empty)).toBeNull();
    expect(
      container.querySelector('[data-testid^="dish-dot-"]'),
    ).toBeNull();
    expect(screen.queryByTestId("menu-engineering-table")).toBeNull();
    expect(screen.queryByTestId("quadrant-card-star")).toBeNull();
  });

});
