/**
 * D1 / L3-18: anti-deal combo (bundle price ≥ sum of components) must show a
 * signed negative savings amount and the no-savings warning — not a clamped
 * green "$0.00" customer save.
 *
 * Pure computeBundleSavings unit tests pass if BundlesManager still clamps
 * with Math.max(0, …). This mounts an existing anti-deal bundle in the editor.
 *
 * Revert-proof: clamp savings display to Math.max(0, delta) and drop the
 * isBundlePriceAtOrAboveRegular warning → no "$-8.00" / no warning chip.
 *
 * Backend rejectAntiDealBundlePrice is covered in bundle_anti_deal_test.go.
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

jest.mock("@/api/currency", () => ({
  formatCurrency: (amount: number) => {
    const n = Number(amount) || 0;
    const sign = n < 0 ? "-" : "";
    return `${sign}$${Math.abs(n).toFixed(2)}`;
  },
}));

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

// Regular total = 12; bundle price = 20 → savings = -8 (anti-deal).
const menu = [
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
      },
    ],
  },
];

const antiDealBundle = {
  id: 9,
  name: "Anti Deal",
  description: "",
  price: 20,
  currency: "USD",
  image: "",
  is_active: true,
  items: [{ menu_item_id: "burger", name: "Burger", quantity: 1 }],
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetBundles.mockResolvedValue([antiDealBundle]);
  mockGetMenu.mockResolvedValue({ categories: menu });
  mockGetBusiness.mockResolvedValue({ default_currency: "USD" });
});

describe("BundlesManager L3-18 anti-deal signed savings (DOM)", () => {
  it("renders negative savings and no-savings warning for price > regular", async () => {
    render(<BundlesManager businessId={1} />);
    fireEvent.click(
      await screen.findByLabelText(`${NS}.aria.edit:name=Anti Deal`),
    );

    await screen.findByTestId("bundle-price-input");

    await waitFor(() => {
      expect(screen.getByTestId("bundle-no-savings-warning")).toBeInTheDocument();
    });

    // Customer-saves row: signed negative (12 − 20 = −8), never clamped $0.00.
    const savesRow = screen
      .getByText(`${NS}.summary.customerSaves`)
      .closest("div");
    expect(savesRow?.textContent).toMatch(/-[\s$]*8[.,]00/);
    expect(savesRow?.textContent).not.toMatch(/\$0[.,]00/);
  });
});
