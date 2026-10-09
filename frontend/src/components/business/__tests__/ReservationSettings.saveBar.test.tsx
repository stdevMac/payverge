/** @jest-environment jsdom */
/**
 * Wave 4 Task 14: ReservationSettings adopts shared SaveBar + unsaved-changes guard.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

const mockSetDirty = jest.fn();
jest.mock("@/hooks/unsavedChangesRegistry", () => ({
  setDirty: (...a: unknown[]) => mockSetDirty(...a),
}));

const baseSettings = {
  id: 1,
  business_id: 1,
  enabled: true,
  min_party_size: 1,
  max_party_size: 10,
  max_advance_days: 30,
  default_duration: 90,
  min_advance_minutes: 30,
  slot_interval_minutes: 30,
  service_buffer_minutes: 15,
  max_covers_per_slot: 50,
  auto_assign_tables: false,
  approval_mode: "auto" as const,
  allow_waitlist: true,
  hold_duration_minutes: 15,
  allow_cancellation: true,
  cancellation_deadline: 24,
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
    getSettings: jest.fn(() => Promise.resolve({ ...baseSettings })),
    updateSettings: jest.fn(),
  },
}));

import ReservationSettings from "../ReservationSettings";

it("renders the shared SaveBar and registers the unsaved-changes guard", async () => {
  render(<ReservationSettings businessId={1} />);
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: /businessSettings\.saveBar\.save/ }),
    ).toBeInTheDocument(),
  );
  expect(mockSetDirty).toHaveBeenCalledWith("reservation-settings", false);
});
