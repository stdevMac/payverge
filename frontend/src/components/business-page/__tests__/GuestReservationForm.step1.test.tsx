/** @jest-environment jsdom */
/**
 * Regression-lock test for GuestReservationForm party/date section.
 *
 * Task 1 live browser diagnosis (2026-06-02) found BOTH defects (#9 party-size
 * popover overlap, #10 chips not populating date) to be ABSENT on production.
 * These tests lock the correct behavior so a future regression is caught
 * immediately. Updated for the single-page Book flow (#85).
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import GuestReservationForm from "../GuestReservationForm";
import { localDateKey } from "@/lib/localDate";

// ── Mocks ─────────────────────────────────────────────────────────────────────

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

// ── Helpers ───────────────────────────────────────────────────────────────────

async function renderForm() {
  const result = render(
    <GuestReservationForm
      customUrl="test"
      businessName="Test"
    />,
  );
  // Wait for settings load — when the mock t() returns keys unchanged,
  // reservationT() falls through to the English fallbackCopy values.
  // "Tomorrow" is the fallbackCopy.quickTomorrow label.
  await screen.findByText("Tomorrow");
  return result;
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("GuestReservationForm — party/date regression locks", () => {
  it("clicking the Tomorrow chip populates the date input; primary CTA is Book", async () => {
    const { container } = await renderForm();

    const dateInput = container.querySelector('input[type="date"]') as HTMLInputElement;
    expect(dateInput).not.toBeNull();
    expect(dateInput.value).toBe("");

    // Single-page flow: primary CTA is Book (not Continue).
    expect(screen.queryByRole("button", { name: /^Continue$/ })).toBeNull();
    const bookButton = screen.getByRole("button", { name: /^Book$/ });
    expect(
      bookButton.hasAttribute("disabled") ||
        bookButton.getAttribute("aria-disabled") === "true",
    ).toBe(true);

    // Click the Tomorrow chip (fallbackCopy.quickTomorrow = "Tomorrow")
    const tomorrowBtn = screen.getByText("Tomorrow");
    fireEvent.click(tomorrowBtn);

    // LOCAL calendar day, mirroring the component's quickDateOptions →
    // localDateKey. The previous toISOString() (UTC day) math failed when run
    // 21:00–24:00 local in UTC-negative timezones.
    const tomorrow = new Date();
    tomorrow.setDate(tomorrow.getDate() + 1);
    const expectedDate = localDateKey(tomorrow);

    await waitFor(() => {
      expect(dateInput.value).toBe(expectedDate);
    });
  });

  it("party-size Select trigger and listbox are present and queryable (popover not clipped)", async () => {
    const { container } = await renderForm();

    const selectTrigger = container.querySelector("[data-slot='trigger']");
    expect(selectTrigger).not.toBeNull();

    expect(container.innerHTML).not.toMatch(/z-index:\s*\d/);
  });

  it("does not repeat the reservation title as both eyebrow and heading (#508)", async () => {
    await renderForm();
    expect(screen.getAllByText("Reserve Your Table")).toHaveLength(1);
  });

  it("associates the visible party-size label with the Select trigger", async () => {
    const { container } = await renderForm();
    const selectTrigger = container.querySelector("[data-slot='trigger']");

    expect(selectTrigger).not.toBeNull();
    expect(selectTrigger).toHaveAccessibleName("Party Size");
  });

  it("does not emit the React Aria missing-label warning", async () => {
    const warnMock = console.warn as jest.Mock;
    warnMock.mockClear();

    await renderForm();

    expect(
      warnMock.mock.calls.some((args) =>
        args.some((arg: unknown) =>
          String(arg).includes(
            "If you do not provide a visible label, you must specify an aria-label or aria-labelledby attribute",
          ),
        ),
      ),
    ).toBe(false);
  });

  it("party-size options have visible labels, not blank rows (L-4)", async () => {
    const { container } = await renderForm();
    const select = container.querySelector("select") as HTMLSelectElement;
    expect(select).not.toBeNull();
    const realOptions = Array.from(select.options).filter((o) => o.value !== "");
    expect(realOptions.length).toBeGreaterThan(0);
    realOptions.forEach((o) => {
      expect(o.textContent?.trim()).not.toBe("");
    });
  });
});
