/** @jest-environment jsdom */
import { renderHook, waitFor } from "@testing-library/react";

import { getOrCreatePrintClientId, usePrintLeader } from "./usePrintLeader";

describe("usePrintLeader", () => {
  afterEach(() => {
    Object.defineProperty(globalThis.navigator, "locks", {
      configurable: true,
      value: undefined,
    });
  });

  it("becomes leader when the Web Lock is granted", async () => {
    const request = jest.fn((_name: string, cb: (lock: unknown) => Promise<void>) => {
      void cb({});
      return new Promise(() => {});
    });
    Object.defineProperty(globalThis.navigator, "locks", {
      configurable: true,
      value: { request },
    });
    const { result } = renderHook(() => usePrintLeader(42));
    await waitFor(() => expect(result.current.current).toBe(true));
    expect(request).toHaveBeenCalledWith(
      "payverge-print-leader-42",
      expect.any(Function),
    );
  });

  it("falls back when Web Locks is unavailable", async () => {
    Object.defineProperty(globalThis.navigator, "locks", {
      configurable: true,
      value: undefined,
    });
    // Mock BroadcastChannel for the fallback path.
    const postMessage = jest.fn();
    (globalThis as any).BroadcastChannel = class {
      onmessage: ((ev: MessageEvent) => void) | null = null;
      postMessage = postMessage;
      close = jest.fn();
    };
    const { result } = renderHook(() => usePrintLeader(7));
    await waitFor(() => expect(result.current.current).toBe(true));
  });

  it("releases leadership on unmount", async () => {
    const request = jest.fn((_name: string, cb: (lock: unknown) => Promise<void>) => {
      void cb({});
      return new Promise(() => {});
    });
    Object.defineProperty(globalThis.navigator, "locks", {
      configurable: true,
      value: { request },
    });
    const { result, unmount } = renderHook(() => usePrintLeader(42));
    await waitFor(() => expect(result.current.current).toBe(true));
    unmount();
    expect(result.current.current).toBe(false);
  });
});

describe("getOrCreatePrintClientId", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("returns a stable UUID across calls", () => {
    const a = getOrCreatePrintClientId();
    const b = getOrCreatePrintClientId();
    expect(a).toBe(b);
    expect(a).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
    );
  });
});
