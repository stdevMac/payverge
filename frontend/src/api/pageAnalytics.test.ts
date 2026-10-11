import { pageAnalyticsAPI } from "@/api/pageAnalytics";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
  },
}));

describe("pageAnalytics api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("passes optional date range params to the admin analytics summary", async () => {
    const summary = {
      total_page_views: 1,
      total_sessions: 1,
      total_interactions: 0,
      total_conversions: 0,
      average_session_time: 0,
      bounce_rate: 0,
      conversion_rate: 0,
      top_pages: [],
      top_interactions: [],
      device_breakdown: {},
      country_breakdown: {},
      hourly_traffic: [],
      conversion_funnel: [],
    };
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: summary });

    await expect(
      pageAnalyticsAPI.getAnalyticsSummary("2026-05-01", "2026-05-23"),
    ).resolves.toEqual(summary);
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/admin/analytics/summary",
      { params: { start_date: "2026-05-01", end_date: "2026-05-23" } },
    );
  });

  it("defaults recent sessions limit to 50", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: [] });

    await pageAnalyticsAPI.getRecentSessions();

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/admin/analytics/sessions",
      { params: { limit: 50 } },
    );
  });
});
