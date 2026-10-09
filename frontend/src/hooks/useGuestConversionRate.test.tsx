/** @jest-environment jsdom */
import { renderHook, waitFor } from "@testing-library/react";
import { convertAmount } from "@/api/currency";
import { useGuestConversionRate } from "./useGuestConversionRate";

jest.mock("@/api/currency", () => {
  const actual = jest.requireActual("@/api/currency");
  return {
    ...actual,
    convertAmount: jest.fn(),
  };
});

describe("useGuestConversionRate", () => {
  beforeEach(() => {
    (convertAmount as jest.Mock).mockReset();
  });

  it("returns a ready 1:1 rate when currencies match and does not fetch", () => {
    const { result } = renderHook(() => useGuestConversionRate("USD", "USD"));
    expect(result.current).toEqual({ rate: 1, status: "ready" });
    expect(convertAmount).not.toHaveBeenCalled();
  });

  it("loads the live rate for mixed currencies", async () => {
    (convertAmount as jest.Mock).mockResolvedValue({ converted_amount: 0.001 });
    const { result } = renderHook(() => useGuestConversionRate("ARS", "USD"));
    expect(result.current.status).toBe("pending");
    await waitFor(() => {
      expect(result.current).toEqual({ rate: 0.001, status: "ready" });
    });
    expect(convertAmount).toHaveBeenCalledWith(1, "ARS", "USD");
  });
});
