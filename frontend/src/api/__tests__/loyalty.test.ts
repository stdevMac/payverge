/** @jest-environment node */
import { getLoyalty } from "../loyalty";

jest.mock("../tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    put: jest.fn(),
    post: jest.fn(),
  },
}));

import { axiosInstance } from "../tools/instance";

describe("loyalty api", () => {
  beforeEach(() => {
    (axiosInstance.get as jest.Mock).mockReset();
  });

  it("getLoyalty hits the right URL", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { program: {}, tiers: [] },
    });
    await getLoyalty(7);
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/7/crm/loyalty"
    );
  });
});
