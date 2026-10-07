/**
 * D1 / L3-16: combo add-item picker must drop inventory-driven 86 states
 * (`inventory_out`), not only `is_available:false`.
 *
 * Pure `filterAvailableMenuItems` unit tests pass if BundlesManager never
 * filters through them. The prior L3-43 suite only covered manual 86. This
 * mounts BundlesManager with useSharedMenu projecting item_orderability.
 *
 * Revert-proof: drop the inventory-state branch in isMenuItemAvailableForBundle
 * → "Soup" (inventory_out, is_available true) reappears in the picker options.
 *
 * @jest-environment jsdom
 */
import React from "react";
import {
  render as rtlRender,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
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

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBundles: (...a: unknown[]) => mockGetBundles(...a),
    getMenu: (...a: unknown[]) => mockGetMenu(...a),
    getBusiness: (...a: unknown[]) => mockGetBusiness(...a),
    deleteBundle: jest.fn(),
    createBundle: jest.fn(),
    updateBundle: jest.fn(),
  },
}));

// Bypass projectMenuOrderability so is_available stays true while inventory
// state is inventory_out — the residual half of L3-16 that pure flag-only
// filtering would miss.
jest.mock("@/hooks/useSharedMenu", () => ({
  useSharedMenu: () => ({
    data: [
      {
        id: "c1",
        name: "Mains",
        description: "",
        items: [
          {
            id: "burger",
            name: "Burger",
            description: "",
            price: 12,
            is_available: true,
            orderability_state: "available",
          },
          {
            id: "soup",
            name: "Soup",
            description: "",
            price: 8,
            // Manual flag still true — only inventory state marks 86.
            is_available: true,
            orderability_state: "inventory_out",
          },
          {
            id: "fries",
            name: "Fries",
            description: "",
            price: 5,
            is_available: false,
            orderability_state: "manual_unavailable",
          },
        ],
      },
    ],
    isLoading: false,
    error: null,
  }),
  menuQueryKey: (id: number) => ["business-menu-categories", id],
  projectMenuOrderability: (cats: unknown) => cats,
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

const NS = "businessDashboard.dashboard.menuBuilder.bundlesManager";

beforeEach(() => {
  jest.clearAllMocks();
  mockGetBundles.mockResolvedValue([]);
  mockGetMenu.mockResolvedValue({ categories: [] });
  mockGetBusiness.mockResolvedValue({ default_currency: "USD" });
});

describe("BundlesManager L3-16 inventory-out picker (DOM)", () => {
  it("excludes inventory_out and manual 86 from the add-item picker options", async () => {
    render(<BundlesManager businessId={1} />);
    fireEvent.click(await screen.findByText(`${NS}.createButton`));

    await waitFor(() => {
      const options = Array.from(document.querySelectorAll("option")).map(
        (el) => el.textContent,
      );
      expect(options).toContain("Burger");
      // Manual 86
      expect(options).not.toContain("Fries");
      // Inventory-driven 86 (residual half of L3-16)
      expect(options).not.toContain("Soup");
    });
  });
});
