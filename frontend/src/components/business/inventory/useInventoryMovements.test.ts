/** @jest-environment jsdom */
import { act, renderHook, waitFor } from "@testing-library/react";

import { inventoryApi } from "@/api/inventory";
import {
  useInventoryMovements,
  useItemMovementHistory,
} from "./useInventoryMovements";

jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    listMovementsPage: jest.fn(),
  },
}));

const mockPage = inventoryApi.listMovementsPage as jest.Mock;

describe("useInventoryMovements", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockPage.mockResolvedValue({ movements: [], total: 0, offset: 0, limit: 25 });
  });

  it("does not fetch while disabled", () => {
    renderHook(() =>
      useInventoryMovements({
        businessId: 7,
        itemFilter: "",
        typeFilter: "",
        enabled: false,
      }),
    );
    expect(mockPage).not.toHaveBeenCalled();
  });

  it("fetches server-side with item + type filters and offset paging", async () => {
    mockPage.mockResolvedValue({
      movements: [{ id: 1 }],
      total: 40,
      offset: 0,
      limit: 25,
    });

    const { result } = renderHook(() =>
      useInventoryMovements({
        businessId: 7,
        itemFilter: "3",
        typeFilter: "waste",
        enabled: true,
      }),
    );

    await waitFor(() => expect(result.current.total).toBe(40));
    expect(mockPage).toHaveBeenCalledWith(7, {
      itemId: 3,
      movementType: "waste",
      limit: 25,
      offset: 0,
    });
    // 40 rows / 25 per page = 2 pages.
    expect(result.current.totalPages).toBe(2);
  });

  it("requests the correct offset when paging forward", async () => {
    mockPage.mockResolvedValue({
      movements: [],
      total: 40,
      offset: 25,
      limit: 25,
    });
    const { result } = renderHook(() =>
      useInventoryMovements({
        businessId: 7,
        itemFilter: "",
        typeFilter: "",
        enabled: true,
      }),
    );
    await waitFor(() => expect(mockPage).toHaveBeenCalled());

    act(() => result.current.setPage(2));

    await waitFor(() =>
      expect(mockPage).toHaveBeenLastCalledWith(7, {
        itemId: undefined,
        movementType: undefined,
        limit: 25,
        offset: 25,
      }),
    );
  });

  it("surfaces fetch errors instead of swallowing them", async () => {
    mockPage.mockRejectedValue(new Error("boom"));
    const { result } = renderHook(() =>
      useInventoryMovements({
        businessId: 7,
        itemFilter: "",
        typeFilter: "",
        enabled: true,
      }),
    );
    await waitFor(() => expect(result.current.error).toBe("boom"));
  });
});

describe("useItemMovementHistory", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockPage.mockResolvedValue({ movements: [], total: 0, offset: 0, limit: 10 });
  });

  it("does not fetch until an item is selected", () => {
    renderHook(() => useItemMovementHistory(7, null));
    expect(mockPage).not.toHaveBeenCalled();
  });

  it("fetches the first page scoped to the item and reports hasMore", async () => {
    mockPage.mockResolvedValue({
      movements: Array.from({ length: 10 }, (_, i) => ({ id: i })),
      total: 25,
      offset: 0,
      limit: 10,
    });
    const { result } = renderHook(() => useItemMovementHistory(7, 3));
    await waitFor(() => expect(result.current.movements).toHaveLength(10));
    expect(mockPage).toHaveBeenCalledWith(7, {
      itemId: 3,
      limit: 10,
      offset: 0,
    });
    expect(result.current.hasMore).toBe(true);
    expect(result.current.total).toBe(25);
  });

  it("grows the window on loadMore", async () => {
    mockPage.mockResolvedValue({
      movements: Array.from({ length: 10 }, (_, i) => ({ id: i })),
      total: 25,
      offset: 0,
      limit: 10,
    });
    const { result } = renderHook(() => useItemMovementHistory(7, 3));
    await waitFor(() => expect(mockPage).toHaveBeenCalled());

    act(() => result.current.loadMore());

    await waitFor(() =>
      expect(mockPage).toHaveBeenLastCalledWith(7, {
        itemId: 3,
        limit: 20,
        offset: 0,
      }),
    );
  });
});
