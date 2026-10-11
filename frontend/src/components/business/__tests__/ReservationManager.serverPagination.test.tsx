/** @jest-environment jsdom */
/**
 * Fix 1: default "today" quick filter must take the server-paginated path
 * (reservationAPI.getReservations) instead of the 1000-cap hydrate
 * (getAllUpcomingReservations).
 */
import React from "react";
import { render, screen, waitFor, within } from "@testing-library/react";
import { localDateKey } from "@/lib/localDate";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    const leaf = key.replace(/^businessDashboard\.reservations\./, "");
    const map: Record<string, string> = {
      "shell.reservationCountLabel": "reservation",
      "shell.reservationCountLabel_other": "reservations",
      "shell.coversCountLabel": "covers",
      "shell.todayReservations": "Reservations today",
      "insights.coversToday": "Covers today",
      "insights.needsTable": "Needs table",
      "insights.waitlist": "Waitlist",
      "insights.nextArrival": "Next arrival",
      "insights.none": "None",
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

type GetReservationsArgs = [
  businessId: number,
  startDate?: string,
  endDate?: string,
  status?: string,
  options?: { page?: number; pageSize?: number },
];

const mockGetReservations = jest.fn<
  Promise<{
    reservations: unknown[];
    total: number;
    page: number;
    page_size: number;
    total_pages: number;
  }>,
  GetReservationsArgs
>(() =>
  Promise.resolve({
    reservations: [],
    total: 0,
    page: 1,
    page_size: 25,
    total_pages: 1,
  }),
);

const mockGetAllUpcoming = jest.fn(
  (
    _businessId?: number,
    _args?: {
      startDate?: string;
      endDate?: string;
      status?: string;
      pageSize?: number;
      maxPages?: number;
      maxRows?: number;
    },
  ) =>
    Promise.resolve({
      items: [],
      metadata: { total: 0, page: 1, page_size: 100, total_pages: 1 },
      capped: false,
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
    getStats: jest.fn(() =>
      Promise.resolve({
        total: 12,
        pending: 0,
        confirmed: 12,
        waitlist: 0,
        seated: 0,
        completed: 0,
        cancelled: 0,
        no_show: 0,
        // True covers (guest seats) from the server aggregate — deliberately
        // different from any booking count so the KPI test can prove the UI
        // renders covers, not bookings.
        covers: 37,
        needs_table: 5,
      }),
    ),
  },
  getAllUpcomingReservations: (
    businessId: number,
    args?: {
      startDate?: string;
      endDate?: string;
      status?: string;
      pageSize?: number;
      maxPages?: number;
      maxRows?: number;
    },
  ) => mockGetAllUpcoming(businessId, args),
}));

jest.mock("@/api/business", () => ({
  businessApi: {
    getBusinessTables: jest.fn(() => Promise.resolve({ tables: [] })),
  },
  getBusiness: jest.fn(() =>
    Promise.resolve({ default_currency: "USD", timezone: "UTC" }),
  ),
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

describe("ReservationManager — server pagination default path (Fix 1)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("loads the default list via getReservations (paged), not the 1000-cap hydrate", async () => {
    render(<ReservationManager businessId={42} />);

    await waitFor(() => {
      expect(mockGetReservations).toHaveBeenCalled();
    });

    const today = localDateKey(new Date());
    const listCall = mockGetReservations.mock.calls.find((call) => {
      const [, start, end, status] = call;
      return (
        start === today &&
        end === today &&
        (status === "all" || status === undefined || status === "")
      );
    });
    expect(listCall).toBeDefined();
    expect(listCall?.[0]).toBe(42);
    expect(listCall?.[1]).toBe(today);
    expect(listCall?.[2]).toBe(today);
  });

  it("does not invoke the complete hydrate for default list filters", async () => {
    render(<ReservationManager businessId={42} />);

    await waitFor(() => {
      expect(mockGetReservations).toHaveBeenCalled();
    });

    // Allow a short settle window so a mistaken complete load would fire.
    await new Promise((r) => setTimeout(r, 50));

    // Fix 1 + Fix 6: complete hydrate gated off for list + server filters;
    // KPIs use getStats, so getAllUpcoming should not fire on the default path.
    expect(mockGetAllUpcoming.mock.calls.length).toBe(0);
  });

  it("renders true covers (guest seats) from the stats aggregate, not a booking count", async () => {
    render(<ReservationManager businessId={42} />);

    // Stats mock: 12 bookings, 37 covers (SUM party_size), 5 needing tables.
    // Header intentionally shows both ("12 reservations · 37 covers"); the
    // insight KPI labeled "Covers today" must use the seats aggregate, not
    // the booking count that happens to be 12.
    // The KPI label renders before the stats query resolves, so wait for the
    // aggregate value itself, not just the label.
    await waitFor(() => {
      const insight = screen.getByText("Covers today").closest("div");
      expect(insight).not.toBeNull();
      expect(
        within(insight as HTMLElement).getByText("37"),
      ).toBeInTheDocument();
    });

    const coversInsight = screen.getByText("Covers today").closest("div");
    expect(within(coversInsight as HTMLElement).queryByText("12")).toBeNull();

    const needsInsight = screen.getByText("Needs table").closest("div");
    expect(needsInsight).not.toBeNull();
    expect(
      within(needsInsight as HTMLElement).getByText("5"),
    ).toBeInTheDocument();

    // Booking count may appear in the header under its own reservation label.
    const bookingStat = screen.getByTitle("Reservations today");
    expect(within(bookingStat).getByText("12")).toBeInTheDocument();
    expect(within(bookingStat).getByText("reservations")).toBeInTheDocument();

    const headerCovers = screen.getByTitle("Covers today");
    expect(within(headerCovers).getByText("37")).toBeInTheDocument();
    expect(within(headerCovers).getByText("covers")).toBeInTheDocument();
    expect(within(headerCovers).queryByText("12")).toBeNull();
  });
});
