/** @jest-environment jsdom */
// src/hooks/useAnalyticsTimeseries.test.tsx
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import React from "react";
import { useAnalyticsTimeseries } from "./useAnalyticsTimeseries";
import { analyticsApi } from "@/api/analytics";

jest.mock("@/api/analytics", () => ({
  __esModule: true,
  analyticsApi: { getTimeseries: jest.fn() },
}));

const wrapper = ({ children }: { children: React.ReactNode }) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
};

describe("useAnalyticsTimeseries", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("returns the series once resolved", async () => {
    (analyticsApi.getTimeseries as jest.Mock).mockResolvedValue({
      buckets: [{ date: "2026-05-01", revenue: 5, tips: 0, bills: 1, transactions: 1, average_ticket: 5 }],
      range: { from: "2026-05-01", to: "2026-05-01" },
    });
    const { result } = renderHook(() => useAnalyticsTimeseries("42"), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.series?.buckets).toHaveLength(1);
  });

  it("is disabled (no fetch) when businessId is empty", () => {
    renderHook(() => useAnalyticsTimeseries(undefined), { wrapper });
    expect(analyticsApi.getTimeseries).not.toHaveBeenCalled();
  });
});
