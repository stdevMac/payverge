/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import type {
  AccountingSummary,
  ProfitLossStatement,
  Timeseries,
} from "@/api/accounting";
import {
  useEntries,
  useProfitLoss,
  useSummary,
  useTimeseries,
} from "@/hooks/accounting/useAccountingQueries";
import { useCostHealth } from "@/hooks/accounting/useCostHealth";
import type { CostHealthView } from "@/hooks/accounting/useCostHealth";
import OverviewTab from "./OverviewTab";
import { useDishRecipeCoverage } from "./useDishRecipeCoverage";
import type { DishRecipeCoverage } from "./dishRecipeCoverage";
import type { ActivityItem } from "./RecentActivity";

jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  useSummary: jest.fn(),
  useTimeseries: jest.fn(),
  useEntries: jest.fn(),
  useProfitLoss: jest.fn(),
}));

jest.mock("@/hooks/accounting/useCostHealth", () => ({
  useCostHealth: jest.fn(),
}));

jest.mock("./useDishRecipeCoverage", () => ({
  useDishRecipeCoverage: jest.fn(),
}));

jest.mock("../premium", () => ({
  PremiumPanel: ({
    children,
    tone,
    className,
    as: As = "div",
    href,
    onClick,
    // Strip non-DOM PremiumPanel props from the mock host element.
    withTexture: _withTexture,
    interactive: _interactive,
    ...rest
  }: {
    children: React.ReactNode;
    tone?: string;
    className?: string;
    as?: "div" | "a" | "button";
    href?: string;
    onClick?: () => void;
    withTexture?: boolean;
    interactive?: boolean;
  }) => {
    void _withTexture;
    void _interactive;
    if (As === "a") {
      return (
        <a
          href={href}
          data-testid="premium-panel"
          data-tone={tone}
          className={className}
          {...rest}
        >
          {children}
        </a>
      );
    }
    if (As === "button") {
      return (
        <button
          type="button"
          onClick={onClick}
          data-testid="premium-panel"
          data-tone={tone}
          className={className}
          {...rest}
        >
          {children}
        </button>
      );
    }
    return (
      <div
        data-testid="premium-panel"
        data-tone={tone}
        className={className}
        {...rest}
      >
        {children}
      </div>
    );
  },
  AnimatedNumberText: ({
    value,
    format,
    className,
  }: {
    value: number;
    format?: (v: number) => string;
    className?: string;
  }) => (
    <span data-testid="animated-number" className={className}>
      {format ? format(value) : String(value)}
    </span>
  ),
}));

jest.mock("./CategoryBreakdown", () => ({
  __esModule: true,
  default: ({
    title,
    items,
  }: {
    title: string;
    items: Array<{ category: string; total: number }>;
  }) => (
    <div data-testid="category-breakdown" data-title={title}>
      {items.map((item) => (
        <div key={item.category}>
          {item.category}:{item.total}
        </div>
      ))}
    </div>
  ),
}));

jest.mock("./NeedsAttentionStrip", () => ({
  __esModule: true,
  default: () => <div data-testid="needs-attention-strip-stub" />,
}));

jest.mock("./RecentActivity", () => ({
  __esModule: true,
  default: ({
    title,
    items,
  }: {
    title: string;
    items: ActivityItem[];
  }) => (
    <div data-testid="recent-activity">
      <span>{title}</span>
      <span data-testid="activity-count">{items.length}</span>
    </div>
  ),
}));

const useSummaryMock = useSummary as jest.Mock;
const useTimeseriesMock = useTimeseries as jest.Mock;
const useEntriesMock = useEntries as jest.Mock;
const useProfitLossMock = useProfitLoss as jest.Mock;
const useCostHealthMock = useCostHealth as jest.Mock;
const useDishRecipeCoverageMock = useDishRecipeCoverage as jest.Mock;

function dishCoverage(
  overrides: Partial<DishRecipeCoverage> = {},
): DishRecipeCoverage {
  return {
    mapped: 0,
    total: 0,
    ratio: null,
    incomplete: false,
    ...overrides,
  };
}

