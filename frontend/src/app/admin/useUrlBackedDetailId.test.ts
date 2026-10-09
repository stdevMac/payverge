/** @jest-environment jsdom */
import { act, renderHook } from "@testing-library/react";
import {
  adminDetailIdFromParam,
  useUrlBackedDetailId,
} from "./useUrlBackedDetailId";

describe("adminDetailIdFromParam", () => {
  it("accepts only positive integer ids", () => {
    expect(adminDetailIdFromParam("5")).toBe(5);
    expect(adminDetailIdFromParam(null)).toBeNull();
    expect(adminDetailIdFromParam("")).toBeNull();
    expect(adminDetailIdFromParam("0")).toBeNull();
    expect(adminDetailIdFromParam("-1")).toBeNull();
    expect(adminDetailIdFromParam("abc")).toBeNull();
  });
});

describe("useUrlBackedDetailId", () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it("stays closed after dismiss while the query param is still present", () => {
    const replace = jest.fn();
    const params = { current: new URLSearchParams("user=5") };
    const { result, rerender } = renderHook(() =>
      useUrlBackedDetailId(params.current, "user", replace),
    );

    expect(result.current.selectedId).toBe(5);

    act(() => {
      result.current.close();
    });
    expect(result.current.selectedId).toBeNull();
    expect(replace).toHaveBeenCalledWith(null);

    // router.replace has not applied yet — searchParams still has user=5
    rerender();
    expect(result.current.selectedId).toBeNull();

    params.current = new URLSearchParams();
    rerender();
    expect(result.current.selectedId).toBeNull();
  });

  it("opens immediately from a row click before the URL settles", () => {
    const replace = jest.fn();
    const params = { current: new URLSearchParams() };
    const { result, rerender } = renderHook(() =>
      useUrlBackedDetailId(params.current, "user", replace),
    );

    act(() => {
      result.current.open(7);
    });
    expect(result.current.selectedId).toBe(7);
    expect(replace).toHaveBeenCalledWith(7);

    params.current = new URLSearchParams("user=7");
    rerender();
    expect(result.current.selectedId).toBe(7);
  });

  it("opens a new id after dismiss even if the previous query param is stale", () => {
    const replace = jest.fn();
    const params = { current: new URLSearchParams("user=5") };
    const { result } = renderHook(() =>
      useUrlBackedDetailId(params.current, "user", replace),
    );

    act(() => {
      result.current.close();
    });
    act(() => {
      result.current.open(9);
    });
    expect(result.current.selectedId).toBe(9);
  });

  it("survives remount while the dismissed query param is still present", () => {
    const replace = jest.fn();
    const params = { current: new URLSearchParams("business=86") };
    const first = renderHook(() =>
      useUrlBackedDetailId(params.current, "business", replace),
    );
    expect(first.result.current.selectedId).toBe(86);
    act(() => {
      first.result.current.close();
    });
    expect(first.result.current.selectedId).toBeNull();
    first.unmount();

    const second = renderHook(() =>
      useUrlBackedDetailId(params.current, "business", replace),
    );
    expect(second.result.current.selectedId).toBeNull();
  });
});
