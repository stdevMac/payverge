import { staffCompensationApi } from "@/api/staffCompensation";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn(), put: jest.fn() },
}));

describe("staffCompensationApi", () => {
  beforeEach(() => {
    (axiosInstance.get as jest.Mock).mockReset();
    (axiosInstance.put as jest.Mock).mockReset();
  });

  it("getCompensation GETs the path and unwraps data.data", async () => {
    const comp = { employment_type: "hourly", hourly_rate: 18.5, annual_salary: 0 };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { data: comp } });
    const result = await staffCompensationApi.getCompensation("7", "9");
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/7/staff/9/compensation",
    );
    expect(result).toEqual(comp);
  });

  it("updateCompensation PUTs the payload and unwraps data.data", async () => {
    const comp = {
      employment_type: "salaried",
      hourly_rate: 0,
      annual_salary: 52000,
    };
    (axiosInstance.put as jest.Mock).mockResolvedValue({ data: { data: comp } });
    const result = await staffCompensationApi.updateCompensation(
      "7",
      "9",
      comp as never,
    );
    expect(axiosInstance.put).toHaveBeenCalledWith(
      "/inside/businesses/7/staff/9/compensation",
      comp,
    );
    expect(result).toEqual(comp);
  });
});
