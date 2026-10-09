import {
  getAvailableCounters,
  getBusinessCounters,
  updateCounterSettings,
} from "@/api/counters";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    put: jest.fn(),
  },
}));

describe("counters api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("uses the business-scoped counter endpoints", async () => {
    (axiosInstance.put as jest.Mock).mockResolvedValue({
      data: { message: "updated" },
    });
    (axiosInstance.get as jest.Mock)
      .mockResolvedValueOnce({
        data: {
          counters: [],
          business: {
            counter_enabled: true,
            counter_count: 3,
            counter_prefix: "BAR",
          },
        },
      })
      .mockResolvedValueOnce({ data: { counters: [] } });

    await updateCounterSettings(8, {
      counter_enabled: true,
      counter_count: 3,
      counter_prefix: "BAR",
    });
    await getBusinessCounters(8);
    await getAvailableCounters(8);

    expect(axiosInstance.put).toHaveBeenCalledWith(
      "/inside/businesses/8/counters/settings",
      {
        counter_enabled: true,
        counter_count: 3,
        counter_prefix: "BAR",
      },
    );
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      1,
      "/inside/businesses/8/counters",
    );
    expect(axiosInstance.get).toHaveBeenNthCalledWith(
      2,
      "/inside/businesses/8/counters/available",
    );
  });
});
