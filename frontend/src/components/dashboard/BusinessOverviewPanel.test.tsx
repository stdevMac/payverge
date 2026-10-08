/** @jest-environment jsdom */
// src/components/dashboard/BusinessOverviewPanel.test.tsx
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { BusinessOverviewPanel } from "./BusinessOverviewPanel";
import type { Business } from "@/api/business";
import type { DashboardSummary } from "@/api/analytics";
import { analyticsApi } from "@/api/analytics";
import { asDollars } from "@/types/money";

jest.mock("@/api/analytics", () => ({
  __esModule: true,
  analyticsApi: { getTimeseries: jest.fn() },
}));
// TrendChart wraps react-chartjs-2's <Line>; stub it so the canvas isn't needed.
jest.mock("react-chartjs-2", () => ({ Line: () => <div data-testid="line" /> }));

const t = (key: string) => key.split(".").pop() || key;

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

function biz(id: number, currency = "USD"): Business {
  return {
    id,
    name: "La Parrilla",
    logo: "",
    is_active: true,
    default_currency: currency,
    owner_address: "0x0",
    address: { street: "", city: "", state: "", postal_code: "", country: "" },
    settlement_address: "",
    tipping_address: "",
    tax_rate: 0,
    service_fee_rate: 0,
    tax_inclusive: false,
    service_inclusive: false,
  } as Business;
}

const summary: DashboardSummary = {
  today: { revenue: asDollars(840), tips: asDollars(50), transactions: 0, bills: 0 },
  week: {
    revenue: asDollars(5000),
    tips: asDollars(0),
    transactions: 0,
    bills: 200,
    unique_customers: 0,
    average_ticket: asDollars(25),
  },
  live: { active_bills: 2 },
  top_items: [],
};

// 15-bucket series with a clean +10% WoW on tips (prior 7 = 10 each, last 7 = 11 each).
function fifteenDaySeries() {
  return {
    buckets: Array.from({ length: 15 }, (_, i) => ({
      date: `2026-05-${String(i + 1).padStart(2, "0")}`,
      revenue: 100,
      tips: i < 8 ? 10 : 11,
      bills: 4,
      transactions: 5,
      average_ticket: 25,
    })),
    range: { from: "2026-05-01", to: "2026-05-15" },
  };
}

describe("BusinessOverviewPanel", () => {
  beforeEach(() => jest.clearAllMocks());

  it("does NOT fetch the timeseries while collapsed", () => {
    render(<BusinessOverviewPanel business={biz(42)} summary={summary} isOpen={false} t={t} />, {
      wrapper,
    });
    expect(analyticsApi.getTimeseries).not.toHaveBeenCalled();
  });

  it("fetches the timeseries and renders the trend chart on open", async () => {
    (analyticsApi.getTimeseries as jest.Mock).mockResolvedValue(fifteenDaySeries());
    render(<BusinessOverviewPanel business={biz(42)} summary={summary} isOpen={true} t={t} />, {
      wrapper,
    });
    await waitFor(() => expect(analyticsApi.getTimeseries).toHaveBeenCalled());
    expect(await screen.findByTestId("line")).toBeInTheDocument();
  });

  it("shows a week-over-week delta on the tips metric computed from the series", async () => {
    (analyticsApi.getTimeseries as jest.Mock).mockResolvedValue(fifteenDaySeries());
    render(<BusinessOverviewPanel business={biz(42)} summary={summary} isOpen={true} t={t} />, {
      wrapper,
    });
    // prior 7 tips = 70, last 7 = 77 → +10.0%.
    expect(await screen.findByText(/10\.0%/)).toBeInTheDocument();
  });

  it("shows paid bills today instead of currently open bills", async () => {
    (analyticsApi.getTimeseries as jest.Mock).mockResolvedValue(fifteenDaySeries());
    render(<BusinessOverviewPanel business={biz(42)} summary={summary} isOpen={true} t={t} />, {
      wrapper,
    });

    expect(await screen.findByTestId("business-today-bills")).toHaveTextContent("0");
    expect(screen.queryByText(/^2$/)).toBeNull();
  });

  it("re-windows the fetch when a period tab is clicked", async () => {
    (analyticsApi.getTimeseries as jest.Mock).mockResolvedValue(fifteenDaySeries());
    render(<BusinessOverviewPanel business={biz(42)} summary={summary} isOpen={true} t={t} />, {
      wrapper,
    });
    await waitFor(() => expect(analyticsApi.getTimeseries).toHaveBeenCalled());
    const callsBefore = (analyticsApi.getTimeseries as jest.Mock).mock.calls.length;
    fireEvent.click(screen.getByRole("button", { name: "period90d" }));
    await waitFor(() =>
      expect((analyticsApi.getTimeseries as jest.Mock).mock.calls.length).toBeGreaterThan(callsBefore),
    );
  });

  it("links Open full analytics to the business analytics tab", async () => {
    (analyticsApi.getTimeseries as jest.Mock).mockResolvedValue(fifteenDaySeries());
    render(<BusinessOverviewPanel business={biz(42)} summary={summary} isOpen={true} t={t} />, {
      wrapper,
    });
    const link = await screen.findByRole("link", { name: /openFullAnalytics/i });
    expect(link).toHaveAttribute("href", "/business/42/dashboard?tab=analytics");
  });
});
