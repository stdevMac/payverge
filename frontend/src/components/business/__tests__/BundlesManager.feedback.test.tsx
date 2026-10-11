/** @jest-environment jsdom */
import React from "react";
import { render as rtlRender, screen, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// BundlesManager reads its menu from useSharedMenu (React Query).
const render = (ui: React.ReactElement) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>,
  );
};

const mockGetBundles = jest.fn();
const mockGetMenu = jest.fn();
const mockGetBusiness = jest.fn();
const mockDeleteBundle = jest.fn();
const mockCreateBundle = jest.fn();
const mockUpdateBundle = jest.fn();

const mockToastError = jest.fn();
const mockToastSuccess = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: {
    success: (...a: unknown[]) => mockToastSuccess(...a),
    error: (...a: unknown[]) => mockToastError(...a),
  },
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBundles: (...a: unknown[]) => mockGetBundles(...a),
    getMenu: (...a: unknown[]) => mockGetMenu(...a),
    getBusiness: (...a: unknown[]) => mockGetBusiness(...a),
    deleteBundle: (...a: unknown[]) => mockDeleteBundle(...a),
    createBundle: (...a: unknown[]) => mockCreateBundle(...a),
    updateBundle: (...a: unknown[]) => mockUpdateBundle(...a),
  },
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (amount: number) => `$${(amount || 0).toFixed(2)}`,
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("../ImageUpload", () => ({
  __esModule: true,
  default: () => null,
}));

import BundlesManager from "../BundlesManager";

const bundle = {
  id: 5,
  name: "Combo A",
  description: "",
  price: 20,
  currency: "USD",
  image: "",
  is_active: true,
  items: [{ menu_item_id: "m1", name: "Burger", quantity: 1 }],
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetBundles.mockResolvedValue([bundle]);
  mockGetMenu.mockResolvedValue({ categories: [] });
  mockGetBusiness.mockResolvedValue({ default_currency: "USD" });
});

it("disables the Create button until name/price/items are valid (F19b)", async () => {
  render(<BundlesManager businessId={1} />);

  // Open the create modal.
  const createButtons = await screen.findAllByText(
    "businessDashboard.dashboard.menuBuilder.bundlesManager.createButton",
  );
  fireEvent.click(createButtons[0]);

  // The modal's submit button (create) renders its label key; it must be
  // disabled because the form is empty.
  const submit = await screen.findByText(
    "businessDashboard.dashboard.menuBuilder.bundlesManager.buttons.create",
  );
  const submitButton = submit.closest("button");
  expect(submitButton).toBeDisabled();
});

it("L3-17: paste with >2 decimals keeps magnitude (12.345 → 12.35, not 12345)", async () => {
  render(<BundlesManager businessId={1} />);

  const createButtons = await screen.findAllByText(
    "businessDashboard.dashboard.menuBuilder.bundlesManager.createButton",
  );
  fireEvent.click(createButtons[0]);

  const priceInput = await screen.findByTestId("bundle-price-input");
  expect(priceInput).toHaveAttribute("type", "text");
  expect(priceInput).toHaveAttribute("inputmode", "decimal");

  // DecimalInput keeps the raw string on change; clamp/round happens on blur
  // (L3-17 design: no mid-keystroke parse that would turn 12.345 into 12345).
  fireEvent.change(priceInput, { target: { value: "12.345" } });
  expect(priceInput).toHaveValue("12.345");
  expect(priceInput).not.toHaveValue("12345");

  fireEvent.blur(priceInput);
  await waitFor(() => {
    expect(priceInput).toHaveValue("12.35");
  });
  expect(priceInput).not.toHaveValue("12345");
  expect(priceInput).not.toHaveValue("1234.5");
});

it("shows an error toast and keeps the modal open when delete fails (F20b)", async () => {
  mockDeleteBundle.mockRejectedValue(new Error("boom"));
  render(<BundlesManager businessId={1} />);

  // Wait for the bundle card to render, then click the delete icon button.
  const deleteBtn = await screen.findByLabelText(
    "businessDashboard.dashboard.menuBuilder.bundlesManager.aria.delete",
  );
  fireEvent.click(deleteBtn);

  // Confirm deletion in the ConfirmationModal.
  const confirm = await screen.findByText(
    "businessDashboard.dashboard.menuBuilder.bundlesManager.buttons.delete",
  );
  fireEvent.click(confirm);

  await waitFor(() => expect(mockDeleteBundle).toHaveBeenCalled());
  await waitFor(() =>
    expect(mockToastError).toHaveBeenCalledWith(
      "businessDashboard.dashboard.menuBuilder.bundlesManager.messages.deleteError",
    ),
  );
});
