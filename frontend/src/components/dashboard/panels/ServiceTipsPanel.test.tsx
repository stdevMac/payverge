/** @jest-environment jsdom */
import { render, screen, waitFor } from "@testing-library/react";
import ServiceTipsPanel, { tipperDisplayName } from "./ServiceTipsPanel";
import { analyticsApi } from "@/api/analytics";
import { getBusiness } from "@/api/business";

jest.mock("@/api/analytics", () => ({ __esModule: true, analyticsApi: { getTipAnalytics: jest.fn() } }));
jest.mock("@/api/business", () => ({ getBusiness: jest.fn().mockResolvedValue({ default_currency: "USD" }) }));
const mockUseTimeseries = jest.fn(
  (_businessId?: unknown, _opts?: { from?: string; to?: string }) => ({
    series: { buckets: [{ date: "2026-05-01", revenue: 0, tips: 5, bills: 1, transactions: 1, average_ticket: 0 }], range: { from: "2026-05-01", to: "2026-05-01" } },
    loading: false, error: null,
  }),
);
jest.mock("@/hooks/useAnalyticsTimeseries", () => ({
  useAnalyticsTimeseries: (businessId: unknown, opts?: { from?: string; to?: string }) =>
    mockUseTimeseries(businessId, opts),
}));
jest.mock("react-chartjs-2", () => ({ Line: () => <div data-testid="line" />, Bar: () => <div data-testid="bar" /> }));

describe("ServiceTipsPanel", () => {
  beforeEach(() => {
    mockUseTimeseries.mockClear();
    (getBusiness as jest.Mock).mockResolvedValue({ default_currency: "USD" });
    (analyticsApi.getTipAnalytics as jest.Mock).mockResolvedValue({
      total_tips: 120, tip_count: 30, average_tip: 4, average_tip_rate: 18,
      // Backend emits currency-neutral bucket keys.
      tip_distribution: { "0_5": 10, "5_10": 15, "10_20": 5 },
      top_tippers: [], hourly_tips: { "19": 40 },
      daily_comparison: { today: 50, yesterday: 40, change_percentage: 25 },
    });
  });

  it("renders the tip metrics, daily tip trend, and distribution bars", async () => {
    render(<ServiceTipsPanel businessId="42" />);
    await waitFor(() => expect(screen.getByTestId("line")).toBeInTheDocument());
    expect(screen.getByRole("list", { name: /distribution/i })).toBeInTheDocument();
  });

  it("labels distribution buckets in the business currency (USD shows $)", async () => {
    render(<ServiceTipsPanel businessId="42" />);
    const list = await screen.findByRole("list", { name: /distribution/i });
    // Currency-correct USD label for the 0_5 bucket: "$0.00–$5.00".
    expect(list.textContent).toContain("$0.00");
    expect(list.textContent).toContain("$5.00");
    // The neutral backend key must never leak to the UI.
    expect(list.textContent).not.toContain("0_5");
  });

  it("requests a 7-day trend window when the period is 'week' (audit M1)", async () => {
    render(<ServiceTipsPanel businessId="42" />);
    // Default period is "week"; the trend chart must scope its from/to window to
    // the selected period rather than always plotting a 30-day month.
    await waitFor(() => expect(mockUseTimeseries).toHaveBeenCalled());
    const call = mockUseTimeseries.mock.calls.find((c) => c[1]?.from);
    expect(call).toBeTruthy();
    const opts = call![1] as { from: string; to: string };
    const days =
      (new Date(opts.to).getTime() - new Date(opts.from).getTime()) / 864e5;
    // Window is [today-7d .. today-1d] → exactly 6 days between the date keys.
    // Exact pin catches both the old 30-day window and a shrink-to-1 regression.
    expect(days).toBe(6);
  });

  it("uses the business currency for bucket labels (AED shows no $ symbol)", async () => {
    (getBusiness as jest.Mock).mockResolvedValue({ default_currency: "AED" });
    render(<ServiceTipsPanel businessId="42" />);
    const list = await screen.findByRole("list", { name: /distribution/i });
    // Intl renders AED as "AED" (not "$"); assert the dollar sign is gone and
    // the currency code is present, proving labels follow the business currency.
    await waitFor(() => expect(list.textContent).toContain("AED"));
    expect(list.textContent).not.toContain("$");
    expect(list.textContent).not.toContain("0_5");
  });

  it("shows CRM guest names for top tippers and never raw wallet addresses", async () => {
    (analyticsApi.getTipAnalytics as jest.Mock).mockResolvedValue({
      total_tips: 40,
      tip_count: 2,
      average_tip: 20,
      average_tip_rate: 15,
      tip_distribution: { "10_20": 2 },
      top_tippers: [
        {
          payer_address: "0x999999999999999999999999999999999999a009",
          guest_name: "Demo Guest 01",
          total_tips: 30,
          tip_count: 2,
          average_tip: 15,
        },
        {
          payer_address: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa00a",
          total_tips: 10,
          tip_count: 1,
          average_tip: 10,
        },
      ],
      hourly_tips: {},
      daily_comparison: { today: 10, yesterday: 5, change_percentage: 100 },
    });
    render(<ServiceTipsPanel businessId="42" />);
    const table = await screen.findByTestId("top-tippers");
    expect(table).toHaveTextContent("Demo Guest 01");
    expect(table).toHaveTextContent("Guest");
    expect(table.textContent).not.toMatch(/0x9999/i);
    expect(table.textContent).not.toMatch(/0xaaaa/i);
  });
});

describe("tipperDisplayName", () => {
  it("never prints a hex guest_name as the label", () => {
    expect(
      tipperDisplayName(
        {
          guest_name: "0x999999999999999999999999999999999999a009",
          payer_address: "0x999999999999999999999999999999999999a009",
        },
        "Guest",
      ),
    ).toBe("Guest");
  });
});
