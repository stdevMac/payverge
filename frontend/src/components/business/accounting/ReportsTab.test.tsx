/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import ReportsTab from "./ReportsTab";

const mockExportLocalizedCsv = jest.fn();
jest.mock("@/utils/exportLocalizedCsv", () => ({
  exportLocalizedCsv: (...args: unknown[]) => mockExportLocalizedCsv(...args),
}));

const mockUseProfitLoss = jest.fn();
jest.mock("@/hooks/accounting/useAccountingQueries", () => ({
  useProfitLoss: (...args: unknown[]) => mockUseProfitLoss(...args),
}));

jest.mock("./PeriodLockCard", () => ({
  __esModule: true,
  default: () => <div data-testid="period-lock" />,
}));

const t = (key: string) => key;

const pnlPayload = {
  current: {
    revenue: 1000,
    other_income: 50,
    cogs: 200,
    labor: 150,
    opex: 100,
    net: 600,
    currency: "USD",
  },
  previous: {
    revenue: 800,
    other_income: 40,
    cogs: 180,
    labor: 140,
    opex: 90,
    net: 430,
    currency: "USD",
  },
  delta: {},
};

describe("ReportsTab P&L CSV (L6-25)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUseProfitLoss.mockReturnValue({
      data: pnlPayload,
      isPending: false,
      isFetching: false,
      isError: false,
    });
  });

  it("includes previous and delta columns when compare is on", () => {
    render(
      <ReportsTab
        businessId="42"
        start="2026-05-01"
        end="2026-05-31"
        locale="en"
        currency="USD"
        t={t}
      />,
    );

    fireEvent.click(screen.getByTestId("pnl-export-csv"));

    expect(mockExportLocalizedCsv).toHaveBeenCalledTimes(1);
    const arg = mockExportLocalizedCsv.mock.calls[0][0] as {
      rows: Array<Record<string, unknown>>;
      columns: Array<{ key: string }>;
    };
    expect(arg.columns.map((c) => c.key)).toEqual(
      expect.arrayContaining(["section", "label", "current", "previous", "delta"]),
    );
    const revenue = arg.rows.find((r) => r.label === "reports.lines.revenue");
    expect(revenue).toBeTruthy();
    expect(revenue!.current).toBe("1000.00");
    expect(revenue!.previous).toBe("800.00");
    expect(revenue!.delta).toBe("200.00");
  });

  it("omits previous/delta values when compare is toggled off", () => {
    render(
      <ReportsTab
        businessId="42"
        start="2026-05-01"
        end="2026-05-31"
        locale="en"
        currency="USD"
        t={t}
      />,
    );

    // Default compare is true; uncheck
    const checkbox = screen.getByRole("checkbox");
    fireEvent.click(checkbox);

    // After uncheck, useProfitLoss is called with compare=false — keep same payload
    mockUseProfitLoss.mockReturnValue({
      data: pnlPayload,
      isPending: false,
      isFetching: false,
      isError: false,
    });

    fireEvent.click(screen.getByTestId("pnl-export-csv"));

    const arg = mockExportLocalizedCsv.mock.calls[
      mockExportLocalizedCsv.mock.calls.length - 1
    ][0] as { rows: Array<Record<string, unknown>> };
    const revenue = arg.rows.find((r) => r.label === "reports.lines.revenue");
    expect(revenue).toBeTruthy();
    expect(revenue!.current).toBe("1000.00");
    expect(revenue!.previous).toBe("");
    expect(revenue!.delta).toBe("");
  });
});
