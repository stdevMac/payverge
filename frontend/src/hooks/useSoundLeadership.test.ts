/** @jest-environment jsdom */
import { renderHook, waitFor } from "@testing-library/react";
import { useSoundLeadership } from "@/hooks/useSoundLeadership";

test("becomes leader when the lock is granted", async () => {
  const request = jest.fn(
    (_name: string, cb: (lock: unknown) => Promise<void>) => {
      // Grant immediately; hold until the returned promise resolves (never).
      void cb({});
      return new Promise(() => {});
    },
  );
  Object.defineProperty(globalThis.navigator, "locks", {
    configurable: true,
    value: { request },
  });
  const { result } = renderHook(() => useSoundLeadership(42));
  await waitFor(() => expect(result.current.current).toBe(true));
  expect(request).toHaveBeenCalledWith(
    "payverge-alert-sound-42",
    expect.any(Function),
  );
});

test("falls back to leader=true when Web Locks is unavailable", () => {
  Object.defineProperty(globalThis.navigator, "locks", {
    configurable: true,
    value: undefined,
  });
  const { result } = renderHook(() => useSoundLeadership(42));
  expect(result.current.current).toBe(true);
});

test("releases leadership on unmount", async () => {
  const request = jest.fn(
    (_name: string, cb: (lock: unknown) => Promise<void>) => {
      void cb({});
      return new Promise(() => {});
    },
  );
  Object.defineProperty(globalThis.navigator, "locks", {
    configurable: true,
    value: { request },
  });
  const { result, unmount } = renderHook(() => useSoundLeadership(42));
  await waitFor(() => expect(result.current.current).toBe(true));
  unmount();
  expect(result.current.current).toBe(false);
});

test("fires onAcquire when the lock is granted (late handover)", async () => {
  // Simulate inheriting the lock after another tab closes: capture the
  // callback, then deliver the grant later while still mounted.
  let grantedCb: ((lock: unknown) => Promise<void>) | null = null;
  const request = jest.fn(
    (_name: string, cb: (lock: unknown) => Promise<void>) => {
      grantedCb = cb;
      return new Promise(() => {});
    },
  );
  Object.defineProperty(globalThis.navigator, "locks", {
    configurable: true,
    value: { request },
  });
  const onAcquire = jest.fn();
  const { result } = renderHook(() => useSoundLeadership(42, onAcquire));
  expect(result.current.current).toBe(false);
  expect(onAcquire).not.toHaveBeenCalled();

  void grantedCb!({}); // previous leader closed; grant arrives now
  await waitFor(() => expect(result.current.current).toBe(true));
  expect(onAcquire).toHaveBeenCalledTimes(1);
});

test("fires onAcquire in the no-Web-Locks fallback too", () => {
  Object.defineProperty(globalThis.navigator, "locks", {
    configurable: true,
    value: undefined,
  });
  const onAcquire = jest.fn();
  const { result } = renderHook(() => useSoundLeadership(42, onAcquire));
  expect(result.current.current).toBe(true);
  expect(onAcquire).toHaveBeenCalledTimes(1);
});

test("a stale grant after cleanup does not fire onAcquire", async () => {
  let grantedCb: ((lock: unknown) => Promise<void>) | null = null;
  const request = jest.fn(
    (_name: string, cb: (lock: unknown) => Promise<void>) => {
      grantedCb = cb;
      return new Promise(() => {});
    },
  );
  Object.defineProperty(globalThis.navigator, "locks", {
    configurable: true,
    value: { request },
  });
  const onAcquire = jest.fn();
  const { unmount } = renderHook(() => useSoundLeadership(42, onAcquire));
  unmount();
  await grantedCb!({});
  expect(onAcquire).not.toHaveBeenCalled();
});

test("a lock granted after cleanup does not claim leadership", async () => {
  // Simulate another tab holding the lock: capture the callback WITHOUT
  // invoking it, unmount, THEN deliver the (now stale) grant.
  let grantedCb: ((lock: unknown) => Promise<void>) | null = null;
  const request = jest.fn(
    (_name: string, cb: (lock: unknown) => Promise<void>) => {
      grantedCb = cb;
      return new Promise(() => {});
    },
  );
  Object.defineProperty(globalThis.navigator, "locks", {
    configurable: true,
    value: { request },
  });
  const { result, unmount } = renderHook(() => useSoundLeadership(42));
  expect(result.current.current).toBe(false);
  unmount();
  await grantedCb!({}); // stale grant fires after cleanup
  expect(result.current.current).toBe(false);
});
