import { getStaffProfile, staffLogout } from "@/api/staffProfile";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

describe("staffProfile api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("loads the current staff profile", async () => {
    const profile = {
      token: "staff-token",
      staff_id: 1,
      business_id: 9,
      role: "manager",
    };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: profile });

    await expect(getStaffProfile()).resolves.toEqual(profile);
    expect(axiosInstance.get).toHaveBeenCalledWith("/staff/profile");
  });

  it("does not throw when server-side logout cleanup fails", async () => {
    const warnSpy = jest.spyOn(console, "warn").mockImplementation(() => {});
    (axiosInstance.post as jest.Mock).mockRejectedValue(new Error("offline"));

    await expect(staffLogout()).resolves.toBeUndefined();
    expect(axiosInstance.post).toHaveBeenCalledWith("/staff/logout");

    warnSpy.mockRestore();
  });
});
