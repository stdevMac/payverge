/**
 * Issue #561 — paid-bill receipt tax/service-fee sublabels share the diner
 * locale decimal convention (de: 8,875% / 10,5%), not raw JS interpolation.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen } from "@testing-library/react";

import ReceiptGenerator from "@/components/receipt/ReceiptGenerator";

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    currentLanguage: "de",
    t: (key: string) => key,
  }),
}));

jest.mock("html2canvas", () => ({ __esModule: true, default: jest.fn() }));
jest.mock("jspdf", () => ({ __esModule: true, default: jest.fn() }));

const data = {
  business: { name: "Cafe", tax_rate: 8.875, service_fee_rate: 10.5 },
  bill: {
    bill_number: "B-1",
    created_at: "2026-06-09T00:00:00Z",
    subtotal: 30,
    tax_amount: 2.66,
    service_fee_amount: 2.33,
    total_amount: 34.99,
  },
  table: { name: "T1" },
  items: [{ name: "Tacos", quantity: 1, subtotal: 30 }],
};

describe("ReceiptGenerator locale decimals (#561)", () => {
  it("formats de tax/service rates with comma decimals", () => {
    render(<ReceiptGenerator data={data as any} currency="USD" />);
    expect(screen.getByText(/8,875/)).toBeInTheDocument();
    expect(screen.getByText(/10,5/)).toBeInTheDocument();
    expect(screen.queryByText(/8\.875/)).not.toBeInTheDocument();
    expect(screen.queryByText(/10\.5/)).not.toBeInTheDocument();
  });
});
