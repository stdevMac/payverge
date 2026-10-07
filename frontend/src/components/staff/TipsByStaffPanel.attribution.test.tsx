/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { getTranslation } from "@/i18n/getTranslation";
import TipsByStaffPanel from "./TipsByStaffPanel";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en" }),
    getTranslation: actual.getTranslation,
  };
});

jest.mock("@/api/analytics", () => ({
  __esModule: true,
  analyticsApi: {
    getTipsByStaff: jest.fn(() =>
      Promise.resolve([
        { staff_id: 2, staff_name: "Ben", total_tips: 41.5, bill_count: 12 },
      ]),
    ),
  },
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (n: number) => `$${n.toFixed(2)}`,
}));

it("renders the live opener-attribution subtitle, not the staff roster heading", async () => {
  const expected = getTranslation(
    "businessDashboard.dashboard.staffManagement.tips.subtitle",
    "en",
  );
  expect(typeof expected).toBe("string");
  expect(expected).toMatch(/opened the check/i);
  expect(expected).toMatch(/Kitchen closers are not credited/i);

  render(<TipsByStaffPanel businessId="1" currency="USD" />);

  const subtitle = await screen.findByTestId("tips-by-staff-subtitle");
  expect(subtitle).toHaveTextContent(String(expected));
  await waitFor(() => expect(screen.getByText("Ben")).toBeInTheDocument());
  expect(screen.getByTestId("tips-by-staff-panel")).toBeInTheDocument();
});
