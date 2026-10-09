/** @jest-environment jsdom */
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import RevenuePanel from "./RevenuePanel";
import { analyticsApi } from "@/api/analytics";

const mockPush = jest.fn();
jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
  usePathname: () => "/business/42/dashboard",
}));

jest.mock("@/api/analytics", () => ({
  __esModule: true,
  analyticsApi: {
    getSalesAnalytics: jest.fn(),
    getLiveBills: jest.fn().mockResolvedValue([]),
    getDashboardSummary: jest.fn().mockResolvedValue(null),
  },
}));
jest.mock("@/api/business", () => ({ getBusiness: jest.fn().mockResolvedValue({ default_currency: "USD" }) }));
// 15 buckets so the "last 7 vs prior 7" period-over-period delta has both
// windows populated (prev7 = 700, last7 = 770 → +10%).
jest.mock("@/hooks/useAnalyticsTimeseries", () => ({
  useAnalyticsTimeseries: () => ({
    series: {
      buckets: Array.from({ length: 15 }, (_, i) => ({
        date: `2026-05-${String(i + 1).padStart(2, "0")}`,
        revenue: i < 8 ? 100 : 110, // prior 7 (indices 1-7) = 100, last 7 (8-14) = 110
        tips: 10,
        bills: 4,
        transactions: 5,
        average_ticket: 25,
      })),
      range: { from: "2026-05-01", to: "2026-05-15" },
    },
    loading: false,
    error: null,
  }),
}));
jest.mock("react-chartjs-2", () => ({ Line: () => <div data-testid="line" />, Bar: () => <div data-testid="bar" /> }));

describe("RevenuePanel", () => {
  it("renders headline metrics and the daily trend, with no doughnut", async () => {
    (analyticsApi.getSalesAnalytics as jest.Mock).mockResolvedValue({
      total_revenue: 300, total_tips: 30, transaction_count: 14, bill_count: 12,
      unique_customers: 9, average_ticket: 25,
      payment_methods: { crypto: 8, card: 6 },
      hourly_breakdown: { "12": { revenue: 120, bill_count: 4, transaction_count: 5 } },
    });
    render(<RevenuePanel businessId="42" />);
    await waitFor(() => expect(screen.getByTestId("line")).toBeInTheDocument()); // daily trend
    expect(screen.getByRole("list", { name: /payment/i })).toBeInTheDocument(); // ranked list, not doughnut
  });

  // The revenue delta carries a PERIOD-AWARE baseline label. The default period
  // is "today", so it reads "vs yesterday" (not the old fixed "vs prior 7 days")
  // and is still distinct from the live tab's "vs typical day" comparison.
  // It is driven by the backend period-matched `growth_rate`.
  it("labels the revenue delta with its period-aware prior baseline", async () => {
    (analyticsApi.getSalesAnalytics as jest.Mock).mockResolvedValue({
      total_revenue: 300, total_tips: 30, transaction_count: 14, bill_count: 12,
      unique_customers: 9, average_ticket: 25,
      growth_rate: 12.5,
      payment_methods: { crypto: 8, card: 6 },
      hourly_breakdown: {},
    });
    render(<RevenuePanel businessId="42" />);
    await waitFor(() => expect(screen.getByTestId("line")).toBeInTheDocument());
    // Default period "today" -> "vs yesterday"; not the live tab's "vs typical day".
    expect(await screen.findByText(/vs yesterday/i)).toBeInTheDocument();
    expect(screen.queryByText(/vs typical day/i)).not.toBeInTheDocument();
  });

  // When the backend omits growth_rate (no prior-period baseline yet) the delta
  // is suppressed rather than fabricated from a mismatched 7d-vs-7d window.
  it("suppresses the delta when the backend reports no growth_rate", async () => {
    (analyticsApi.getSalesAnalytics as jest.Mock).mockResolvedValue({
      total_revenue: 300, total_tips: 30, transaction_count: 14, bill_count: 12,
      unique_customers: 9, average_ticket: 25,
      payment_methods: { crypto: 8, card: 6 },
      hourly_breakdown: {},
    });
    render(<RevenuePanel businessId="42" />);
    await waitFor(() => expect(screen.getByTestId("line")).toBeInTheDocument());
    expect(screen.queryByTestId("metric-delta")).not.toBeInTheDocument();
  });

  // E1: zero-data was a dead end (a static icon + "No sales data available"
  // with no next step). Now it's a directed TabEmptyState pointing the
  // operator back to Overview to finish setup.
  it("renders a directed empty state (not a dead end) when there is no sales data yet", async () => {
    mockPush.mockClear();
    (analyticsApi.getSalesAnalytics as jest.Mock).mockResolvedValue(null);
    render(<RevenuePanel businessId="42" />);

    expect(await screen.findByText("No sales yet")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Analytics light up after your first order. Get your menu and QR codes live to take it.",
      ),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Go to setup" }));
    expect(mockPush).toHaveBeenCalledWith("/business/42/dashboard?tab=overview");
  });
});
