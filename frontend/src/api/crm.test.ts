import { businessCRMAPI, crmAPI } from "@/api/crm";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    post: jest.fn(),
    get: jest.fn(),
    put: jest.fn(),
    delete: jest.fn(),
  },
}));

describe("crmAPI", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("uses cookie-backed self profile endpoint", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { id: 10 } });

    await crmAPI.getProfile();

    expect(axiosInstance.get).toHaveBeenCalledWith("/customer/profile");
  });

  it("connects authenticated customer without sending customer_id", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { success: true },
    });

    await crmAPI.connectToBusiness(42, true);

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/customer/connect-business",
      {
        business_id: 42,
        opt_in_marketing: true,
      },
    );
  });

  it("calls customer logout endpoint", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { success: true },
    });

    await crmAPI.logout();

    expect(axiosInstance.post).toHaveBeenCalledWith("/customer/logout");
  });

  it("updates business customer tags through the CRM tags endpoint", async () => {
    (axiosInstance.put as jest.Mock).mockResolvedValue({
      data: { success: true },
    });

    await businessCRMAPI.updateCustomerTags(42, 7, '["vip","birthday"]');

    expect(axiosInstance.put).toHaveBeenCalledWith(
      "/inside/businesses/42/crm/customers/7/tags",
      { tags: '["vip","birthday"]' },
    );
  });

  it("sets an absolute loyalty point balance through the existing adjust API", async () => {
    (axiosInstance.put as jest.Mock).mockResolvedValue({
      data: { message: "Loyalty points updated", loyalty_points: 50 },
    });

    await businessCRMAPI.adjustCustomerLoyaltyPoints(42, 7, 50, "service recovery");

    expect(axiosInstance.put).toHaveBeenCalledWith(
      "/inside/businesses/42/crm/customers/7/loyalty-points",
      { loyalty_points: 50, reason: "service recovery" },
    );
  });

  it("sends server sort params and omits tier when All", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { customers: [] } });

    await businessCRMAPI.getCustomers(42, 2, 20, "acme", "All", {
      sortBy: "total_spent",
      sortDir: "asc",
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/crm/customers",
      {
        params: {
          page: 2,
          page_size: 20,
          search: "acme",
          sort_by: "total_spent",
          sort_dir: "asc",
        },
      },
    );
  });

  it("sends segment and drops sort params when drilling into a segment", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { customers: [] } });

    await businessCRMAPI.getCustomers(42, 1, 20, "", "", {
      segment: "lapsed",
      sortBy: "total_spent",
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/crm/customers",
      {
        params: { page: 1, page_size: 20, search: "", segment: "lapsed" },
      },
    );
  });
});

describe("getSegments", () => {
  beforeEach(() => jest.clearAllMocks());

  it("propagates errors instead of swallowing them into zeros", async () => {
    const { getSegments } = await import("@/api/crm");
    (axiosInstance.get as jest.Mock).mockRejectedValue(new Error("503"));

    await expect(getSegments(42)).rejects.toThrow("503");
  });
});