function costHealthView(
  overrides: Partial<CostHealthView> = {},
): CostHealthView {
  return {
    status: "insufficient_data",
    foodPct: null,
    laborPct: null,
    primePct: null,
    recipeCoveragePct: null,
    lowCoverage: false,
    report: null,
    ...overrides,
  };
}

const t = (key: string) => key;
const tWith = (key: string, replacements: Record<string, string | number>) => {
  let value = key;
  Object.entries(replacements).forEach(([name, replacement]) => {
    value = value.replace(new RegExp(`\\{${name}\\}`, "g"), String(replacement));
  });
  return value;
};

function tWithVisibleParams(
  key: string,
  replacements: Record<string, string | number>,
): string {
  let value = key;
  Object.entries(replacements).forEach(([name, replacement]) => {
    const token = `{${name}}`;
    value = value.includes(token)
      ? value.replace(new RegExp(`\\{${name}\\}`, "g"), String(replacement))
      : `${value} ${replacement}`;
  });
  return value;
}

const fmtMoney = (value: number, currency: string) =>
  `${currency} ${value.toFixed(2)}`;

function summary(
  overrides: Partial<AccountingSummary> = {},
): AccountingSummary {
  return {
    start_date: "2026-01-01",
    end_date: "2026-01-31",
    currency: "USD",
    auto_income_total: 1000 as AccountingSummary["auto_income_total"],
    manual_income_total: 200 as AccountingSummary["manual_income_total"],
    expense_total: 400 as AccountingSummary["expense_total"],
    payroll_total: 300 as AccountingSummary["payroll_total"],
    billed_total: 1200 as AccountingSummary["billed_total"],
    collected_total: 1000 as AccountingSummary["collected_total"],
    collection_gap: 200 as AccountingSummary["collection_gap"],
    income_breakdown: [
      { category: "catering", total: 150 as AccountingSummary["manual_income_total"] },
      { category: "event", total: 50 as AccountingSummary["manual_income_total"] },
    ],
    expense_breakdown: [
      { category: "rent", total: 250 as AccountingSummary["expense_total"] },
      { category: "utilities", total: 150 as AccountingSummary["expense_total"] },
    ],
    payroll_summary: {
      paid_runs: 2,
      total_gross: 280 as AccountingSummary["payroll_total"],
      total_bonus: 20 as AccountingSummary["payroll_total"],
      total_deduction: 0 as AccountingSummary["payroll_total"],
      total_net: 300 as AccountingSummary["payroll_total"],
    },
    ...overrides,
  };
}

function timeseries(points: Timeseries["series"] = []): Timeseries {
  return {
    start: "2026-01-01",
    end: "2026-01-31",
    bucket: "day",
    currency: "USD",
    series: points,
  };
}

/** P&L statement: net = 1000+200−80 cogs−300 labor−400 opex = 420 */
function profitLoss(
  overrides: Partial<ProfitLossStatement["current"]> = {},
): ProfitLossStatement {
  return {
    current: {
      start_date: "2026-01-01",
      end_date: "2026-01-31",
      currency: "USD",
      revenue: 1000 as ProfitLossStatement["current"]["revenue"],
      other_income: 200 as ProfitLossStatement["current"]["other_income"],
      other_income_by_category: [],
      cogs: 80 as ProfitLossStatement["current"]["cogs"],
      labor: 300 as ProfitLossStatement["current"]["labor"],
      opex: 400 as ProfitLossStatement["current"]["opex"],
      opex_by_category: [],
      net: 420 as ProfitLossStatement["current"]["net"],
      ...overrides,
    },
  };
}

const activityItems: ActivityItem[] = [
  {
    id: "entry-1",
    kind: "entry_created",
    title: "Entry recorded",
    detail: "Catering · USD 50.00",
    occurredAt: "2026-01-20T12:00:00Z",
  },
];

function renderOverview(
  props: Partial<React.ComponentProps<typeof OverviewTab>> = {},
) {
  return render(
    <OverviewTab
      businessId="42"
      start="2026-01-01"
      end="2026-01-31"
      locale="en"
      t={t}
      tWith={tWith}
      fmtMoney={fmtMoney}
      activityItems={activityItems}
      formatOccurredAt={(iso) => iso}
      onSubTabChange={jest.fn()}
      {...props}
    />,
  );
}

