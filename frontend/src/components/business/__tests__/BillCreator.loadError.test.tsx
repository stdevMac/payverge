/** @jest-environment jsdom */
/**
 * Wave 4 Task 17: regression-lock for BillCreator menu load-error retry panel.
 */
import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
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

beforeEach(() => {
  jest.clearAllMocks();
});

it("shows the retry panel, not the empty state, when the menu load fails", async () => {
  mockGetTablesWithStatus.mockRejectedValue(new Error("network"));
  mockGetMenu.mockRejectedValue(new Error("network"));
  mockGetOffers.mockRejectedValue(new Error("network"));
  mockGetBundles.mockRejectedValue(new Error("network"));
  mockGetSummary.mockResolvedValue(null);
  mockGetAvailableCounters.mockResolvedValue({ counters: [] });

  render(
    <BillCreator
      isOpen
      onClose={jest.fn()}
      businessId={1}
      onBillCreated={jest.fn()}
    />,
  );
  expect(
    await screen.findByTestId("bill-creator-load-error"),
  ).toBeInTheDocument();
  expect(screen.queryByText("billCreator.menu.noItemsAvailable")).toBeNull();

  // Retry re-runs the load
  mockGetTablesWithStatus.mockResolvedValue({ tables: [] });
  mockGetMenu.mockResolvedValue({ categories: [] });
  mockGetOffers.mockResolvedValue([]);
  mockGetBundles.mockResolvedValue([]);
  fireEvent.click(screen.getByRole("button", { name: "billCreator.buttons.retry" }));
  await waitFor(() =>
    expect(screen.queryByTestId("bill-creator-load-error")).toBeNull(),
  );
});
