/** @jest-environment jsdom */
/**
 * Guest reservation form — required email + "hear back by" pending state.
 *
 * Backend now rejects public guest bookings without a valid email, and for
 * manual-approval businesses the create response carries `approval_deadline`
 * (RFC3339 UTC) when the reservation lands in "pending". These tests lock:
 *   1. empty email   → submit disabled, createReservation never called
 *   2. invalid email → submit blocked with the email error, no API call
 *   3. valid email   → payload carries the trimmed email; pending modal
 *                      surfaces the "hear back by {deadline}" line
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import GuestReservationForm from "../GuestReservationForm";
import {
  DATE_TIME_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";

// ── Mocks ─────────────────────────────────────────────────────────────────────

// Stable identity: the form's loadSettings useCallback depends on
// reservationT → t, so a per-render t would re-trigger the settings load
// (loading spinner flicker) on every render.
const mockTranslation = { t: (k: string) => k };
jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => mockTranslation,
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
  error: jest.fn(),
  success: jest.fn(),
}));

// One open slot tomorrow evening (local time) so the guest can reach step 3.
const slotDate = new Date();
slotDate.setDate(slotDate.getDate() + 1);
slotDate.setHours(19, 0, 0, 0);
const mockSlotTime = slotDate.toISOString();

const mockApprovalDeadline = "2026-06-11T18:00:00Z";

const mockCreateReservation = jest.fn().mockResolvedValue({
  id: 42,
  status: "pending",
  confirmation_code: "ABC123",
  approval_deadline: mockApprovalDeadline,
});

jest.mock("@/api/reservations", () => ({
  guestReservationAPI: {
    getSettings: jest.fn().mockResolvedValue({
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
    }),
    getAvailability: jest.fn().mockImplementation(() =>
      Promise.resolve({
        available_slots: [
          { time: mockSlotTime, available_tables: 2, recommended: false },
        ],
        waitlist_available: false,
        next_available_slot: null,
      }),
    ),
    createReservation: (...args: unknown[]) => mockCreateReservation(...args),
  },
}));

// ── Helpers ───────────────────────────────────────────────────────────────────

const isDisabled = (button: HTMLElement) =>
  button.hasAttribute("disabled") ||
  button.getAttribute("aria-disabled") === "true";

/** Render and walk the wizard to the contact step with name + phone filled. */
async function renderAtContactStep() {
  const result = render(
    <GuestReservationForm customUrl="test" businessName="Test" />,
  );

  fireEvent.click(await screen.findByText("Tomorrow"));
  await waitFor(() => {
    const slots = Array.from(
      document.querySelectorAll("button[aria-pressed]"),
    ).filter(
      (b) => !b.closest('[aria-labelledby="reservation-quick-dates-label"]'),
    );
    expect(slots.length).toBeGreaterThan(0);
  });
  const slotButton = Array.from(
    document.querySelectorAll("button[aria-pressed]"),
  ).find(
    (b) =>
      !b.closest('[aria-labelledby="reservation-quick-dates-label"]') &&
      b.getAttribute("aria-disabled") !== "true",
  ) as HTMLElement;
  fireEvent.click(slotButton);

  // Contact details are on the same page (placeholders from fallbackCopy).
  const nameInput = await screen.findByPlaceholderText("John Doe");
  fireEvent.change(nameInput, { target: { value: "Jane Guest" } });
  const phoneInput = screen.getByPlaceholderText("+1 (555) 123-4567");
  fireEvent.change(phoneInput, { target: { value: "+1 555 000 1234" } });

  return result;
}

const getEmailInput = () =>
  screen.getByPlaceholderText("john@example.com") as HTMLInputElement;

const getSubmitButton = () =>
  screen.getByRole("button", { name: /Book/ });

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("GuestReservationForm — email is required to book online", () => {
  beforeEach(() => {
    mockCreateReservation.mockClear();
  });

  it("keeps submit disabled and never calls the API while email is empty", async () => {
    await renderAtContactStep();

    expect(getEmailInput().value).toBe("");
    expect(isDisabled(getSubmitButton())).toBe(true);

    fireEvent.click(getSubmitButton());
    await waitFor(() => {
      expect(mockCreateReservation).not.toHaveBeenCalled();
    });
  });

  it("blocks submission with an invalid email and shows the email error", async () => {
    await renderAtContactStep();

    fireEvent.change(getEmailInput(), { target: { value: "not-an-email" } });
    // GUEST-006: submit stays disabled for malformed email; error surfaces on blur.
    fireEvent.blur(getEmailInput());
    expect(isDisabled(getSubmitButton())).toBe(true);
    // fallbackCopy.invalidEmail
    await screen.findByText("Please enter a valid email address.");
    fireEvent.click(getSubmitButton());
    expect(mockCreateReservation).not.toHaveBeenCalled();
  });

  it("submits with a valid email (trimmed in the payload) and shows the hear-back-by deadline for pending requests", async () => {
    await renderAtContactStep();

    fireEvent.change(getEmailInput(), {
      target: { value: "  jane@example.com  " },
    });
    fireEvent.click(getSubmitButton());

    await waitFor(() => {
      expect(mockCreateReservation).toHaveBeenCalledTimes(1);
    });
    expect(mockCreateReservation.mock.calls[0][1]).toMatchObject({
      customer_email: "jane@example.com",
    });

    // Pending confirmation modal surfaces the approval deadline via
    // formatBusinessDateTime (business TZ; UTC when no timezone is set) —
    // not the viewer's local toLocaleString (see venue-timezone fix).
    const expectedDeadline = formatBusinessDateTime(
      mockApprovalDeadline,
      "en",
      null,
      DATE_TIME_SHORT,
    );
    await screen.findByText(
      `The restaurant reviews each request. You'll hear back by ${expectedDeadline}.`,
    );
  });
});
