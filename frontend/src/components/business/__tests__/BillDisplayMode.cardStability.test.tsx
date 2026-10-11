/** @jest-environment jsdom */
/**
 * Finding 3 regression: BillDisplayMode's 1s clock must NOT re-render the whole
 * kanban card-by-card. The card is module-scope + React.memo keyed on
 * minute-granularity `ageMinutes`, so a sub-minute tick keeps the exact same
 * card DOM node (a remount would produce a new node).
 */
import React from "react";
import { render, screen, act } from "@testing-library/react";

jest.mock("@/api/bills", () => ({
  ...jest.requireActual("@/api/bills"),
  getBill: jest.fn().mockResolvedValue(null),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));

jest.mock("@/components/business/BillItemEditor", () => ({
  BillItemEditor: () => null,
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (value: number) => `$${value}`,
}));

import BillDisplayMode from "@/components/business/BillDisplayMode";
import type { Bill } from "@/api/bills";

const openBill: Bill = {
  id: 1,
  business_id: 42,
  table_id: 7,
  bill_number: "B-STABLE",
  notes: "",
  items: "[]",
  subtotal: 10,
  tax_amount: 0,
  service_fee_amount: 0,
  total_amount: 10,
  paid_amount: 0,
  tip_amount: 0,
  currency: "USD",
  status: "open",
  settlement_address: "0x",
  tipping_address: "0x",
  // 5 minutes old so the elapsed chip is stable across a 1.1s tick.
  created_at: new Date(Date.now() - 5 * 60_000).toISOString(),
  updated_at: new Date().toISOString(),
  physical_item_quantity: 1,
} as Bill;

beforeEach(() => {
  jest.useFakeTimers();
});
afterEach(() => {
  jest.useRealTimers();
});

it("keeps the same card DOM node across a 1s clock tick", () => {
  render(
    <BillDisplayMode
      bills={[openBill]}
      orders={{}}
      onExit={jest.fn()}
      onCloseBill={jest.fn()}
      onCreateBill={jest.fn()}
      onBillUpdated={jest.fn()}
      businessId={42}
      tString={(key: string) => key}
      currency="USD"
    />,
  );

  const heading = screen.getByText("#B-STABLE");

  act(() => {
    jest.advanceTimersByTime(1100);
  });

  expect(document.body.contains(heading)).toBe(true);
  expect(screen.getByText("#B-STABLE")).toBe(heading);
});
