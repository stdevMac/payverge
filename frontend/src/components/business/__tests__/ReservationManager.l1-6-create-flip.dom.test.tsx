/** @jest-environment jsdom */
/**
 * D1 / L1-6: creating a reservation for another day while on "today" must flip
 * the date filter to "upcoming" so the new row is visible — and must not leave
 * the operator stuck on an empty "today" list (silent-failure look).
 * Asserts the shipped ReservationManager create path, not only postCreate helpers.
 */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { localDateKey } from "@/lib/localDate";

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    const leaf = key.replace(/^businessDashboard\.reservations\./, "");
    const map: Record<string, string> = {
      createReservation: "Create reservation",
      dateFilter: "Date filter",
      "filters.today": "Today",
      "filters.upcoming": "Upcoming",
      "filters.past30Days": "Past 30 days",
      "filters.customRange": "Custom",
      "filters.allIncludingPast": "All including past",
      "modals.create.title": "New reservation",
      "modals.create.subtitle": "Add a guest",
      "modals.create.cancel": "Cancel",
      "modals.create.create": "Create",
      "modals.create.customerInfo": "Customer",
      "modals.create.customerName": "Name",
      "modals.create.customerPhone": "Phone",
      "modals.create.customerEmail": "Email",
      "modals.create.reservationDetails": "Details",
      "modals.create.partySize": "Party size",
      "modals.create.dateTime": "Date and time",
      "modals.create.duration": "Duration",
      minutes: "min",
      "modals.create.additionalInfo": "More",
      "modals.create.specialRequests": "Special",
      "modals.create.specialRequestsPlaceholder": "",
      "modals.create.notes": "Notes",
      "modals.create.notesPlaceholder": "",
      "timeEntry.modeLabel": "Time zone",
      "timeEntry.business": "Business",
      "timeEntry.device": "Device",
      "timeEntry.preview": "Preview",
      "occupancy.available": "Available",
      "occupancy.occupied": "Occupied",
      "occupancy.staleOccupied": "Stale",
      "occupancy.override": "Override",
      "occupancy.overrideRequired": "Override required",
      "occupancy.reservationConflict": "Conflict",
      "occupancy.capacityConflict": "Capacity",
      "toasts.createSuccess": "Created",
      "empty.todayTitle": "No reservations today",
      "empty.viewUpcomingAction": "View upcoming",
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

const tomorrow = new Date();
tomorrow.setDate(tomorrow.getDate() + 2);
const tomorrowKey = localDateKey(tomorrow);
const tomorrowWall = `${tomorrowKey}T19:00`;
const tomorrowIso = new Date(
  tomorrow.getFullYear(),
  tomorrow.getMonth(),
  tomorrow.getDate(),
  19,
  0,
  0,
).toISOString();

const createdRow = {
  id: 901,
  business_id: 48,
  customer_name: "Flip Guest",
  customer_phone: "5551234567",
  party_size: 2,
  reservation_time: tomorrowIso,
  duration: 90,
  status: "confirmed" as const,
  created_at: new Date().toISOString(),
  updated_at: new Date().toISOString(),
};

type CreateReservationArgs = Parameters<
  typeof import("@/api/reservations").reservationAPI.createReservation
>;

const mockGetReservations = jest.fn(
  (
    _businessId: number,
    startDate?: string,
    endDate?: string,
    _status?: string,
  ) => {
    // After flip, upcoming window includes tomorrow — surface the new row.
    if (startDate && endDate && endDate === startDate) {
      return Promise.resolve({
        reservations: [],
        total: 0,
        page: 1,
        page_size: 25,
        total_pages: 1,
      });
    }
    if (startDate && endDate && endDate > startDate) {
      return Promise.resolve({
        reservations: [createdRow],
        total: 1,
        page: 1,
        page_size: 25,
        total_pages: 1,
      });
    }
    return Promise.resolve({
      reservations: [],
      total: 0,
      page: 1,
      page_size: 25,
      total_pages: 1,
    });
  },
);

const mockCreate = jest.fn((..._args: CreateReservationArgs) =>
  Promise.resolve(createdRow),
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
    updateReservation: jest.fn(),
    createReservation: (...args: CreateReservationArgs) => mockCreate(...args),
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
import toast from "react-hot-toast";
import { getDateFilterSelect } from "./_reservationManagerTestUtils";
import userEvent from "@testing-library/user-event";

describe("ReservationManager L1-6 create flips to upcoming (D1)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("after creating a non-today reservation, date filter is upcoming and row is visible", async () => {
    const user = userEvent.setup();
    const { container } = render(<ReservationManager businessId={48} />);

    await waitFor(() => {
      expect(
        screen.getByTestId("reservation-create-button"),
      ).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("reservation-create-button"));

    await waitFor(() => {
      expect(
        screen.getByTestId("reservation-form-modal-create"),
      ).toBeInTheDocument();
    });

    // Fill required fields + move datetime to another day (NextUI onValueChange).
    await user.clear(screen.getByLabelText(/^Name/));
    await user.type(screen.getByLabelText(/^Name/), "Flip Guest");
    await user.clear(screen.getByTestId("reservation-customer-phone"));
    await user.type(
      screen.getByTestId("reservation-customer-phone"),
      "5551234567",
    );

    const dt = screen.getByLabelText(/Date and time/i);
    await act(async () => {
      fireEvent.change(dt, { target: { value: tomorrowWall } });
    });

    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreate).toHaveBeenCalled();
    });

    // Date filter flipped to upcoming.
    await waitFor(() => {
      expect(getDateFilterSelect(container).value).toBe("upcoming");
    });

    await waitFor(() => {
      expect(screen.getByText("Flip Guest")).toBeInTheDocument();
    });

    // Upcoming window was requested (not stuck on today-only bounds).
    expect(
      mockGetReservations.mock.calls.some(
        (c) =>
          typeof c[1] === "string" &&
          typeof c[2] === "string" &&
          (c[2] as string) > (c[1] as string),
      ),
    ).toBe(true);

    expect(toast.success).toHaveBeenCalled();
    expect(screen.queryByText("No reservations today")).not.toBeInTheDocument();
  });
});
