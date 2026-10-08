/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { BillCreator } from "../BillCreator";

const mockCreateBill = jest.fn();
const mockCreateOrder = jest.fn();
const mockGetTablesWithStatus = jest.fn();
const mockGetMenu = jest.fn();
const mockGetOffers = jest.fn();
const mockGetBundles = jest.fn();
const mockGetAvailableCounters = jest.fn();
const mockGetSummary = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: {
    success: jest.fn(),
    error: jest.fn(),
  },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/bills", () => ({
  createBill: (...a: unknown[]) => mockCreateBill(...a),
  isActiveBillStatus: (status: string | undefined) =>
    status === "open" || status === "partial",
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getOffers: (...a: unknown[]) => mockGetOffers(...a),
    getBundles: (...a: unknown[]) => mockGetBundles(...a),
  },
  getTablesWithStatus: (...a: unknown[]) => mockGetTablesWithStatus(...a),
  getMenu: (...a: unknown[]) => mockGetMenu(...a),
}));

jest.mock("@/api/orders", () => ({
  createOrder: (...a: unknown[]) => mockCreateOrder(...a),
}));

jest.mock("@/hooks/useOrderQuote", () => ({
  useOrderQuote: () => ({
    quote: {
      subtotal: 10,
      discount: 0,
      net_subtotal: 10,
      tax: 0,
      service_fee: 0,
      tip: 0,
      total: 10,
      lines: [{ key: "m1", unit_price: 10, quantity: 1, subtotal: 10 }],
    },
    isPending: false,
    error: null,
    isValid: true,
    refresh: jest.fn(),
  }),
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (amount: number) => `$${(amount || 0).toFixed(2)}`,
}));

jest.mock("@/api/counters", () => ({
  getAvailableCounters: (...a: unknown[]) => mockGetAvailableCounters(...a),
}));

jest.mock("@/api/inventory", () => ({
  inventoryApi: {
    getSummary: (...a: unknown[]) => mockGetSummary(...a),
  },
}));

jest.mock("../ItemCustomizer", () => ({
  ItemCustomizer: () => null,
}));

function seedHappyPath() {
  mockGetTablesWithStatus.mockResolvedValue({
    tables: [
      {
        table: { id: 1, name: "Table 1", is_active: true },
        status: "available",
        active_bills: [],
        active_bills_count: 0,
        active_bill_physical_item_quantity: 0,
        reservations: [],
        reservations_count: 0,
      },
    ],
  });
  mockGetMenu.mockResolvedValue({
    categories: [
      {
        name: "Mains",
        items: [
          {
            id: "m1",
            name: "Soup",
            description: "",
            price: 10,
            is_available: true,
          },
        ],
      },
    ],
  });
  mockGetOffers.mockResolvedValue([]);
  mockGetBundles.mockResolvedValue([]);
  mockGetAvailableCounters.mockResolvedValue({ counters: [] });
  mockGetSummary.mockResolvedValue(null);
  mockCreateBill.mockResolvedValue({ bill: { id: 77 } });
  mockCreateOrder.mockResolvedValue({});
}

beforeEach(() => {
  jest.clearAllMocks();
});

it("shows a success step with View bill after creating", async () => {
  seedHappyPath();
  const onViewBill = jest.fn();
  render(
    <BillCreator
      isOpen
      onClose={jest.fn()}
      businessId={1}
      onBillCreated={jest.fn()}
      onViewBill={onViewBill}
      initialTableId={1}
    />,
  );
  fireEvent.click(await screen.findByText("Soup"));
  const createBtn = await screen.findByText(/billCreator\.buttons\.createBill/);
  fireEvent.click(createBtn);
  const success = await screen.findByTestId("bill-creator-success");
  expect(success).toBeInTheDocument();
  fireEvent.click(
    screen.getByRole("button", { name: "billCreator.success.viewBill" }),
  );
  expect(onViewBill).toHaveBeenCalledWith(77);
});
