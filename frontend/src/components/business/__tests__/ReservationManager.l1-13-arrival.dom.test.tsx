/** @jest-environment jsdom */
/**
 * D1 / L1-13: seat / no-show must not appear on future-dated confirmed rows.
 * Asserts the mounted action buttons on ReservationManager list view.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    const leaf = key.replace(/^businessDashboard\.reservations\./, "");
    const map: Record<string, string> = {
      seat: "Seat",
      markSeated: "Mark seated",
      noShow: "No-show",
      markNoShow: "Mark no-show",
      moreReservationActionsAria: "More actions",
      viewReservationAria: "View",
      guests: "guests",
      "serviceDetails.unassigned": "Unassigned",
    };
    return map[leaf] ?? key;
  },
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({
    access: null,
    loading: false,
    error: null,
    hasAccess: true,
    isSuspended: false,
    lockState: "active",
    aiConfigured: false,
    refetch: jest.fn(),
  }),
}));

function mockMakeRes(id: number, reservation_time: string, name: string) {
  return {
    id,
    business_id: 48,
    customer_name: name,
    customer_phone: "5550001111",
    party_size: 2,
    reservation_time,
    duration: 90,
    status: "confirmed" as const,
    created_at: reservation_time,
    updated_at: reservation_time,
  };
}

const mockNowIso = new Date().toISOString();
const mockFutureDate = new Date();
mockFutureDate.setDate(mockFutureDate.getDate() + 5);
const mockFutureIso = mockFutureDate.toISOString();

type GetReservationsArgs = Parameters<
  typeof import("@/api/reservations").reservationAPI.getReservations
>;

const mockGetReservations = jest.fn((..._args: GetReservationsArgs) =>
  Promise.resolve({
    reservations: [
      mockMakeRes(1, mockNowIso, "Now Guest"),
      mockMakeRes(2, mockFutureIso, "Future Guest"),
    ],
    total: 2,
    page: 1,
    page_size: 25,
    total_pages: 1,
  }),
);

jest.mock("@/api/reservations", () => ({
  reservationAPI: {
    getReservations: (...args: GetReservationsArgs) =>
      mockGetReservations(...args),
    getSettings: () =>
      Promise.resolve({
        enabled: true,
        min_party_size: 1,
        max_party_size: 10,
        max_advance_days: 30,
        default_duration: 90,
        min_advance_minutes: 30,
        slot_interval_minutes: 15,
        max_covers_per_slot: 50,
        hold_duration_minutes: 15,
        service_buffer_minutes: 0,
        no_show_grace_minutes: 15,
        reminder_hours_before: 2,
        cancellation_deadline: 2,
        allow_cancellation: true,
        allow_waitlist: true,
        auto_assign_tables: false,
        approval_mode: "auto",
        send_confirmation_email: true,
        send_reminder_email: true,
        external_partner_links: [],
      }),
    getReservation: jest.fn(),
    updateReservation: jest.fn(),
    createReservation: jest.fn(),
    cancelReservation: jest.fn(),
    checkIn: jest.fn(),
    markNoShow: jest.fn(),
    promoteWaitlist: jest.fn(),
    assignTable: jest.fn(),
    claimReservation: jest.fn(() => Promise.resolve({})),
    releaseReservation: jest.fn(() => Promise.resolve({})),
    getTableOptions: jest.fn(() => Promise.resolve({ tables: [] })),
    getStats: jest.fn(() =>
      Promise.resolve({
        total: 2,
        pending: 0,
        confirmed: 2,
        waitlist: 0,
        seated: 0,
        completed: 0,
        cancelled: 0,
        no_show: 0,
      }),
    ),
  },
  getAllUpcomingReservations: jest.fn(() =>
    Promise.resolve({
      items: [
        mockMakeRes(1, mockNowIso, "Now Guest"),
        mockMakeRes(2, mockFutureIso, "Future Guest"),
      ],
      metadata: { total: 2, page: 1, page_size: 100, total_pages: 1 },
      capped: false,
    }),
  ),
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusinessTables: jest.fn(() => Promise.resolve({ tables: [] })),
  },
  getBusiness: jest.fn(() =>
    Promise.resolve({ default_currency: "USD", timezone: "UTC" }),
  ),
  getBusinessOperatingHours: jest.fn(() => Promise.resolve([])),
}));

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: () => ({ retriesExhausted: false, reconnect: jest.fn() }),
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: null, isStaffUser: false }),
}));

jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: [],
    rolePermissions: [],
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: jest.fn(),
  }),
}));

jest.mock("../ReservationToggle", () => ({
  __esModule: true,
  default: () => null,
  useReservationStatus: () => ({
    enabled: true,
    loading: false,
    setEnabled: jest.fn(),
  }),
}));

import ReservationManager from "../ReservationManager";
import userEvent from "@testing-library/user-event";

describe("ReservationManager L1-13 arrival actions DOM (D1)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("shows seat for current-slot confirmed row, hides seat for future-dated row", async () => {
    const user = userEvent.setup();
    render(<ReservationManager businessId={48} />);

    await waitFor(() => {
      expect(screen.getByText("Now Guest")).toBeInTheDocument();
      expect(screen.getByText("Future Guest")).toBeInTheDocument();
    });

    // Open the row-actions menu for the current-time guest.
    const nowMenu = screen.getByLabelText(/More actions.*Now Guest/i);
    await user.click(nowMenu);

    await waitFor(() => {
      // Owner (non-staff) has canEdit — seat should appear for current slot.
      const seatItem =
        screen.queryByRole("menuitem", { name: /seat|mark seated/i }) ||
        screen.queryByText(/Seat|Mark seated/i);
      expect(seatItem).toBeTruthy();
    });

    // Close menu if open, open future guest menu.
    await user.keyboard("{Escape}");

    const futureMenu = screen.getByLabelText(/More actions.*Future Guest/i);
    await user.click(futureMenu);

    await waitFor(() => {
      // Future-dated: no seat / no-show in the menu for that row.
      const items = screen.queryAllByRole("menuitem");
      const labels = items.map((el) => (el.textContent || "").toLowerCase());
      expect(
        labels.some(
          (t) =>
            t.includes("seat") ||
            t.includes("no-show") ||
            t.includes("no show"),
        ),
      ).toBe(false);
    });
  });
});
