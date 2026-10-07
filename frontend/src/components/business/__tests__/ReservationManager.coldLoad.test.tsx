/** @jest-environment jsdom */
/**
 * #681: cold-load must not flash "Reservas pausadas" / "Bookings paused"
 * while reservation settings (enabled: true) are still in flight, including
 * the unlock frame after the access-gate hasAccess flip.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    if (key === "common.loadingReservations") return "Cargando reservaciones…";
    if (key === "businessDashboard.reservations.shell.disabled") {
      return "Reservas pausadas";
    }
    if (key === "businessDashboard.reservations.shell.enabled") {
      return "Reservas activas";
    }
    if (key === "businessDashboard.reservations.createReservation") {
      return "Crear Reservación";
    }
    if (key === "businessDashboard.reservations.title") {
      return "Reservaciones";
    }
    return key;
  },
}));

const mockAccessState = {
  access: null,
  loading: true,
  error: null as string | null,
  hasAccess: false,
  isSuspended: false,
  lockState: "active",
  aiConfigured: false,
  refetch: jest.fn(),
};

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => mockAccessState,
}));

const enabledSettings = {
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
};

let resolveSettings: (value: typeof enabledSettings) => void = () => {};

jest.mock("@/api/reservations", () => ({
  reservationAPI: {
    getSettings: jest.fn(
      () =>
        new Promise<typeof enabledSettings>((resolve) => {
          resolveSettings = resolve;
        }),
    ),
    getReservations: jest.fn(() =>
      Promise.resolve({
        reservations: [],
        total: 0,
        page: 1,
        page_size: 25,
        total_pages: 1,
      }),
    ),
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
        total: 0,
        pending: 0,
        confirmed: 0,
        waitlist: 0,
        seated: 0,
        completed: 0,
        cancelled: 0,
        no_show: 0,
        covers: 0,
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
    Promise.resolve({ default_currency: "USD", timezone: "America/New_York" }),
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

import ReservationManager from "../ReservationManager";

const pausedCopy = /Reservas pausadas|Bookings paused/;

describe("ReservationManager cold-load enabled gate", () => {
  it("does not flash Reservas pausadas across unlock + enabled settings", async () => {
    const { rerender } = render(<ReservationManager businessId={85} />);

    expect(screen.queryByText(pausedCopy)).not.toBeInTheDocument();

    mockAccessState.loading = false;
    mockAccessState.hasAccess = true;
    rerender(<ReservationManager businessId={85} />);

    expect(screen.queryByText(pausedCopy)).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");

    resolveSettings(enabledSettings);

    await waitFor(() => {
      expect(screen.getByText("Reservas activas")).toBeInTheDocument();
    });
    expect(screen.queryByText(pausedCopy)).not.toBeInTheDocument();
    expect(screen.getByTestId("reservation-create-button")).toBeInTheDocument();
  });
});
