/** @jest-environment jsdom */
/**
 * F13/F14/F15: the reservation confirmation modal previously rendered raw
 * English fallbackCopy strings, hardcoded "Add to calendar", and a dead
 * "Check Out Menu" button (querySelector('[data-tab="menu"]')).
 *
 * These tests prove the modal now routes copy through the guest translator and
 * that "Check Out Menu" invokes the onViewMenu callback.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import GuestReservationForm from "../GuestReservationForm";
import { guestReservationAPI } from "@/api/reservations";
import { downloadReservationIcs } from "../guestReservationCalendar";

jest.mock("../guestReservationCalendar", () => {
  const actual = jest.requireActual("../guestReservationCalendar");
  return {
    ...actual,
    downloadReservationIcs: jest.fn(),
  };
});

// Identity-ish translator that marks any modal.* key so we can assert the
// modal uses the translator path rather than the raw English literal.
jest.mock("@/i18n/GuestTranslationProvider", () => {
  const translation = {
    t: (key: string, params?: Record<string, string | number>) => {
      if (key.startsWith("businessPage.info.reservationForm.modal.")) {
        const leaf = key.split(".").pop();
        return `XL_${leaf}`;
      }
      if (key === "businessPage.info.reservationForm.addToCalendar") {
        return "XL_addToCalendar";
      }
      if (key === "businessPage.info.reservationForm.addToGoogleCalendar") {
        return "XL_addToGoogleCalendar";
      }
      if (key === "businessPage.info.reservationForm.calendarEventTitle") {
        return `Reserva en ${params?.businessName ?? ""}`;
      }
      if (key === "businessPage.info.reservationForm.calendarEventDetails") {
        return `Reserva para ${params?.partySize ?? ""} invitados en ${params?.businessName ?? ""}.`;
      }
      // Echo other keys (with naive interpolation) so the form renders.
      let out = key;
      if (params) {
        for (const [k, v] of Object.entries(params)) {
          out = out.replace(new RegExp(`\\{\\{?${k}\\}?\\}`, "g"), String(v));
        }
      }
      return out;
    },
  };

  return {
    // Keep the translator reference stable across renders, matching the real
    // provider. A fresh function here changes loadSettings' dependencies on
    // every render and creates an artificial request/render loop.
    useGuestTranslation: () => translation,
  };
});

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
  error: jest.fn(),
  success: jest.fn(),
}));

jest.mock("@/api/reservations", () => ({
  guestReservationAPI: {
    getSettings: jest.fn(),
    getAvailability: jest.fn(),
    createReservation: jest.fn(),
  },
}));

const settings = {
  enabled: true,
  min_party_size: 1,
  max_party_size: 8,
  max_advance_days: 30,
  min_advance_minutes: 0,
  approval_mode: "manual",
  allow_cancellation: false,
  allow_waitlist: false,
  default_duration: 60,
  cancellation_deadline: 24,
  external_partner_links: [],
};

const slotTime = "2026-06-20T19:00:00.000Z";

const flush = (ms = 40) => new Promise((r) => setTimeout(r, ms));

// In a CPU-contended parallel suite each setTimeout can stretch several-fold,
// so every wait below is budgeted by wall clock with generous headroom (waits
// exit early when green, so the happy path stays fast).
const WAIT = { timeout: 15_000 } as const;
const TEST_TIMEOUT = 90_000;

/**
 * Repeatedly run `attempt` (which fires events) until `done` holds, re-trying
 * across React re-renders. Side-effecting retries live here, not inside
 * waitFor's matcher, keeping the act() boundaries clean and the flow stable.
 * Deadline-based, not try-count based: the old 100×60ms try budget assumed
 * sleeps actually take ~60ms, which doesn't hold under suite-wide CPU load.
 */
async function retryUntil(
  attempt: () => void,
  done: () => boolean,
  { timeoutMs = 30_000, gap = 40 }: { timeoutMs?: number; gap?: number } = {},
) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (done()) return;
    attempt();
    await flush(gap);
  }
  if (!done()) throw new Error("retryUntil: condition never met");
}

function primeApi() {
  (guestReservationAPI.getSettings as jest.Mock).mockResolvedValue(settings);
  (guestReservationAPI.getAvailability as jest.Mock).mockResolvedValue({
    available_slots: [
      {
        time: slotTime,
        available_tables: 3,
        reason_code: "available",
        recommended: false,
      },
    ],
    waitlist_available: false,
    next_available_slot: null,
  });
  (guestReservationAPI.createReservation as jest.Mock).mockResolvedValue({
    id: 42,
    status: "pending",
    confirmation_code: "ABC123",
  });
}

