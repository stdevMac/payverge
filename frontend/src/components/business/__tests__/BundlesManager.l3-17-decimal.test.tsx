/** @jest-environment jsdom */
/**
 * L3-17: typing 12.50 character-by-character on the real BundlesManager price
 * field must keep the separator. Rule 3: this suite must fail if the
 * production site reverts to value={String(n)} / Number(onChange).
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

function typeChars(el: HTMLElement, chars: string) {
  let acc = (el as HTMLInputElement).value || "";
  for (const ch of chars) {
    acc = acc + ch;
    fireEvent.change(el, { target: { value: acc } });
  }
}

async function openCreateModalAndPriceInput() {
  render(<BundlesManager businessId={1} />);
  const createButtons = await screen.findAllByText(
    "businessDashboard.dashboard.menuBuilder.bundlesManager.createButton",
  );
  fireEvent.click(createButtons[0]);
  return screen.findByTestId("bundle-price-input");
}

beforeEach(() => {
  jest.clearAllMocks();
  mockGetBundles.mockResolvedValue([]);
  mockGetMenu.mockResolvedValue({ categories: [] });
  mockGetBusiness.mockResolvedValue({ default_currency: "USD" });
});

describe("L3-17 BundlesManager price decimal survival (real component)", () => {
  it("keeps 12.50 through character-by-character typing on production input", async () => {
    const priceInput = await openCreateModalAndPriceInput();
    fireEvent.change(priceInput, { target: { value: "" } });
    typeChars(priceInput, "12.50");
    expect((priceInput as HTMLInputElement).value).toBe("12.50");
    expect((priceInput as HTMLInputElement).value).not.toBe("1250");
    expect((priceInput as HTMLInputElement).value).not.toBe("1250.00");
  });

  it("does not turn 12.50 into 1250 after blur", async () => {
    const priceInput = await openCreateModalAndPriceInput();
    fireEvent.change(priceInput, { target: { value: "" } });
    typeChars(priceInput, "12.50");
    fireEvent.blur(priceInput);
    await waitFor(() => {
      const v = (priceInput as HTMLInputElement).value;
      expect(v).not.toBe("1250");
      expect(v).not.toBe("1250.00");
    });
    // Accept "12.5" or "12.50" after blur snap — never 100× magnitude.
    expect(Number((priceInput as HTMLInputElement).value)).toBeCloseTo(12.5, 5);
  });
});
