/**
 * D1 / L3-14: percentage discount > 100 must fail validation in the real
 * OffersManager editor (visible error + blocked submit), not only in the pure
 * isOfferDiscountValid helper.
 *
 * Revert-proof: make isOfferDiscountValid always true → submit enables and
 * updateOffer fires with 500.
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

const mockGetOffers = jest.fn();
const mockGetBundles = jest.fn();
const mockGetMenu = jest.fn();
const mockGetBusiness = jest.fn();
const mockUpdateOffer = jest.fn();

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
    updateOffer: (...a: unknown[]) => mockUpdateOffer(...a),
  },
  getBusiness: (...a: unknown[]) => mockGetBusiness(...a),
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

import OffersManager from "../OffersManager";

const NS = "businessDashboard.dashboard.menuBuilder.offersManager";

const offer = {
  id: 7,
  name: "Summer Special",
  description: "",
  image: "",
  code: "",
  discount_type: "percentage",
  discount_value: 10,
  is_active: true,
  start_date: "",
  end_date: "",
  weekday_mask: 127,
  applicable_to: "all",
  target_id: undefined,
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetOffers.mockResolvedValue([offer]);
  mockGetBundles.mockResolvedValue([]);
  mockGetMenu.mockResolvedValue({ categories: [] });
  mockGetBusiness.mockResolvedValue({ default_currency: "USD" });
  mockUpdateOffer.mockResolvedValue({});
});

describe("OffersManager L3-14 percentage discount bound (DOM)", () => {
  it("rejects 500% with visible error and blocked update", async () => {
    render(<OffersManager businessId={1} />);
    fireEvent.click(await screen.findByLabelText(`${NS}.editOfferAria`));
    const input = (await screen.findByTestId(
      "offer-discount-value",
    )) as HTMLInputElement;

    fireEvent.change(input, { target: { value: "500" } });
    expect(input.value).toBe("500");

    // Localized error key path (mock tString returns key).
    await waitFor(() => {
      expect(
        screen.getByText(`${NS}.messages.discountPercentMax`),
      ).toBeInTheDocument();
    });

    const submit = screen
      .getByText(`${NS}.buttons.update`)
      .closest("button") as HTMLButtonElement;
    expect(submit).toBeDisabled();
    expect(mockUpdateOffer).not.toHaveBeenCalled();
  });
});