async function fillAndSubmit(
  onViewMenu?: () => void,
  status: "pending" | "confirmed" | "waitlist" = "pending",
) {
  (guestReservationAPI.createReservation as jest.Mock).mockResolvedValue({
    id: 42,
    status,
    confirmation_code: "ABC123",
  });
  render(
    <GuestReservationForm
      customUrl="test"
      businessName="Test Bistro"
      onViewMenu={onViewMenu}
    />,
  );

  // The mock translator ECHOES non-modal keys, so reservationT treats them as
  // "untranslated" and renders the English fallbackCopy values. Navigate by
  // those English labels; only the modal.* keys come back translated (markers).

  // Step 1: pick a date via the "Today" quick button.
  const todayBtn = await screen.findByRole("button", { name: "Today" }, WAIT);
  fireEvent.click(todayBtn);
  await flush();

  // Single-page Book flow: slots appear after a date is selected. Scope slot
  // lookups to buttons outside the quick-dates group (those also use aria-pressed).
  const slotButtons = () =>
    (
      Array.from(
        document.querySelectorAll("button[aria-pressed]"),
      ) as HTMLButtonElement[]
    ).filter(
      (b) => !b.closest('[aria-labelledby="reservation-quick-dates-label"]'),
    );
  await retryUntil(
    () => {},
    () => slotButtons().length > 0,
  );

  // Select the slot until aria-pressed flips true. PG-11 replaced the slot's
  // `disabled` attribute with aria-disabled, so skip those explicitly.
  await retryUntil(
    () => {
      const slot = slotButtons().find(
        (b) => b.getAttribute("aria-disabled") !== "true",
      );
      if (slot) fireEvent.click(slot);
    },
    () =>
      slotButtons().some((b) => b.getAttribute("aria-pressed") === "true"),
  );

  // Guest details are on the same page — wait for the name field.
  await retryUntil(
    () => {},
    () => !!document.querySelector('input[autocomplete="name"]'),
  );

  // Fill required name + phone + email sequentially (email became required
  // for online booking — confirmations are delivered there).
  const nameInput = document.querySelector(
    'input[autocomplete="name"]',
  ) as HTMLInputElement;
  fireEvent.change(nameInput, { target: { value: "Jane Guest" } });
  await flush();
  const phoneInput = document.querySelector(
    'input[autocomplete="tel"]',
  ) as HTMLInputElement;
  fireEvent.change(phoneInput, { target: { value: "3125550100" } });
  await flush();
  const emailInput = document.querySelector(
    'input[autocomplete="email"]',
  ) as HTMLInputElement;
  fireEvent.change(emailInput, { target: { value: "jane@example.com" } });
  await flush();

  // Submit once the button is enabled, then wait for the modal to open.
  await retryUntil(
    () => {
      const b = screen.getByRole("button", { name: /Book/ });
      if (!b.hasAttribute("disabled")) fireEvent.click(b);
    },
    () =>
      (guestReservationAPI.createReservation as jest.Mock).mock.calls.length > 0,
  );
}

describe("GuestReservationForm confirmation modal (F13/F14/F15)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    primeApi();
  });

  it("renders modal copy through the translator (not raw English fallback)", async () => {
    await fillAndSubmit();
    // Modal title resolves via translator to the marked value.
    expect(await screen.findByText("XL_title", undefined, WAIT)).toBeInTheDocument();
    // The confirmation-required block + details labels also route through it.
    expect(screen.getByText("XL_confirmationRequiredTitle")).toBeInTheDocument();
    expect(screen.getByText("XL_details")).toBeInTheDocument();
    // "Add to calendar" is translated, not the hardcoded literal.
    expect(screen.getByText("XL_addToCalendar")).toBeInTheDocument();
    expect(screen.queryByText("Add to calendar")).not.toBeInTheDocument();
  }, TEST_TIMEOUT);

  it("uses the confirmed heading when the reservation is confirmed (#486)", async () => {
    await fillAndSubmit(undefined, "confirmed");
    expect(
      await screen.findByText("XL_confirmedTitle", undefined, WAIT),
    ).toBeInTheDocument();
    expect(screen.queryByText("XL_title")).not.toBeInTheDocument();
    expect(screen.queryByText("Reservation Requested!")).not.toBeInTheDocument();
  }, TEST_TIMEOUT);

  it("localizes the calendar invite and offers an ICS download (#511)", async () => {
    const open = jest.fn();
    window.open = open;
    await fillAndSubmit();
    fireEvent.click(await screen.findByText("XL_addToCalendar", undefined, WAIT));
    expect(downloadReservationIcs).toHaveBeenCalled();
    const [icsDataUri, filename] = (downloadReservationIcs as jest.Mock).mock
      .calls[0];
    expect(filename).toBe("reservation.ics");
    expect(decodeURIComponent(String(icsDataUri))).toContain(
      "Reserva en Test Bistro",
    );
    expect(decodeURIComponent(String(icsDataUri))).not.toContain(
      "Reservation at Test Bistro",
    );

    fireEvent.click(screen.getByText("XL_addToGoogleCalendar"));
    expect(open).toHaveBeenCalled();
    const googleUrl = String(open.mock.calls[0][0]);
    expect(new URL(googleUrl).searchParams.get("text")).toBe(
      "Reserva en Test Bistro",
    );
    expect(googleUrl).not.toContain("Reservation+at+Test+Bistro");
  }, TEST_TIMEOUT);

  it("Check Out Menu invokes onViewMenu", async () => {
    const onViewMenu = jest.fn();
    await fillAndSubmit(onViewMenu);
    const checkMenu = await screen.findByText("XL_checkMenu", undefined, WAIT);
    fireEvent.click(checkMenu);
    await waitFor(() => {
      expect(onViewMenu).toHaveBeenCalledTimes(1);
    }, WAIT);
  }, TEST_TIMEOUT);
});
