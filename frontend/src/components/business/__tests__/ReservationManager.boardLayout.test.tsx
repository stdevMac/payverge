/** @jest-environment jsdom */
/**
 * Host-stand Board must be side-by-side lanes (horizontal), not a vertical
 * stack of full-width cards with huge empty min-heights.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { localDateKey } from "@/lib/localDate";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    const leaf = key.replace(/^businessDashboard\.reservations\./, "");
    const map: Record<string, string> = {
      "viewToggle.list": "List",
      "viewToggle.board": "Board",
      guests: "guests",
      "serviceDetails.unassigned": "Unassigned",
      "shell.reservationCountLabel": "reservation",
      "shell.reservationCountLabel_other": "reservations",
      "shell.coversCountLabel": "covers",
      "shell.todayReservations": "Reservations today",
      "insights.coversToday": "Covers today",
      "insights.waitlist": "Waitlist",
      "insights.needsTable": "Needs a table",
      "insights.nextArrival": "Next arrival",
      nothingInColumn: "Nothing in {column}.",
      "board.scrollHint": "Scroll sideways to reach Waitlist, Seated, and Completed",
      "board.showMore": "Show {count} more",
      "status.pending": "Pending",
      "status.confirmed": "Confirmed",
      "status.waitlist": "Waitlist",
      "status.seated": "Seated",
      "status.completed": "Completed",
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

const mockRow = {
  id: 11,
  business_id: 1,
  customer_name: "Board Guest",
  customer_phone: "",
  party_size: 2,
  reservation_time: mockTodayAt19,
  duration: 90,
  status: "confirmed" as const,
  created_at: mockTodayAt19,
  updated_at: mockTodayAt19,
};

jest.mock("@/api/reservations", () => ({
  reservationAPI: {
    getReservations: jest.fn(() =>
      Promise.resolve({
        reservations: [mockRow],
        total: 1,
        page: 1,
        page_size: 25,
        total_pages: 1,
      }),
    ),
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
        covers: 4,
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

describe("ReservationManager board layout", () => {
  it("renders horizontal board lanes without 280px empty min-height", async () => {
    const user = userEvent.setup();
    render(<ReservationManager businessId={1} />);

    await waitFor(() => {
      expect(screen.getByLabelText("Board")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("Board"));

    const board = await screen.findByTestId("reservations-board");
    const scroller = screen.getByTestId("reservations-board-scroller");
    expect(board.className).toMatch(/flex/);
    expect(board.className).toMatch(/\bw-max\b/);
    expect(board.className).toMatch(/min-w-max/);
    expect(board.className).not.toMatch(/grid-cols-1/);
    expect(board.className).not.toMatch(/overflow-x-auto/);
    expect(board.className).not.toMatch(/\bw-full\b/);
    expect(scroller).toHaveStyle({ overflowX: "auto" });
    expect(scroller.className).toMatch(/overflow-x-auto/);
    expect(scroller.className).toMatch(/min-w-0/);
    expect(scroller.className).toMatch(/max-w-full/);
    expect(scroller.className).toMatch(/flex-1/);
    expect(scroller.className).toMatch(
      /min-h-\[min\(16rem,calc\(100dvh-22rem\)\)\]/,
    );
    const chrome = screen.getByTestId("reservation-board-chrome");
    expect(chrome.className).toMatch(/max-h-\[12rem\]/);
    expect(chrome.className).toMatch(/overflow-y-auto/);
    expect(screen.getByTestId("reservation-board-insights")).toBeInTheDocument();
    expect(screen.getByText("Scroll sideways to reach Waitlist, Seated, and Completed")).toBeInTheDocument();

    const lane = board.firstElementChild as HTMLElement | null;
    expect(lane).not.toBeNull();
    expect(lane?.className || "").toMatch(/min-w-\[240px\]/);
    expect(lane?.className || "").not.toMatch(/min-h-\[280px\]/);
  });

  it("collapses empty lanes to content height without a large min-height", async () => {
    const user = userEvent.setup();
    render(<ReservationManager businessId={1} />);

    await waitFor(() => {
      expect(screen.getByLabelText("Board")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("Board"));

    const board = await screen.findByTestId("reservations-board");
    const emptyLanes = board.querySelectorAll(
      '[data-testid="reservation-board-lane"][data-empty="true"]',
    );
    expect(emptyLanes.length).toBeGreaterThanOrEqual(3);

    const largeMinHeight = /min-h-(?:\[[1-9]\d{2,}px\]|(?:4[4-9]|[5-9]\d|[1-9]\d{2,}))/;
    emptyLanes.forEach((lane) => {
      const className = (lane as HTMLElement).className || "";
      expect(className).toMatch(/min-h-0/);
      expect(className).toMatch(/\bh-auto\b/);
      expect(className).not.toMatch(largeMinHeight);
      expect(className).not.toMatch(/min-h-\[200px\]/);
      expect(className).not.toMatch(/min-h-\[280px\]/);
    });

    const emptyPlaceholders = screen.getAllByTestId("reservation-board-empty");
    expect(emptyPlaceholders.length).toBe(emptyLanes.length);
    emptyPlaceholders.forEach((placeholder) => {
      const className = placeholder.className || "";
      expect(className).toMatch(/min-h-0/);
      expect(className).toMatch(/\bh-auto\b/);
      expect(className).not.toMatch(largeMinHeight);
      expect(className).not.toMatch(/min-h-\[200px\]/);
    });

    expect(screen.getByText("Nothing in waitlist.")).toBeInTheDocument();
  });
});
