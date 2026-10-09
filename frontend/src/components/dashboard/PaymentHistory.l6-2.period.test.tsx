/** @jest-environment jsdom */
/**
 * D1 / L6-2: when the dashboard shares period=yesterday, PaymentHistory must
 * request that period on the wire — not fall back to local "month".
 * Source greps of Dashboard.tsx alone are pass-on-revert; this asserts the
 * shipped getPaymentHistoryPage query.
 */
import React from "react";
import { render, waitFor } from "@testing-library/react";
import PaymentHistory from "./PaymentHistory";
import { getPaymentHistoryPage } from "@/api/payments";

jest.mock("@/api/payments", () => ({
  __esModule: true,
  getPaymentHistoryPage: jest.fn(),
  exportPayments: jest.fn(),
}));
jest.mock("@/api/business", () => ({
  getBusiness: jest.fn().mockResolvedValue({ default_currency: "USD" }),
}));
jest.mock("@/api/bills", () => ({
  __esModule: true,
  getBill: jest.fn(),
}));
jest.mock("@/components/business/BillDetailsModal", () => ({
  BillDetailsModal: () => null,
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

const mockPage = getPaymentHistoryPage as jest.Mock;

describe("PaymentHistory L6-2 period=yesterday on the wire (D1)", () => {
  beforeEach(() => {
    mockPage.mockReset();
    mockPage.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      available_methods: [],
    });
  });

  it("forwards shared period yesterday into getPaymentHistoryPage", async () => {
    render(
      <PaymentHistory
        businessId="48"
        currency="USD"
        period="yesterday"
        onPeriodChange={jest.fn()}
      />,
    );

    await waitFor(() => expect(mockPage).toHaveBeenCalled());
    const calls = mockPage.mock.calls as unknown[][];
    // Every fetch for this mount must carry period=yesterday (never month).
    for (const call of calls) {
      const query = call[1] as { period?: string };
      expect(query.period).toBe("yesterday");
      expect(query.period).not.toBe("month");
    }
  });

  it("defaults to all-time when no shared period is provided", async () => {
    render(<PaymentHistory businessId="48" currency="USD" />);
    await waitFor(() => expect(mockPage).toHaveBeenCalled());
    const query = mockPage.mock.calls[0][1] as { period?: string };
    expect(query.period).toBeUndefined();
  });
});
