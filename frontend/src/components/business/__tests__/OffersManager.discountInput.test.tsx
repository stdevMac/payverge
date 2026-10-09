/** @jest-environment jsdom */
/**
 * L3-42 — the discount field must not eat the decimal separator.
 *
 * The field used to store a NUMBER and re-render `String(number)`, so the
 * intermediate "10." collapsed to "10" and the next keystroke produced "105" —
 * a 10x money corruption (percentages are capped at 100, fixed amounts are
 * not). Per the S-5 primitive the input keeps RAW STRING state and only parses
 * at validation/submit time.
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

/** Types character-by-character the way a real operator does. */
const typeInto = (el: HTMLInputElement, text: string) => {
  for (const ch of text) {
    fireEvent.change(el, { target: { value: el.value + ch } });
  }
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetOffers.mockResolvedValue([offer]);
  mockGetBundles.mockResolvedValue([]);
  mockGetMenu.mockResolvedValue({ categories: [] });
  mockGetBusiness.mockResolvedValue({ default_currency: "USD" });
  mockUpdateOffer.mockResolvedValue({});
});

const openEditor = async () => {
  render(<OffersManager businessId={1} />);
  fireEvent.click(await screen.findByLabelText(`${NS}.editOfferAria`));
  const input = (await screen.findByTestId(
    "offer-discount-value",
  )) as HTMLInputElement;
  fireEvent.change(input, { target: { value: "" } });
  return input;
};

describe("L3-42 offer discount input keeps the decimal", () => {
  it("submits 10.5 after typing '10.5' character-wise", async () => {
    const input = await openEditor();

    typeInto(input, "10.5");

    expect(input.value).toBe("10.5");

    fireEvent.click(screen.getByText(`${NS}.buttons.update`));
    await waitFor(() => expect(mockUpdateOffer).toHaveBeenCalled());
    expect(mockUpdateOffer.mock.calls[0][2]).toEqual(
      expect.objectContaining({ discount_value: 10.5 }),
    );
  });

  it("keeps a trailing separator visible while typing (no collapse to 10)", async () => {
    const input = await openEditor();

    typeInto(input, "10.");

    expect(input.value).toBe("10.");
  });

  it("blocks submit while the discount is unparseable", async () => {
    const input = await openEditor();

    typeInto(input, "abc");

    const submit = screen
      .getByText(`${NS}.buttons.update`)
      .closest("button") as HTMLButtonElement;
    expect(submit).toBeDisabled();
  });
});
