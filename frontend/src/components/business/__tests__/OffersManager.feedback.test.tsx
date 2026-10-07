/** @jest-environment jsdom */
import React from "react";
import { render as rtlRender, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// OffersManager reads its menu from useSharedMenu (React Query) — wrap renders in
// a fresh QueryClient so the shared-menu query has a client.
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
const mockDeleteOffer = jest.fn();
const mockCreateOffer = jest.fn();
const mockUpdateOffer = jest.fn();

const mockToastError = jest.fn();
const mockToastSuccess = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: {
    success: (...a: unknown[]) => mockToastSuccess(...a),
    error: (...a: unknown[]) => mockToastError(...a),
  },
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getOffers: (...a: unknown[]) => mockGetOffers(...a),
    getBundles: (...a: unknown[]) => mockGetBundles(...a),
    getMenu: (...a: unknown[]) => mockGetMenu(...a),
    deleteOffer: (...a: unknown[]) => mockDeleteOffer(...a),
    createOffer: (...a: unknown[]) => mockCreateOffer(...a),
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

it("disables the Create submit button until required fields are valid (F25)", async () => {
  render(<OffersManager businessId={1} />);

  // Open the create modal via the header create button.
  const createButtons = await screen.findAllByText(`${NS}.createButton`);
  fireEvent.click(createButtons[0]);

  // The modal submit button renders the buttons.create label key; with an
  // empty name and 0 discount it must be disabled (no silent no-op).
  const submit = await screen.findByText(`${NS}.buttons.create`);
  const submitButton = submit.closest("button");
  expect(submitButton).toBeDisabled();
});

it("shows an error toast and keeps the delete modal context when delete fails (F26)", async () => {
  mockDeleteOffer.mockRejectedValue(new Error("boom"));
  render(<OffersManager businessId={1} />);

  // Open the per-offer delete via its aria-labelled icon button.
  const deleteBtn = await screen.findByLabelText(`${NS}.deleteOfferAria`);
  fireEvent.click(deleteBtn);

  // Confirm deletion in the ConfirmationModal (confirm uses buttons.delete? —
  // OffersManager passes confirmLabel; click whichever confirm renders).
  const confirm = await screen.findByText(`${NS}.buttons.delete`);
  fireEvent.click(confirm);

  await waitFor(() => expect(mockDeleteOffer).toHaveBeenCalled());
  await waitFor(() =>
    expect(mockToastError).toHaveBeenCalledWith(`${NS}.messages.deleteError`),
  );
});

it("closes the edit modal on Escape and keeps Cancel in the dialog (#665)", async () => {
  render(<OffersManager businessId={1} />);
  fireEvent.click(await screen.findByLabelText(`${NS}.editOfferAria`));
  expect(await screen.findByText(`${NS}.modal.editTitle`)).toBeInTheDocument();
  expect(screen.getByTestId("offer-edit-cancel")).toBeInTheDocument();
  fireEvent.keyDown(document, { key: "Escape" });
  await waitFor(() =>
    expect(screen.queryByText(`${NS}.modal.editTitle`)).not.toBeInTheDocument(),
  );
});

it("edits recurring weekdays and business-time window in the offer payload", async () => {
  mockGetOffers.mockResolvedValue([
    {
      ...offer,
      weekday_mask: 127,
      start_minute: 600,
      end_minute: 720,
    },
  ]);
  mockUpdateOffer.mockResolvedValue({});
  render(<OffersManager businessId={1} />);

  fireEvent.click(await screen.findByLabelText(`${NS}.editOfferAria`));

  expect(await screen.findByLabelText(`${NS}.weekdays.monday`)).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  expect(screen.getByLabelText(`${NS}.form.startTimeLabel`)).toHaveValue("10:00");
  expect(screen.getByLabelText(`${NS}.form.endTimeLabel`)).toHaveValue("12:00");

  fireEvent.click(screen.getByLabelText(`${NS}.weekdays.monday`));
  fireEvent.change(screen.getByLabelText(`${NS}.form.startTimeLabel`), {
    target: { value: "11:15" },
  });

  fireEvent.click(screen.getByText(`${NS}.buttons.update`));

  await waitFor(() => expect(mockUpdateOffer).toHaveBeenCalled());
  expect(mockUpdateOffer.mock.calls[0][2]).toEqual(
    expect.objectContaining({
      weekday_mask: 125,
      start_minute: 675,
      end_minute: 720,
    }),
  );
});

// #665 — the offer edit modal must fit inside the viewport it is centered in.
// NextUI pairs its `sm:my-16` (8rem of vertical margin) with a margin-aware cap
// of `max-h-[calc(100%-8rem)]`; overriding the cap with a raw viewport unit
// (e.g. `max-h-[90vh]`) makes the dialog's outer box 90vh + 8rem tall, which
// pushes the footer — and therefore Cancel — below the fold on short screens.
const OUTER_MARGIN_REM_PER_SIDE = (cls: string): number => {
  const token = /(?:^|\s)sm:my-(\d+)(?:\s|$)/.exec(cls);
  return token ? Number(token[1]) * 0.25 : 0;
};

const dialogOuterHeightPx = (cls: string, viewportPx: number): number => {
  const marginPx = OUTER_MARGIN_REM_PER_SIDE(cls) * 16 * 2;
  const cap = /(?:^|\s)max-h-\[([^\]]+)\]/.exec(cls);
  if (!cap) return viewportPx;
  const raw = cap[1].replace(/_/g, " ");
  const calcRem = /^calc\(\s*100%\s*-\s*([\d.]+)rem\s*\)$/.exec(raw);
  if (calcRem) return viewportPx - Number(calcRem[1]) * 16 + marginPx;
  const viewportUnit = /^([\d.]+)(?:vh|dvh|svh|lvh)$/.exec(raw);
  if (viewportUnit) return (Number(viewportUnit[1]) / 100) * viewportPx + marginPx;
  const percent = /^([\d.]+)%$/.exec(raw);
  if (percent) return (Number(percent[1]) / 100) * viewportPx + marginPx;
  return viewportPx;
};

