/** @jest-environment jsdom */
import { renderHook, act } from "@testing-library/react";
import { nextPollDelayMs, POLL_MAX_BACKOFF_MS, usePolling } from "@/hooks/usePolling";

jest.mock("@/utils/errorLogger", () => ({
  logError: jest.fn(),
}));

function setDocumentHidden(hidden: boolean) {
  Object.defineProperty(document, "hidden", {
    configurable: true,
    get: () => hidden,
  });
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    get: () => (hidden ? "hidden" : "visible"),
  });
}

describe("nextPollDelayMs", () => {
  const full = () => 1;

  it("stays at the base interval before the failure threshold", () => {
    expect(nextPollDelayMs(1000, 0, 3, full)).toBe(1000);
    expect(nextPollDelayMs(1000, 2, 3, full)).toBe(1000);
  });

  it("grows exponentially after N consecutive failures and caps", () => {
    expect(nextPollDelayMs(1000, 3, 3, full)).toBe(2000);
    expect(nextPollDelayMs(1000, 4, 3, full)).toBe(4000);
    expect(nextPollDelayMs(1000, 20, 3, full)).toBe(POLL_MAX_BACKOFF_MS);
  });

  it("applies half-to-full jitter on the ceiling", () => {
    expect(nextPollDelayMs(1000, 0, 3, () => 0)).toBe(500);
    expect(nextPollDelayMs(1000, 0, 3, () => 1)).toBe(1000);
    expect(nextPollDelayMs(1000, 0, 3, () => 0.5)).toBe(750);
  });
});

