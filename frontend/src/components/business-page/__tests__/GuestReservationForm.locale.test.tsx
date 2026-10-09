/** @jest-environment jsdom */
import { formatTimeSlot } from "../GuestReservationForm";

describe("formatTimeSlot — guest-locale formatting (I18N-4)", () => {
  const iso = "2026-06-20T18:30:00Z";

  it("formats with an explicit locale (de uses 24h, no AM/PM)", () => {
    const de = formatTimeSlot(iso, "UTC", "de");
    expect(de).not.toMatch(/AM|PM/i);
  });

  it("formats with en-US 12h when the locale is en", () => {
    const en = formatTimeSlot(iso, "UTC", "en-US");
    expect(en).toMatch(/PM/i);
  });
});
