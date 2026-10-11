/** @jest-environment jsdom */
import fs from "fs";
import path from "path";
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
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

let mockReservations: Array<Record<string, unknown>> = [];

const mockReservationAPI = {
  getReservations: jest.fn(() =>
    Promise.resolve({
      reservations: mockReservations,
      total: mockReservations.length,
      page: 1,
      page_size: 25,
      total_pages: 1,
    }),
  ),
  getStats: jest.fn(() =>
    Promise.resolve({ total: 0, cancelled: 0, no_show: 0, waitlist: 0 }),
  ),
  getSettings: jest.fn(() =>
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
  ),
  updateSettings: jest.fn(() => Promise.resolve({})),
  createReservation: jest.fn(() => Promise.resolve({})),
  updateReservation: jest.fn(() => Promise.resolve({})),
  cancelReservation: jest.fn(() => Promise.resolve({})),
  checkIn: jest.fn(() => Promise.resolve({})),
  markNoShow: jest.fn(() => Promise.resolve({})),
  promoteWaitlist: jest.fn(() => Promise.resolve({})),
  assignTable: jest.fn(() => Promise.resolve({})),
  claimReservation: jest.fn(),
  releaseReservation: jest.fn(() => Promise.resolve({})),
  getReservation: jest.fn(),
};

jest.mock("@/api/reservations", () => {
  const actual = jest.requireActual("@/api/reservations") as Record<string, unknown>;
  return {
    ...actual,
    reservationAPI: {
      getReservations: (...args: unknown[]) =>
        mockReservationAPI.getReservations(...(args as [])),
      getStats: (...args: unknown[]) => mockReservationAPI.getStats(...(args as [])),
      getSettings: (...args: unknown[]) =>
        mockReservationAPI.getSettings(...(args as [])),
      updateSettings: (...args: unknown[]) =>
        mockReservationAPI.updateSettings(...(args as [])),
      createReservation: (...args: unknown[]) =>
        mockReservationAPI.createReservation(...(args as [])),
      updateReservation: (...args: unknown[]) =>
        mockReservationAPI.updateReservation(...(args as [])),
      cancelReservation: (...args: unknown[]) =>
        mockReservationAPI.cancelReservation(...(args as [])),
      checkIn: (...args: unknown[]) => mockReservationAPI.checkIn(...(args as [])),
      markNoShow: (...args: unknown[]) =>
        mockReservationAPI.markNoShow(...(args as [])),
      promoteWaitlist: (...args: unknown[]) =>
        mockReservationAPI.promoteWaitlist(...(args as [])),
      assignTable: (...args: unknown[]) =>
        mockReservationAPI.assignTable(...(args as [])),
      claimReservation: (...args: unknown[]) =>
        mockReservationAPI.claimReservation(...(args as [])),
      releaseReservation: (...args: unknown[]) =>
        mockReservationAPI.releaseReservation(...(args as [])),
      getReservation: (...args: unknown[]) =>
        mockReservationAPI.getReservation(...(args as [])),
    },
    getAllUpcomingReservations: () =>
      Promise.resolve({
        items: mockReservations,
        metadata: {
          total: mockReservations.length,
          page: 1,
          page_size: 100,
          total_pages: 1,
        },
        capped: false,
      }),
  };
});

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

const mockUseAuth = jest.fn();
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => mockUseAuth(),
}));

jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: [
      "reservations:read",
      "reservations:create",
      "reservations:write",
      "reservations:delete",
    ],
    rolePermissions: [
      "reservations:read",
      "reservations:create",
      "reservations:write",
      "reservations:delete",
    ],
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: jest.fn(),
  }),
}));

jest.mock("../modals/ConfirmationModal", () => ({
  __esModule: true,
  default: ({
    isOpen,
    confirmLabel,
    cancelLabel,
    title,
    description,
    onConfirm,
    onOpenChange,
  }: {
    isOpen: boolean;
    confirmLabel?: string;
    cancelLabel?: string;
    title?: string;
    description?: string;
    onConfirm: () => void;
    onOpenChange: () => void;
  }) =>
    isOpen ? (
      <div data-testid="steal-confirm-modal">
        <h2>{title}</h2>
        <p>{description}</p>
        <button type="button" onClick={onConfirm}>
          {confirmLabel}
        </button>
        <button type="button" onClick={onOpenChange}>
          {cancelLabel}
        </button>
      </div>
    ) : null,
}));

import ReservationManager from "@/components/business/ReservationManager";

const now = new Date().toISOString();
const reservation = {
  id: 42,
  business_id: 1,
  customer_name: "Jane",
  customer_phone: "555-0101",
  customer_email: "jane@example.com",
  party_size: 2,
  reservation_time: now,
  duration: 120,
  status: "confirmed",
  table_id: undefined,
  table: undefined,
  created_at: now,
  updated_at: now,
  status_history: [],
};

describe("ReservationManager steal confirmation", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockUseAuth.mockReturnValue({
      isWeb3User: false,
      isStaffUser: true,
      isOAuthUser: false,
      oauthData: null,
      staffData: {
        id: 1,
        name: "Manager",
        email: "mgr@example.com",
        business_id: 1,
        is_active: true,
        role: "manager",
      },
      refreshStaffData: jest.fn(),
      walletLinked: false,
      walletAddress: null,
      linkWallet: jest.fn(),
      unlinkWallet: jest.fn(),
      refreshOAuthUser: jest.fn(),
      isLoading: false,
      isInitialized: true,
    });
    mockReservations = [reservation];
    mockReservationAPI.getReservation.mockResolvedValue(reservation);
    mockReservationAPI.claimReservation
      .mockRejectedValueOnce({
        response: {
          status: 409,
          data: { code: "claim_steal_required", claimed_by_name: "Bob" },
        },
      })
      .mockResolvedValueOnce({
        claimed_by_staff_id: 1,
        claimed_by_name: "Manager",
        claimed_by_role: "manager",
      });
  });

  it("does not use window.confirm", () => {
    const src = fs.readFileSync(
      path.join(__dirname, "../ReservationManager.tsx"),
      "utf8",
    );
    expect(src).not.toMatch(/window\.confirm/);
    expect(src).toMatch(/ConfirmationModal/);
  });

  it("renders the takeover modal and steals on confirm", async () => {
    render(<ReservationManager businessId={1} />);
    await screen.findAllByText(/Jane/);
    fireEvent.click(
      screen.getByRole("button", { name: /viewToggle\.board$|^Board$/ }),
    );
    fireEvent.click(await screen.findByRole("button", { name: /Seat/i }));

    expect(await screen.findByTestId("steal-confirm-modal")).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "businessDashboard.reservations.claim.takeOver",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "businessDashboard.reservations.claim.cancel",
      }),
    ).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", {
        name: "businessDashboard.reservations.claim.takeOver",
      }),
    );

    await waitFor(() => {
      expect(mockReservationAPI.claimReservation).toHaveBeenLastCalledWith(1, 42, {
        steal: true,
      });
    });
    await waitFor(() => {
      expect(mockReservationAPI.checkIn).toHaveBeenCalledWith(1, 42);
    });
  });
});
