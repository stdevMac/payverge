import { getTablesWithStatus } from "@/api/business";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
  },
}));

describe("getTablesWithStatus null arrays", () => {
  it("normalizes null active_bills and reservations to empty arrays", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        tables: [
          {
            table: { id: 1, name: "Patio 1", is_active: true },
            status: "available",
            active_bills: null,
            active_bills_count: 0,
            active_bill_physical_item_quantity: 0,
            reservations: null,
            reservations_count: 0,
          },
        ],
      },
    });

    const result = await getTablesWithStatus(9);

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/9/tables/status",
    );
    expect(result.tables[0].active_bills).toEqual([]);
    expect(result.tables[0].reservations).toEqual([]);
    expect(Array.isArray(result.tables[0].active_bills)).toBe(true);
    expect(Array.isArray(result.tables[0].reservations)).toBe(true);
  });
});
