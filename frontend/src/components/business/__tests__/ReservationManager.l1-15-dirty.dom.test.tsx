/** @jest-environment jsdom */
/**
 * D1 / L1-15: opening create must not mark the form dirty (seeded
 * reservation_time is baseline), but editing a field must. Asserts the dirty
 * flag passed to useUnsavedChangesGuard — the shipped guard path — not a
 * source grep of useDirtyForm.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

const dirtySnapshots: boolean[] = [];

jest.mock("@/hooks/useUnsavedChangesGuard", () => ({
  useUnsavedChangesGuard: (dirty: boolean) => {
    dirtySnapshots.push(dirty);
  },
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

describe("ReservationManager L1-15 dirty baseline DOM (D1)", () => {
  beforeEach(() => {
    dirtySnapshots.length = 0;
  });

  it("pristine create open is not dirty; typing a name becomes dirty", async () => {
    const user = userEvent.setup();
    render(<ReservationManager businessId={48} />);

    await waitFor(() => {
      expect(screen.getByTestId("reservation-create-button")).toBeInTheDocument();
    });

    // Before open, form closed → dirty false.
    expect(dirtySnapshots[dirtySnapshots.length - 1]).toBe(false);

    await user.click(screen.getByTestId("reservation-create-button"));

    await waitFor(() => {
      expect(screen.getByTestId("reservation-form-modal-create")).toBeInTheDocument();
    });

    // After open + markClean, dirty must settle false (seeded time is baseline).
    await waitFor(() => {
      expect(dirtySnapshots[dirtySnapshots.length - 1]).toBe(false);
    });

    await user.type(screen.getByLabelText(/^Name/), "Ada");

    await waitFor(() => {
      expect(dirtySnapshots[dirtySnapshots.length - 1]).toBe(true);
    });
  });
});
