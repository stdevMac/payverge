import {
  completeOnboarding,
  getSetupStatus,
  markQRPreviewed,
} from "@/api/onboarding";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
    put: jest.fn(),
  },
}));

describe("onboarding api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("completes onboarding through the canonical route", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { completed_at: "2026-05-23T00:00:00Z" },
    });

    await expect(completeOnboarding("biz-1")).resolves.toEqual({
      completed_at: "2026-05-23T00:00:00Z",
    });
    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/biz-1/onboarding-state/complete",
      {},
    );
  });

  it("records a table QR preview through the pre-billing milestone route", async () => {
    const milestone = {
      qr_previewed: true as const,
      qr_previewed_at: "2026-07-31T00:00:00Z",
    };
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: milestone });

    await expect(markQRPreviewed(42, 7)).resolves.toEqual(milestone);
    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/onboarding/qr-preview",
      { table_id: 7 },
    );
  });

  describe("getSetupStatus", () => {
    it("calls the correct endpoint and returns the response", async () => {
      const mockResponse = {
        steps: {
          business_profile: { done: true, has_name: true, has_address: true, has_currency: true },
          tables: { done: true, count: 3 },
          menu: { done: false, categories: 0, items: 0 },
          staff: { done: false, count: 0 },
          payment: { done: false, count: 0 },
        },
        completed_count: 2,
        total_count: 5,
        required_done: false,
        all_done: false,
      };

      (axiosInstance.get as jest.Mock).mockResolvedValue({ data: mockResponse });

      const result = await getSetupStatus(42);

      expect(axiosInstance.get).toHaveBeenCalledWith(
        "/inside/businesses/42/setup-status",
      );
      expect(result).toEqual(mockResponse);
    });

    it("accepts string business IDs", async () => {
      (axiosInstance.get as jest.Mock).mockResolvedValue({
        data: {
          steps: {},
          completed_count: 0,
          total_count: 5,
          required_done: false,
          all_done: false,
        },
      });

      await getSetupStatus("42");

      expect(axiosInstance.get).toHaveBeenCalledWith(
        "/inside/businesses/42/setup-status",
      );
    });
  });
});
