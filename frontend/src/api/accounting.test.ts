import { accountingApi } from "@/api/accounting";
import { axiosInstance } from "@/api/tools/instance";
import { asDollars } from "@/types/money";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
  },
}));

describe("accountingApi", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (axiosInstance.get as jest.Mock).mockReset();
    (axiosInstance.post as jest.Mock).mockReset();
  });

  it("loads an accounting summary for a date range", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { data: { auto_income_total: 85 } } });

    await accountingApi.getSummary("42", "2026-01-01", "2026-01-31");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/accounting/summary",
      { params: { start: "2026-01-01", end: "2026-01-31" } },
    );
  });

  it("rejects accounting summary dates that are not YYYY-MM-DD", async () => {
    await expect(
      accountingApi.getSummary("42", "2026-01-01T00:00:00Z", "2026-01-31"),
    ).rejects.toThrow("Accounting dates must use YYYY-MM-DD");

    expect(axiosInstance.get).not.toHaveBeenCalled();
  });

  it("rejects accounting entry range dates that are not YYYY-MM-DD", async () => {
    await expect(
      accountingApi.listEntries("42", {
        start: "2026-01-01",
        end: "2026-01-31T00:00:00Z",
      }),
    ).rejects.toThrow("Accounting dates must use YYYY-MM-DD");

    expect(axiosInstance.get).not.toHaveBeenCalled();
  });

  it("creates a manual accounting entry", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: { data: { id: 7 } } });

    await accountingApi.createEntry("42", {
      entry_type: "expense",
      category: "inventory",
      amount: asDollars(25.5),
      currency: "USD",
      occurred_at: "2026-01-15T12:00:00Z",
      description: "Produce restock",
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/accounting/entries",
      {
        entry_type: "expense",
        category: "inventory",
        amount: 25.5,
        currency: "USD",
        occurred_at: "2026-01-15T12:00:00Z",
        description: "Produce restock",
      },
    );
  });

  it("marks a payroll run as paid", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: { data: { id: 9, status: "paid" } } });

    await accountingApi.markPayrollRunPaid("42", 9);

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/accounting/payroll-runs/9/mark-paid",
      {},
    );
  });

  it("loads a food-cost report with the period param", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { data: { period: "week", blended_food_cost_pct: 0.31, items: [] } },
    });

    const report = await accountingApi.getFoodCost("42", "week");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/accounting/food-cost",
      { params: { period: "week" } },
    );
    expect(report.blended_food_cost_pct).toBe(0.31);
  });

  it("defaults the food-cost period to week", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { data: { period: "week", blended_food_cost_pct: 0, items: [] } },
    });

    await accountingApi.getFoodCost("42");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/accounting/food-cost",
      { params: { period: "week" } },
    );
  });

  it("loads a food-cost report with an explicit start/end range", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { data: { period: "custom", blended_food_cost_pct: 0.22, items: [] } },
    });

    await accountingApi.getFoodCost("42", "week", {
      start: "2026-07-01",
      end: "2026-07-15",
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/accounting/food-cost",
      { params: { start: "2026-07-01", end: "2026-07-15" } },
    );
  });

  it("listEntries forwards filters/sort/pagination and unwraps the page envelope", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        success: true,
        data: {
          entries: [
            {
              id: 1,
              business_id: 42,
              entry_type: "expense",
              category: "rent",
              amount: 1200,
              currency: "USD",
              occurred_at: "2026-06-15T00:00:00Z",
              description: "June rent",
            },
          ],
          total: 1,
          page: 2,
          page_size: 20,
          total_pages: 1,
        },
      },
    });

    const page = await accountingApi.listEntries("42", {
      start: "2026-06-01",
      end: "2026-06-30",
      type: "expense",
      status: "active",
      category: "rent",
      q: "june",
      sort: "amount_desc",
      page: 2,
      page_size: 20,
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/accounting/entries",
      {
        params: {
          start: "2026-06-01",
          end: "2026-06-30",
          type: "expense",
          status: "active",
          category: "rent",
          q: "june",
          sort: "amount_desc",
          page: 2,
          page_size: 20,
        },
      },
    );
    expect(page.entries).toHaveLength(1);
    expect(page.total).toBe(1);
    expect(page.page).toBe(2);
    expect(page.page_size).toBe(20);
    expect(page.total_pages).toBe(1);
  });

  it("listEntries omits occurred_at_desc sort (backend default DESC)", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        success: true,
        data: { entries: [], total: 0, page: 1, page_size: 20, total_pages: 0 },
      },
    });

    await accountingApi.listEntries("42", {
      start: "2026-01-01",
      end: "2026-01-31",
      sort: "occurred_at_desc",
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/accounting/entries",
      {
        params: {
          start: "2026-01-01",
          end: "2026-01-31",
        },
      },
    );
  });

  it("listPayrollRunsPage always sends page and unwraps PayrollRunsPage", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        success: true,
        data: {
          runs: [
            {
              id: 9,
              period_start: "2026-06-01",
              period_end: "2026-06-15",
              status: "draft",
              gross_total: 1500,
              bonus_total: 100,
              deduction_total: 50,
              net_total: 1550,
              line_items: [],
              payee_count: 2,
            },
          ],
          total: 1,
          page: 1,
          page_size: 20,
          total_pages: 1,
        },
      },
    });

    const page = await accountingApi.listPayrollRunsPage("42", {
      start: "2026-06-01",
      end: "2026-06-30",
      status: "draft",
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/accounting/payroll-runs",
      {
        params: {
          start: "2026-06-01",
          end: "2026-06-30",
          status: "draft",
          page: 1,
        },
      },
    );
    expect(page.runs).toHaveLength(1);
    expect(page.runs[0].payee_count).toBe(2);
    expect(page.total).toBe(1);
    expect(page.page).toBe(1);
    expect(page.page_size).toBe(20);
    expect(page.total_pages).toBe(1);
  });

  it("listPayrollRunsPage forwards explicit page and page_size", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        success: true,
        data: { runs: [], total: 0, page: 3, page_size: 10, total_pages: 0 },
      },
    });

    await accountingApi.listPayrollRunsPage("42", {
      start: "2026-01-01",
      end: "2026-01-31",
      page: 3,
      page_size: 10,
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/accounting/payroll-runs",
      {
        params: {
          start: "2026-01-01",
          end: "2026-01-31",
          page: 3,
          page_size: 10,
        },
      },
    );
  });

  it("getTimeseries requests the day-bucket series and unwraps data", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        success: true,
        data: {
          start: "2026-06-01",
          end: "2026-06-03",
          bucket: "day",
          currency: "USD",
          series: [
            { date: "2026-06-01", income: 100, expense: 40, payroll: 0 },
            { date: "2026-06-02", income: 0, expense: 10, payroll: 200 },
            { date: "2026-06-03", income: 50, expense: 0, payroll: 0 },
          ],
        },
      },
    });

    const ts = await accountingApi.getTimeseries("42", "2026-06-01", "2026-06-03");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/accounting/timeseries",
      { params: { start: "2026-06-01", end: "2026-06-03" } },
    );
    expect(ts.bucket).toBe("day");
    expect(ts.currency).toBe("USD");
    expect(ts.series).toHaveLength(3);
    expect(ts.series[1]).toEqual({
      date: "2026-06-02",
      income: 0,
      expense: 10,
      payroll: 200,
    });
  });

  it("rejects getTimeseries dates that are not YYYY-MM-DD", async () => {
    await expect(
      accountingApi.getTimeseries("42", "2026-06-01T00:00:00Z", "2026-06-03"),
    ).rejects.toThrow("Accounting dates must use YYYY-MM-DD");
    expect(axiosInstance.get).not.toHaveBeenCalled();
  });

  it("entriesExportUrl builds a CSV href with filters (no page/sort)", () => {
    const url = accountingApi.entriesExportUrl("42", {
      start: "2026-06-21",
      end: "2026-07-20",
      status: "active",
      type: "expense",
      category: "rent",
      q: "june",
    });

    expect(url).toContain("/inside/businesses/42/accounting/entries/export.csv?");
    expect(url).toContain("start=2026-06-21");
    expect(url).toContain("end=2026-07-20");
    expect(url).toContain("status=active");
    expect(url).toContain("type=expense");
    expect(url).toContain("category=rent");
    expect(url).toContain("q=june");
    expect(url).not.toContain("page=");
    expect(url).not.toContain("sort=");
  });

  // The backend localizes the streamed CSV headers and falls back to
  // Accept-Language when no lang is supplied, so the operator's UI locale has to
  // travel on the href — otherwise the browser language decides.
  it("entriesExportUrl carries the operator locale as lang", () => {
    expect(
      accountingApi.entriesExportUrl("42", {
        start: "2026-06-21",
        end: "2026-07-20",
        lang: "es",
      }),
    ).toContain("lang=es");

    expect(
      accountingApi.entriesExportUrl("42", {
        start: "2026-06-21",
        end: "2026-07-20",
        lang: "es-AR",
      }),
    ).toContain("lang=es-AR");

    // No locale supplied → no lang param (backend keeps its header fallback).
    expect(
      accountingApi.entriesExportUrl("42", {
        start: "2026-06-21",
        end: "2026-07-20",
      }),
    ).not.toContain("lang=");
  });

  it("payrollExportUrl builds a CSV href with range and status", () => {
    const url = accountingApi.payrollExportUrl("42", {
      start: "2026-06-01",
      end: "2026-06-30",
      status: "draft",
    });

    expect(url).toContain("/inside/businesses/42/accounting/payroll-runs/export.csv?");
    expect(url).toContain("start=2026-06-01");
    expect(url).toContain("end=2026-06-30");
    expect(url).toContain("status=draft");
  });

  // #930: payroll and P&L are plain <a href> downloads like the entries export,
  // and the backend now localizes their headers too — but only the entries
  // builder ever sent the operator locale, so an es-AR dashboard downloaded
  // English-headed payroll/P&L books whenever the browser advertised English.
  it("payrollExportUrl carries the operator locale as lang", () => {
    expect(
      accountingApi.payrollExportUrl("42", {
        start: "2026-06-01",
        end: "2026-06-30",
        lang: "es-AR",
      }),
    ).toContain("lang=es-AR");

    expect(
      accountingApi.payrollExportUrl("42", {
        start: "2026-06-01",
        end: "2026-06-30",
      }),
    ).not.toContain("lang=");
  });

  it("profitLossExportUrl builds a CSV href and carries the operator locale", () => {
    const url = accountingApi.profitLossExportUrl(
      "42",
      "2026-06-01",
      "2026-06-30",
      "es",
    );

    expect(url).toContain(
      "/inside/businesses/42/accounting/profit-loss/export.csv?",
    );
    expect(url).toContain("start=2026-06-01");
    expect(url).toContain("end=2026-06-30");
    expect(url).toContain("lang=es");

    expect(
      accountingApi.profitLossExportUrl("42", "2026-06-01", "2026-06-30"),
    ).not.toContain("lang=");
  });
});
