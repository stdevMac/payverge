/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { localDateKey } from "@/lib/localDate";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    const leaf = key.replace(/^businessDashboard\.reservations\./, "");
    const map: Record<string, string> = {
      moreReservationActionsAria: "More actions",
      viewReservationAria: "View",
      edit: "Edit",
      cancel: "Cancel",
      actions: "Actions",
      guests: "guests",
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

const mockRow = {
  id: 262,
  business_id: 1,
  customer_name: "QA Test Franky",
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
        covers: 2,
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

describe("ReservationManager row overflow menu", () => {
  it("opens downward outside the clipped card and closes on Escape", async () => {
    const user = userEvent.setup();
    render(<ReservationManager businessId={1} />);

    const trigger = await screen.findByTestId("reservation-actions-262");
    const triggerWrap = trigger.parentElement;
    expect(triggerWrap).not.toBeNull();
    jest.spyOn(triggerWrap as HTMLElement, "getBoundingClientRect").mockReturnValue({
      x: 800,
      y: 220,
      top: 220,
      left: 800,
      bottom: 252,
      right: 832,
      width: 32,
      height: 32,
      toJSON: () => ({}),
    });

    await user.click(trigger);

    const edit = await screen.findByRole("menuitem", { name: "Edit" });
    const cancel = screen.getByRole("menuitem", { name: "Cancel" });
    expect(edit).toBeInTheDocument();
    expect(cancel).toBeInTheDocument();

    const menu = edit.closest("[role='menu']");
    expect(menu).not.toBeNull();
    expect(menu).toHaveAttribute("data-testid", "row-actions-menu");
    expect(menu?.className).toMatch(/z-\[100\]/);
    // Always opens below the trigger so Editar / Cancelar cannot cover Acciones.
    expect(menu).toHaveStyle({ top: "256px" });

    const listPanel = screen.getByTestId("reservation-list-panel");
    expect(listPanel.className).toMatch(/overflow-visible/);
    expect(listPanel).toHaveStyle({ overflow: "visible" });
    expect(listPanel.contains(edit)).toBe(false);
    expect(document.body.contains(edit)).toBe(true);

    await user.keyboard("{Escape}");
    await waitFor(() => {
      expect(screen.queryByRole("menuitem", { name: "Edit" })).not.toBeInTheDocument();
    });
  });
});
