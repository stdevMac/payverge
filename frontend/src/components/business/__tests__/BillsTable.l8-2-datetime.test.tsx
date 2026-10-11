/**
 * D1 / L8-2 / N-5: BillsTable timestamps must use DATE_TIME_SHORT so the
 * year is always present (audit: "8 jul, 20:53" without year).
 *
 * Asserts **rendered DOM**, not a source grep of DATE_TIME_SHORT.
 * Revert production to ad-hoc options without year → this suite goes red.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { BillsTable } from "../BillsTable";
import type { Bill } from "../../../api/bills";
import { asDollars } from "../../../types/money";
import { formatBusinessDateTime, DATE_TIME_SHORT } from "@/utils/businessTime";

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({
    isWeb3User: false,
    isStaffUser: false,
    isOAuthUser: false,
    oauthData: null,
    staffData: null,
    isLoading: false,
    isInitialized: true,
  }),
}));

const makeBill = (overrides: Partial<Bill> = {}): Bill => ({
  id: 1,
  business_id: 7,
  table_id: 3,
  bill_number: "B-0001",
  notes: "",
  items: "[]",
  subtotal: asDollars(20),
  tax_amount: asDollars(0),
  service_fee_amount: asDollars(0),
  total_amount: asDollars(20.5),
  paid_amount: asDollars(0),
  tip_amount: asDollars(0),
  currency: "USD",
  status: "open",
  settlement_address: "",
  tipping_address: "",
  created_at: "2026-07-08T20:53:00.000Z",
  updated_at: "2026-07-08T20:53:00.000Z",
  ...overrides,
});

const tString = (key: string) => key;

describe("L8-2 BillsTable DATE_TIME_SHORT in DOM", () => {
  it("renders the year 2026 for the audit instant (not bare day/month)", () => {
    const expected = formatBusinessDateTime(
      "2026-07-08T20:53:00.000Z",
      "en",
      "UTC",
      DATE_TIME_SHORT,
    );
    expect(expected).toMatch(/2026/);

    render(
      <BillsTable
        bills={[makeBill()]}
        isActive
        actionLoading={null}
        onViewBill={jest.fn()}
        onCloseBill={jest.fn()}
        onCreateBill={jest.fn()}
        onResetFilters={jest.fn()}
        tString={tString}
        locale="en"
        businessTimezone="UTC"
      />,
    );

    // Full formatted string must appear in the table (year required).
    expect(screen.getByText(expected)).toBeInTheDocument();
    expect(screen.getByText(expected).textContent).toMatch(/2026/);
  });

  it("es locale still includes year under DATE_TIME_SHORT", () => {
    const expected = formatBusinessDateTime(
      "2026-07-08T20:53:00.000Z",
      "es",
      "UTC",
      DATE_TIME_SHORT,
    );
    expect(expected).toMatch(/2026/);

    render(
      <BillsTable
        bills={[makeBill()]}
        isActive
        actionLoading={null}
        onViewBill={jest.fn()}
        onCloseBill={jest.fn()}
        onCreateBill={jest.fn()}
        onResetFilters={jest.fn()}
        tString={tString}
        locale="es"
        businessTimezone="UTC"
      />,
    );

    expect(screen.getByText(expected)).toBeInTheDocument();
  });
});
