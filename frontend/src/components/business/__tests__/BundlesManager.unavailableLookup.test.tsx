/** @jest-environment jsdom */
/**
 * D1 / L3-43 — an existing bundle line whose menu item is 86'd must still resolve.
 *
 * The lookup used to be built from the SAME availability-filtered list that
 * feeds the "add item" picker (L3-16), so editing a bundle that already
 * contains an unavailable item showed that line at $0, understated the regular
 * total, and could falsely flip the rose tone + "no savings" warning.
 * The picker stays filtered; the lookup must not be.
 *
 * Revert-proof: build menuItemLookup from filtered `menuItems` instead of
 * `allMenuItems` → Fries eachPrice becomes 0.00, regular total drops to $12,
 * and the no-savings warning falsely appears.
 */
import React from "react";
import {
  render as rtlRender,
  screen,
  fireEvent,
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

jest.mock("@/api/currency", () => ({
  formatCurrency: (amount: number) => `$${(amount || 0).toFixed(2)}`,
}));

// Params-aware stub: the per-line price ships through an interpolated key, so
// an identity stub would hide the value under test.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (
    key: string,
    _locale?: string,
    params?: Record<string, string | number>,
  ) =>
    params
      ? `${key}:${Object.entries(params)
          .map(([k, v]) => `${k}=${v}`)
          .join(",")}`
      : key,
}));

jest.mock("../ImageUpload", () => ({
  __esModule: true,
  default: () => null,
}));

import BundlesManager from "../BundlesManager";

const NS = "businessDashboard.dashboard.menuBuilder.bundlesManager";

const menu = [
  {
    id: "c1",
    name: "Mains",
    description: "",
    items: [
      {
        id: "m1",
        name: "Burger",
        description: "",
        price: 12,
        is_available: true,
      },
      {
        // 86'd — still part of an already-saved bundle.
        id: "m2",
        name: "Fries",
        description: "",
        price: 5,
        is_available: false,
      },
    ],
  },
];

const bundle = {
  id: 5,
  name: "Combo A",
  description: "",
  price: 20,
  currency: "USD",
  image: "",
  is_active: true,
  items: [
    { menu_item_id: "m1", name: "Burger", quantity: 1 },
    { menu_item_id: "m2", name: "Fries", quantity: 2 },
  ],
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetBundles.mockResolvedValue([bundle]);
  mockGetMenu.mockResolvedValue({ categories: menu });
  mockGetBusiness.mockResolvedValue({ default_currency: "USD" });
});

const openEditor = async () => {
  render(<BundlesManager businessId={1} />);
  fireEvent.click(await screen.findByLabelText(`${NS}.aria.edit:name=Combo A`));
  await screen.findByTestId("bundle-price-input");
};

describe("L3-43 bundle lookup covers unavailable items", () => {
  it("prices an existing 86'd line and the regular total correctly", async () => {
    await openEditor();

    // Fries ($5) must not render at $0.00.
    expect(
      screen.getByText(`${NS}.form.eachPrice:value=5.00`),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(`${NS}.form.eachPrice:value=0.00`),
    ).not.toBeInTheDocument();

    // 12*1 + 5*2 = 22 (not 12).
    expect(screen.getByText("$22.00")).toBeInTheDocument();
  });

  it("does not flip the no-savings warning from an understated total", async () => {
    await openEditor();

    // Bundle price 20 < regular 22 — this is a real saving.
    expect(
      screen.queryByTestId("bundle-no-savings-warning"),
    ).not.toBeInTheDocument();
  });

  it("still keeps 86'd items out of the add-item picker (L3-16)", async () => {
    await openEditor();

    // NextUI mirrors the full Select collection into a hidden native select.
    const options = Array.from(document.querySelectorAll("option")).map(
      (el) => el.textContent,
    );
    expect(options).toContain("Burger");
    expect(options).not.toContain("Fries");
  });
});
