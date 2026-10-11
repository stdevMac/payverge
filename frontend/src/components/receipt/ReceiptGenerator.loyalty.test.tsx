/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";

import ReceiptGenerator from "@/components/receipt/ReceiptGenerator";

// Audit B-01: the downloadable receipt must show the loyalty-discount line that
// the backend bakes into total_amount, otherwise subtotal+tax+service_fee won't
// reconcile with the printed "Total paid" when points were redeemed.

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t: (key: string) => key }),
}));

// Heavy PDF deps are only used on download; stub so the render stays light.
jest.mock("html2canvas", () => ({ __esModule: true, default: jest.fn() }));
jest.mock("jspdf", () => ({ __esModule: true, default: jest.fn() }));

const baseData = (loyaltyDiscount?: number) => ({
  business: { name: "Cafe", tax_rate: 5, service_fee_rate: 7.5 },
  bill: {
    bill_number: "B-1",
    created_at: "2026-06-09T00:00:00Z",
    subtotal: 378,
    tax_amount: 18.9,
    service_fee_amount: 28.35,
    loyalty_discount: loyaltyDiscount,
    total_amount: loyaltyDiscount ? 425.25 - loyaltyDiscount : 425.25,
  },
  table: { name: "Terrace 1" },
  items: [{ name: "VIP Tasting Bundle", quantity: 1, subtotal: 420 }],
});

describe("ReceiptGenerator loyalty discount line", () => {
  it("renders a negative loyalty-discount line when points were redeemed", () => {
    render(<ReceiptGenerator data={baseData(40) as any} currency="USD" />);
    const label = screen.getByText("bill.loyaltyDiscount:");
    expect(label).toBeInTheDocument();
    const row = label.parentElement;
    expect(row?.textContent ?? "").toMatch(/-/);
    expect(row?.textContent ?? "").toMatch(/40/);
  });

  it("omits the loyalty line when there is no redemption", () => {
    render(<ReceiptGenerator data={baseData(0) as any} currency="USD" />);
    expect(screen.queryByText("bill.loyaltyDiscount:")).toBeNull();
  });

  it("formats bill created_at in the venue timezone, not the device timezone", () => {
    // 2026-07-23T01:14:46Z → 9:14 PM Jul 22 America/New_York (not 5:14 AM Dubai).
    const data = {
      ...baseData(),
      business: {
        name: "Cafe",
        tax_rate: 5,
        service_fee_rate: 7.5,
        timezone: "America/New_York",
      },
      bill: {
        ...baseData().bill,
        created_at: "2026-07-23T01:14:46.475623Z",
      },
    };
    render(<ReceiptGenerator data={data as any} currency="USD" />);
    expect(
      screen.getByText(/Jul 22, 2026,\s*(9:14\s*PM|21:14)/i),
    ).toBeInTheDocument();
    expect(screen.queryByText(/5:14\s*AM/i)).not.toBeInTheDocument();
  });
});