describe("OverviewTab", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    useSummaryMock.mockReturnValue({
      data: summary(),
      isLoading: false,
      isError: false,
    });
    useTimeseriesMock.mockReturnValue({
      data: timeseries([
        { date: "2026-01-01", income: 10, expense: 5, payroll: 2 },
        { date: "2026-01-02", income: 20, expense: 8, payroll: 3 },
        { date: "2026-01-03", income: 15, expense: 4, payroll: 1 },
      ]),
      isLoading: false,
      isError: false,
    });
    useEntriesMock.mockReturnValue({ data: undefined });
    useProfitLossMock.mockReturnValue({
      data: profitLoss(),
      isLoading: false,
      isError: false,
    });
    useCostHealthMock.mockReturnValue({
      data: costHealthView(),
      isLoading: false,
      isError: false,
    });
    useDishRecipeCoverageMock.mockReturnValue({
      data: dishCoverage(),
      isLoading: false,
    });
  });

  it("reads KPI totals from useSummary and sparklines from useTimeseries", () => {
    renderOverview();

    expect(useSummaryMock).toHaveBeenCalledWith("42", {
      start: "2026-01-01",
      end: "2026-01-31",
    });
    expect(useTimeseriesMock).toHaveBeenCalledWith("42", {
      start: "2026-01-01",
      end: "2026-01-31",
    });
    expect(useProfitLossMock).toHaveBeenCalledWith(
      "42",
      { start: "2026-01-01", end: "2026-01-31" },
      false,
    );
    // Overview must not hydrate the ledger for charts/KPIs.
    expect(useEntriesMock).not.toHaveBeenCalled();

    expect(screen.getByText("overview.kpi.billIncome")).toBeInTheDocument();
    expect(screen.getByText("overview.kpi.otherIncome")).toBeInTheDocument();
    expect(screen.getByText("overview.kpi.expenses")).toBeInTheDocument();
    expect(screen.getByText("overview.kpi.payroll")).toBeInTheDocument();

    // Metric values for the four KPIs + net (from P&L) + gap (at least).
    // Metric owns zero/empty/negative tone — no more AnimatedNumberText here.
    // Amounts also appear on the P&L composition strip, so use getAllByText.
    const text = document.body.textContent ?? "";
    expect(text).toContain("USD 1000.00");
    expect(text).toContain("USD 200.00");
    expect(text).toContain("USD 400.00");
    expect(text).toContain("USD 300.00");
    // Net from P&L (includes COGS), not the legacy summary-only net.
    expect(text).toContain("USD 420.00");

    // Non-zero series render sparklines via Metric (all-zero would be suppressed).
    const series = screen.getAllByTestId("metric-series");
    expect(series.length).toBeGreaterThanOrEqual(3);
  });

  it("renders CategoryBreakdown panels from summary income/expense breakdowns", () => {
    renderOverview();
    const panels = screen.getAllByTestId("category-breakdown");
    expect(panels).toHaveLength(2);
    expect(panels[0]).toHaveAttribute(
      "data-title",
      "overview.breakdown.incomeTitle",
    );
    expect(panels[1]).toHaveAttribute(
      "data-title",
      "overview.breakdown.expenseTitle",
    );
    expect(panels[0]).toHaveTextContent("catering:150");
    expect(panels[1]).toHaveTextContent("rent:250");
  });

  it("tones net P&L danger when negative (from useProfitLoss)", () => {
    useProfitLossMock.mockReturnValue({
      data: profitLoss({ net: -120 as ProfitLossStatement["current"]["net"] }),
      isLoading: false,
      isError: false,
    });
    renderOverview();
    const netPanel = screen.getByTestId("overview-net-pnl");
    expect(netPanel).toHaveAttribute("data-tone", "danger");
    expect(netPanel).toHaveTextContent("USD -120.00");
  });

  it("collection-gap tile deep-links to outstanding subtab", () => {
    renderOverview();
    const gap = screen.getByTestId("overview-collection-gap");
    const href = gap.getAttribute("href") || "";
    expect(href).toMatch(/tab=accounting/);
    expect(href).toMatch(/sub=outstanding/);
  });

  it("discloses collection gap is this date range and Outstanding includes older debt", () => {
    renderOverview({
      start: "2026-07-16",
      end: "2026-08-14",
      tWith: tWithVisibleParams,
    });

    const gap = screen.getByTestId("overview-collection-gap");
    expect(gap).toHaveTextContent("overview.kpi.collectionGapHint");

    const scope = screen.getByTestId("overview-collection-gap-scope");
    expect(scope).toHaveTextContent("overview.kpi.collectionGapScope");
    expect(scope).toHaveTextContent("Jul 16, 2026");
    expect(scope).toHaveTextContent("Aug 14, 2026");
  });

  it("shows P&L composition strip with COGS and RecentActivity full-width", () => {
    renderOverview();
    expect(
      screen.getByText("overview.reconciliation.title"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("overview.reconciliation.revenue"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("overview.reconciliation.otherIncome"),
    ).toBeInTheDocument();
    expect(screen.getByText("overview.reconciliation.cogs")).toBeInTheDocument();
    expect(
      screen.getByText("overview.reconciliation.labor"),
    ).toBeInTheDocument();
    expect(screen.getByText("overview.reconciliation.opex")).toBeInTheDocument();
    expect(screen.getByTestId("overview-pnl-cogs")).toHaveTextContent(
      "USD 80.00",
    );
    expect(screen.getByTestId("overview-pnl-composition")).toBeInTheDocument();
    expect(screen.getByTestId("recent-activity")).toBeInTheDocument();
    expect(screen.getByTestId("activity-count")).toHaveTextContent("1");

    // Legacy 5-row preview tables removed.
    expect(screen.queryByText("entries.title")).toBeNull();
    expect(screen.queryByText("payrollRuns.title")).toBeNull();
  });

  it("shows currency note from summary currency", () => {
    renderOverview();
    expect(screen.getByText("overview.currencyNote")).toBeInTheDocument();
  });

  it("renders Cost health strip linking to Analytics with food/labor/prime stats", async () => {
    useCostHealthMock.mockReturnValue({
      data: costHealthView({
        status: "ok",
        foodPct: 0.28,
        laborPct: 0.3,
        primePct: 0.58,
        recipeCoveragePct: 1,
        lowCoverage: false,
      }),
      isLoading: false,
      isError: false,
    });

    renderOverview();

    const strip = await screen.findByTestId("overview-cost-health");
    expect(strip).toHaveAttribute("href", "?tab=analytics");
    expect(screen.getByText("overview.costHealth")).toBeInTheDocument();
    expect(screen.getByText("overview.viewInAnalytics")).toBeInTheDocument();

    await waitFor(() => {
      expect(strip).toHaveTextContent("28%");
      expect(strip).toHaveTextContent("30%");
      expect(strip).toHaveTextContent("58%");
    });

    // Full cost cards no longer live on Accounting Overview.
    expect(screen.queryByTestId("labor-cost-card")).toBeNull();
    expect(screen.queryByTestId("waste-variance-card")).toBeNull();
    expect(screen.queryByTestId("cost-health-coverage-caveat")).toBeNull();
  });

  it("caveats food cost and net P&L when recipe coverage is thin", async () => {
    useCostHealthMock.mockReturnValue({
      data: costHealthView({
        status: "ok",
        foodPct: 0.1,
        laborPct: 0.31,
        primePct: 0.41,
        recipeCoveragePct: 0.35,
        lowCoverage: true,
      }),
      isLoading: false,
      isError: false,
    });

    renderOverview();

    await waitFor(() => {
      expect(screen.getByTestId("cost-health-coverage-caveat")).toHaveTextContent(
        "overview.costHealthCoverageCaveat",
      );
      expect(screen.getByTestId("overview-net-cogs-caveat")).toHaveTextContent(
        "overview.kpi.netCogsCaveat",
      );
      expect(screen.getByTestId("overview-pnl-cogs-hint")).toHaveTextContent(
        "overview.reconciliation.cogsCoverageHint",
      );
    });
    expect(screen.getByTestId("cost-health-food")).toHaveTextContent("10%");
    expect(screen.getByTestId("cost-health-food").className).toContain(
      "text-amber-800",
    );
  });

  it("requests cost-health for the selected start/end range via useCostHealth", () => {
    renderOverview({ start: "2026-07-01", end: "2026-07-15" });

    expect(useCostHealthMock).toHaveBeenCalledWith("42", {
      start: "2026-07-01",
      end: "2026-07-15",
    });
    expect(useDishRecipeCoverageMock).toHaveBeenCalledWith("42");
  });

  it("caveats a 10% food-cost headline when only 3 of 6 dishes have recipes", async () => {
    useCostHealthMock.mockReturnValue({
      data: costHealthView({
        status: "ok",
        foodPct: 0.1,
        laborPct: 0.31,
        primePct: 0.41,
        recipeCoveragePct: 0.8,
        lowCoverage: false,
      }),
      isLoading: false,
      isError: false,
    });
    useDishRecipeCoverageMock.mockReturnValue({
      data: dishCoverage({
        mapped: 3,
        total: 6,
        ratio: 0.5,
        incomplete: true,
      }),
      isLoading: false,
    });

    renderOverview();

    const strip = await screen.findByTestId("overview-cost-health");
    const stats = strip.querySelector("[data-low-coverage]");
    expect(stats).toHaveAttribute("data-low-coverage", "true");
    expect(stats).toHaveAttribute("data-coverage-kind", "dish");
    expect(stats).toHaveAttribute("data-dish-mapped", "3");
    expect(stats).toHaveAttribute("data-dish-total", "6");
    expect(screen.getByTestId("cost-health-coverage-caveat")).toHaveAttribute(
      "data-coverage-kind",
      "dish",
    );
    expect(screen.getByTestId("cost-health-coverage-caveat")).toHaveTextContent(
      "overview.costHealthDishCoverageCaveat",
    );
    expect(screen.getByTestId("cost-health-food")).toHaveTextContent("10%");
    expect(screen.getByTestId("cost-health-food").className).toContain(
      "text-amber-800",
    );
    expect(screen.getByTestId("overview-net-cogs-caveat")).toHaveTextContent(
      "overview.kpi.netCogsDishCaveat",
    );
    expect(screen.getByTestId("overview-pnl-cogs-hint")).toHaveTextContent(
      "overview.reconciliation.cogsDishHint",
    );
  });

  it("shows not-enough-data copy when cost ratios have no revenue basis", async () => {
    useCostHealthMock.mockReturnValue({
      data: costHealthView({ status: "insufficient_data" }),
      isLoading: false,
      isError: false,
    });

    renderOverview();

    await waitFor(() => {
      expect(screen.getByTestId("cost-health-food")).toHaveTextContent(
        "overview.costHealthNotEnoughData",
      );
      expect(screen.getByTestId("cost-health-labor")).toHaveTextContent(
        "overview.costHealthNotEnoughData",
      );
      expect(screen.getByTestId("cost-health-prime")).toHaveTextContent(
        "overview.costHealthNotEnoughData",
      );
    });
    expect(screen.queryByText("0%")).toBeNull();
  });

  it("surfaces implausible status without inventing a prime percentage", async () => {
    useCostHealthMock.mockReturnValue({
      data: costHealthView({
        status: "implausible",
        reason: "labor_exceeds_revenue",
        foodPct: 0.1,
        laborPct: 3.47,
        primePct: null,
      }),
      isLoading: false,
      isError: false,
    });

    renderOverview();

    await waitFor(() => {
      expect(screen.getByTestId("cost-health-labor")).toHaveTextContent("347%");
      expect(screen.getByTestId("cost-health-labor").className).toContain(
        "text-rose-700",
      );
      // Prime omitted — show implausible copy, never a composed green %.
      expect(screen.getByTestId("cost-health-prime")).toHaveTextContent(
        "overview.costHealthImplausible",
      );
    });
    expect(screen.queryByText("0%")).toBeNull();
  });
});
