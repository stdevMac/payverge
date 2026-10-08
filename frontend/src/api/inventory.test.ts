import { inventoryApi } from "@/api/inventory";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
    put: jest.fn(),
    delete: jest.fn(),
  },
}));

describe("inventory api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("lists items with include_inactive only when requested", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { items: [] } });

    await inventoryApi.listItems(7);
    await inventoryApi.listItems(7, true);

    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      1,
      "/inside/businesses/7/inventory/items",
      { params: undefined },
    );
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      2,
      "/inside/businesses/7/inventory/items",
      { params: { include_inactive: true } },
    );
  });

  it("sends adjustment payloads to the inventory adjustment endpoint", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { item: { id: 1 }, movement: { id: 2 } },
    });

    await inventoryApi.createAdjustment(7, {
      inventory_item_id: 3,
      quantity_change: -2,
      reason: "spoilage",
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/7/inventory/adjustments",
      {
        inventory_item_id: 3,
        quantity_change: -2,
        reason: "spoilage",
      },
    );
  });

  it("pages movements with server filters and returns the total", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { movements: [{ id: 1 }], total: 40, offset: 15, limit: 15 },
    });

    const page = await inventoryApi.listMovementsPage(7, {
      itemId: 3,
      movementType: "waste",
      limit: 15,
      offset: 15,
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/7/inventory/movements",
      { params: { limit: 15, offset: 15, item_id: 3, movement_type: "waste" } },
    );
    expect(page.total).toBe(40);
    expect(page.movements).toHaveLength(1);
  });

  it("omits filter params when not provided and defaults paging", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { movements: [], total: 0, offset: 0, limit: 25 },
    });

    await inventoryApi.listMovementsPage(7);

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/7/inventory/movements",
      { params: { limit: 25, offset: 0 } },
    );
  });
});
