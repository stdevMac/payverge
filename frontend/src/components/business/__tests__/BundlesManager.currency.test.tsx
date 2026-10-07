/** @jest-environment jsdom */
import React from "react";
import { render as rtlRender, screen, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

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
const mockCreateBundle = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBundles: (...a: unknown[]) => mockGetBundles(...a),
    getMenu: (...a: unknown[]) => mockGetMenu(...a),
    getBusiness: (...a: unknown[]) => mockGetBusiness(...a),
    createBundle: (...a: unknown[]) => mockCreateBundle(...a),
    updateBundle: jest.fn(),
    deleteBundle: jest.fn(),
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

jest.mock("@/hooks/useSharedMenu", () => ({
  useSharedMenu: () => ({
    data: [
      {
        id: "c1",
        name: "Mains",
        items: [{ id: "burger", name: "Burger", price: 20, is_available: true }],
      },
    ],
  }),
}));

import BundlesManager from "../BundlesManager";

const NS = "businessDashboard.dashboard.menuBuilder.bundlesManager";

beforeEach(() => {
  jest.clearAllMocks();
  mockGetBundles.mockResolvedValue([]);
  mockGetMenu.mockResolvedValue({ categories: [] });
  mockGetBusiness.mockResolvedValue({ default_currency: "ARS" });
  mockCreateBundle.mockResolvedValue({ id: 1 });
});

it("has no multi-currency picker; price is denominated in the business default", async () => {
  render(<BundlesManager businessId={1} />);

  const createButtons = await screen.findAllByText(`${NS}.createButton`);
  fireEvent.click(createButtons[0]);

  await screen.findByTestId("bundle-price-input");
  expect(
    screen.queryByLabelText(`${NS}.form.currencyLabel`),
  ).not.toBeInTheDocument();
  expect(screen.queryByText("EUR")).not.toBeInTheDocument();
  expect(screen.queryByText("USD")).not.toBeInTheDocument();
});
