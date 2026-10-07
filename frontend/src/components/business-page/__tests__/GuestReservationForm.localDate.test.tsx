/** @jest-environment jsdom */
/**
 * F16: the reservation date picker derived "today" from
 * new Date().toISOString().split("T")[0] (UTC), which blocks same-day
 * booking for guests behind UTC in the evening (US Pacific/Mountain/etc.).
 *
 * The date input min= and the "Today" quick button must reflect the guest's
 * LOCAL calendar day (via localDateKey), never the UTC day. We pin "now" to a
 * late-evening instant and, when the test runner is not on UTC, assert the
 * min/Today value diverges from the buggy toISOString day.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import GuestReservationForm from "../GuestReservationForm";
import { localDateKey } from "@/lib/localDate";

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t: (k: string) => k }),
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
  error: jest.fn(),
  success: jest.fn(),
}));

jest.mock("@/api/reservations", () => ({
  guestReservationAPI: {
    getSettings: jest.fn().mockResolvedValue({
      enabled: true,
      min_party_size: 1,
      max_party_size: 8,
      max_advance_days: 30,
      min_advance_minutes: 0,
      approval_mode: "auto",
      allow_cancellation: false,
      allow_waitlist: false,
      default_duration: 60,
      cancellation_deadline: 24,
      external_partner_links: [],
    }),
    getAvailability: jest.fn().mockResolvedValue({
      available_slots: [],
      waitlist_available: false,
      next_available_slot: null,
    }),
  },
}));

describe("GuestReservationForm local-day date keys (F16)", () => {
  const RealDate = Date;
  // 23:30 wall-clock on the local "today" so the instant is late-evening in
  // any negative-offset zone, where UTC has already rolled to the next day.
  const PINNED = new RealDate(2026, 5, 10, 23, 30, 0).getTime();

  beforeEach(() => {
    // Pin "now" without fake timers so RTL findBy / async load resolve on real
    // microtasks. new Date(arg) keeps working; only zero-arg now is fixed.
    global.Date = class extends RealDate {
      constructor(...args: unknown[]) {
        if (args.length === 0) {
          super(PINNED);
        } else {
          // @ts-expect-error spread into Date constructor
          super(...args);
        }
      }
      static now() {
        return PINNED;
      }
    } as DateConstructor;
  });
  afterEach(() => {
    global.Date = RealDate;
  });

  it("min date and Today button reflect the local day (localDateKey), not UTC", async () => {
    const localDay = localDateKey(new RealDate(PINNED)); // expected local day
    const utcDay = new RealDate(PINNED).toISOString().split("T")[0]; // buggy idiom

    const { container } = render(
      <GuestReservationForm
        customUrl="test"
        businessName="Test"
      />,
    );

    await screen.findByText("Today");

    const dateInput = container.querySelector(
      'input[type="date"]',
    ) as HTMLInputElement;
    expect(dateInput).not.toBeNull();

    // Always: min reflects the LOCAL calendar day.
    expect(dateInput.getAttribute("min")).toBe(localDay);

    // Clicking "Today" selects the local day.
    fireEvent.click(screen.getByText("Today"));
    await waitFor(() => {
      expect(dateInput.value).toBe(localDay);
    });

    // On a non-UTC runner the buggy UTC day would differ; prove we are not
    // using it. (On a UTC runner localDay === utcDay and this is a no-op.)
    if (utcDay !== localDay) {
      expect(dateInput.getAttribute("min")).not.toBe(utcDay);
    }
  });
});
