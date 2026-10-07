/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { computeAccessibleName } from "dom-accessibility-api";
import {
  announcedSwitchName,
  labelledByTargetsHaveText,
} from "@/components/ui/namedControl";

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

const baseSettings = {
  id: 1,
  business_id: 7,
  enabled: true,
  min_party_size: 1,
  max_party_size: 10,
  max_advance_days: 30,
  default_duration: 90,
  min_advance_minutes: 30,
  slot_interval_minutes: 15,
  service_buffer_minutes: 0,
  max_covers_per_slot: 50,
  auto_assign_tables: true,
  approval_mode: "auto" as const,
  allow_waitlist: true,
  hold_duration_minutes: 15,
  allow_cancellation: true,
  cancellation_deadline: 2,
  no_show_grace_minutes: 15,
  send_confirmation_email: true,
  send_reminder_email: false,
  reminder_hours_before: 2,
  external_partner_links: [],
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

jest.mock("@/api/reservations", () => ({
  reservationAPI: {
    getSettings: () => Promise.resolve({ ...baseSettings }),
    updateSettings: jest.fn(),
  },
}));

import ReservationSettings from "@/components/business/ReservationSettings";

describe("ReservationSettings accessible names (#446)", () => {
  it("names policy switches by purpose and state, and approval by purpose plus value", async () => {
    render(<ReservationSettings businessId={7} />);

    const autoAssign = await screen.findByRole("switch", {
      name: "Auto-assign tables",
    });
    expect(announcedSwitchName(autoAssign)).toBe("Auto-assign tables, on");
    expect(labelledByTargetsHaveText(autoAssign)).toBe(true);
    expect(autoAssign.getAttribute("aria-describedby")).toBeTruthy();

    expect(
      announcedSwitchName(screen.getByRole("switch", { name: "Allow waitlist" })),
    ).toBe("Allow waitlist, on");
    expect(
      announcedSwitchName(
        screen.getByRole("switch", { name: "Allow Cancellations" }),
      ),
    ).toBe("Allow Cancellations, on");
    expect(
      announcedSwitchName(
        screen.getByRole("switch", { name: "Send Confirmation Emails" }),
      ),
    ).toBe("Send Confirmation Emails, on");
    expect(
      announcedSwitchName(
        screen.getByRole("switch", { name: "Send Reminder Emails" }),
      ),
    ).toBe("Send Reminder Emails, off");

    const approval = screen.getByRole("button", {
      name: "Booking approval, Auto-confirm bookings",
    });
    expect(computeAccessibleName(approval)).toBe(
      "Booking approval, Auto-confirm bookings",
    );
    expect(labelledByTargetsHaveText(approval)).toBe(true);
  });
});
