import { exportPayments, getPaymentHistoryPage } from "@/api/payments";
import { axiosInstance } from "@/api/tools/instance";
import { logError } from "@/utils/errorLogger";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
  },
}));

jest.mock("@/utils/errorLogger", () => ({
  logError: jest.fn(),
}));

describe("payments api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("exports payment history through the dedicated payment export route", async () => {
    const blob = new Blob(["csv"]);
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: blob,
    });

    const result = await exportPayments(42, { period: "month" }, "csv");

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/payments/export",
      {
        params: {
          period: "month",
          format: "csv",
        },
        responseType: "blob",
      },
    );
    expect(result).toBe(blob);
  });

  it("accepts reversed and refunded payment history statuses", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        items: [
          {
            id: 1,
            bill_id: 10,
            bill_number: "B-10",
            table_name: "Patio",
            payer_address: "0xabc",
            amount: 12.5,
            tip_amount: 0,
            currency: "USD",
            tx_hash: "0xreversed",
            status: "reversed",
            created_at: "2026-05-01T00:00:00Z",
            updated_at: "2026-05-02T00:00:00Z",
          },
          {
            id: 2,
            bill_id: 11,
            bill_number: "B-11",
            table_name: "Bar",
            payer_address: "0xdef",
            amount: 8,
            tip_amount: 1,
            currency: "JPY",
            tx_hash: "0xrefunded",
            status: "refunded",
            created_at: "2026-05-01T00:00:00Z",
            updated_at: "2026-05-02T00:00:00Z",
          },
        ],
        total: 2,
        page: 1,
        page_size: 20,
      },
    });

    const result = await getPaymentHistoryPage(42);

    expect(result.items).toHaveLength(2);
    expect(logError).not.toHaveBeenCalled();
  });

  it("requests a server page and parses the {items,total,page,page_size,available_methods} envelope", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        items: [],
        total: 137,
        page: 3,
        page_size: 20,
        available_methods: ["crypto", "card", "cash"],
      },
    });

    const result = await getPaymentHistoryPage(42, {
      period: "month",
      q: "B-10",
      status: "confirmed",
      method: "crypto",
      page: 3,
      pageSize: 20,
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/payments/history",
      {
        params: {
          period: "month",
          q: "B-10",
          status: "confirmed",
          method: "crypto",
          page: "3",
          page_size: "20",
        },
      },
    );
    expect(result.total).toBe(137);
    expect(result.page).toBe(3);
    expect(result.items).toEqual([]);
    expect(result.available_methods).toEqual(["crypto", "card", "cash"]);
  });

  it("omits status/method params when set to 'all' and defaults page to 1", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { items: [], total: 0, page: 1, page_size: 20 },
    });

    await getPaymentHistoryPage(42, { period: "week", status: "all", method: "all" });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/payments/history",
      { params: { period: "week", page: "1" } },
    );
  });
});
