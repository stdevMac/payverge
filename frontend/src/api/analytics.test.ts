import { analyticsApi } from "@/api/analytics";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

describe("analytics api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("normalizes legacy tip analytics payloads into the dashboard contract", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        data: {
          total_tips: 15,
          average_tip: 7.5,
          average_tip_rate: 12.5,
          tip_distribution: {
            "$5-10": 1,
            "$10-20": 1,
          },
          top_tippers: [],
        },
      },
    });

    const analytics = await analyticsApi.getTipAnalytics("42", "week");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/analytics/tips?period=week",
    );
    expect(analytics.tip_count).toBe(2);
    expect(analytics.hourly_tips).toEqual({});
    expect(analytics.daily_comparison).toEqual({
      today: 0,
      yesterday: 0,
      change_percentage: 0,
    });
  });

  it("maps backend item analytics aliases used by the dashboard", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        data: [
          {
            item_id: "burger",
            item_name: "Burger",
            category: "Mains",
            total_sold: 9,
            revenue: 108,
            bills_featured: 6,
            average_price: 12,
            popularity: 75,
          },
        ],
      },
    });

    const items = await analyticsApi.getItemAnalytics("42", "month");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/analytics/items?period=month",
    );
    expect(items).toEqual([
      expect.objectContaining({
        item_id: "burger",
        avg_price: 12,
        popularity_rank: 75,
      }),
    ]);
  });

  it("normalizes dashboard top items to the shared item stats shape", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        data: {
          today: { revenue: 10, tips: 1, transactions: 1, bills: 1 },
          week: {
            revenue: 10,
            tips: 1,
            transactions: 1,
            bills: 1,
            unique_customers: 1,
            average_ticket: 10,
          },
          live: { active_bills: 1 },
          top_items: [
            {
              item_id: "fries",
              item_name: "Fries",
              category: "Sides",
              total_sold: 4,
              revenue: 20,
              bills_featured: 3,
              average_price: 5,
              popularity: 60,
            },
          ],
        },
      },
    });

    const summary = await analyticsApi.getDashboardSummary("42");

    expect(summary.top_items).toEqual([
      expect.objectContaining({
        item_id: "fries",
        avg_price: 5,
        popularity_rank: 60,
      }),
    ]);
  });

  it("loads live bills through the maintained analytics route", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        data: [
          {
            id: 9,
            bill_number: "B-9",
            table_name: "Patio 1",
            table_code: "P1",
            total_amount: 32,
            paid_amount: 10,
            remaining_amount: 22,
            tip_amount: 2,
            status: "open",
            created_at: "2026-04-10T10:00:00Z",
            updated_at: "2026-04-10T10:05:00Z",
          },
        ],
      },
    });

    const liveBills = await analyticsApi.getLiveBills("42");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/analytics/live-bills",
    );
    expect(liveBills).toEqual([
      expect.objectContaining({
        id: 9,
        bill_number: "B-9",
      }),
    ]);
  });

  it("loads dashboard summaries in one batch request", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        data: {
          "42": {
            today: { revenue: 10, tips: 1, transactions: 2, bills: 2 },
            week: {
              revenue: 40,
              tips: 4,
              transactions: 8,
              bills: 8,
              unique_customers: 6,
              average_ticket: 5,
            },
            live: { active_bills: 1 },
            top_items: [],
          },
        },
      },
    });

    const summaries = await analyticsApi.getDashboardSummaries(["42"]);

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/analytics/dashboard-summaries",
      { business_ids: [42] },
    );
    expect(summaries["42"].today.revenue).toBe(10);
  });

  it("normalizes untabled delivery checks and floor remaining on dashboard summary", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        data: {
          today: {
            revenue: 118.71,
            collected_revenue: 0,
            floor_remaining: 118.71,
            tips: 0,
            transactions: 0,
            bills: 0,
            order_count: 5,
          },
          week: {
            revenue: 0,
            tips: 0,
            transactions: 0,
            bills: 0,
            unique_customers: 0,
            average_ticket: 0,
          },
          live: {
            active_bills: 5,
            open_tables: 4,
            untabled_bills: 1,
          },
          top_items: [],
        },
      },
    });

    const summary = await analyticsApi.getDashboardSummary("86");
    expect(summary.live.active_bills).toBe(5);
    expect(summary.live.open_tables).toBe(4);
    expect(summary.live.untabled_bills).toBe(1);
    expect(summary.today.floor_remaining).toBe(118.71);
    expect(summary.today.collected_revenue).toBe(0);
    expect(summary.today.revenue).toBe(0);
    expect(summary.today.order_count).toBe(5);
  });

  it("builds the timeseries URL and normalizes buckets into Dollars", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        data: {
          buckets: [
            { date: "2026-05-01", revenue: 100, tips: 10, bills: 4, transactions: 5, average_ticket: 25 },
            { date: "2026-05-02" }, // sparse day — missing fields default to 0
          ],
          range: { from: "2026-05-01", to: "2026-05-02" },
        },
      },
    });

    const series = await analyticsApi.getTimeseries("42", { from: "2026-05-01", to: "2026-05-02" });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/analytics/timeseries?from=2026-05-01&to=2026-05-02&bucket=day",
    );
    expect(series.buckets).toHaveLength(2);
    expect(series.buckets[0].revenue).toBe(100);
    expect(series.buckets[1].revenue).toBe(0);
    expect(series.buckets[1].bills).toBe(0);
    expect(series.range).toEqual({ from: "2026-05-01", to: "2026-05-02" });
  });

  it("forwards period= on the timeseries URL when from/to are omitted", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { data: { buckets: [], range: { from: "2026-08-13", to: "2026-08-19" } } },
    });
    await analyticsApi.getTimeseries("42", { period: "7d" });
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/analytics/timeseries?period=7d&bucket=day",
    );
  });
});
