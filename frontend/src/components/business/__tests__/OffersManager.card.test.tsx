/** @jest-environment jsdom */
/**
 * Related to issue 151: offer cards hide empty image slabs, show a relative
 * end cue, and do not invent redemption counts the API does not return.
 */
import React from "react";
import { render as rtlRender, screen } from "@testing-library/react";
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
  formatCurrency: (amount: number) => `$${(amount || 0).toFixed(2)}`,
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const { getTranslation } = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en" }),
    getTranslation,
  };
});

jest.mock("../ImageUpload", () => ({ __esModule: true, default: () => null }));

import OffersManager from "../OffersManager";

function daysFromToday(days: number): string {
  const d = new Date();
  d.setHours(12, 0, 0, 0);
  d.setDate(d.getDate() + days);
  return d.toISOString();
}

const offer = {
  id: 7,
  name: "Weekday Lunch",
  description: "15% off lunch",
  image: "",
  code: "",
  discount_type: "percentage",
  discount_value: 15,
  is_active: true,
  start_date: daysFromToday(-1),
  end_date: daysFromToday(30),
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
});

describe("OffersManager cards (#151)", () => {
  it("shows Add photo instead of an empty grey image and a relative end cue", async () => {
    render(<OffersManager businessId={1} />);

    expect(await screen.findByTestId("offer-card-image-placeholder")).toHaveTextContent(
      "Add photo",
    );
    expect(screen.queryByTestId("offer-card-image")).not.toBeInTheDocument();

    const cue = await screen.findByTestId("offer-ends-cue");
    expect(cue).toHaveTextContent("Ends in 30 days");

    // Raw ISO / yyyy-mm-dd ranges stay off the card.
    expect(screen.queryByText(/2026-\d{2}-\d{2}/)).not.toBeInTheDocument();
    expect(screen.queryByText(/T\d{2}:\d{2}/)).not.toBeInTheDocument();

    // No invented redemption counts, and no "not tracked" filler slot.
    expect(screen.queryByText(/redeemed/i)).not.toBeInTheDocument();
    expect(screen.queryByTestId("offer-usage-unavailable")).not.toBeInTheDocument();
    expect(screen.queryByText(/Redemptions not tracked/i)).not.toBeInTheDocument();
  });
});
