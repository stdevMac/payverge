/** @jest-environment jsdom */
/**
 * Verifies the booking-approval Select renders, defaults to the loaded
 * approval_mode, and that switching it to "manual" is persisted through
 * the save path (updateSettings receives approval_mode: "manual").
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

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
  auto_assign_tables: false,
  approval_mode: "auto" as "auto" | "manual",
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

const mockGetSettings = jest.fn(() => Promise.resolve({ ...baseSettings }));
const mockUpdateSettings = jest.fn((_businessId: number, settings: unknown) =>
  Promise.resolve({ ...(settings as object) }),
);

jest.mock("@/api/reservations", () => ({
  reservationAPI: {
    getSettings: () => mockGetSettings(),
    updateSettings: (businessId: number, settings: unknown) =>
      mockUpdateSettings(businessId, settings),
  },
}));

import ReservationSettings from "@/components/business/ReservationSettings";

describe("ReservationSettings approval mode", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  // NextUI's <Select> drives state through a visually hidden native <select>;
  // it is not reachable via label queries, so grab it off the container.
  const findHiddenSelect = async (container: HTMLElement) => {
    let select: HTMLSelectElement | null = null;
    await waitFor(() => {
      select = container.querySelector("select");
      expect(select).not.toBeNull();
    });
    return select as unknown as HTMLSelectElement;
  };

  it("renders the approval-mode select with the auto summary by default", async () => {
    const { container } = render(<ReservationSettings businessId={7} />);

    const select = await findHiddenSelect(container);
    expect(select.value).toBe("auto");
    expect(
      screen.getByText(
        "businessDashboard.reservations.settings.approvalModeAutoSummary",
      ),
    ).toBeInTheDocument();
  });

  it("changing to manual updates the summary and saves approval_mode", async () => {
    const { container } = render(<ReservationSettings businessId={7} />);

    const select = await findHiddenSelect(container);
    fireEvent.change(select, { target: { value: "manual" } });

    expect(
      screen.getByText(
        "businessDashboard.reservations.settings.approvalModeManualSummary",
      ),
    ).toBeInTheDocument();

    fireEvent.click(

      screen.getByText("businessSettings.saveBar.save"),
    );

    await waitFor(() => expect(mockUpdateSettings).toHaveBeenCalledTimes(1));
    expect(mockUpdateSettings).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ approval_mode: "manual" }),
    );
  });

  it("ignores an empty deselection event instead of flipping to auto", async () => {
    mockGetSettings.mockResolvedValueOnce({
      ...baseSettings,
      approval_mode: "manual" as const,
    });
    const { container } = render(<ReservationSettings businessId={7} />);

    const select = await findHiddenSelect(container);
    await waitFor(() => expect(select.value).toBe("manual"));

    fireEvent.change(select, { target: { value: "" } });

    expect(
      screen.getByText(
        "businessDashboard.reservations.settings.approvalModeManualSummary",
      ),
    ).toBeInTheDocument();
  });

  it("names reservation policy switches (#446)", async () => {
    render(<ReservationSettings businessId={7} />);
    expect(
      await screen.findByRole("switch", {
        name: "businessDashboard.reservations.settings.autoAssignTables",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("switch", {
        name: "businessDashboard.reservations.settings.allowWaitlist",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("switch", {
        name: "businessDashboard.reservations.settings.allowCancellation",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("switch", {
        name: "businessDashboard.reservations.settings.sendConfirmationEmail",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("switch", {
        name: "businessDashboard.reservations.settings.sendReminderEmail",
      }),
    ).toBeInTheDocument();
  });
});
