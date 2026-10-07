import { scheduleSettingsApi } from "./scheduleSettings";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn(), put: jest.fn() },
}));

const mocked = axiosInstance as unknown as {
  get: jest.Mock;
  put: jest.Mock;
};

describe("scheduleSettingsApi", () => {
  beforeEach(() => jest.clearAllMocks());

  it("gets settings for a business", async () => {
    mocked.get.mockResolvedValue({ data: { data: { business_id: 42, week_start_day: 1 } } });
    const out = await scheduleSettingsApi.get("42");
    expect(mocked.get).toHaveBeenCalledWith("/inside/businesses/42/schedule/settings");
    expect(out.week_start_day).toBe(1);
  });

  it("updates settings with a partial body", async () => {
    mocked.put.mockResolvedValue({ data: { data: { business_id: 42, posted_lead_days: 14 } } });
    const out = await scheduleSettingsApi.update("42", { posted_lead_days: 14 });
    expect(mocked.put).toHaveBeenCalledWith("/inside/businesses/42/schedule/settings", { posted_lead_days: 14 });
    expect(out.posted_lead_days).toBe(14);
  });
});
