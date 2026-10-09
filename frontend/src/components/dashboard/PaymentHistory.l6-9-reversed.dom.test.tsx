/** @jest-environment jsdom */
/**
 * D1 / L6-9: Personalizado with start > end must paint a visible reversed-range
 * error in the DOM and must not call the list API with that inverted window.
 * Covers the PeriodTabs empty-selectedKey trap that cleared customActive.
 */
import React from "react";
import { render, screen, waitFor, fireEvent, act } from "@testing-library/react";
import PaymentHistory from "./PaymentHistory";
import { getPaymentHistoryPage } from "@/api/payments";
import { DATE_RANGE_DEBOUNCE_MS } from "./paymentHistoryDateCommit";

jest.mock("@/api/payments", () => ({
  __esModule: true,
  getPaymentHistoryPage: jest.fn(),
  exportPayments: jest.fn(),
}));
jest.mock("@/api/business", () => ({
  getBusiness: jest.fn().mockResolvedValue({ default_currency: "USD" }),
}));
jest.mock("@/api/bills", () => ({ __esModule: true, getBill: jest.fn() }));
jest.mock("@/components/business/BillDetailsModal", () => ({
  BillDetailsModal: () => null,
}));

jest.mock("@nextui-org/react", () => {
  const React = require("react");
  const { parseDate: mockParseDate } = require("@internationalized/date");
  const actual = jest.requireActual("@nextui-org/react");
  function MockDatePicker(props: any) {
    const name = props["aria-label"] || props.label || "date";
    const tid = props["data-testid"] || `date-${name}`;
    const str = props.value ? String(props.value.toString?.() ?? props.value) : "";
    return React.createElement("input", {
      "data-testid": tid,
      "aria-label": name,
      value: str,
      onChange: (e: any) => {
        const v = e.target.value;
        if (!v) props.onChange?.(null);
        else {
          try { props.onChange?.(mockParseDate(v)); }
          catch { props.onChange?.({ toString: () => v }); }
        }
      },
    });
  }
  return { ...actual, DatePicker: MockDatePicker };
});

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    if (key === "businessDashboard.dateRange.reversedRange")
      return "Start date must be on or before the end date.";
    if (key === "businessDashboard.dateRange.presets.custom") return "Custom";
    if (key === "businessDashboard.dateRange.startDate") return "Start date";
    if (key === "businessDashboard.dateRange.endDate") return "End date";
    if (key === "businessDashboard.dateRange.presetsAriaLabel") return "Period";
    if (key.endsWith(".error")) return "Error";
    return key;
  },
}));

const mockPage = getPaymentHistoryPage as jest.Mock;

describe("PaymentHistory L6-9 reversed custom range DOM (D1)", () => {
  beforeEach(() => {
    mockPage.mockReset();
    mockPage.mockResolvedValue({
      items: [], total: 0, page: 1, page_size: 20, available_methods: [],
    });
  });

  it("shows reversed-range copy and skips list fetch for inverted custom dates", async () => {
    render(<PaymentHistory businessId="48" currency="USD" period="month" />);
    await waitFor(() => expect(mockPage).toHaveBeenCalled());

    await act(async () => {
      fireEvent.click(screen.getByTestId("payment-history-custom-toggle"));
    });

    expect(screen.getByTestId("payment-history-custom-toggle")).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByTestId("payment-history-start-date")).toBeInTheDocument();
    expect(screen.getByTestId("payment-history-end-date")).toBeInTheDocument();

    await act(async () => {
      fireEvent.change(screen.getByTestId("payment-history-start-date"), {
        target: { value: "2026-08-10" },
      });
      fireEvent.change(screen.getByTestId("payment-history-end-date"), {
        target: { value: "2026-08-01" },
      });
      await new Promise((r) => setTimeout(r, DATE_RANGE_DEBOUNCE_MS + 80));
    });

    expect(
      screen.getByText("Start date must be on or before the end date."),
    ).toBeInTheDocument();

    const inverted = mockPage.mock.calls.filter((c) => {
      const q = c[1] as { startDate?: string; endDate?: string };
      return q?.startDate === "2026-08-10" && q?.endDate === "2026-08-01";
    });
    expect(inverted).toHaveLength(0);
  });
});
