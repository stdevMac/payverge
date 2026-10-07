/** @jest-environment jsdom */
/**
 * Fix 4: after the first successful load, a background refresh must not swap
 * the list for ReservationsSkeleton.
 */
import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) =>
    key === "common.loadingReservations"
      ? "Loading reservations…"
      : key === "businessDashboard.reservations.refreshing"
        ? "Refreshing…"
        : key,
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

function makeSample(name = "Pat Guest") {
  return {
    id: 7,
    business_id: 42,
    customer_name: name,
    customer_phone: "555-0100",
    party_size: 2,
    reservation_time: new Date().toISOString(),
    duration: 90,
    status: "confirmed" as const,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  };
}

let resolveSecondPage: ((value: unknown) => void) | null = null;
let getReservationsCall = 0;

const mockGetReservations = jest.fn(
  (
    _businessId: number,
    startDate?: string,
    _endDate?: string,
    status?: string,
  ) => {
    if (status === "pending" && startDate === undefined) {
      return Promise.resolve({
        reservations: [],
        total: 0,
        page: 1,
        page_size: 100,
        total_pages: 1,
      });
    }
    getReservationsCall += 1;
    if (getReservationsCall === 1) {
      return Promise.resolve({
        reservations: [makeSample()],
        total: 1,
        page: 1,
        page_size: 25,
        total_pages: 1,
      });
    }
    return new Promise((resolve) => {
      resolveSecondPage = resolve;
    });
  },
);

jest.mock("@/api/reservations", () => ({
  reservationAPI: {
    getReservations: (...args: unknown[]) =>
      (mockGetReservations as (...a: unknown[]) => unknown)(...args),
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
    getStats: jest.fn(() =>
      Promise.resolve({
        total: 1,
        pending: 0,
        confirmed: 1,
        waitlist: 0,
        seated: 0,
        completed: 0,
        cancelled: 0,
        no_show: 0,
      }),
    ),
    updateReservation: jest.fn(),
    createReservation: jest.fn(),
    cancelReservation: jest.fn(),
    checkIn: jest.fn(),
    markNoShow: jest.fn(),
    promoteWaitlist: jest.fn(),
    assignTable: jest.fn(),
    claimReservation: jest.fn(() => Promise.resolve({})),
    releaseReservation: jest.fn(() => Promise.resolve({})),
  },
  getAllUpcomingReservations: jest.fn(() =>
    Promise.resolve({
      items: [
        {
          id: 7,
          business_id: 42,
          customer_name: "Pat Guest",
          party_size: 2,
          reservation_time: new Date().toISOString(),
          duration: 90,
          status: "confirmed",
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        },
      ],
      metadata: { total: 1, page: 1, page_size: 100, total_pages: 1 },
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
}));

let sseOnEvent:
  | ((event: { type: string; data?: Record<string, unknown> }) => void)
  | null = null;

jest.mock("@/hooks/useSSEEvents", () => ({
  useSSEEvents: (opts: {
    onEvent: (event: { type: string; data?: Record<string, unknown> }) => void;
  }) => {
    sseOnEvent = opts.onEvent;
    return { retriesExhausted: false, reconnect: jest.fn() };
  },
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
import { getAllUpcomingReservations } from "@/api/reservations";
import { fireEvent } from "@testing-library/react";

describe("ReservationManager — background refresh (Fix 4)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    getReservationsCall = 0;
    resolveSecondPage = null;
    sseOnEvent = null;
  });

  it("keeps prior rows visible and does not remount the skeleton on refresh", async () => {
    render(<ReservationManager businessId={42} />);

    await waitFor(() => {
      expect(screen.getByText("Pat Guest")).toBeTruthy();
    });

    expect(screen.queryByText(/loading reservations/i)).toBeNull();

    await act(async () => {
      sseOnEvent?.({ type: "reservation.updated" });
      await Promise.resolve();
    });

    expect(screen.getByText("Pat Guest")).toBeTruthy();
    expect(screen.queryByText(/loading reservations/i)).toBeNull();
    expect(screen.getByTestId("reservations-refreshing")).toBeTruthy();

    await act(async () => {
      resolveSecondPage?.({
        reservations: [makeSample("Pat Guest Refreshed")],
        total: 1,
        page: 1,
        page_size: 25,
        total_pages: 1,
      });
      await Promise.resolve();
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(screen.getByText("Pat Guest Refreshed")).toBeTruthy();
    });
    expect(screen.queryByTestId("reservations-refreshing")).toBeNull();
  });

  it("shows the skeleton, not the empty state, when switching to board before its hydrate resolves", async () => {
    // Defer the board hydrate so we can observe the in-flight state.
    let resolveHydrate: ((value: unknown) => void) | null = null;
    (getAllUpcomingReservations as jest.Mock).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveHydrate = resolve;
        }),
    );

    render(<ReservationManager businessId={42} />);

    // List view loads via the paged path first.
    await waitFor(() => {
      expect(screen.getByText("Pat Guest")).toBeTruthy();
    });

    // Switch to the kanban board — its data source (complete hydrate) has
    // never loaded, so the surface must show the skeleton while it loads,
    // never a dishonest "no reservations" empty state.
    fireEvent.click(
      screen.getByRole("button", { name: /viewToggle\.board$|^Board$/ }),
    );

    await waitFor(() => {
      expect(screen.getByText(/loading reservations/i)).toBeTruthy();
    });
    expect(screen.queryByText(/empty\.title$/)).toBeNull();
    expect(screen.queryByText(/empty\.noMatchTitle$/)).toBeNull();

    await act(async () => {
      resolveHydrate?.({
        items: [makeSample("Board Guest")],
        metadata: { total: 1, page: 1, page_size: 100, total_pages: 1 },
        capped: false,
      });
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(screen.getByText("Board Guest")).toBeTruthy();
    });
    expect(screen.queryByText(/loading reservations/i)).toBeNull();
  });
});
