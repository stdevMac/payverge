/** @jest-environment jsdom */
/**
 * First-load must wait for reservation settings. hasAccess is false while the
 * tier query is in flight; treating that as "locked/disabled" painted
 * "Bookings paused" and a blank body.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    if (key === "common.loadingReservations") return "Loading reservations…";
    if (key === "businessDashboard.reservations.shell.disabled") {
      return "Bookings paused";
    }
    if (key === "businessDashboard.reservations.shell.enabled") {
      return "Live booking";
    }
    return key;
  },
}));

const mockAccessState = {
  access: null,
  loading: true,
  error: null,
  hasAccess: false,
  isSuspended: false,
  lockState: "active",
  aiConfigured: false,
  refetch: jest.fn(),
};

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => mockAccessState,
}));

jest.mock("@/api/reservations", () => ({
  reservationAPI: {
    getSettings: jest.fn(
      () =>
        new Promise(() => {
          /* hang: this spec asserts the in-flight gate only */
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

describe("ReservationManager first-load settings gate", () => {
  it("does not paint Bookings paused while settings are still loading", async () => {
    render(<ReservationManager businessId={85} />);

    expect(screen.queryByText("Bookings paused")).not.toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
    expect(screen.getByText(/loading reservations/i)).toBeInTheDocument();
  });
});
