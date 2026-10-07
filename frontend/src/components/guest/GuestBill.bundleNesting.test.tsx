/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import GuestBill from "./GuestBill";
import type { BillWithItemsResponse } from "@/api/bills";
import { asDollars } from "@/types/money";

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => key,
    currentLanguage: "en",
  }),
}));

jest.mock("@/contexts/CustomerAuthContext", () => ({
  useCustomerAuth: () => ({ customer: null, isAuthenticated: false }),
}));

jest.mock("@/api/orders", () => ({
  getGuestOrdersByBillNumber: jest.fn().mockResolvedValue({ orders: [], total: 0 }),
  guestCancelOrder: jest.fn(),
  parseOrderItems: () => [],
}));

jest.mock("@/api/bills", () => {
  const actual = jest.requireActual("@/api/bills");
  return {
    ...actual,
    getBillByNumber: jest.fn(),
    getOpenBillByTableCode: jest.fn(),
  };
});

jest.mock("../../api/currency", () => ({
  getGuestMenuTranslations: jest.fn().mockResolvedValue({
    translations: {
      menu_item: {
        "demo-steak": { name: "Plato de bistec" },
        "demo-cocktail": { name: "Spritz demo" },
        "demo-dessert": { name: "Tarta de chocolate" },
      },
      bundle: {
        "10": { name: "Cita romántica para dos" },
      },
    },
  }),
}));

jest.mock("./PaymentSection", () => () => null);
jest.mock("../splitting/GuestBillSplitPanel", () => () => null);
jest.mock("../common/CurrencyConverter", () => ({
  __esModule: true,
  default: () => null,
  CurrencyPrice: ({ amount }: { amount: number }) => <span>{amount}</span>,
}));

const bill = {
  bill: {
    id: 755,
    bill_number: "G75",
    public_token: "tok-755",
    status: "open",
    total_amount: asDollars(408),
    paid_amount: asDollars(0),
    subtotal: asDollars(408),
    tax_amount: asDollars(0),
    service_fee_amount: asDollars(0),
    created_at: new Date().toISOString(),
    settlement_address: "0xabc",
    tipping_address: "0xdef",
  },
  items: [
    {
      id: "b1",
      menu_item_id: "bundle:10",
      name: "Date Night for Two",
      price: asDollars(68),
      quantity: 1,
      options: [],
      subtotal: asDollars(68),
      item_type: "bundle" as const,
      bundle_id: 10,
      bundle_occurrence_id: "occ-1",
    },
    {
      id: "c1",
      menu_item_id: "demo-steak",
      name: "Steak Plate",
      price: asDollars(0),
      quantity: 1,
      options: [],
      subtotal: asDollars(0),
      item_type: "bundle_item" as const,
      parent_bundle_id: 10,
      bundle_occurrence_id: "occ-1",
    },
    {
      id: "c2",
      menu_item_id: "demo-cocktail",
      name: "Demo Spritz",
      price: asDollars(0),
      quantity: 2,
      options: [],
      subtotal: asDollars(0),
      item_type: "bundle_item" as const,
      parent_bundle_id: 10,
      bundle_occurrence_id: "occ-1",
    },
    {
      id: "b2",
      menu_item_id: "bundle:10",
      name: "Date Night for Two",
      price: asDollars(68),
      quantity: 1,
      options: [],
      subtotal: asDollars(68),
      item_type: "bundle" as const,
      bundle_id: 10,
      bundle_occurrence_id: "occ-2",
    },
    {
      id: "c3",
      menu_item_id: "demo-dessert",
      name: "Chocolate Tart",
      price: asDollars(0),
      quantity: 1,
      options: [],
      subtotal: asDollars(0),
      item_type: "bundle_item" as const,
      parent_bundle_id: 10,
      bundle_occurrence_id: "occ-2",
    },
  ],
} as unknown as BillWithItemsResponse;

const business = {
  id: 75,
  name: "Demo Lounge",
} as any;

describe("GuestBill bundle nesting (#75)", () => {
  it("nests each occurrence's own children once and labels included pricing", () => {
    render(
      <GuestBill
        bill={bill}
        business={business}
        tableCode="demo-75"
        onPaymentComplete={jest.fn()}
      />,
    );

    // Two parent rows, not a fan-out of every child under every parent.
    expect(screen.getAllByText("Date Night for Two")).toHaveLength(2);
    expect(screen.getAllByText(/Steak Plate/)).toHaveLength(1);
    expect(screen.getAllByText(/Demo Spritz/)).toHaveLength(1);
    expect(screen.getAllByText(/Chocolate Tart/)).toHaveLength(1);
    // $0.00 spam replaced with included label.
    expect(screen.getAllByText("bill.includedInBundle").length).toBeGreaterThanOrEqual(3);
  });

  it("translates nested bundle children, not only the parent (#445)", async () => {
    render(
      <GuestBill
        bill={bill}
        business={business}
        tableCode="demo-75"
        selectedLanguage="es"
        onPaymentComplete={jest.fn()}
      />,
    );

    await waitFor(() => {
      expect(screen.getAllByText("Cita romántica para dos")).toHaveLength(2);
    });
    expect(screen.getByText(/Plato de bistec/)).toBeInTheDocument();
    expect(screen.getByText(/Spritz demo/)).toBeInTheDocument();
    expect(screen.getByText(/Tarta de chocolate/)).toBeInTheDocument();
    expect(screen.queryByText("Steak Plate")).not.toBeInTheDocument();
    expect(screen.queryByText("Demo Spritz")).not.toBeInTheDocument();
    expect(screen.queryByText("Chocolate Tart")).not.toBeInTheDocument();
    expect(screen.queryByText("Date Night for Two")).not.toBeInTheDocument();
  });
});