it("keeps the offer edit modal (and its Cancel button) inside the viewport (#665)", async () => {
  render(<OffersManager businessId={1} />);
  fireEvent.click(await screen.findByLabelText(`${NS}.editOfferAria`));
  await screen.findByText(`${NS}.modal.editTitle`);

  const dialog = document.querySelector('[role="dialog"]');
  expect(dialog).not.toBeNull();
  const cls = (dialog as HTMLElement).className;

  // A 700px-tall viewport is an ordinary laptop with browser chrome — the shape
  // QA filmed with "Cerrar / Cancelar sits outside the viewport".
  const viewportPx = 700;
  expect(dialogOuterHeightPx(cls, viewportPx)).toBeLessThanOrEqual(viewportPx);

  // Cancel must live in the modal footer, outside the scrolling body, so it
  // cannot scroll away with the form.
  const cancel = screen.getByTestId("offer-edit-cancel");
  const body = document.querySelector(".overflow-y-auto");
  expect(body?.contains(cancel)).not.toBe(true);
});

it("closes the edit modal when Escape is pressed from a field inside it (#665)", async () => {
  render(<OffersManager businessId={1} />);
  fireEvent.click(await screen.findByLabelText(`${NS}.editOfferAria`));
  expect(await screen.findByText(`${NS}.modal.editTitle`)).toBeInTheDocument();

  // Real operators press Escape with focus inside a field, not on document.
  const field = document.querySelector('[role="dialog"] input') as HTMLElement;
  expect(field).not.toBeNull();
  field.focus();
  fireEvent.keyDown(field, { key: "Escape", bubbles: true });

  await waitFor(() =>
    expect(screen.queryByText(`${NS}.modal.editTitle`)).not.toBeInTheDocument(),
  );
});

it("closes the edit modal on a native window-capture Escape (#665)", async () => {
  render(<OffersManager businessId={1} />);
  fireEvent.click(await screen.findByLabelText(`${NS}.editOfferAria`));
  expect(await screen.findByText(`${NS}.modal.editTitle`)).toBeInTheDocument();

  const field = document.querySelector('[role="dialog"] input') as HTMLElement;
  expect(field).not.toBeNull();
  field.focus();
  act(() => {
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
  });

  await waitFor(() =>
    expect(screen.queryByText(`${NS}.modal.editTitle`)).not.toBeInTheDocument(),
  );
});

it("keeps the offer modal open when a stacked overlay owns Escape (#665)", async () => {
  render(<OffersManager businessId={1} />);
  fireEvent.click(await screen.findByLabelText(`${NS}.editOfferAria`));
  expect(await screen.findByText(`${NS}.modal.editTitle`)).toBeInTheDocument();

  const stacked = document.createElement("div");
  stacked.setAttribute("role", "dialog");
  stacked.setAttribute("aria-modal", "true");
  stacked.setAttribute("data-testid", "stacked-overlay");
  document.body.appendChild(stacked);

  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.getByText(`${NS}.modal.editTitle`)).toBeInTheDocument();

  stacked.remove();
  fireEvent.keyDown(document, { key: "Escape" });
  await waitFor(() =>
    expect(screen.queryByText(`${NS}.modal.editTitle`)).not.toBeInTheDocument(),
  );
});

it("closes an in-modal listbox on the first Escape and the modal on the second (#665)", async () => {
  render(<OffersManager businessId={1} />);
  fireEvent.click(await screen.findByLabelText(`${NS}.editOfferAria`));
  expect(await screen.findByText(`${NS}.modal.editTitle`)).toBeInTheDocument();
  const marked = screen.getByTestId("offer-edit-modal");
  const dialog = marked.matches("[role='dialog']")
    ? marked
    : (marked.querySelector("[role='dialog']") ?? marked);

  // Dedicated trigger inside the keyed dialog — not a wrapper sibling.
  const combo = document.createElement("div");
  combo.setAttribute("role", "combobox");
  combo.setAttribute("aria-expanded", "true");
  combo.setAttribute("aria-controls", "offer-test-listbox");
  combo.addEventListener("click", () => {
    combo.setAttribute("aria-expanded", "false");
  });
  dialog.appendChild(combo);
  const listbox = document.createElement("div");
  listbox.id = "offer-test-listbox";
  listbox.setAttribute("role", "listbox");
  document.body.appendChild(listbox);

  fireEvent.keyDown(combo, { key: "Escape", bubbles: true });
  expect(screen.getByText(`${NS}.modal.editTitle`)).toBeInTheDocument();

  combo.setAttribute("aria-expanded", "false");
  listbox.remove();
  fireEvent.keyDown(combo, { key: "Escape", bubbles: true });
  await waitFor(() =>
    expect(screen.queryByText(`${NS}.modal.editTitle`)).not.toBeInTheDocument(),
  );
});
