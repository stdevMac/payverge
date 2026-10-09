import { getUserProfile } from "@/api/users/profile";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
  },
}));

describe("users/profile api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("loads user profile by protected user identifier", async () => {
    const profile = { address: "0xowner", email: "owner@example.com" };
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      status: 200,
      data: profile,
    });

    await expect(getUserProfile("0xowner")).resolves.toEqual(profile);
    expect(axiosInstance.get).toHaveBeenCalledWith("/inside/get_user/0xowner");
  });
});
