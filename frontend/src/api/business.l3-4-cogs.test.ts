/**
 * D1 / L3-4: plate COGS must ride on production addMenuItem and updateMenuItem
 * wire payloads (not a pure helper). Revert-proof: drop `cogs` from the
 * whitelist in business.ts → objectContaining fails.
 */
import { asDollars } from "@/types/money";
import { axiosInstance } from "@/api/tools/instance";
import { addMenuItem, updateMenuItem } from "@/api/business";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    put: jest.fn(),
    post: jest.fn(),
  },
}));

describe("L3-4 menu item cogs whitelist", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { message: "ok" },
    });
    (axiosInstance.put as jest.Mock).mockResolvedValue({
      data: { message: "ok" },
    });
  });

  const item = {
    id: "item-1",
    name: "Taco",
    description: "",
    price: asDollars(12),
    cogs: asDollars(3.5),
    currency: "USD",
    is_available: true,
  };

  it("includes cogs on addMenuItem", async () => {
    await addMenuItem(42, 0, item as never, 3, "cat-1", true);
    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/menu/items",
      expect.objectContaining({
        item: expect.objectContaining({ cogs: asDollars(3.5) }),
      }),
    );
  });

  it("includes cogs on updateMenuItem", async () => {
    await updateMenuItem(42, 0, 0, item as never, 3, "cat-1", "item-1", true);
    expect(axiosInstance.put).toHaveBeenCalledWith(
      "/inside/businesses/42/menu/items",
      expect.objectContaining({
        item: expect.objectContaining({ cogs: asDollars(3.5) }),
      }),
    );
  });
});
