/** @jest-environment jsdom */
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { CombinedOverviewPanel } from "./CombinedOverviewPanel";
import type { Business } from "@/api/business";
import { analyticsApi } from "@/api/analytics";

jest.mock("@/api/analytics", () => ({
  __esModule: true,
  analyticsApi: { getTimeseries: jest.fn() },
}));

jest.mock("react-chartjs-2", () => ({
  Line: () => <div data-testid="line" />,
}));

const t = (key: string) => key.split(".").pop() || key;

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

function biz(id: number, currency = "USD"): Business {
  return {
    id,
    name: `Venue ${id}`,
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

function activeSeries() {
  return {
    buckets: [
      {
        date: "2026-08-01",
        revenue: 120,
        tips: 10,
        bills: 3,
        transactions: 3,
        average_ticket: 40,
      },
    ],
    range: { from: "2026-08-01", to: "2026-08-01" },
  };
}

describe("CombinedOverviewPanel", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("says No activity today instead of asking to select a business", async () => {
    (analyticsApi.getTimeseries as jest.Mock).mockResolvedValue({
      buckets: [],
      range: { from: "", to: "" },
    });

    render(
      <CombinedOverviewPanel
        businesses={[biz(1), biz(2)]}
        todayHasActivity={false}
        statsLoading={false}
        t={t}
      />,
      { wrapper },
    );

    await waitFor(() =>
      expect(analyticsApi.getTimeseries).toHaveBeenCalledTimes(2),
    );
    expect(await screen.findByTestId("chart-empty")).toHaveTextContent(
      "noActivityToday",
    );
    expect(screen.queryByText(/selectBusinessHint/i)).toBeNull();
  });

  it("renders the combined series when venues have history even if today is quiet", async () => {
    (analyticsApi.getTimeseries as jest.Mock).mockResolvedValue(activeSeries());

    render(
      <CombinedOverviewPanel
        businesses={[biz(1), biz(2)]}
        todayHasActivity={false}
        statsLoading={false}
        t={t}
      />,
      { wrapper },
    );

    expect(await screen.findByTestId("line")).toBeInTheDocument();
    expect(screen.getByTestId("portfolio-no-activity-today")).toHaveTextContent(
      "noActivityToday",
    );
    expect(screen.queryByText(/selectBusinessHint/i)).toBeNull();
  });

  it("does not show the today-empty caption when today already has activity", async () => {
    (analyticsApi.getTimeseries as jest.Mock).mockResolvedValue(activeSeries());

    render(
      <CombinedOverviewPanel
        businesses={[biz(1)]}
        todayHasActivity
        statsLoading={false}
        t={t}
      />,
      { wrapper },
    );

    expect(await screen.findByTestId("line")).toBeInTheDocument();
    expect(screen.queryByTestId("portfolio-no-activity-today")).toBeNull();
    expect(screen.queryByText(/selectBusinessHint/i)).toBeNull();
  });
});
