/** @jest-environment jsdom */
/**
 * D1 / L1-7: "All including past" must request explicit far-past→horizon bounds
 * (not undefined start that BE treats as today→horizon). Assert the wire args
 * from ReservationManager after the date Select changes — not only the pure
 * computeReservationDateRange helper.
 */
import React from "react";
import { fireEvent, render, waitFor } from "@testing-library/react";
import { localDateKey } from "@/lib/localDate";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    if (key === "businessDashboard.reservations.dateFilter")
      return "Date filter";
    if (key === "businessDashboard.reservations.filters.allIncludingPast") {
      return "All including past";
    }
    if (key === "businessDashboard.reservations.filters.today") return "Today";
    if (key === "businessDashboard.reservations.filters.upcoming") {
      return "Upcoming";
    }
    return key;
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

type GetReservationsArgs = Parameters<
  typeof import("@/api/reservations").reservationAPI.getReservations
>;

const mockGetReservations = jest.fn((..._args: GetReservationsArgs) =>
  Promise.resolve({
    reservations: [],
    total: 0,
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
        max_advance_days: 45,
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
    getStats: jest.fn(() =>
      Promise.resolve({
        total: 0,
        pending: 0,
        confirmed: 0,
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
      items: [],
      metadata: { total: 0, page: 1, page_size: 100, total_pages: 1 },
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
import { getDateFilterSelect } from "./_reservationManagerTestUtils";

describe("ReservationManager L1-7 all_time wire bounds (D1)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("selecting all_time loads with start 2000-01-01 through bookable horizon", async () => {
    const { container } = render(<ReservationManager businessId={48} />);

    await waitFor(() => expect(mockGetReservations).toHaveBeenCalled());

    const nativeSelect = getDateFilterSelect(container);
    const options = Array.from(nativeSelect.querySelectorAll("option")).map(
      (o) => o.value,
    );
    // Retired "all" key must not appear in the picker (duplicate of all_time).
    expect(options).not.toContain("all");
    expect(options).toContain("all_time");

    fireEvent.change(nativeSelect, { target: { value: "all_time" } });

    await waitFor(() => {
      const allTimeCall = mockGetReservations.mock.calls.find(
        (c) => c[1] === "2000-01-01",
      );
      expect(allTimeCall).toBeDefined();
      const end = allTimeCall?.[2] as string;
      // Horizon max(30, max_advance_days=45) = 45 days out.
      const expectedEnd = localDateKey(
        new Date(Date.now() + 45 * 24 * 60 * 60 * 1000),
      );
      // Allow ±1 day for timezone/offset math.
      const endMs = new Date(end + "T12:00:00").getTime();
      const expectMs = new Date(expectedEnd + "T12:00:00").getTime();
      expect(Math.abs(endMs - expectMs)).toBeLessThanOrEqual(
        2 * 24 * 60 * 60 * 1000,
      );
      expect(allTimeCall?.[1]).toBe("2000-01-01");
      expect(allTimeCall?.[2]).toBeTruthy();
    });
  });

  it("upcoming remains today→horizon, not the far-past floor", async () => {
    const { container } = render(<ReservationManager businessId={48} />);
    await waitFor(() => expect(mockGetReservations).toHaveBeenCalled());

    fireEvent.change(getDateFilterSelect(container), {
      target: { value: "upcoming" },
    });

    await waitFor(() => {
      const upcoming = mockGetReservations.mock.calls.find(
        (c) =>
          typeof c[1] === "string" &&
          typeof c[2] === "string" &&
          c[1] !== "2000-01-01" &&
          c[2] !== c[1] &&
          c[2] !== "2000-01-01",
      );
      expect(upcoming).toBeDefined();
      expect(upcoming?.[1]).not.toBe("2000-01-01");
    });
  });
});
