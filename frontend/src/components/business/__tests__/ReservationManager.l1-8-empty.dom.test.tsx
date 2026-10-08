/** @jest-environment jsdom */
/**
 * D1 / L1-8: empty "today" must not claim first-run onboarding (jump to Settings).
 * Asserts the mounted EmptyState title/CTA from ReservationManager, not only
 * resolveReservationEmptyKind helper return values.
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { localDateKey } from "@/lib/localDate";

const tMap: Record<string, string> = {
  "businessDashboard.reservations.empty.title": "No reservations yet",
  "businessDashboard.reservations.empty.subtitle": "First-run subtitle",
  "businessDashboard.reservations.empty.action": "Set up reservations",
  "businessDashboard.reservations.empty.todayTitle": "No reservations today",
  "businessDashboard.reservations.empty.todaySubtitle": "Nothing booked today",
  "businessDashboard.reservations.empty.viewUpcomingAction": "View upcoming",
  "businessDashboard.reservations.empty.noMatchTitle":
    "No matching reservations",
  "businessDashboard.reservations.empty.noMatchSubtitle": "No match",
  "businessDashboard.reservations.empty.clearFiltersAction": "Clear filters",
  "businessDashboard.reservations.dateFilter": "Date filter",
  "businessDashboard.reservations.filters.today": "Today",
  "businessDashboard.reservations.filters.upcoming": "Upcoming",
  "businessDashboard.reservations.filters.past30Days": "Past 30 days",
  "businessDashboard.reservations.filters.customRange": "Custom",
  "businessDashboard.reservations.filters.allIncludingPast":
    "All including past",
};

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => tMap[key] ?? key,
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

const emptyPage = {
  reservations: [] as unknown[],
  total: 0,
  page: 1,
  page_size: 25,
  total_pages: 1,
};

type GetReservationsArgs = Parameters<
  typeof import("@/api/reservations").reservationAPI.getReservations
>;

const mockGetReservations = jest.fn((..._args: GetReservationsArgs) =>
  Promise.resolve(emptyPage),
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

describe("ReservationManager L1-8 empty-state DOM (D1)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGetReservations.mockResolvedValue(emptyPage);
  });

  it("default today empty shows today copy, not first-run Settings CTA", async () => {
    render(<ReservationManager businessId={48} />);

    await waitFor(() => {
      expect(screen.getByText("No reservations today")).toBeInTheDocument();
    });
    expect(screen.getByText("View upcoming")).toBeInTheDocument();
    expect(screen.queryByText("No reservations yet")).not.toBeInTheDocument();
    expect(screen.queryByText("Set up reservations")).not.toBeInTheDocument();

    // Initial list load used today's bounds.
    const today = localDateKey(new Date());
    expect(
      mockGetReservations.mock.calls.some(
        (c) => c[1] === today && c[2] === today,
      ),
    ).toBe(true);
  });

  it("all_time empty shows first-run onboarding CTA", async () => {
    const { container } = render(<ReservationManager businessId={48} />);

    await waitFor(() => {
      expect(screen.getByText("No reservations today")).toBeInTheDocument();
    });

    fireEvent.change(getDateFilterSelect(container), {
      target: { value: "all_time" },
    });

    await waitFor(() => {
      expect(screen.getByText("No reservations yet")).toBeInTheDocument();
    });
    expect(screen.getByText("Set up reservations")).toBeInTheDocument();
    expect(screen.queryByText("No reservations today")).not.toBeInTheDocument();
  });
});
