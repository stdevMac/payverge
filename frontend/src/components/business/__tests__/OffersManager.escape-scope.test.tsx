/** @jest-environment jsdom */
/**
 * #743 — Escape on the open Applies-to dropdown must close the listbox
 * without wiping the painted Applies-to / Target values. The 1024×622
 * footer must not steal hits on Target "Show suggestions".
 */
import React from "react";
import {
  render as rtlRender,
  screen,
  fireEvent,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  OFFER_FOOTER_OVERLAP_1024x622,
  hitTestTopmost,
} from "../offerModalHitTest";

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

jest.mock("../ImageUpload", () => ({
  __esModule: true,
  default: () => null,
}));

import OffersManager from "../OffersManager";

const steakOffer = {
  id: 7,
  name: "$5 Off the Steak Plate",
  description: "",
  image: "",
  code: "",
  discount_type: "fixed" as const,
  discount_value: 5,
  is_active: true,
  start_date: "2026-08-08T00:00:00Z",
  end_date: "2026-09-14T23:59:59Z",
  weekday_mask: 127,
  applicable_to: "item",
  target_id: "steak-1",
};

beforeEach(() => {
  jest.clearAllMocks();
  mockGetOffers.mockResolvedValue([steakOffer]);
  mockGetBundles.mockResolvedValue([]);
  mockGetMenu.mockResolvedValue({
    categories: [
      {
        id: "mains",
        name: "Mains",
        items: [{ id: "steak-1", name: "Steak Plate" }],
      },
    ],
  });
  mockGetBusiness.mockResolvedValue({ default_currency: "USD" });
});

function appliesToTrigger(): HTMLElement {
  const root = screen.getByTestId("offer-applies-to");
  if (root.matches("button, [role='button']")) return root;
  return within(root).getByRole("button");
}

function targetField(): HTMLInputElement {
  const root = screen.getByTestId("offer-target");
  if (root.matches("input, textarea, [role='combobox']")) {
    return root as HTMLInputElement;
  }
  return (
    within(root).queryByRole("combobox") ??
    within(root).getByRole("textbox")
  ) as HTMLInputElement;
}

async function openSteakEdit() {
  render(<OffersManager businessId={1} />);
  fireEvent.click(await screen.findByLabelText("Edit offer"));
  expect(await screen.findByText("Edit Offer")).toBeInTheDocument();
  await waitFor(() => {
    expect(appliesToTrigger()).toHaveTextContent("Menu item");
    expect(targetField()).toHaveValue("Steak Plate");
  });
}

it("keeps painted Applies-to and Target after Escape on the open dropdown (#743)", async () => {
  const user = userEvent.setup();
  await openSteakEdit();

  expect(screen.getByTestId("offer-edit-submit")).not.toBeDisabled();
  expect(appliesToTrigger()).toHaveTextContent("Menu item");
  expect(targetField()).toHaveValue("Steak Plate");

  await user.click(appliesToTrigger());
  await waitFor(() => {
    expect(appliesToTrigger()).toHaveAttribute("aria-expanded", "true");
  });
  const listbox = await screen.findByRole("listbox");
  expect(
    within(listbox).getByRole("option", { name: "Menu item" }),
  ).toBeInTheDocument();

  await user.keyboard("{Escape}");

  await waitFor(() => {
    expect(appliesToTrigger()).toHaveAttribute("aria-expanded", "false");
  });
  expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  expect(screen.getByText("Edit Offer")).toBeInTheDocument();
  expect(appliesToTrigger()).toHaveTextContent("Menu item");
  expect(targetField()).toHaveValue("Steak Plate");
  expect(targetField()).toHaveAttribute("data-filled", "true");
  expect(screen.getByTestId("offer-edit-submit")).toHaveTextContent(
    "Update Offer",
  );
  expect(screen.getByTestId("offer-edit-submit")).not.toBeDisabled();
});

it("lets Target Show suggestions win a 1024×622 footer hit-test (#743)", async () => {
  const user = userEvent.setup();
  Object.defineProperty(window, "innerWidth", {
    value: OFFER_FOOTER_OVERLAP_1024x622.viewport.width,
    configurable: true,
  });
  Object.defineProperty(window, "innerHeight", {
    value: OFFER_FOOTER_OVERLAP_1024x622.viewport.height,
    configurable: true,
  });

  await openSteakEdit();

  const suggest = screen
    .getAllByRole("button", { name: /show suggestions/i })
    .find((btn) => !btn.className.includes("pointer-events-none"));
  if (!suggest) {
    throw new Error("Target Show suggestions chevron not found");
  }
  const footer =
    screen.getByTestId("offer-edit-footer") ??
    screen.getByTestId("offer-edit-cancel").closest("footer");
  expect(footer).not.toBeNull();
  expect((footer as HTMLElement).className).toMatch(
    /(?:^|\s)pointer-events-none(?:\s|$)/,
  );

  const { point, suggestions, footer: footerRect } =
    OFFER_FOOTER_OVERLAP_1024x622;
  expect(
    hitTestTopmost(point.x, point.y, [
      { el: footer as Element, rect: footerRect },
      { el: suggest, rect: suggestions },
    ]),
  ).toBe(suggest);

  await user.click(suggest);
  await waitFor(() => {
    expect(suggest).toHaveAttribute("aria-expanded", "true");
  });
});
