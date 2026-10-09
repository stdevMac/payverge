/** @jest-environment jsdom */
/**
 * D1 / L6-7: clicking Export on RevenuePanel must toast a localized range-too-large
 * message when the API returns 413 — not console.error alone.
 */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import toast from "react-hot-toast";
import { analyticsApi } from "@/api/analytics";
import RevenuePanel from "./RevenuePanel";
import enCommon from "@/i18n/messages/en/common.json";

const mockPush = jest.fn();
jest.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
  usePathname: () => "/business/42/dashboard",
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
}));

jest.mock("@/api/analytics", () => ({
  __esModule: true,
  analyticsApi: {
    getSalesAnalytics: jest.fn(),
    getLiveBills: jest.fn().mockResolvedValue([]),
    getDashboardSummary: jest.fn().mockResolvedValue(null),
    exportAnalyticsData: jest.fn(),
  },
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn().mockResolvedValue({ default_currency: "USD" }),
}));

jest.mock("@/hooks/useAnalyticsTimeseries", () => ({
  useAnalyticsTimeseries: () => ({
    series: {
      buckets: Array.from({ length: 8 }, (_, i) => ({
        date: `2026-05-${String(i + 1).padStart(2, "0")}`,
        revenue: 100,
        tips: 10,
        bills: 4,
        transactions: 5,
        average_ticket: 25,
      })),
      range: { from: "2026-05-01", to: "2026-05-08" },
    },
    loading: false,
    error: null,
  }),
}));

jest.mock("react-chartjs-2", () => ({
  Line: () => <div data-testid="line" />,
  Bar: () => <div data-testid="bar" />,
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    if (key.endsWith(".export")) return "Export";
    if (key.endsWith(".exportFailed")) return "Could not export sales data";
    if (key.endsWith(".loading")) return "Loading";
    if (key.endsWith(".subtitle")) return "Revenue";
    if (key.includes("periods.")) return key.split(".").pop() ?? key;
    if (key.includes("metrics.")) return key.split(".").pop() ?? key;
    return key;
  },
}));

const sales = {
  total_revenue: 300,
  total_tips: 30,
  transaction_count: 14,
  bill_count: 12,
  unique_customers: 9,
  average_ticket: 25,
  payment_methods: { crypto: 8, card: 6 },
  hourly_breakdown: { "12": { revenue: 120, bill_count: 4, transaction_count: 5 } },
};

describe("RevenuePanel L6-7 export 413 toast DOM (D1)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (analyticsApi.getSalesAnalytics as jest.Mock).mockResolvedValue(sales);
    (analyticsApi.exportAnalyticsData as jest.Mock).mockRejectedValue(
      Object.assign(new Error("request failed"), {
        status: 413,
        response: {
          status: 413,
          data: { error: "choose a period of 31 days or less" },
        },
      }),
    );
  });

  it("toasts rangeTooLarge when Export hits a 413", async () => {
    render(<RevenuePanel businessId="42" period="year" />);
    await waitFor(() => expect(screen.getByTestId("line")).toBeInTheDocument());

    fireEvent.click(screen.getByRole("button", { name: /export/i }));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalled();
    });
    const msg = (toast.error as jest.Mock).mock.calls[0][0] as string;
    expect(msg).toBe(enCommon.errors.rangeTooLarge);
    expect(msg.toLowerCase()).toMatch(/31 days|too large|period/);
  });
});
