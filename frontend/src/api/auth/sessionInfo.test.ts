import { getSessionInfo } from "./sessionInfo";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn() },
}));

describe("getSessionInfo", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("returns the body when backend reports unauthenticated", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { authenticated: false },
    });

    await expect(getSessionInfo()).resolves.toEqual({ authenticated: false });
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/auth/session-info",
      expect.objectContaining({ _skipErrorToast: true, _useCache: false }),
    );
  });

  it("returns the body when backend reports authenticated", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { authenticated: true, type: "user", user_id: 1 },
    });

    await expect(getSessionInfo()).resolves.toMatchObject({
      authenticated: true,
      type: "user",
    });
  });

  it("throws on transport error rather than masking as unauthenticated", async () => {
    const error = Object.assign(new Error("Network"), {
      response: { status: 500 },
    });
    (axiosInstance.get as jest.Mock).mockRejectedValue(error);

    await expect(getSessionInfo()).rejects.toThrow("Network");
  });

  it("throws on 502 rather than masking as unauthenticated", async () => {
    const error = Object.assign(new Error("Bad gateway"), {
      response: { status: 502 },
    });
    (axiosInstance.get as jest.Mock).mockRejectedValue(error);

    await expect(getSessionInfo()).rejects.toThrow("Bad gateway");
  });
});
