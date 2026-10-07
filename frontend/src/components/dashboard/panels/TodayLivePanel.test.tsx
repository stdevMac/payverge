/** @jest-environment jsdom */
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import TodayLivePanel from "./TodayLivePanel";
import { analyticsApi } from "@/api/analytics";

jest.mock("@/api/analytics", () => ({
  __esModule: true,
  analyticsApi: {
    getDashboardSummary: jest.fn(),
    getLiveBills: jest.fn().mockResolvedValue([]),
    // Fix 5: TodayLivePanel now drives the shared live poller (useLivePulse),
    // which reads live bills via getLiveBillsPage in the same cycle.
    getLiveBillsPage: jest
      .fn()
      .mockResolvedValue({ bills: [], capped: false, limit: 200 }),
  },
}));
jest.mock("@/api/business", () => ({ getBusiness: jest.fn().mockResolvedValue({ default_currency: "USD" }) }));
jest.mock("../LiveBills", () => ({ __esModule: true, default: () => <div data-testid="live-bills" /> }));

describe("TodayLivePanel", () => {
  // The "vs typical day" baseline excludes today: with today's revenue 500 and
  // week total 2100, the prior-6-day average is (2100-500)/6 = 266.67, so the
  // delta is (500-266.67)/266.67 = +87.5% (not the +66.7% you'd get if today
  // polluted its own baseline via week/7).
  it("compares today tips against a baseline that excludes today", async () => {
    (analyticsApi.getDashboardSummary as jest.Mock).mockResolvedValue({
      today: { revenue: 500, tips: 60, transactions: 20, bills: 18 },
      week: { revenue: 2100, tips: 210, transactions: 84, bills: 70, unique_customers: 50, average_ticket: 30 },
      live: { active_bills: 3 },
      top_items: [],
    });
    render(<TodayLivePanel businessId="42" />);
    // Tips: today 60, week 210 → prior avg (210-60)/6 = 25 → +140%
    await waitFor(() => expect(screen.getByText(/140\.0%/)).toBeInTheDocument());
    expect(screen.getByTestId("live-bills")).toBeInTheDocument();
    expect(screen.getByTestId("today-live-revenue-basis")).toBeInTheDocument();
  });

  it("does not repeat hero revenue/bills/open-tables as a second KPI row", async () => {
    (analyticsApi.getDashboardSummary as jest.Mock).mockResolvedValue({
      today: { revenue: 500, tips: 60, transactions: 20, bills: 18 },
      week: { revenue: 2100, tips: 210, transactions: 84, bills: 70, unique_customers: 50, average_ticket: 30 },
      live: { open_tables: 2, active_bills: 3 },
      top_items: [],
    });
    render(<TodayLivePanel businessId="42" />);
    await waitFor(() =>
      expect(screen.getByTestId("today-live-revenue")).toBeInTheDocument(),
    );
    // Hero owns these once; tips is the only MetricStat under it.
    expect(screen.getAllByTestId("today-live-revenue")).toHaveLength(1);
    expect(screen.getAllByTestId("today-live-bills")).toHaveLength(1);
    expect(screen.getAllByTestId("today-live-open-tables")).toHaveLength(1);
  });

  // Early in the day a $0 tile against a real week baseline would compute a
  // blunt -100% "vs typical day", which reads as catastrophic. Instead we
  // suppress the delta and show a neutral "no sales yet today" note.
  it("suppresses the -100% delta on a zero-value day and shows a neutral note", async () => {
    (analyticsApi.getDashboardSummary as jest.Mock).mockResolvedValue({
      today: { revenue: 0, tips: 0, transactions: 0, bills: 0 },
      week: { revenue: 2100, tips: 210, transactions: 84, bills: 70, unique_customers: 50, average_ticket: 30 },
      live: { active_bills: 0 },
      top_items: [],
    });
    render(<TodayLivePanel businessId="42" />);
    await waitFor(() =>
      expect(screen.getAllByTestId("metric-neutral-note").length).toBeGreaterThan(0),
    );
    // No catastrophic down-delta anywhere.
    expect(screen.queryByText(/100\.0%/)).not.toBeInTheDocument();
    expect(screen.queryByTestId("metric-delta")).not.toBeInTheDocument();
  });

  it("shows an error banner when the summary fetch fails", async () => {
    (analyticsApi.getDashboardSummary as jest.Mock).mockRejectedValue(
      new Error("boom"),
    );
    render(<TodayLivePanel businessId="42" />);
    // L20: the banner shows a localized generic message (common.errors.default),
    // never the raw axios/error text — a bare Error maps to the default key.
    await waitFor(() =>
      expect(
        screen.getByText(/an unexpected error occurred/i),
      ).toBeInTheDocument(),
    );
    expect(screen.queryByText(/boom/)).not.toBeInTheDocument();
    expect(screen.getByRole("alert")).toBeInTheDocument();

    (analyticsApi.getDashboardSummary as jest.Mock).mockResolvedValueOnce({
      today: { revenue: 0, tips: 0, transactions: 0, bills: 0 },
      week: { revenue: 0, tips: 0, transactions: 0, bills: 0 },
      live: { active_bills: 0 },
      top_items: [],
    });
    const callsBeforeRetry = (analyticsApi.getDashboardSummary as jest.Mock)
      .mock.calls.length;
    fireEvent.click(screen.getByRole("button", { name: /try again/i }));
    await waitFor(() =>
      expect(
        (analyticsApi.getDashboardSummary as jest.Mock).mock.calls.length,
      ).toBeGreaterThan(callsBeforeRetry),
    );
  });

  // Task 16: "Open tables" must count distinct occupied tables, not active_bills.
  // Three delivery/counter bills (no table) must not inflate the metric.
  it("shows open_tables (distinct tables), not active_bills bill count", async () => {
    (analyticsApi.getDashboardSummary as jest.Mock).mockResolvedValue({
      today: { revenue: 100, tips: 10, transactions: 4, bills: 4 },
      week: {
        revenue: 700,
        tips: 70,
        transactions: 28,
        bills: 24,
        unique_customers: 20,
        average_ticket: 30,
      },
      // 5 active bills but only 2 tables occupied; open_tables is the truth.
      live: {
        active_bills: 5,
        open_tables: 2,
        active_bills_by_table: {
          1: { count: 1, oldest_bill_id: 10, oldest_minutes: 20 },
          2: { count: 1, oldest_bill_id: 11, oldest_minutes: 15 },
        },
      },
      top_items: [],
    });
    render(<TodayLivePanel businessId="42" />);
    await waitFor(() =>
      expect(screen.getByTestId("today-live-open-tables")).toBeInTheDocument(),
    );
    // Hero open-tables Metric shows 2, never the inflated bill count 5.
    const openTablesMetric = screen.getByTestId("today-live-open-tables");
    expect(openTablesMetric).toHaveTextContent("2");
    expect(openTablesMetric).not.toHaveTextContent("5");
  });

  it("labels leftover remaining separately and does not sell it as collected", async () => {
    (analyticsApi.getDashboardSummary as jest.Mock).mockResolvedValue({
      today: {
        revenue: 0,
        collected_revenue: 0,
        floor_remaining: 23.86,
        tips: 0,
        transactions: 0,
        bills: 0,
      },
      week: {
        revenue: 19.5,
        tips: 8.4,
        transactions: 1,
        bills: 1,
        unique_customers: 1,
        average_ticket: 19.5,
      },
      live: { active_bills: 2, open_tables: 2 },
      top_items: [],
    });
    render(<TodayLivePanel businessId="86" />);
    await waitFor(() =>
      expect(screen.getByTestId("today-live-remaining")).toBeInTheDocument(),
    );
    expect(screen.getByTestId("today-live-remaining")).toHaveTextContent(
      "23.86",
    );
    expect(screen.getByTestId("today-live-bills")).toHaveTextContent("0");
    expect(screen.getByTestId("today-live-revenue-basis")).toBeInTheDocument();
  });
});
