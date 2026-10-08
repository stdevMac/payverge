/**
 * D1 / L1-29 / N-7: BillDisplayMode must render multi-day ages via the single
 * humanizeDuration path (e.g. "18d ago"), never raw hour walls like "450h".
 *
 * Both the list card Chip and the detail panel Chip call formatElapsedLabel /
 * getElapsedLabel — this test mounts the real component so a second unbounded
 * hours path cannot hide behind a pure-helper unit test.
 *
 * Revert-proof: if formatElapsedLabel returns `${Math.floor(m/60)}h …`, the
 * card DOM shows "450h" and this fails.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import BillDisplayMode from "@/components/business/BillDisplayMode";
import type { Bill } from "@/api/bills";

const mockNow = new Date("2026-08-06T12:00:00.000Z");
// 18 days earlier → humanize as 18d; raw hours would be ~432h.
const mockCreatedAt = "2026-07-19T12:00:00.000Z";

jest.mock("@/api/bills", () => ({
  ...jest.requireActual("@/api/bills"),
  getBill: jest.fn().mockImplementation(async (_biz: number, billId: number) => ({
    bill: {
      id: billId,
      business_id: 42,
      table_id: 7,
      bill_number: "B-AGED",
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
      created_at: mockCreatedAt,
      updated_at: mockCreatedAt,
    },
    items: [],
  })),
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
  Button: ({
    children,
    onPress,
    ...rest
  }: {
    children?: React.ReactNode;
    onPress?: () => void;
  }) => (
    <button type="button" onClick={onPress} {...rest}>
      {children}
    </button>
  ),
  Chip: ({ children }: { children?: React.ReactNode }) => (
    <span data-testid="elapsed-chip">{children}</span>
  ),
  Divider: () => <hr />,
  Input: () => <input />,
  Spinner: () => <div>spinner</div>,
}));

const agedOpenBill = {
  id: 99,
  business_id: 42,
  table_id: 7,
  bill_number: "B-AGED",
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
  created_at: mockCreatedAt,
  updated_at: mockCreatedAt,
  physical_item_quantity: 1,
} as Bill;

const tString = (key: string): string => {
  if (key === "display.justNow") return "Just now";
  if (key === "display.elapsedAgo") return "{duration} ago";
  if (key === "display.preparedUnitsOne") return "{count} prepared unit";
  if (key === "display.preparedUnitsOther") return "{count} prepared units";
  if (key === "display.preparedUnits") return "{count} prepared units";
  if (key === "display.itemCountCompact") return "{count} items";
  if (key === "billStatuses.open") return "Open";
  if (key === "bills.copyBillNumber") return "Copy";
  if (key === "bills.copiedBillNumber") return "Copied";
  return key;
};

describe("BillDisplayMode L1-29 / N-7 elapsed DOM", () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.setSystemTime(mockNow);
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it("card chip shows 18d ago for an 18-day-old open bill, never 450h / 432h walls", () => {
    render(
      <BillDisplayMode
        bills={[agedOpenBill]}
        orders={{}}
        onExit={jest.fn()}
        onCloseBill={jest.fn()}
        onCreateBill={jest.fn()}
        onBillUpdated={jest.fn()}
        businessId={42}
        tString={tString}
        currency="USD"
      />,
    );

    const chips = screen.getAllByTestId("elapsed-chip");
    const ageChips = chips.filter((c) => /ago|Just now/i.test(c.textContent || ""));
    expect(ageChips.length).toBeGreaterThanOrEqual(1);
    for (const chip of ageChips) {
      expect(chip.textContent).toMatch(/18d ago/);
      expect(chip.textContent).not.toMatch(/450/);
      expect(chip.textContent).not.toMatch(/432/);
      expect(chip.textContent).not.toMatch(/\d{3,}h/);
    }
  });

  it("detail panel elapsed uses the same humanized path after selecting the card", async () => {
    const user = userEvent.setup({ advanceTimers: jest.advanceTimersByTime });
    render(
      <BillDisplayMode
        bills={[agedOpenBill]}
        orders={{}}
        onExit={jest.fn()}
        onCloseBill={jest.fn()}
        onCreateBill={jest.fn()}
        onBillUpdated={jest.fn()}
        businessId={42}
        tString={tString}
        currency="USD"
      />,
    );

    // Click the bill card to open the detail panel (loads via getBill).
    await user.click(screen.getByText(/B-AGED|99/i).closest("button") || screen.getByRole("button", { name: /B-AGED/i }));

    await waitFor(() => {
      const chips = screen.getAllByTestId("elapsed-chip");
      const ages = chips.map((c) => c.textContent || "").filter((t) => /ago/.test(t));
      expect(ages.length).toBeGreaterThanOrEqual(1);
      for (const label of ages) {
        expect(label).toMatch(/18d ago/);
        expect(label).not.toMatch(/\d{3,}h/);
      }
    });
  });

  it("formatElapsedLabel is the only multi-day path in BillDisplayMode source", () => {
    const fs = require("fs") as typeof import("fs");
    const path = require("path") as typeof import("path");
    const src = fs.readFileSync(
      path.join(process.cwd(), "src/components/business/BillDisplayMode.tsx"),
      "utf8",
    );
    // Both call sites must go through formatElapsedLabel / getElapsedLabel.
    expect(src).toMatch(/formatElapsedLabel\(entry\.ageMinutes/);
    expect(src).toMatch(/getElapsedLabel\(selectedBillLive\.created_at\)/);
    // No second raw hours builder (historical dual implementation).
    expect(src).not.toMatch(/Math\.floor\(\s*ageMinutes\s*\/\s*60\s*\)/);
    expect(src).not.toMatch(/hoursMinutesAgo/);
    const humanizeCount = (src.match(/humanizeDurationMinutes/g) || []).length;
    expect(humanizeCount).toBeGreaterThanOrEqual(1);
  });
});
