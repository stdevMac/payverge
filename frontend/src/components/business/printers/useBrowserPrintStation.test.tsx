/** @jest-environment jsdom */
import { act, renderHook } from "@testing-library/react";

import { useBrowserPrintStation } from "./useBrowserPrintStation";

describe("useBrowserPrintStation", () => {
  beforeEach(() => window.localStorage.clear());

  it("starts inactive and persists one selected printer per business", () => {
    const { result } = renderHook(() => useBrowserPrintStation(42));
    expect(result.current.printerId).toBeNull();

    act(() => result.current.selectPrinter(7));

    expect(result.current.printerId).toBe(7);
    expect(window.localStorage.getItem("payverge_print_station:42")).toBe("7");
    expect(window.localStorage.getItem("payverge_print_station:9")).toBeNull();
  });

  it("synchronizes settings and the dashboard agent in the same browser tab", () => {
    const settings = renderHook(() => useBrowserPrintStation(42));
    const agent = renderHook(() => useBrowserPrintStation(42));

    act(() => settings.result.current.selectPrinter(9));
    expect(agent.result.current.printerId).toBe(9);

    act(() => agent.result.current.stopStation());
    expect(settings.result.current.printerId).toBeNull();
    expect(window.localStorage.getItem("payverge_print_station:42")).toBeNull();
  });

  it("treats a throwing localStorage as no armed station instead of crashing (O6)", () => {
    // Privacy modes / blocked site data make the storage accessors themselves
    // throw SecurityError. Since RecentPrintJobs now consumes this hook, an
    // unguarded read would crash the whole Recent jobs list.
    const denied = () => {
      throw new DOMException("The operation is insecure.", "SecurityError");
    };
    const getSpy = jest
      .spyOn(Storage.prototype, "getItem")
      .mockImplementation(denied);
    const setSpy = jest
      .spyOn(Storage.prototype, "setItem")
      .mockImplementation(denied);
    const removeSpy = jest
      .spyOn(Storage.prototype, "removeItem")
      .mockImplementation(denied);

    try {
      const { result } = renderHook(() => useBrowserPrintStation(42));
      expect(result.current.printerId).toBeNull();

      expect(() => {
        act(() => result.current.selectPrinter(7));
      }).not.toThrow();
      expect(result.current.printerId).toBeNull();

      expect(() => {
        act(() => result.current.stopStation());
      }).not.toThrow();
    } finally {
      getSpy.mockRestore();
      setSpy.mockRestore();
      removeSpy.mockRestore();
    }
  });
});
