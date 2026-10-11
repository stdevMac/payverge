/** @jest-environment jsdom */
/**
 * #681: a hard navigation to `?tab=reservations` rendered
 * "Reservaciones · Reservas pausadas" over a completely empty body.
 *
 * Two independent reads of `/reservations/settings` back that screen — the
 * `useReservationStatus` hook (header pill + tab content) and the
 * `ReservationToggle` card the hook renders when it believes bookings are
 * off. The tab can only paint "paused + blank" when those two disagree:
 *
 *   1. the hook's single read fails on the cold-load frame and is reported as
 *      a confident `enabled: false` (axios no longer retries GETs and this
 *      hook is not React Query backed, so nothing re-reads it), and
 *   2. the card then renders `null` — either because its own read succeeds
 *      with `enabled: true`, or because `isLocked` is still derived from a
 *      `hasAccess` the access-gate query has not resolved yet.
 *
 * Both vectors are asserted here against the real component tree.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "es", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    const map: Record<string, string> = {
      "businessDashboard.reservations.title": "Reservaciones",
      "businessDashboard.reservations.shell.disabled": "Reservas pausadas",
      "businessDashboard.reservations.shell.enabled": "Reservas activas",
      "businessDashboard.reservations.createReservation": "Crear Reservación",
      "businessDashboard.reservations.toggle.title": "Activar reservas",
      "businessDashboard.reservations.toggle.description":
        "Recibí reservas desde tu página.",
      "businessDashboard.reservations.toggle.enable": "Activar",
      "common.loadingReservations": "Cargando reservaciones…",
    };
    return map[key] ?? key;
  },
}));

const mockAccessState = {
  access: null,
  loading: false,
  error: null as string | null,
  hasAccess: true,
  isSuspended: false,
  lockState: "active",
  aiConfigured: false,
  refetch: jest.fn(),
};

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => mockAccessState,
}));

const settingsBody = {
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

/** Rejects the first `failures` reads, then answers with `enabled`. */
let mockSettingsFailuresLeft = 0;
let mockSettingsEnabled = true;
const mockGetSettings = jest.fn(() => {
  if (mockSettingsFailuresLeft > 0) {
    mockSettingsFailuresLeft -= 1;
    return Promise.reject(new Error("Request failed with status code 429"));
  }
  return Promise.resolve({ ...settingsBody, enabled: mockSettingsEnabled });
});

jest.mock("@/api/reservations", () => ({
  reservationAPI: {
    getSettings: (...args: unknown[]) =>
      mockGetSettings(...(args as Parameters<typeof mockGetSettings>)),
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

describe("ReservationManager settings-read failure (#681)", () => {
  beforeEach(() => {
    mockGetSettings.mockClear();
    mockSettingsFailuresLeft = 0;
    mockSettingsEnabled = true;
    mockAccessState.loading = false;
    mockAccessState.hasAccess = true;
  });

  it("recovers from a transient cold-load read instead of latching Reservas pausadas", async () => {
    mockSettingsFailuresLeft = 1;

    render(<ReservationManager businessId={85} />);

    await waitFor(
      () => {
        expect(screen.getByText("Reservas activas")).toBeInTheDocument();
      },
      { timeout: 4000 },
    );
    expect(screen.queryByText("Reservas pausadas")).not.toBeInTheDocument();
    expect(screen.getByTestId("reservation-create-button")).toBeInTheDocument();
  });

  it("never paints a paused header over an empty body when reads keep failing", async () => {
    // Every read the hook makes fails; the card's own read then succeeds with
    // enabled=true. The card must not silently render nothing.
    mockSettingsFailuresLeft = 3;

    render(<ReservationManager businessId={85} />);

    await waitFor(
      () => {
        expect(screen.getByText("Reservas activas")).toBeInTheDocument();
      },
      { timeout: 4000 },
    );
    expect(screen.getByTestId("reservation-create-button")).toBeInTheDocument();
  });

  it("keeps the activation card on screen while the access gate is still resolving", async () => {
    // Genuinely paused venue, cold access-gate query: `hasAccess` is still false
    // because the access fetch has not answered. The tab header already renders, so the
    // body must render the activation card, not nothing.
    mockSettingsEnabled = false;
    mockAccessState.loading = true;
    mockAccessState.hasAccess = false;

    render(<ReservationManager businessId={85} />);

    await waitFor(() => {
      expect(screen.getByText("Reservas pausadas")).toBeInTheDocument();
    });
    expect(
      await screen.findByRole(
        "heading",
        { name: "Activar reservas" },
        { timeout: 4000 },
      ),
    ).toBeInTheDocument();
  });
});
