import { wasteVarianceApi } from "@/api/wasteVariance";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn() },
}));

describe("wasteVarianceApi.getWasteVariance", () => {
  beforeEach(() => {
    (axiosInstance.get as jest.Mock).mockReset();
  });

  it("GETs the /inside waste-variance path with the period param and unwraps data.data", async () => {
    const report = {
      period: "week",
      tracked_loss_cost: 45.5,
      total_variance_cost: 60.0,
      theoretical_usage_cost: 200.0,
      loss_by_reason: [],
      ingredients: [],
      items_without_recipe: 0,
      sparse: false,
    };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { data: report } });

    const result = await wasteVarianceApi.getWasteVariance("7", "week");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/7/accounting/waste-variance",
      { params: { period: "week" } },
    );
    expect(result).toEqual(report);
  });

  it("defaults the period to week", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { data: { period: "week", ingredients: [], loss_by_reason: [] } },
    });
    await wasteVarianceApi.getWasteVariance("7");
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/7/accounting/waste-variance",
      { params: { period: "week" } },
    );
  });

  it("passes the month period param when requested", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { data: { period: "month", ingredients: [], loss_by_reason: [] } },
    });
    const result = await wasteVarianceApi.getWasteVariance("42", "month");
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/accounting/waste-variance",
      { params: { period: "month" } },
    );
    expect(result).toMatchObject({ period: "month" });
  });
});
