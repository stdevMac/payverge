/** @jest-environment jsdom */
/**
 * Wave 4 Task 18: tips-by-staff rollup panel on the Team tab.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/analytics", () => ({
  __esModule: true,
  analyticsApi: {
    getTipsByStaff: jest.fn(() =>
      Promise.resolve([
        { staff_id: 2, staff_name: "Ben", total_tips: 41.5, bill_count: 12 },
        { staff_id: null, staff_name: "", total_tips: 8, bill_count: 3 },
      ]),
    ),
  },
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (n: number) => `$${n.toFixed(2)}`,
}));

import TipsByStaffPanel from "./TipsByStaffPanel";

it("renders the rollup with an unattributed bucket", async () => {
  render(<TipsByStaffPanel businessId="1" currency="USD" />);
  await waitFor(() => expect(screen.getByText("Ben")).toBeInTheDocument());
  expect(
    screen.getByText(
      "businessDashboard.dashboard.staffManagement.tips.unattributed",
    ),
  ).toBeInTheDocument();
});
