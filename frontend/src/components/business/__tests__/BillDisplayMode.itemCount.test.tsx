/** @jest-environment jsdom */
// Operator cards show physical preparation work, not financial row count.
// Prefer the server aggregate and use the shared fulfillment projector only
// for legacy snapshots.
import React from "react";
import { render, screen } from "@testing-library/react";

import BillDisplayMode from "@/components/business/BillDisplayMode";
import type { Bill } from "@/api/bills";

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

jest.mock("@nextui-org/react", () => ({
  Button: ({ children }: { children?: React.ReactNode }) => (
    <button type="button">{children}</button>
  ),
  Chip: ({ children }: { children?: React.ReactNode }) => (
    <span>{children}</span>
  ),
  Divider: () => <hr />,
  Input: () => <input />,
  Spinner: () => <div>spinner</div>,
}));

const baseBill = (overrides: Partial<Bill>): Bill =>
  ({
    id: 1,
    business_id: 42,
    table_id: 7,
    bill_number: "B-1",
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
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  }) as Bill;

const renderDisplay = (bills: Bill[]) =>
  render(
    <BillDisplayMode
      bills={bills}
      orders={{}}
      onExit={jest.fn()}
      onCloseBill={jest.fn()}
      onCreateBill={jest.fn()}
      onBillUpdated={jest.fn()}
      businessId={42}
      tString={(key: string) => {
        if (key === "display.itemCountCompact") return "{count} items";
        // L8-1: correct plural forms (tests previously asserted always-plural).
        if (key === "display.preparedUnitsOne") return "{count} item";
        if (key === "display.preparedUnitsOther") return "{count} items";
        if (key === "display.preparedUnits") return "{count} items";
        return key;
      }}
      currency="USD"
    />,
  );

describe("BillDisplayMode physical quantity", () => {
  it("prefers the server-projected physical quantity over the empty JSON snapshot", () => {
    renderDisplay([
      baseBill({
        id: 1,
        bill_number: "B-modern",
        items: "[]",
        item_count: 5,
        physical_item_quantity: 2,
      }),
    ]);
    expect(screen.getAllByText("2 items").length).toBeGreaterThan(0);
    expect(screen.queryByText("5 items")).toBeNull();
  });

  it("falls back to the JSON snapshot for legacy bills without bill_items", () => {
    renderDisplay([
      baseBill({
        id: 2,
        bill_number: "B-legacy",
        items: '[{"name":"Old","quantity":1}]',
        item_count: 0,
      }),
    ]);
    // L8-1: count=1 must be singular "item", not the old buggy always-plural.
    expect(screen.getAllByText("1 item").length).toBeGreaterThan(0);
  });

  it("uses physical quantity rather than financial row count", () => {
    renderDisplay([
      baseBill({
        id: 3,
        bill_number: "B-physical",
        physical_item_quantity: 3,
        item_count: 5,
      }),
    ]);
    expect(screen.getAllByText("3 items").length).toBeGreaterThan(0);
    expect(screen.queryByText("5 items")).toBeNull();
  });
});
