/** @jest-environment jsdom */
import { renderHook, waitFor } from "@testing-library/react";
import { axiosInstance } from "@/api/tools/instance";
import { resetInstanceCacheForTests, useInstance } from "./useInstance";

jest.mock("@/api/tools/instance", () => ({ axiosInstance: { get: jest.fn() } }));

const get = axiosInstance.get as jest.Mock;

beforeEach(() => {
  resetInstanceCacheForTests();
  get.mockReset();
  delete (window as unknown as Record<string, unknown>).__PAYVERGE_INSTANCE__;
});

describe("useInstance", () => {
  it("uses the layout seed without a request", () => {
    (window as unknown as Record<string, unknown>).__PAYVERGE_INSTANCE__ = {
      product_name: "Trattoria OS",
      registration_mode: "closed",
      features: { ai: false },
    };
    const { result } = renderHook(() => useInstance());
    expect(result.current.productName).toBe("Trattoria OS");
    expect(result.current.isOff("ai")).toBe(true);
    expect(get).not.toHaveBeenCalled();
  });

  it("fetches once and shares the module cache", async () => {
    get.mockResolvedValue({
      data: { registration_mode: "open", features: { ai: true } },
    });
    const a = renderHook(() => useInstance());
    const b = renderHook(() => useInstance());
    await waitFor(() => expect(a.result.current.loading).toBe(false));
    await waitFor(() => expect(b.result.current.isOn("ai")).toBe(true));
    expect(get).toHaveBeenCalledTimes(1);
  });

  it("never reports a feature off when the probe fails", async () => {
    get.mockRejectedValue(new Error("down"));
    const { result } = renderHook(() => useInstance());
    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.isOff("ai")).toBe(false);
    expect(result.current.productName).toBe("Payverge");
  });
});