describe("usePolling isPolling reactivity", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    setDocumentHidden(false);
  });

  afterEach(() => {
    jest.clearAllTimers();
    jest.useRealTimers();
    jest.restoreAllMocks();
    setDocumentHidden(false);
  });

  it("isPolling is reactive across start/stop", () => {
    const { result } = renderHook(() =>
      usePolling({ callback: () => {}, enabled: false, immediate: false }),
    );

    // Not auto-started (enabled: false)
    expect(result.current.isPolling).toBe(false);

    act(() => {
      result.current.startPolling();
    });
    expect(result.current.isPolling).toBe(true);

    act(() => {
      result.current.stopPolling();
    });
    expect(result.current.isPolling).toBe(false);
  });

  it("restarts the timer at the new rate when the interval changes (no extra immediate fetch)", async () => {
    // Pin jitter to the full ceiling so this assertion stays on exact cadence.
    jest.spyOn(Math, "random").mockReturnValue(1);
    // usePolling now skips a tick that lands while the previous cycle is still
    // awaiting (in-flight guard). The callback resolves and we flush microtasks
    // between ticks so each cycle's in-flight lock is released before the next.
    const callback = jest.fn().mockResolvedValue(undefined);
    const { rerender } = renderHook(
      ({ interval }) =>
        usePolling({ callback, enabled: true, immediate: true, interval }),
      { initialProps: { interval: 1000 } },
    );

    // Immediate fetch on mount; release its in-flight lock before the first tick.
    expect(callback).toHaveBeenCalledTimes(1);
    await act(async () => {
      await Promise.resolve();
    });

    // One tick at 1s (lock free), then flush that cycle's lock.
    await act(async () => {
      jest.advanceTimersByTime(1000);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(2);

    // Slow the rate to 5s. This must NOT fire an extra immediate fetch...
    rerender({ interval: 5000 });
    expect(callback).toHaveBeenCalledTimes(2);

    // ...and the OLD 1s cadence must be gone: 1s passes, no new call.
    await act(async () => {
      jest.advanceTimersByTime(1000);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(2);

    // At the new 5s cadence the next tick fires (4s more = 5s total).
    await act(async () => {
      jest.advanceTimersByTime(4000);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(3);
  });

  it("isPolling re-syncs to false when enabled flips true->false", () => {
    const { result, rerender } = renderHook(
      ({ enabled }) =>
        usePolling({ callback: jest.fn(), enabled, immediate: false }),
      { initialProps: { enabled: true } },
    );

    // Auto-started by the enabled effect (immediate:false only skips the
    // first run; startPolling still sets isPolling true).
    expect(result.current.isPolling).toBe(true);

    rerender({ enabled: false });

    // Effect re-runs on the enabled change and takes the else branch ->
    // stopPolling() -> setIsPolling(false). Guards against a stuck state.
    expect(result.current.isPolling).toBe(false);
  });
});

describe("usePolling error backoff and jitter", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    setDocumentHidden(false);
  });

  afterEach(() => {
    jest.clearAllTimers();
    jest.useRealTimers();
    jest.restoreAllMocks();
    setDocumentHidden(false);
  });

  it("grows the interval after N consecutive failures (capped exponential)", async () => {
    jest.spyOn(Math, "random").mockReturnValue(1);
    const callback = jest.fn().mockRejectedValue(new Error("poll failed"));

    renderHook(() =>
      usePolling({
        callback,
        interval: 1000,
        enabled: true,
        immediate: true,
        failureThreshold: 2,
      }),
    );

    await act(async () => {
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(1);

    // First failure is below the threshold — still the base 1s cadence.
    await act(async () => {
      jest.advanceTimersByTime(999);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(1);

    await act(async () => {
      jest.advanceTimersByTime(1);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(2);

    // N=2 consecutive failures → next wait is 2× the base interval.
    await act(async () => {
      jest.advanceTimersByTime(1999);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(2);

    await act(async () => {
      jest.advanceTimersByTime(1);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(3);

    // Next wait is 4×. A 2s advance must not fire.
    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(3);

    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(4);
  });

  it("resets the interval to the base cadence after a successful poll", async () => {
    jest.spyOn(Math, "random").mockReturnValue(1);
    let shouldFail = true;
    const callback = jest.fn(() =>
      shouldFail ? Promise.reject(new Error("fail")) : Promise.resolve(),
    );

    renderHook(() =>
      usePolling({
        callback,
        interval: 1000,
        enabled: true,
        immediate: true,
        failureThreshold: 2,
      }),
    );

    await act(async () => {
      await Promise.resolve();
    });
    await act(async () => {
      jest.advanceTimersByTime(1000);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(2);

    shouldFail = false;

    // Two failures → 2s backoff. A single 1s step must not tick.
    await act(async () => {
      jest.advanceTimersByTime(1000);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(2);

    await act(async () => {
      jest.advanceTimersByTime(1000);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(3);

    // Success resets to the base 1s cadence.
    await act(async () => {
      jest.advanceTimersByTime(1000);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(4);
  });

  it("jitters so two hooks do not share identical poll timestamps", async () => {
    let n = 0;
    jest.spyOn(Math, "random").mockImplementation(() => {
      n += 1;
      return n % 2 === 1 ? 0 : 1;
    });

    const ticksA: number[] = [];
    const ticksB: number[] = [];

    renderHook(() =>
      usePolling({
        callback: () => {
          ticksA.push(jest.now());
        },
        interval: 1000,
        enabled: true,
        immediate: false,
      }),
    );
    renderHook(() =>
      usePolling({
        callback: () => {
          ticksB.push(jest.now());
        },
        interval: 1000,
        enabled: true,
        immediate: false,
      }),
    );

    await act(async () => {
      jest.advanceTimersByTime(4000);
      await Promise.resolve();
    });

    expect(ticksA.length).toBeGreaterThan(0);
    expect(ticksB.length).toBeGreaterThan(0);
    expect(ticksA).not.toEqual(ticksB);
  });

  it("does not stack a tick while a cycle is in flight", async () => {
    jest.spyOn(Math, "random").mockReturnValue(1);
    let release = () => {};
    const callback = jest.fn(
      () =>
        new Promise<void>((resolve) => {
          release = resolve;
        }),
    );

    renderHook(() =>
      usePolling({
        callback,
        interval: 1000,
        enabled: true,
        immediate: true,
      }),
    );
    expect(callback).toHaveBeenCalledTimes(1);

    await act(async () => {
      jest.advanceTimersByTime(3000);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(1);

    await act(async () => {
      release();
      await Promise.resolve();
    });
    await act(async () => {
      jest.advanceTimersByTime(1000);
      await Promise.resolve();
    });
    expect(callback).toHaveBeenCalledTimes(2);
  });

  it("pauses while the document is hidden", async () => {
    jest.spyOn(Math, "random").mockReturnValue(1);
    const callback = jest.fn().mockResolvedValue(undefined);

    renderHook(() =>
      usePolling({
        callback,
        interval: 1000,
        enabled: true,
        immediate: false,
        pauseWhenHidden: true,
      }),
    );

    setDocumentHidden(true);
    act(() => {
      document.dispatchEvent(new Event("visibilitychange"));
    });

    await act(async () => {
      jest.advanceTimersByTime(5000);
      await Promise.resolve();
    });
    expect(callback).not.toHaveBeenCalled();
  });

  it("aborts an in-flight cycle on unmount", async () => {
    let seen: AbortSignal | undefined;
    const callback = jest.fn((signal?: AbortSignal) => {
      seen = signal;
      return new Promise<void>(() => {});
    });

    const { unmount } = renderHook(() =>
      usePolling({ callback, interval: 1000, immediate: true }),
    );
    await act(async () => {
      await Promise.resolve();
    });
    expect(seen?.aborted).toBe(false);

    unmount();
    expect(seen?.aborted).toBe(true);
  });
});
