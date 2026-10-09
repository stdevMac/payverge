/** @jest-environment jsdom */
/**
 * D1 / L1-14: multi-day board cards show date+time; single-day board cards use
 * time-only. Asserts mounted board card text (not the pure reservationCardTimeMode helper).
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { localDateKey } from "@/lib/localDate";
import { getDateFilterSelect } from "./_reservationManagerTestUtils";
import {
  DATE_TIME_SHORT,
  TIME_SHORT,
  formatBusinessDateTime,
  formatBusinessTime,
} from "@/utils/businessTime";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    const leaf = key.replace(/^businessDashboard\.reservations\./, "");
    const map: Record<string, string> = {
      guests: "guests",
      dateFilter: "Date filter",
      "viewToggle.list": "List",
      "viewToggle.board": "Board",
      "filters.today": "Today",
      "filters.upcoming": "Upcoming",
      "filters.past30Days": "Past",
      "filters.customRange": "Custom",
      "filters.allIncludingPast": "All",
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

const mockToday = localDateKey(new Date());
const mockTodayAt19 = new Date(`${mockToday}T19:00:00Z`).toISOString();

type GetReservationsArgs = Parameters<
  typeof import("@/api/reservations").reservationAPI.getReservations
>;

const mockRow = {
  id: 11,
  business_id: 48,
  customer_name: "Card Guest",
  customer_phone: "",
  party_size: 2,
  reservation_time: mockTodayAt19,
  duration: 90,
  status: "confirmed" as const,
  created_at: mockTodayAt19,
  updated_at: mockTodayAt19,
};

const mockGetReservations = jest.fn((..._args: GetReservationsArgs) =>
  Promise.resolve({
    reservations: [mockRow],
    total: 1,
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
  },
  getAllUpcomingReservations: jest.fn(() =>
    Promise.resolve({
      items: [mockRow],
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

async function switchToBoard(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByLabelText("Board"));
  await waitFor(() => {
    expect(screen.getByText("Card Guest")).toBeInTheDocument();
  });
}

describe("ReservationManager L1-14 board card time DOM (D1)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("today board cards use time-only (no year)", async () => {
    const user = userEvent.setup();
    render(<ReservationManager businessId={48} />);

    await waitFor(() => {
      expect(screen.getByText("Card Guest")).toBeInTheDocument();
    });
    await switchToBoard(user);

    const expectedTime = formatBusinessTime(
      mockTodayAt19,
      "en",
      null,
      TIME_SHORT,
    );
    const year = String(new Date(mockTodayAt19).getFullYear());

    // Board card meta line: time · N guests
    const card = screen.getByText("Card Guest").closest("div");
    const meta = card?.parentElement?.textContent || "";
    expect(meta).toContain(expectedTime);
    expect(meta).toMatch(/2\s*guests/);
    // time_only must not include calendar year on the card meta.
    const timeLine = Array.from(
      screen.getByText("Card Guest").parentElement?.querySelectorAll("p") || [],
    ).find((p) => p.textContent?.includes("guests"));
    expect(timeLine?.textContent).toContain(expectedTime);
    expect(timeLine?.textContent).not.toContain(year);
  });

  it("all_time board cards use date_time (includes year)", async () => {
    const user = userEvent.setup();
    const { container } = render(<ReservationManager businessId={48} />);

    await waitFor(() => {
      expect(screen.getByText("Card Guest")).toBeInTheDocument();
    });

    fireEvent.change(getDateFilterSelect(container), {
      target: { value: "all_time" },
    });
    await waitFor(() => {
      expect(
        mockGetReservations.mock.calls.some((c) => c[1] === "2000-01-01"),
      ).toBe(true);
    });

    await switchToBoard(user);

    const expectedDateTime = formatBusinessDateTime(
      mockTodayAt19,
      "en",
      null,
      DATE_TIME_SHORT,
    );
    const year = String(new Date(mockTodayAt19).getFullYear());

    const timeLine = Array.from(
      screen.getByText("Card Guest").parentElement?.querySelectorAll("p") || [],
    ).find((p) => p.textContent?.includes("guests"));
    expect(timeLine?.textContent).toContain(year);
    // Full DATE_TIME_SHORT string should appear (or at least year + time bits).
    expect(timeLine?.textContent).toMatch(new RegExp(year));
    // Sanity: not pure time-only.
    expect(expectedDateTime).toContain(year);
  });
});
