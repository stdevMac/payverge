import { engagementBadgesApi } from "@/api/engagementBadges";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn() },
}));

const mockGet = axiosInstance.get as jest.Mock;

describe("engagementBadgesApi", () => {
  beforeEach(() => jest.clearAllMocks());

  it("gets the caller's badge counts and unwraps res.data.data", async () => {
    mockGet.mockResolvedValue({
      data: { data: { unacked_announcements: 2, pending_checklists: 1 } },
    });
    const res = await engagementBadgesApi.get("42");
    expect(mockGet).toHaveBeenCalledWith("/inside/businesses/42/me/engagement-badges");
    expect(res.unacked_announcements).toBe(2);
    expect(res.pending_checklists).toBe(1);
  });
});
