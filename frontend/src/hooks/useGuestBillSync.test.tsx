/** @jest-environment jsdom */
import { act, renderHook, waitFor } from "@testing-library/react";
import {
  MAX_CONSECUTIVE_STREAM_ERRORS,
  STREAM_BACKOFF_MAX_MS,
  STREAM_BACKOFF_MIN_MS,
  streamBackoffDelay,
  useGuestBillSync,
} from "./useGuestBillSync";
import { getOpenBillByTableCode } from "@/api/bills";

jest.mock("@/api/bills", () => ({ getOpenBillByTableCode: jest.fn() }));

class MockEventSource {
  static latest: MockEventSource;
  static instances = 0;
  listeners = new Map<string, (event: MessageEvent) => void>();
  onerror: (() => void) | null = null;
  readyState = 0;
  close = jest.fn();
  constructor(public url: string) { MockEventSource.latest = this; MockEventSource.instances += 1; }
  addEventListener(name: string, listener: EventListener) {
    this.listeners.set(name, listener as (event: MessageEvent) => void);
  }
  emit(name: string, data: object) {
    this.listeners.get(name)?.({ data: JSON.stringify(data) } as MessageEvent);
  }
}

describe("useGuestBillSync", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.clearAllMocks();
    (global as any).EventSource = MockEventSource;
    MockEventSource.instances = 0;
    (getOpenBillByTableCode as jest.Mock).mockResolvedValue({ bill: { id: 1 }, items: [] });
  });
  afterEach(() => jest.useRealTimers());

  it("applies a live close without exposing bill contents", () => {
    const onChange = jest.fn();
    renderHook(() => useGuestBillSync({ tableCode: "T1", hasActiveBill: true, onActiveBillChange: onChange }));
    act(() => MockEventSource.latest.emit("table.bill_changed", { has_active_bill: false }));
    expect(onChange).toHaveBeenCalledWith(false);
  });

  it("recovers on stream errors and every 15 seconds while active", async () => {
    const onChange = jest.fn();
    (getOpenBillByTableCode as jest.Mock).mockResolvedValue({ bill: null, items: [] });
    renderHook(() => useGuestBillSync({ tableCode: "T1", hasActiveBill: true, onActiveBillChange: onChange }));
    act(() => MockEventSource.latest.onerror?.());
    await waitFor(() => expect(getOpenBillByTableCode).toHaveBeenCalledTimes(1));
    act(() => jest.advanceTimersByTime(15_000));
    await waitFor(() => expect(getOpenBillByTableCode).toHaveBeenCalledTimes(2));
  });

  it("reports connected after a connected event and clears it after an error", () => {
    const { result } = renderHook(() =>
      useGuestBillSync({
        tableCode: "T1",
        hasActiveBill: true,
        onActiveBillChange: jest.fn(),
      }),
    );
    expect(result.current.connected).toBe(false);
    act(() => {
      MockEventSource.latest.emit("connected", {});
    });
    expect(result.current.connected).toBe(true);
    act(() => {
      MockEventSource.latest.onerror?.();
    });
    expect(result.current.connected).toBe(false);
  });

  it("calls onBillEvent after active-state handling on table.bill_changed", () => {
    const order: string[] = [];
    const onChange = jest.fn(() => {
      order.push("active");
    });
    const onBillEvent = jest.fn(() => {
      order.push("event");
    });
    renderHook(() =>
      useGuestBillSync({
        tableCode: "T1",
        hasActiveBill: true,
        onActiveBillChange: onChange,
        onBillEvent,
      }),
    );
    act(() => {
      MockEventSource.latest.emit("table.bill_changed", {
        has_active_bill: false,
      });
    });
    expect(order).toEqual(["active", "event"]);
    expect(onBillEvent).toHaveBeenCalledTimes(1);
  });

  it("stops recovery polling after closure", () => {
    (getOpenBillByTableCode as jest.Mock).mockResolvedValue({ bill: null, items: [] });
    const { rerender } = renderHook(
      ({ active }) => useGuestBillSync({ tableCode: "T1", hasActiveBill: active, onActiveBillChange: jest.fn() }),
      { initialProps: { active: true } },
    );
    rerender({ active: false });
    act(() => jest.advanceTimersByTime(30_000));
    expect(getOpenBillByTableCode).not.toHaveBeenCalled();
  });

  it("jitters the park window between 30 and 120 seconds", () => {
    expect(streamBackoffDelay(() => 0)).toBe(STREAM_BACKOFF_MIN_MS);
    expect(streamBackoffDelay(() => 0.999999)).toBeLessThan(STREAM_BACKOFF_MAX_MS);
    expect(streamBackoffDelay(() => 0.5)).toBe(75_000);
  });

  it("closes on a terminal capacity frame and reconnects only after the jittered backoff", () => {
    const random = jest.spyOn(Math, "random").mockReturnValue(0);
    const { result } = renderHook(() =>
      useGuestBillSync({ tableCode: "T1", hasActiveBill: true, onActiveBillChange: jest.fn() }),
    );
    const first = MockEventSource.latest;
    act(() => first.emit("connected", {}));
    expect(result.current.connected).toBe(true);
    act(() => first.emit("error", { code: "capacity", message: "Too many live connections" }));
    expect(first.close).toHaveBeenCalled();
    expect(result.current.connected).toBe(false);
    expect(getOpenBillByTableCode).toHaveBeenCalled();
    // Transport errors after parking must not reopen or count.
    act(() => first.onerror?.());
    act(() => jest.advanceTimersByTime(STREAM_BACKOFF_MIN_MS - 1));
    expect(MockEventSource.instances).toBe(1);
    act(() => jest.advanceTimersByTime(1));
    expect(MockEventSource.instances).toBe(2);
    expect(MockEventSource.latest).not.toBe(first);
    random.mockRestore();
  });

  it("ignores error frames that are not a capacity rejection", () => {
    renderHook(() =>
      useGuestBillSync({ tableCode: "T1", hasActiveBill: true, onActiveBillChange: jest.fn() }),
    );
    act(() => MockEventSource.latest.emit("error", { code: "other" }));
    expect(MockEventSource.latest.close).not.toHaveBeenCalled();
  });

  it("parks the stream after repeated transport errors instead of reconnecting forever", () => {
    renderHook(() =>
      useGuestBillSync({ tableCode: "T1", hasActiveBill: true, onActiveBillChange: jest.fn() }),
    );
    const source = MockEventSource.latest;
    for (let i = 1; i < MAX_CONSECUTIVE_STREAM_ERRORS; i += 1) {
      act(() => source.onerror?.());
    }
    expect(source.close).not.toHaveBeenCalled();
    act(() => source.onerror?.());
    expect(source.close).toHaveBeenCalled();
    act(() => jest.advanceTimersByTime(STREAM_BACKOFF_MAX_MS));
    expect(MockEventSource.instances).toBe(2);
  });

  it("parks on the first error when the browser closed the stream for good", () => {
    const random = jest.spyOn(Math, "random").mockReturnValue(0);
    renderHook(() =>
      useGuestBillSync({ tableCode: "T1", hasActiveBill: true, onActiveBillChange: jest.fn() }),
    );
    const source = MockEventSource.latest;
    // A 429/502 response: EventSource sets readyState CLOSED and never retries.
    source.readyState = 2;
    act(() => source.onerror?.());
    expect(source.close).toHaveBeenCalled();
    expect(MockEventSource.instances).toBe(1);
    act(() => jest.advanceTimersByTime(STREAM_BACKOFF_MIN_MS));
    expect(MockEventSource.instances).toBe(2);
    random.mockRestore();
  });

  it("does not park on a reconnecting transport error", () => {
    renderHook(() =>
      useGuestBillSync({ tableCode: "T1", hasActiveBill: true, onActiveBillChange: jest.fn() }),
    );
    const source = MockEventSource.latest;
    // CONNECTING: the browser is retrying on its own.
    source.readyState = 0;
    act(() => source.onerror?.());
    expect(source.close).not.toHaveBeenCalled();
  });

  it("cancels a pending backoff reconnect on unmount", () => {
    const { unmount } = renderHook(() =>
      useGuestBillSync({ tableCode: "T1", hasActiveBill: true, onActiveBillChange: jest.fn() }),
    );
    act(() => MockEventSource.latest.emit("error", { code: "capacity" }));
    unmount();
    act(() => jest.advanceTimersByTime(STREAM_BACKOFF_MAX_MS));
    expect(MockEventSource.instances).toBe(1);
  });
});
