/** @jest-environment jsdom */
import { act, renderHook, waitFor } from "@testing-library/react";
import { useOrderQuote } from "./useOrderQuote";
import { quoteGuestOrder } from "@/api/orders";
import { asDollars } from "@/types/money";

jest.mock("@/api/orders", () => ({
  quoteGuestOrder: jest.fn(),
  quoteBusinessOrder: jest.fn(),
}));

describe("useOrderQuote", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.clearAllMocks();
  });

  afterEach(() => jest.useRealTimers());

  it("debounces cart edits by 250ms", async () => {
    (quoteGuestOrder as jest.Mock).mockResolvedValue({
      subtotal: 1,
      discount: 0,
      total: 1,
      lines: [],
    });
    renderHook(() =>
      useOrderQuote({
        tableCode: "T1",
        draft: {
          items: [{ menu_item_name: "Tea", quantity: 1, price: asDollars(1) }],
        },
      }),
    );
    act(() => jest.advanceTimersByTime(249));
    expect(quoteGuestOrder).not.toHaveBeenCalled();
    act(() => jest.advanceTimersByTime(1));
    await waitFor(() => expect(quoteGuestOrder).toHaveBeenCalledTimes(1));
  });

  it("quotes immediately at checkout", async () => {
    (quoteGuestOrder as jest.Mock).mockResolvedValue({
      subtotal: 1,
      discount: 0,
      total: 1,
      lines: [],
    });
    const { result } = renderHook(() =>
      useOrderQuote({
        tableCode: "T1",
        immediate: true,
        draft: {
          items: [{ menu_item_name: "Tea", quantity: 1, price: asDollars(1) }],
        },
      }),
    );
    expect(result.current.isValid).toBe(false);
    act(() => jest.advanceTimersByTime(0));
    await waitFor(() => expect(quoteGuestOrder).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(result.current.isValid).toBe(true));
  });

  it("does not expose a quote from a previous draft while the next quote is pending", async () => {
    (quoteGuestOrder as jest.Mock)
      .mockResolvedValueOnce({ subtotal: 1, discount: 0, total: 1, lines: [] })
      .mockResolvedValueOnce({ subtotal: 2, discount: 0, total: 2, lines: [] });
    const { result, rerender } = renderHook(
      ({ quantity }) =>
        useOrderQuote({
          tableCode: "T1",
          immediate: true,
          draft: {
            items: [{ menu_item_name: "Tea", quantity, price: asDollars(1) }],
          },
        }),
      { initialProps: { quantity: 1 } },
    );
    act(() => jest.advanceTimersByTime(0));
    await waitFor(() => expect(result.current.quote?.total).toBe(1));

    rerender({ quantity: 2 });
    expect(result.current.quote).toBeNull();
    expect(result.current.isValid).toBe(false);
    act(() => jest.advanceTimersByTime(0));
    await waitFor(() => expect(result.current.quote?.total).toBe(2));
  });

  it("keeps the quote neutral when the current draft fails", async () => {
    (quoteGuestOrder as jest.Mock)
      .mockResolvedValueOnce({ subtotal: 1, discount: 0, total: 1, lines: [] })
      .mockRejectedValueOnce(new Error("quote unavailable"));
    const { result, rerender } = renderHook(
      ({ quantity }) =>
        useOrderQuote({
          tableCode: "T1",
          immediate: true,
          draft: {
            items: [{ menu_item_name: "Tea", quantity, price: asDollars(1) }],
          },
        }),
      { initialProps: { quantity: 1 } },
    );
    act(() => jest.advanceTimersByTime(0));
    await waitFor(() => expect(result.current.quote?.total).toBe(1));

    rerender({ quantity: 2 });
    expect(result.current.quote).toBeNull();
    act(() => jest.advanceTimersByTime(0));
    await waitFor(() => expect(result.current.error).toBeInstanceOf(Error));
    expect(result.current.quote).toBeNull();
    expect(result.current.isValid).toBe(false);
  });

  it("invalidates a quote when any authoritative line is not orderable", async () => {
    (quoteGuestOrder as jest.Mock).mockResolvedValue({
      subtotal: 10,
      discount: 0,
      total: 10,
      lines: [
        {
          key: "burger",
          line_type: "menu_item",
          unit_price: 10,
          quantity: 1,
          subtotal: 10,
          orderability: { state: "inventory_out", orderable: false },
        },
      ],
    });
    const { result } = renderHook(() =>
      useOrderQuote({
        tableCode: "T1",
        immediate: true,
        draft: {
          items: [
            { menu_item_name: "Burger", quantity: 1, price: asDollars(10) },
          ],
        },
      }),
    );

    act(() => jest.advanceTimersByTime(0));
    await waitFor(() => expect(result.current.quote).not.toBeNull());
    expect(result.current.isValid).toBe(false);
    expect(result.current.blockedLines).toEqual([
      expect.objectContaining({ key: "burger" }),
    ]);
  });

  it("keeps inventory-warning quotes valid because warnings remain orderable", async () => {
    (quoteGuestOrder as jest.Mock).mockResolvedValue({
      subtotal: 10,
      discount: 0,
      total: 10,
      lines: [
        {
          key: "burger",
          line_type: "menu_item",
          unit_price: 10,
          quantity: 1,
          subtotal: 10,
          orderability: { state: "inventory_warning", orderable: true },
        },
      ],
    });
    const { result } = renderHook(() =>
      useOrderQuote({
        tableCode: "T1",
        immediate: true,
        draft: {
          items: [
            { menu_item_name: "Burger", quantity: 1, price: asDollars(10) },
          ],
        },
      }),
    );

    act(() => jest.advanceTimersByTime(0));
    await waitFor(() => expect(result.current.isValid).toBe(true));
    expect(result.current.blockedLines).toEqual([]);
  });
});
