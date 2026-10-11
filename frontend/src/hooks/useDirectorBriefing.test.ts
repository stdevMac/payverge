/** @jest-environment jsdom */
import { renderHook, waitFor, act } from "@testing-library/react";

import { useDirectorBriefing } from "./useDirectorBriefing";
import { getDirectorBriefing, type BriefingResponse } from "@/api/directorConsole";
import type { SSEEvent } from "@/hooks/useSSEEvents";

jest.mock("@/api/directorConsole", () => ({
  getDirectorBriefing: jest.fn(),
}));

// Capture the SSE hook's callbacks so the test can drive `bill.stuck` /
// reconnect without a real EventSource (jsdom has none).
let capturedOnEvent: ((e: SSEEvent) => void) | undefined;
let capturedOnReconnect: (() => void) | undefined;
let capturedEnabled: boolean | undefined;
jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: (opts: {
    enabled: boolean;
    onEvent: (e: SSEEvent) => void;
    onReconnect?: () => void;
  }) => {
    capturedOnEvent = opts.onEvent;
    capturedOnReconnect = opts.onReconnect;
    capturedEnabled = opts.enabled;
    return { degraded: false, blocked: false, reconnect: jest.fn() };
  },
}));

const mockGet = getDirectorBriefing as jest.Mock;

function fixture(overrides: Partial<BriefingResponse> = {}): BriefingResponse {
  return {
    state: "active",
    pulse: {
      revenue: 4200,
      projected: 4700,
      typical_day: 4200,
      pace_pct: 12,
      orders: 84,
      avg_ticket: 37,
      food_cost_pct: 0.28,
      labor_cost_pct: 0.24,
      open_bills: 3,
    },
    insights: [],
    play: null,
    win: null,
    ...overrides,
  };
}

describe("useDirectorBriefing", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    capturedOnEvent = undefined;
    capturedOnReconnect = undefined;
    capturedEnabled = undefined;
  });

  it("loads the briefing on mount when enabled", async () => {
    mockGet.mockResolvedValue(fixture());
    const { result } = renderHook(() => useDirectorBriefing(42, true));

    await waitFor(() => expect(result.current.briefing).not.toBeNull());
    expect(mockGet).toHaveBeenCalledWith(42);
    expect(result.current.briefing?.pulse.orders).toBe(84);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBe(false);
    expect(result.current.fetchedAt).toBeInstanceOf(Date);
  });

  it("does not fetch (or subscribe to SSE) when disabled", async () => {
    mockGet.mockResolvedValue(fixture());
    renderHook(() => useDirectorBriefing(42, false));
    await waitFor(() => expect(capturedEnabled).toBe(false));
    expect(mockGet).not.toHaveBeenCalled();
  });

  it("refetches the whole briefing on a bill.stuck SSE event", async () => {
    mockGet.mockResolvedValue(fixture());
    renderHook(() => useDirectorBriefing(42, true));
    await waitFor(() => expect(mockGet).toHaveBeenCalledTimes(1));

    await act(async () => {
      capturedOnEvent?.({ type: "bill.stuck", data: {} });
    });
    await waitFor(() => expect(mockGet).toHaveBeenCalledTimes(2));
  });

  it("ignores unrelated SSE events", async () => {
    mockGet.mockResolvedValue(fixture());
    renderHook(() => useDirectorBriefing(42, true));
    await waitFor(() => expect(mockGet).toHaveBeenCalledTimes(1));

    await act(async () => {
      capturedOnEvent?.({ type: "order.created", data: {} });
    });
    // No second fetch for an unrelated event.
    expect(mockGet).toHaveBeenCalledTimes(1);
  });

  it("refetches on SSE reconnect", async () => {
    mockGet.mockResolvedValue(fixture());
    renderHook(() => useDirectorBriefing(42, true));
    await waitFor(() => expect(mockGet).toHaveBeenCalledTimes(1));

    await act(async () => {
      capturedOnReconnect?.();
    });
    await waitFor(() => expect(mockGet).toHaveBeenCalledTimes(2));
  });

  it("retains the last-good briefing on a fetch error and flags error", async () => {
    mockGet.mockResolvedValueOnce(fixture({ state: "active" }));
    const { result } = renderHook(() => useDirectorBriefing(42, true));
    await waitFor(() => expect(result.current.briefing).not.toBeNull());

    mockGet.mockRejectedValueOnce(new Error("boom"));
    await act(async () => {
      result.current.refetch();
    });
    await waitFor(() => expect(result.current.error).toBe(true));
    // Last-good briefing is preserved — the front door never goes void.
    expect(result.current.briefing?.pulse.orders).toBe(84);
    expect(result.current.fetchedAt).toBeInstanceOf(Date);
  });
});
