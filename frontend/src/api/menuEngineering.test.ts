import { menuEngineeringApi } from "@/api/menuEngineering";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn() },
}));

describe("menuEngineeringApi.getMenuEngineering", () => {
  beforeEach(() => {
    (axiosInstance.get as jest.Mock).mockReset();
  });

  it("GETs the /inside menu-engineering path with the period param and unwraps data.data", async () => {
    const report = {
      period: "week",
      median_food_cost_pct: 0.3,
      median_qty_sold: 12,
      dishes: [],
      rollups: [],
      items_needing_cost: 0,
      sparse: true,
      has_sales: false,
    };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { data: report } });

    const result = await menuEngineeringApi.getMenuEngineering("7", "week");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/7/accounting/menu-engineering",
      { params: { period: "week" } },
    );
    expect(result).toEqual(report);
  });

  it("defaults the period to week", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { data: { period: "week", dishes: [], rollups: [] } } });
    await menuEngineeringApi.getMenuEngineering("7");
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/7/accounting/menu-engineering",
      { params: { period: "week" } },
    );
  });
});
