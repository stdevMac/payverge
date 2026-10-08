import { errorLogsAPI } from "@/api/errorLogs";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
  },
}));

describe("errorLogs api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("passes admin error filter params through unchanged", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { errors: [], total: 0 },
    });

    await errorLogsAPI.getErrors({
      source: "frontend",
      component: "checkout",
      offset: 20,
      limit: 10,
    });

    expect(axiosInstance.get).toHaveBeenCalledWith("/admin/errors", {
      params: {
        source: "frontend",
        component: "checkout",
        offset: 20,
        limit: 10,
      },
      _useCache: false,
    });
  });
});
