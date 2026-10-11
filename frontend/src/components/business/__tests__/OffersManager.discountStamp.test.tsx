/** @jest-environment jsdom */
import React from "react";
import { render as rtlRender, screen } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// OffersManager reads its menu from useSharedMenu (React Query).
const render = (ui: React.ReactElement) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>,
  );
};

const mockGetOffers = jest.fn();
const mockGetBundles = jest.fn();
const mockGetMenu = jest.fn();
const mockGetBusiness = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getOffers: (...a: unknown[]) => mockGetOffers(...a),
    getBundles: (...a: unknown[]) => mockGetBundles(...a),
    getMenu: (...a: unknown[]) => mockGetMenu(...a),
    deleteOffer: jest.fn(),
    createOffer: jest.fn(),
    updateOffer: jest.fn(),
  },
  getBusiness: (...a: unknown[]) => mockGetBusiness(...a),
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (amount: number, currency: string) =>
    `${currency} ${(amount || 0).toFixed(2)}`,
}));

// Use the REAL message catalogs + real getTranslation so the {{amount}} /
// {{value}} interpolation contract is actually tested — the D4a bug was an
// unsubstituted "$ {{value}} OFF" reaching operators.
jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const { getTranslation } = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "es" }),
    getTranslation,
  };
});

jest.mock("../ImageUpload", () => ({ __esModule: true, default: () => null }));

import OffersManager from "../OffersManager";

const fixedOffer = {
  id: 1,
  name: "Postre gratis",
  description: "",
  image: "",
  code: "",
  discount_type: "fixed",
  discount_value: 20,
  is_active: true,
  applicable_to: "all",
  target_id: undefined,
};

const percentOffer = {
  id: 2,
  name: "Verano",
  description: "",
  image: "",
  code: "",
  discount_type: "percentage",
  discount_value: 10,
  is_active: true,
  applicable_to: "all",
  target_id: undefined,
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetOffers.mockResolvedValue([fixedOffer, percentOffer]);
  mockGetBundles.mockResolvedValue([]);
  mockGetMenu.mockResolvedValue({ categories: [] });
  mockGetBusiness.mockResolvedValue({ default_currency: "AED" });
});

it("renders the fixed-discount stamp with the interpolated amount, no {{value}} residue, no hardcoded $ (D4a)", async () => {
  render(<OffersManager businessId={1} />);
  // AED 20.00 formatted by the currency mock, wrapped in the es catalog string.
  expect(await screen.findByText("AED 20.00 de descuento")).toBeInTheDocument();
  expect(screen.queryByText(/\{\{value\}\}/)).not.toBeInTheDocument();
  expect(screen.queryByText(/\$ \{\{/)).not.toBeInTheDocument();
});

it("renders the percentage stamp localized (no English OFF in es) (D4a)", async () => {
  render(<OffersManager businessId={1} />);
  expect(await screen.findByText("10% de descuento")).toBeInTheDocument();
  expect(screen.queryByText("10% OFF")).not.toBeInTheDocument();
});
