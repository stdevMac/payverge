/** @jest-environment jsdom */
/**
 * M6 — ephemeral per-tab state retention. Kitchen view/filter and Menu view/
 * search survive rail switches for the length of the browser tab session
 * (sessionStorage), not forever (localStorage would leak yesterday's shift
 * into today). Mirrors useLocalStorageState's contract: SSR-safe initial
 * render, null-key guard, malformed-storage tolerance — plus an isValid
 * validator so a persisted value that no longer exists (e.g. a retired
 * kitchen tab key) falls back to the default instead of bricking the view.
 */
import { renderHook, act } from "@testing-library/react";
import { useSessionTabState } from "../useSessionTabState";

const KEY = "payverge_test_session:7";

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

describe("useSessionTabState", () => {
  it("hydrates from sessionStorage on mount", () => {
    window.sessionStorage.setItem(KEY, JSON.stringify("kds"));
    const { result } = renderHook(() =>
      useSessionTabState(KEY, "list"),
    );
    expect(result.current[0]).toBe("kds");
  });

  it("persists updates to sessionStorage", () => {
    const { result } = renderHook(() => useSessionTabState(KEY, "list"));
    act(() => result.current[1]("kds"));
    expect(result.current[0]).toBe("kds");
    expect(window.sessionStorage.getItem(KEY)).toBe(JSON.stringify("kds"));
  });

  it("never touches localStorage", () => {
    const { result } = renderHook(() => useSessionTabState(KEY, "list"));
    act(() => result.current[1]("kds"));
    expect(window.localStorage.getItem(KEY)).toBeNull();
  });

  it("behaves like plain useState when the key is null", () => {
    const { result } = renderHook(() => useSessionTabState(null, "list"));
    act(() => result.current[1]("kds"));
    expect(result.current[0]).toBe("kds");
    expect(window.sessionStorage.length).toBe(0);
  });

  it("falls back to initial when the persisted value fails validation", () => {
    window.sessionStorage.setItem(KEY, JSON.stringify("retired-tab"));
    const { result } = renderHook(() =>
      useSessionTabState(KEY, "approved", (v) =>
        ["approved", "preparing", "ready"].includes(String(v)),
      ),
    );
    expect(result.current[0]).toBe("approved");
  });

  it("keeps a valid persisted value", () => {
    window.sessionStorage.setItem(KEY, JSON.stringify("ready"));
    const { result } = renderHook(() =>
      useSessionTabState(KEY, "approved", (v) =>
        ["approved", "preparing", "ready"].includes(String(v)),
      ),
    );
    expect(result.current[0]).toBe("ready");
  });

  it("tolerates malformed stored JSON", () => {
    window.sessionStorage.setItem(KEY, "{not json");
    const { result } = renderHook(() => useSessionTabState(KEY, "list"));
    expect(result.current[0]).toBe("list");
  });

  it("re-keys when the business scope changes", () => {
    window.sessionStorage.setItem("payverge_test_session:8", JSON.stringify("kds"));
    const { result, rerender } = renderHook(
      ({ biz }: { biz: number }) =>
        useSessionTabState(`payverge_test_session:${biz}`, "list"),
      { initialProps: { biz: 7 } },
    );
    expect(result.current[0]).toBe("list");
    rerender({ biz: 8 });
    expect(result.current[0]).toBe("kds");
  });
});
