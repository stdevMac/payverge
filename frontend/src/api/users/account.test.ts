import {
  downloadAccountExport,
  requestAccountDeletion,
} from "@/api/users/account";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    post: jest.fn(),
  },
}));

describe("users/account api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("downloads the account export from the protected account route", async () => {
    const payload = {
      generated_at: "2026-05-23T00:00:00Z",
      export_ttl_at: "2026-05-24T00:00:00Z",
      profile: {
        id: 1,
        email: "owner@example.com",
        name: "Owner",
        username: "owner",
        wallet_address: "0xowner",
        role: "user",
        auth_method: "email",
        email_verified: true,
        language_selected: "en",
        created_at: "2026-05-01T00:00:00Z",
      },
      businesses: [],
    };
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: payload });

    await expect(downloadAccountExport()).resolves.toEqual(payload);
    expect(axiosInstance.post).toHaveBeenCalledWith("/inside/account/export");
  });

  it("sends deletion confirmation and optional reason", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        success: true,
        deleted_at: "2026-05-23T00:00:00Z",
        deletion_scheduled_at: "2026-06-22T00:00:00Z",
      },
    });

    await requestAccountDeletion("owner@example.com", "closing");

    expect(axiosInstance.post).toHaveBeenCalledWith("/inside/account/delete", {
      confirm_email: "owner@example.com",
      reason: "closing",
    });
  });
});
