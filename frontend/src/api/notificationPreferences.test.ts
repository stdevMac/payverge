import {
  getEmailNotificationPreferences,
  updateEmailNotificationPreferences,
  type AccountEmailPreferences,
} from "@/api/notificationPreferences";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    put: jest.fn(),
  },
}));

const mockGet = axiosInstance.get as jest.Mock;
const mockPut = axiosInstance.put as jest.Mock;

const prefs: AccountEmailPreferences = {
  email_enabled: true,
  transactional_enabled: true,
  reports_enabled: false,
  news_enabled: false,
  updates_enabled: true,
  security_enabled: true,
  statistics_enabled: false,
};

describe("notificationPreferences api", () => {
  beforeEach(() => jest.clearAllMocks());

  it("reads account-level email preferences from the user-scoped endpoint", async () => {
    mockGet.mockResolvedValue({ data: { address: "0xabc", preferences: prefs } });

    const result = await getEmailNotificationPreferences();

    expect(mockGet).toHaveBeenCalledWith("/inside/settings/notifications");
    expect(result).toEqual(prefs);
  });

  it("writes the full preference object so untouched flags are preserved", async () => {
    const updated = { ...prefs, news_enabled: true };
    mockPut.mockResolvedValue({ data: { preferences: updated } });

    const result = await updateEmailNotificationPreferences(updated);

    expect(mockPut).toHaveBeenCalledWith("/inside/settings/notifications", {
      preferences: updated,
    });
    expect(result).toEqual(updated);
  });
});
