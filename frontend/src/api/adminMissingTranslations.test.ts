import {
  getMissingTranslations,
  updateMissingTranslationStatus,
} from "./adminMissingTranslations";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    patch: jest.fn(),
  },
}));

const mockedAxios = axiosInstance as jest.Mocked<typeof axiosInstance>;

describe("admin missing translations API", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("normalizes backend occurrence_count rows into hit_count for UI clients", async () => {
    mockedAxios.get.mockResolvedValue({
      data: {
        rows: [
          {
            id: 42,
            locale: "en",
            key_path: "businessDashboard.dashboard.liveBills.loading",
            page: "/business/demo/dashboard",
            fallback_used: "leaf",
            occurrence_count: 7,
            first_seen_at: "2026-07-01T00:00:00Z",
            last_seen_at: "2026-07-06T00:00:00Z",
            status: "open",
            status_updated_at: null,
          },
        ],
        total: 1,
        limit: 100,
        offset: 0,
      },
    });

    const response = await getMissingTranslations({ limit: 100, offset: 0 });

    expect(mockedAxios.get).toHaveBeenCalledWith(
      "/admin/analytics/missing-translations",
      { params: { limit: 100, offset: 0 }, _useCache: false },
    );
    expect(response.rows[0].hit_count).toBe(7);
  });

  it("forwards the lifecycle status when listing reports", async () => {
    mockedAxios.get.mockResolvedValue({
      data: { rows: [], total: 0, limit: 100, offset: 0 },
    });

    await getMissingTranslations({
      limit: 100,
      offset: 0,
      locale: "es-AR",
      status: "resolved",
    });

    expect(mockedAxios.get).toHaveBeenCalledWith(
      "/admin/analytics/missing-translations",
      {
        params: {
          limit: 100,
          offset: 0,
          locale: "es-AR",
          status: "resolved",
        },
        _useCache: false,
      },
    );
  });

  it("patches the lifecycle status for one report", async () => {
    mockedAxios.patch.mockResolvedValue({ data: { success: true } });

    await updateMissingTranslationStatus(42, "ignored");

    expect(mockedAxios.patch).toHaveBeenCalledWith(
      "/admin/analytics/missing-translations/42/status",
      { status: "ignored" },
    );
  });
});
