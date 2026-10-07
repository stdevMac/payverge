import { updateUser } from "@/api/users/updateUser";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    put: jest.fn(),
  },
}));

jest.mock("@/utils/errorLogger", () => ({
  logError: jest.fn(),
}));

describe("users/updateUser api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("updates the authenticated user through the protected update route", async () => {
    const payload = {
      email: "owner@example.com",
      name: "Owner",
    };
    (axiosInstance.put as jest.Mock).mockResolvedValue({
      status: 200,
      data: payload,
    });

    await expect(updateUser(payload as never)).resolves.toBe(true);
    expect(axiosInstance.put).toHaveBeenCalledWith(
      "/inside/update_user",
      payload,
    );
  });
});
