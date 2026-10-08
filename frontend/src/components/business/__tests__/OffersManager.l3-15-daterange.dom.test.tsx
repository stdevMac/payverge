/**
 * D1 / L3-15: offer end date must show a localized range error when end < start,
 * without a native min= attribute (Chrome English bubble).
 *
 * The prior harness mirrored a private OfferDateRangeFields + source-grep —
 * both pass if OffersManager wiring drifts. This mounts the real manager.
 *
 * Revert-proof: re-add min={formData.start_date} on end date → attribute present;
 * drop isInvalid/error for inverted range → alert gone.
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
  // ISO timestamps so toDateInput/localDateKey seed stable yyyy-mm-dd values.
  start_date: "2026-08-10T12:00:00.000Z",
  end_date: "2026-08-20T12:00:00.000Z",
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

describe("OffersManager L3-15 date range (DOM)", () => {
  it("shows localized inverted-range error without min= on end date", async () => {
    render(<OffersManager businessId={1} />);
    fireEvent.click(await screen.findByLabelText(`${NS}.editOfferAria`));

    const endHost = await screen.findByTestId("offer-end-date");
    // No native min coupling to start (Chrome English bubble root cause).
    expect(endHost).not.toHaveAttribute("min");
    const endInput =
      (endHost.matches("input")
        ? endHost
        : endHost.querySelector("input")) as HTMLInputElement | null;
    if (endInput) {
      expect(endInput).not.toHaveAttribute("min");
    }

    // Seed start via label, then invert end. NextUI date Inputs listen to change.
    const startInput = screen.getByLabelText(
      `${NS}.form.startDateLabel`,
    ) as HTMLInputElement;
    fireEvent.change(startInput, { target: { value: "2026-08-10" } });

    const endField =
      endInput ??
      (screen.getByLabelText(`${NS}.form.endDateLabel`) as HTMLInputElement);
    fireEvent.change(endField, { target: { value: "2026-08-01" } });

    await waitFor(() => {
      // isInvalid errorMessage + role=alert both paint the same key.
      expect(
        screen.getAllByText(`${NS}.messages.dateRangeError`).length,
      ).toBeGreaterThan(0);
    });

    const submit = screen
      .getByText(`${NS}.buttons.update`)
      .closest("button") as HTMLButtonElement;
    expect(submit).toBeDisabled();
    expect(mockUpdateOffer).not.toHaveBeenCalled();
  });
});
