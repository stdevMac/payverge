import {
  getGoogleAuthURL,
  getWalletChallenge,
  login,
  register,
  requestPasswordReset,
  resetPassword,
} from "@/api/auth";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

describe("auth api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("requests a wallet challenge through the auth challenge endpoint", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { challenge: "sign-me" },
    });

    await expect(getWalletChallenge("0xabc")).resolves.toEqual({
      challenge: "sign-me",
    });
    expect(axiosInstance.post).toHaveBeenCalledWith("/auth/challenge", {
      address: "0xabc",
    });
  });

  it("binds a launch invite to the Google OAuth state request", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { url: "https://accounts.google.test", state: "state" },
    });

    await getGoogleAuthURL("  cohort-ALPHA  ");

    expect(axiosInstance.get).toHaveBeenCalledWith("/auth/google", {
      params: { invite_code: "cohort-ALPHA" },
    });
  });

  it("uses canonical password reset payload keys", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { message: "ok" },
    });

    await requestPasswordReset("owner@example.com");
    await resetPassword("reset-token", "new-password");

    expect(axiosInstance.post).toHaveBeenNthCalledWith(
      1,
      "/auth/password/reset-request",
      { email: "owner@example.com" },
    );
    expect(axiosInstance.post).toHaveBeenNthCalledWith(
      2,
      "/auth/password/reset",
      { token: "reset-token", new_password: "new-password" },
    );
  });


  it("skips the interceptor toast on login and register so the form owns the error", async () => {
    const payload = {
      success: true,
      token: "t",
      user_id: 1,
      email: "owner@example.com",
      name: "Owner",
      wallet_linked: false,
      provider: "email",
      is_new_user: false,
    };
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: payload });

    await login({ email: "owner@example.com", password: "secret" });
    await register({
      email: "owner@example.com",
      password: "secret",
      name: "Owner",
    });

    expect(axiosInstance.post).toHaveBeenNthCalledWith(
      1,
      "/auth/login",
      { email: "owner@example.com", password: "secret" },
      { _skipErrorToast: true },
    );
    expect(axiosInstance.post).toHaveBeenNthCalledWith(
      2,
      "/auth/register",
      { email: "owner@example.com", password: "secret", name: "Owner" },
      { _skipErrorToast: true },
    );
  });
});
