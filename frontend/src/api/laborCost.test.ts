import { laborCostApi } from "@/api/laborCost";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn() },
}));

describe("laborCostApi.getLaborCost", () => {
  beforeEach(() => {
    (axiosInstance.get as jest.Mock).mockReset();
  });

  it("GETs the /inside labor-cost path with the period param and unwraps data.data", async () => {
    const report = {
      period: "week",
      labor_cost: 5300.0,
      net_sales: 16000.0,
      labor_cost_pct: 0.331,
      payroll_run_count: 2,
      has_data: true,
      contributions: [],
      food_cost_pct: 0.29,
      prime_cost_pct: 0.621,
    };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { data: report } });

    const result = await laborCostApi.getLaborCost("7", "week");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/7/accounting/labor-cost",
      { params: { period: "week" } },
    );
    expect(result).toEqual(report);
  });

  it("defaults the period to week", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { data: { period: "week", has_data: false, contributions: [] } },
    });
    await laborCostApi.getLaborCost("7");
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/7/accounting/labor-cost",
      { params: { period: "week" } },
    );
  });

  it("passes an explicit start/end range instead of period", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { data: { period: "custom", has_data: true, contributions: [] } },
    });
    await laborCostApi.getLaborCost("7", "week", {
      start: "2026-07-01",
      end: "2026-07-15",
    });
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/7/accounting/labor-cost",
      { params: { start: "2026-07-01", end: "2026-07-15" } },
    );
  });

  it("passes the month period param when requested", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { data: { period: "month", has_data: false, contributions: [] } },
    });
    const result = await laborCostApi.getLaborCost("42", "month");
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/accounting/labor-cost",
      { params: { period: "month" } },
    );
    expect(result).toMatchObject({ period: "month" });
  });
});

describe("laborCostApi.getActual", () => {
  beforeEach(() => {
    (axiosInstance.get as jest.Mock).mockReset();
  });

  it("GETs the labor-cost path with basis=actual + period and unwraps data.data", async () => {
    const report = {
      period: "week",
      basis: "actual",
      labor_cost_pct: 0.28,
      worked_hours: 120.5,
      has_data: true,
      staff_contributions: [{ staff_id: 7, worked_minutes: 360, worked_hours: 6, labor_cost: 90 }],
      labor_cost: 3400,
    };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { data: report } });

    const result = await laborCostApi.getActual("7", "week");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/7/accounting/labor-cost",
      { params: { basis: "actual", period: "week" } },
    );
    expect(result).toEqual(report);
  });
});
