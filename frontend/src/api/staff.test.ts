import {
  acceptInvitation,
  getInvitationPreview,
  requestLoginCode,
  resendInvitation,
  revokeInvitation,
  verifyLoginCode,
} from "@/api/staff";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    post: jest.fn(),
    get: jest.fn(),
    put: jest.fn(),
    delete: jest.fn(),
  },
}));

describe("staff API", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("loads invitation preview metadata by token", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { email: "invitee@example.com" } });

    await getInvitationPreview("preview-token");

    expect(axiosInstance.get).toHaveBeenCalledWith("/staff/invitation-preview", {
      params: { token: "preview-token" },
    });
  });

  it("accepts a staff invitation", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: { staff: { id: 1 } } });

    await acceptInvitation({ token: "invite-token", name: "Accepted Staff" });

    expect(axiosInstance.post).toHaveBeenCalledWith("/staff/accept-invitation", {
      token: "invite-token",
      name: "Accepted Staff",
    });
  });

  it("requests a staff login code", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: { message: "ok" } });

    await requestLoginCode({ email: "staff@example.com" });

    expect(axiosInstance.post).toHaveBeenCalledWith("/staff/request-login-code", {
      email: "staff@example.com",
    });
  });

  it("verifies a staff login code", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: { staff: { id: 1 } } });

    await verifyLoginCode({ email: "staff@example.com", code: "123456" });

    expect(axiosInstance.post).toHaveBeenCalledWith("/staff/verify-login-code", {
      email: "staff@example.com",
      code: "123456",
    });
  });

  it("rethrows the structured API error from verifyLoginCode (code/error survive for localization)", async () => {
    const apiError = Object.assign(new Error("Request failed"), {
      response: {
        status: 400,
        data: { error: "Invalid or expired login code", code: "STAFF_LOGIN_CODE_INVALID" },
      },
    });
    (axiosInstance.post as jest.Mock).mockRejectedValueOnce(apiError);

    await expect(
      verifyLoginCode({ email: "staff@example.com", code: "000000" }),
    ).rejects.toMatchObject({
      response: {
        data: { error: "Invalid or expired login code", code: "STAFF_LOGIN_CODE_INVALID" },
      },
    });
  });

  it("falls back to a plain descriptive Error for non-API throws from verifyLoginCode", async () => {
    (axiosInstance.post as jest.Mock).mockRejectedValueOnce(new TypeError("boom"));

    await expect(
      verifyLoginCode({ email: "staff@example.com", code: "000000" }),
    ).rejects.toThrow("Failed to verify login code");
  });

  it("rethrows the structured API error from requestLoginCode", async () => {
    const apiError = Object.assign(new Error("Request failed"), {
      response: { status: 429, data: { error: "Too many attempts. Try again later." } },
    });
    (axiosInstance.post as jest.Mock).mockRejectedValueOnce(apiError);

    await expect(requestLoginCode({ email: "staff@example.com" })).rejects.toMatchObject({
      response: { data: { error: "Too many attempts. Try again later." } },
    });
  });

  it("resends an invitation by id", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: { message: "ok", invitation_id: 5 } });
    await resendInvitation("42", 5);
    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/staff/invitations/5/resend",
    );
  });

  it("revokes an invitation by id", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { message: "Staff invitation revoked", invitation_id: 5 },
    });
    const res = await revokeInvitation("42", 5);
    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/staff/invitations/5/revoke",
    );
    expect(res.invitation_id).toBe(5);
  });
});
