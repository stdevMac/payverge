import {
  getKitchenOrdersStatus,
  toggleKitchenAndOrders,
} from "@/api/kitchenOrders";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

describe("kitchenOrders api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("reads and toggles kitchen/order status through operational routes", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { kitchen_enabled: true, orders_enabled: false },
    });
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        message: "updated",
        kitchen_enabled: false,
        orders_enabled: false,
      },
    });

    await expect(getKitchenOrdersStatus(12)).resolves.toEqual({
      kitchen_enabled: true,
      orders_enabled: false,
    });
    await toggleKitchenAndOrders(12, false);

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/12/kitchen-orders-status",
    );
    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/12/toggle-kitchen-orders",
      { enabled: false },
    );
  });
});
