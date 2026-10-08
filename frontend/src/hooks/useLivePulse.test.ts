/** @jest-environment jsdom */
import { renderHook, waitFor } from "@testing-library/react";
import { useLivePulse } from "./useLivePulse";
import { analyticsApi } from "@/api/analytics";

jest.mock("@/api/analytics", () => ({
  __esModule: true,
  analyticsApi: {
    getDashboardSummary: jest.fn(),
    getLiveBillsPage: jest.fn(),
  },
}));
// Keep polling inert so the test doesn't leak timers — we only assert the
// initial consolidated fetch.
jest.mock("@/hooks/usePolling", () => ({
  usePolling: () => ({ isPolling: true, startPolling: jest.fn(), stopPolling: jest.fn() }),
}));

const mockSummary = analyticsApi.getDashboardSummary as jest.Mock;
const mockBills = analyticsApi.getLiveBillsPage as jest.Mock;

describe("useLivePulse", () => {
  beforeEach(() => jest.clearAllMocks());

  it("fetches summary AND live bills in one initial cycle and exposes both", async () => {
    mockSummary.mockResolvedValue({
      today: { revenue: 100, tips: 10, bills: 5 },
      week: { revenue: 700, tips: 70, bills: 35 },
      live: { active_bills: 2 },
      top_items: [],
    });
    mockBills.mockResolvedValue({
      bills: [{ id: 1, bill_number: "B1" }],
      capped: true,
      limit: 200,
    });

    const { result } = renderHook(() => useLivePulse("42"));

    await waitFor(() => expect(result.current.summary).not.toBeNull());
    // ONE consolidated cycle → each endpoint hit exactly once, not twice.
    expect(mockSummary).toHaveBeenCalledTimes(1);
    expect(mockBills).toHaveBeenCalledTimes(1);
    expect(result.current.liveBills).toHaveLength(1);
    expect(result.current.capped).toBe(true);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("surfaces an error and keeps loading false when a read fails", async () => {
    mockSummary.mockRejectedValue(new Error("boom"));
    mockBills.mockResolvedValue({ bills: [], capped: false, limit: 200 });

    const { result } = renderHook(() => useLivePulse("42"));

    await waitFor(() => expect(result.current.error).toBeTruthy());
    expect(result.current.loading).toBe(false);
  });
});
