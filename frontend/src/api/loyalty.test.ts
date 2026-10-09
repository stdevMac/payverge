import { getLoyaltyRate } from "@/api/loyalty";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

describe("getLoyaltyRate (#895 currency-neutral rate)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (axiosInstance.get as jest.Mock).mockReset();
  });

  it("reads the currency-neutral rate on a venue that is not priced in dollars", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        points_per_currency_unit: 1,
        currency: "ARS",
      },
    });

    await expect(getLoyaltyRate("10DSRZ4YP2")).resolves.toBe(1);
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/guest/table/10DSRZ4YP2/loyalty-rate",
    );
  });

  it("ignores the retired points_per_dollar key", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { points_per_dollar: 100 },
    });

    await expect(getLoyaltyRate("ABC123")).resolves.toBe(0);
  });

  it("returns 0 when the payload carries no rate at all", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: {} });

    await expect(getLoyaltyRate("ABC123")).resolves.toBe(0);
  });
});
