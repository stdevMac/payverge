/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import LaborCostCard, {
  type LaborCostCardProps,
} from "./LaborCostCard";
import type { LaborCostReport } from "@/api/laborCost";
import { asDollars } from "@/types/money";

const formatMoney = (v: number, c: string) => `${c} ${v.toFixed(2)}`;

function makeLabels(): LaborCostCardProps["labels"] {
  return {
    title: "Labor Cost",
    subtitle: "Labor as a share of sales",
    periodSelectorLabel: "Select period",
    periods: { week: "Week", month: "Month" },
    laborPct: "Labor cost",
    primeCost: "Prime cost",
    netSales: "Net sales",
    provenance: (count: number) => `Based on ${count} payroll run(s)`,
    target: "Target ~25–35% labor · ~55–65% prime",
    states: {
      empty: "Record a payroll run to see labor cost",
      noSales: "No sales in this period",
      loading: "Loading labor cost…",
    },
  };
}

function makeReport(overrides: Partial<LaborCostReport> = {}): LaborCostReport {
  return {
    period: "week",
    labor_cost: asDollars(5300),
    net_sales: asDollars(16000),
    labor_cost_pct: 0.33,
    payroll_run_count: 2,
    has_data: true,
    contributions: [],
    food_cost_pct: 0.29,
    prime_cost_pct: 0.62,
    ...overrides,
  };
}

function renderCard(overrides: Partial<LaborCostCardProps> = {}) {
  const onPeriodChange = jest.fn();
  const props: LaborCostCardProps = {
    report: makeReport(),
    loading: false,
    currency: "USD",
    selectedPeriod: "week",
    onPeriodChange,
    formatMoney,
    labels: makeLabels(),
    ...overrides,
  };
  const utils = render(<LaborCostCard {...props} />);
  return { onPeriodChange, ...utils };
}

describe("LaborCostCard", () => {
  it("renders the labor % headline (amber band at 0.33) and prime cost %", () => {
    renderCard();
    const labor = screen.getByTestId("labor-pct");
    expect(labor.textContent).toBe("33%");
    expect(labor.className).toContain("text-amber-600"); // 0.33 is >0.30, <=0.35
    const prime = screen.getByTestId("prime-pct");
    expect(prime.textContent).toBe("62%");
    expect(prime.className).toContain("text-amber-600"); // 0.62 is >0.60, <=0.65
  });

  it("colors a healthy labor % emerald and a high one rose", () => {
    const { rerender } = render(
      <LaborCostCard
        report={makeReport({ labor_cost_pct: 0.25, prime_cost_pct: 0.5 })}
        currency="USD"
        selectedPeriod="week"
        onPeriodChange={jest.fn()}
        formatMoney={formatMoney}
        labels={makeLabels()}
      />,
    );
    expect(screen.getByTestId("labor-pct").className).toContain("text-emerald-600");
    expect(screen.getByTestId("prime-pct").className).toContain("text-emerald-600");

    rerender(
      <LaborCostCard
        report={makeReport({ labor_cost_pct: 0.42, prime_cost_pct: 0.72 })}
        currency="USD"
        selectedPeriod="week"
        onPeriodChange={jest.fn()}
        formatMoney={formatMoney}
        labels={makeLabels()}
      />,
    );
    expect(screen.getByTestId("labor-pct").className).toContain("text-rose-600");
    expect(screen.getByTestId("prime-pct").className).toContain("text-rose-600");
  });

  it("shows the empty state (not 0%) when has_data is false", () => {
    renderCard({ report: makeReport({ has_data: false }) });
    expect(
      screen.getByText("Record a payroll run to see labor cost"),
    ).toBeInTheDocument();
    expect(screen.queryByTestId("labor-pct")).not.toBeInTheDocument();
    expect(screen.queryByText("0%")).not.toBeInTheDocument();
  });

  it("renders — for percentages and the no-sales hint when net_sales is 0", () => {
    renderCard({
      report: makeReport({ net_sales: asDollars(0), labor_cost_pct: 0, prime_cost_pct: 0 }),
    });
    expect(screen.getByTestId("labor-pct").textContent).toBe("—");
    expect(screen.getByTestId("prime-pct").textContent).toBe("—");
    expect(screen.getByText("No sales in this period")).toBeInTheDocument();
    // labor dollars still shown
    expect(screen.getByText(/USD 5300\.00/)).toBeInTheDocument();
  });

  it("renders provenance with the payroll run count", () => {
    renderCard({ report: makeReport({ payroll_run_count: 3 }) });
    expect(screen.getByText("Based on 3 payroll run(s)")).toBeInTheDocument();
  });

  it("surfaces implausible labor in rose and omits prime percentage", () => {
    renderCard({
      report: makeReport({
        status: "implausible",
        reason: "labor_exceeds_revenue",
        labor_cost_pct: 3.47,
        food_cost_pct: 0.1,
        prime_cost_pct: undefined,
        net_sales: asDollars(5600),
        labor_cost: asDollars(19440),
      }),
      labels: {
        ...makeLabels(),
        states: {
          ...makeLabels().states,
          implausible: "Labor exceeds sales — check data",
        },
      },
    });
    const labor = screen.getByTestId("labor-pct");
    expect(labor.textContent).toBe("347%");
    expect(labor.className).toContain("text-rose-700");
    expect(screen.getByTestId("prime-pct").textContent).toBe("—");
    expect(screen.getByTestId("labor-implausible")).toHaveTextContent(
      "Labor exceeds sales — check data",
    );
  });

  it("drives onPeriodChange when a period tab is clicked", () => {
    const { onPeriodChange } = renderCard();
    fireEvent.click(screen.getByRole("tab", { name: "Month" }));
    expect(onPeriodChange).toHaveBeenCalledWith("month");
  });

  it("shows the loading placeholder when loading=true", () => {
    renderCard({ loading: true, report: null });
    expect(screen.getByLabelText("Loading labor cost…")).toBeInTheDocument();
    expect(screen.queryByTestId("labor-pct")).not.toBeInTheDocument();
  });

  it("returns nothing when report is null and not loading", () => {
    const { container } = renderCard({ report: null, loading: false });
    expect(container.firstChild).toBeNull();
  });
});
