/** @jest-environment jsdom */
/**
 * D1 / L1-17: create form must refuse a start that cannot finish duration+buffer
 * before close — toast the outside-window message and never call create.
 * Asserts the shipped ReservationManager path, not only reservationFitsClose math.
 */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import toast from "react-hot-toast";

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
      "validation.outsideOperatingWindow":
        "That start time cannot finish before closing (duration and service buffer).",
      "validation.timeOutsideHours":
        "Selected time is outside operating hours.",
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

type CreateReservationArgs = Parameters<
  typeof import("@/api/reservations").reservationAPI.createReservation
>;

const mockCreate = jest.fn((..._args: CreateReservationArgs) =>
  Promise.resolve({ id: 1 }),
);

// Every weekday open 09:00–17:00 so any date resolves the same window.
const mockHours = [0, 1, 2, 3, 4, 5, 6].map((day_of_week) => ({
  id: day_of_week + 1,
  business_id: 48,
  day_of_week,
  open_time: "09:00",
  close_time: "17:00",
  is_closed: false,
  created_at: "",
  updated_at: "",
}));

jest.mock("@/api/reservations", () => ({
  reservationAPI: {
    getReservations: jest.fn(() =>
      Promise.resolve({
        reservations: [],
        total: 0,
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
        min_advance_minutes: 0,
        slot_interval_minutes: 15,
        max_covers_per_slot: 50,
        hold_duration_minutes: 15,
        // 90 + 15 = 105 min service; 16:00 → 17:45 past 17:00 close.
        service_buffer_minutes: 15,
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
  getBusinessOperatingHours: jest.fn(() => Promise.resolve(mockHours)),
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

// REV-3: Re-enabled. Passes 2/2 in isolation (including on main). Prior skip
// comment claimed "Fails identically on main" which was false — only flakes
// under full-suite load were observed historically. Keep l1-6/l1-7 skipped.
describe("ReservationManager L1-17 close-fit create gate (D1)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("toasts outside-window and does not create when start cannot finish before close", async () => {
    const user = userEvent.setup();
    render(<ReservationManager businessId={48} />);

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

    await user.type(screen.getByLabelText(/^Name/), "Late Guest");
    await user.type(
      screen.getByTestId("reservation-customer-phone"),
      "5551234567",
    );

    // Tomorrow at 16:00 — duration 90 + buffer 15 needs 17:45; close is 17:00.
    const tomorrow = new Date();
    tomorrow.setUTCDate(tomorrow.getUTCDate() + 1);
    const y = tomorrow.getUTCFullYear();
    const m = String(tomorrow.getUTCMonth() + 1).padStart(2, "0");
    const d = String(tomorrow.getUTCDate()).padStart(2, "0");
    const wall = `${y}-${m}-${d}T16:00`;

    const dt = screen.getByLabelText(/Date and time/i);
    await act(async () => {
      fireEvent.change(dt, { target: { value: wall } });
    });

    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        expect.stringMatching(/finish before closing|outside operating/i),
      );
    });
    expect(mockCreate).not.toHaveBeenCalled();
  });

  it("allows create when start + duration + buffer fits before close", async () => {
    const user = userEvent.setup();
    render(<ReservationManager businessId={48} />);

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

    await user.type(screen.getByLabelText(/^Name/), "Fit Guest");
    await user.type(
      screen.getByTestId("reservation-customer-phone"),
      "5559876543",
    );

    // 14:00 + 90 + 15 = 15:45 < 17:00 close → allowed.
    const tomorrow = new Date();
    tomorrow.setUTCDate(tomorrow.getUTCDate() + 1);
    const y = tomorrow.getUTCFullYear();
    const m = String(tomorrow.getUTCMonth() + 1).padStart(2, "0");
    const d = String(tomorrow.getUTCDate()).padStart(2, "0");
    const wall = `${y}-${m}-${d}T14:00`;

    const dt = screen.getByLabelText(/Date and time/i);
    await act(async () => {
      fireEvent.change(dt, { target: { value: wall } });
    });

    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreate).toHaveBeenCalled();
    });
    expect(toast.error).not.toHaveBeenCalledWith(
      expect.stringMatching(/finish before closing|outside operating/i),
    );
  });
});
