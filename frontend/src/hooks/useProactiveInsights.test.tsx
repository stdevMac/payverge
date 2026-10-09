/** @jest-environment jsdom */
import { renderHook, waitFor, act } from "@testing-library/react";
import { useProactiveInsights } from "./useProactiveInsights";
import { getDirectorProactiveInsights } from "@/api/directorConsole";
import type { SSEEvent } from "@/hooks/useSSEEvents";

jest.mock("@/api/directorConsole", () => ({
  getDirectorProactiveInsights: jest.fn(),
}));

// Capture the options useSSEEvents was called with so the test can drive the
// shared-stream callbacks (onEvent / onReconnect) directly.
let lastSSEOptions: {
  businessId: number;
  enabled: boolean;
  onEvent: (event: SSEEvent) => void;
  onReconnect?: () => void;
} | null = null;
jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: (options: unknown) => {
    lastSSEOptions = options as typeof lastSSEOptions;
    return { degraded: false, blocked: false, reconnect: jest.fn() };
  },
}));

const mockGet = getDirectorProactiveInsights as jest.Mock;

describe("useProactiveInsights", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    lastSSEOptions = null;
    mockGet.mockResolvedValue({ insights: [{ id: "stale_open_bills" }] });
  });

  it("fetches insights once on mount when enabled", async () => {
    const { result } = renderHook(() => useProactiveInsights(42, true));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mockGet).toHaveBeenCalledTimes(1);
    expect(mockGet).toHaveBeenCalledWith(42);
    expect(result.current.insights).toHaveLength(1);
  });

  it("does not fetch when disabled", () => {
    renderHook(() => useProactiveInsights(42, false));
    expect(mockGet).not.toHaveBeenCalled();
  });

  it("does not fetch when businessId is missing", () => {
    renderHook(() => useProactiveInsights(undefined, true));
    expect(mockGet).not.toHaveBeenCalled();
  });

  it("refetches when a bill.stuck SSE event fires (the watchdog's intended hookup)", async () => {
    const { result } = renderHook(() => useProactiveInsights(42, true));
    await waitFor(() => expect(mockGet).toHaveBeenCalledTimes(1));

    await act(async () => {
      lastSSEOptions?.onEvent({ type: "bill.stuck", data: {} });
    });
    await waitFor(() => expect(mockGet).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.loading).toBe(false));
  });

  it("ignores unrelated SSE events (no spurious refetch)", async () => {
    const { result } = renderHook(() => useProactiveInsights(42, true));
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mockGet).toHaveBeenCalledTimes(1);

    await act(async () => {
      lastSSEOptions?.onEvent({ type: "order.created", data: {} });
    });
    expect(mockGet).toHaveBeenCalledTimes(1);
  });

  it("refetches on SSE reconnect to recover anything missed during a drop", async () => {
    const { result } = renderHook(() => useProactiveInsights(42, true));
    await waitFor(() => expect(mockGet).toHaveBeenCalledTimes(1));

    await act(async () => {
      lastSSEOptions?.onReconnect?.();
    });
    await waitFor(() => expect(mockGet).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(result.current.loading).toBe(false));
  });

  it("subscribes to the shared SSE stream only when enabled", async () => {
    const { result } = renderHook(() => useProactiveInsights(42, true));
    expect(lastSSEOptions?.enabled).toBe(true);
    expect(lastSSEOptions?.businessId).toBe(42);
    await waitFor(() => expect(result.current.loading).toBe(false));
  });
});
