/** @jest-environment jsdom */
/**
 * Verifies ReservationManager hides controls each staff role lacks
 * permission for, mirroring backend StaffRolePermissions:
 *   - manager: full access
 *   - host: read+write+create+delete (no settings → hide toggle + Settings tab)
 *   - server: read+create only (additionally hide edit/cancel/status menu items)
 *
 * Owners (staffData=null) keep manager-equivalent access.
 */
import React from "react";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

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

let mockReservations: any[] = [];
let mockSettingsEnabled = true;
let mockSettingsPromise: Promise<{ enabled: boolean }> | null = null;

const mockReservationAPI = {
  getReservations: jest.fn(() => Promise.resolve({ reservations: mockReservations })),
  getAllUpcomingReservations: jest.fn(() =>
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
  ),
  getSettings: jest.fn(() =>
    mockSettingsPromise ||
    Promise.resolve({
      enabled: mockSettingsEnabled,
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
  claimReservation: jest.fn(() => Promise.resolve({})),
  releaseReservation: jest.fn(() => Promise.resolve({})),
  getReservation: jest.fn((businessId: number, reservationId: number) =>
    Promise.resolve(
      mockReservations.find((reservation) => reservation.id === reservationId) ||
        mockReservations[0],
    ),
  ),
};

jest.mock("@/api/reservations", () => {
  const reservationAPI = {
    getReservations: (...args: Parameters<typeof mockReservationAPI.getReservations>) =>
      mockReservationAPI.getReservations(...args),
    getSettings: (...args: Parameters<typeof mockReservationAPI.getSettings>) =>
      mockReservationAPI.getSettings(...args),
    updateSettings: (...args: Parameters<typeof mockReservationAPI.updateSettings>) =>
      mockReservationAPI.updateSettings(...args),
    createReservation: (...args: Parameters<typeof mockReservationAPI.createReservation>) =>
      mockReservationAPI.createReservation(...args),
    updateReservation: (...args: Parameters<typeof mockReservationAPI.updateReservation>) =>
      mockReservationAPI.updateReservation(...args),
    cancelReservation: (...args: Parameters<typeof mockReservationAPI.cancelReservation>) =>
      mockReservationAPI.cancelReservation(...args),
    checkIn: (...args: Parameters<typeof mockReservationAPI.checkIn>) =>
      mockReservationAPI.checkIn(...args),
    markNoShow: (...args: Parameters<typeof mockReservationAPI.markNoShow>) =>
      mockReservationAPI.markNoShow(...args),
    promoteWaitlist: (...args: Parameters<typeof mockReservationAPI.promoteWaitlist>) =>
      mockReservationAPI.promoteWaitlist(...args),
    assignTable: (...args: Parameters<typeof mockReservationAPI.assignTable>) =>
      mockReservationAPI.assignTable(...args),
    claimReservation: (...args: Parameters<typeof mockReservationAPI.claimReservation>) =>
      mockReservationAPI.claimReservation(...args),
    releaseReservation: (...args: Parameters<typeof mockReservationAPI.releaseReservation>) =>
      mockReservationAPI.releaseReservation(...args),
    getReservation: (...args: Parameters<typeof mockReservationAPI.getReservation>) =>
      mockReservationAPI.getReservation(...args),
  };

  return {
    reservationAPI,
    getAllUpcomingReservations: (
      ...args: Parameters<typeof mockReservationAPI.getAllUpcomingReservations>
    ) => mockReservationAPI.getAllUpcomingReservations(...args),
  };
});

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

const mockUseAuth = jest.fn();
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => mockUseAuth(),
}));

// ReservationManager derives its RBAC capabilities from the staff-permissions
// context (reservations:{read,create,write,delete,settings}) via
// getReservationCapabilities — not from the auth role alone. Drive that
// context per-test so RBAC assertions keep proving "no permission → no control".
let mockStaffPermissions: string[] = [];
jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: mockStaffPermissions,
    rolePermissions: mockStaffPermissions,
    customGrants: [],
    isLoading: false,
    isError: false,
    refetch: jest.fn(),
  }),
}));

import ReservationManager from "@/components/business/ReservationManager";

const baseStaff = {
  id: 1,
  name: "Test User",
  email: "test@example.com",
  business_id: 1,
  is_active: true,
};

const buildReservation = (overrides: Record<string, any> = {}) => {
  // Anchor to the CURRENT local instant, which satisfies two constraints at
  // once and at every hour of the day:
  //   1. It is always inside the local calendar day, which is what the board
  //      keys on (not the UTC day — audit L6 #16).
  //   2. It is inside the seat/no-show temporal window (L1-13), so arrival
  //      quick-actions render. A fixed local-noon anchor silently stopped
  //      rendering them for any run started before 11:30 local.
  // This keeps the RBAC assertions honest: when Seat is absent for `server`
  // it is because the role lacks canEdit, never because of the wall clock.
  const now = new Date().toISOString();

  return {
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
    ...overrides,
  };
};

// Effective permissions per staff role, mirroring the backend
// StaffRolePermissions defaults named in this suite's header:
//   manager: full access
//   host:    read+write+create+delete (no settings)
//   server:  read+create only
const rolePermissions: Record<string, string[]> = {
  manager: [
    "reservations:read",
    "reservations:create",
    "reservations:write",
    "reservations:delete",
    "reservations:settings",
  ],
  host: [
    "reservations:read",
    "reservations:create",
    "reservations:write",
    "reservations:delete",
  ],
  server: ["reservations:read", "reservations:create"],
};

const renderWithRole = (role: string | null) => {
  mockStaffPermissions = role === null ? [] : (rolePermissions[role] ?? []);
  mockUseAuth.mockReturnValue({
    isWeb3User: role === null,
    isStaffUser: role !== null,
    isOAuthUser: false,
    oauthData: null,
    staffData: role === null ? null : { ...baseStaff, role },
    refreshStaffData: jest.fn(),
    walletLinked: false,
    walletAddress: null,
    linkWallet: jest.fn(),
    unlinkWallet: jest.fn(),
    refreshOAuthUser: jest.fn(),
    isLoading: false,
    isInitialized: true,
  });
  return render(<ReservationManager businessId={1} />);
};

describe("ReservationManager RBAC", () => {
  beforeEach(() => {
    mockStaffPermissions = [];
    mockUseAuth.mockReset();
    Object.values(mockReservationAPI).forEach((mockFn) => mockFn.mockClear());
    mockReservations = [];
    mockSettingsEnabled = true;
    mockSettingsPromise = null;
  });

  it("hides reservation toggle and Settings tab for host", async () => {
    renderWithRole("host");
    // Wait for the dashboard shell to settle (the title is the first stable signal).
    await waitFor(() =>
      expect(screen.getByText("businessDashboard.reservations.title")).toBeInTheDocument(),
    );
    // Settings tab title text must not appear for host.
    expect(
      screen.queryByText("businessDashboard.reservations.tabs.settings"),
    ).not.toBeInTheDocument();
    // Header toggle button has a testid wrapper that only renders for canManageSettings.
    expect(screen.queryByTestId("reservation-toggle-button")).not.toBeInTheDocument();
  });

  it("shows reservation toggle and Settings tab for manager", async () => {
    renderWithRole("manager");
    await waitFor(() =>
      expect(screen.getByText("businessDashboard.reservations.title")).toBeInTheDocument(),
    );
    expect(
      screen.getByText("businessDashboard.reservations.tabs.settings"),
    ).toBeInTheDocument();
    // After §B the toggle lives inside the Settings sub-tab, not the page
    // header — flip to Settings to surface it.
    fireEvent.click(
      screen.getByText("businessDashboard.reservations.tabs.settings"),
    );
    await waitFor(() =>
      expect(screen.getByTestId("reservation-toggle-button")).toBeInTheDocument(),
    );
  });

  it("hides edit/cancel actions for server but keeps Create button", async () => {
    mockReservations = [buildReservation()];
    renderWithRole("server");
    await screen.findAllByText(/Jane/);

    expect(screen.queryByTestId("reservation-toggle-button")).not.toBeInTheDocument();
    expect(
      screen.queryByText("businessDashboard.reservations.tabs.settings"),
    ).not.toBeInTheDocument();

    // Server has no canEdit → the operations-view "Seat" quick-action must be absent.
    expect(screen.queryByRole("button", { name: /Seat/i })).not.toBeInTheDocument();

    // Create button still visible (server has reservations:create).
    expect(screen.getByTestId("reservation-create-button")).toBeInTheDocument();
  });

  it("keeps the Seat quick-action and Create button for host", async () => {
    mockReservations = [buildReservation()];
    renderWithRole("host");
    await screen.findAllByText(/Jane/);
    // The Seat quick-action only renders in the kanban Board view (list
    // view uses an actions dropdown). After the UX review, list is the
    // default — switch into the board so we exercise the operations path.
    // Mock getTranslation returns the key verbatim — the Board button label
    // becomes the full key path. Match the suffix to stay locale-agnostic.
    fireEvent.click(
      screen.getByRole("button", { name: /viewToggle\.board$|^Board$/ }),
    );
    // The board's data source (complete hydrate) loads on the view switch, so
    // the quick-action appears once it resolves.
    // Host has canEdit → the Seat quick-action button is visible (status=confirmed).
    expect(
      await screen.findByRole("button", { name: /Seat/i }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("reservation-create-button")).toBeInTheDocument();
  });

  /**
   * H1: withReservationClaim must claim then release so front-line operators
   * are not locked out for the 5m idle TTL after a successful seat mutate.
   * Rule 3: if release is dropped from the finally path, this fails.
   */
  it("H1: seating claims then releases the reservation lock", async () => {
    mockReservations = [buildReservation({ id: 42, status: "confirmed" })];
    mockReservationAPI.claimReservation.mockResolvedValue({
      claimed_by_staff_id: 1,
      claimed_by_name: "Test User",
      claimed_by_role: "host",
    });
    mockReservationAPI.releaseReservation.mockResolvedValue({});
    mockReservationAPI.checkIn.mockResolvedValue({});

    renderWithRole("host");
    await screen.findAllByText(/Jane/);
    fireEvent.click(
      screen.getByRole("button", { name: /viewToggle\.board$|^Board$/ }),
    );
    const seatBtn = await screen.findByRole("button", { name: /Seat/i });
    fireEvent.click(seatBtn);

    await waitFor(() =>
      expect(mockReservationAPI.checkIn).toHaveBeenCalledWith(1, 42),
    );
    expect(mockReservationAPI.claimReservation).toHaveBeenCalledWith(
      1,
      42,
      undefined,
    );
    await waitFor(() =>
      expect(mockReservationAPI.releaseReservation).toHaveBeenCalledWith(1, 42, {
        selfOnly: true,
      }),
    );
    // Order: claim before mutate, release after (finally).
    const claimOrder =
      mockReservationAPI.claimReservation.mock.invocationCallOrder[0];
    const checkInOrder = mockReservationAPI.checkIn.mock.invocationCallOrder[0];
    const releaseOrder =
      mockReservationAPI.releaseReservation.mock.invocationCallOrder[0];
    expect(claimOrder).toBeLessThan(checkInOrder);
    expect(checkInOrder).toBeLessThan(releaseOrder);
  });

  it("treats owner (staffData=null) as manager-equivalent", async () => {
    renderWithRole(null);
    await waitFor(() =>
      expect(screen.getByText("businessDashboard.reservations.title")).toBeInTheDocument(),
    );
    expect(
      screen.getByText("businessDashboard.reservations.tabs.settings"),
    ).toBeInTheDocument();
    fireEvent.click(
      screen.getByText("businessDashboard.reservations.tabs.settings"),
    );
    await waitFor(() =>
      expect(screen.getByTestId("reservation-toggle-button")).toBeInTheDocument(),
    );
  });

  it("renders contact-owner placeholder for non-manager when reservations disabled", async () => {
    mockSettingsEnabled = false;
    renderWithRole("server");
    await waitFor(() =>
      expect(
        screen.getByTestId("reservations-disabled-placeholder"),
      ).toBeInTheDocument(),
    );
    expect(screen.queryByTestId("reservation-toggle-button")).not.toBeInTheDocument();
  });

  it("does NOT render the placeholder for manager when reservations disabled", async () => {
    mockSettingsEnabled = false;
    renderWithRole("manager");
    await waitFor(() =>
      expect(screen.getByText("businessDashboard.reservations.title")).toBeInTheDocument(),
    );
    expect(
      screen.queryByTestId("reservations-disabled-placeholder"),
    ).not.toBeInTheDocument();
  });

  it("keeps server detail drawer read-only", async () => {
    mockReservations = [buildReservation()];
    renderWithRole("server");

    await waitFor(() => expect(screen.getByText("Jane")).toBeInTheDocument());
    fireEvent.click(
      screen.getByRole("button", { name: /viewReservationAria|View reservation/i }),
    );

    await waitFor(() =>
      expect(
        screen.getByText("businessDashboard.reservations.guestDetails"),
      ).toBeInTheDocument(),
    );
    expect(
      screen.queryByText("businessDashboard.reservations.assignTable"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("businessDashboard.reservations.quickActions"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: /^businessDashboard\.reservations\.edit$/i,
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: /^businessDashboard\.reservations\.cancel$/i,
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: /^businessDashboard\.reservations\.seat$/i,
      }),
    ).not.toBeInTheDocument();

    expect(mockReservationAPI.updateReservation).not.toHaveBeenCalled();
    expect(mockReservationAPI.cancelReservation).not.toHaveBeenCalled();
    expect(mockReservationAPI.assignTable).not.toHaveBeenCalled();
    expect(mockReservationAPI.checkIn).not.toHaveBeenCalled();
    expect(mockReservationAPI.markNoShow).not.toHaveBeenCalled();
    expect(mockReservationAPI.promoteWaitlist).not.toHaveBeenCalled();
  });

  it("returns keyboard focus to the reservation details trigger after Escape", async () => {
    const user = userEvent.setup();
    const reservation = buildReservation();
    let resolveDetails!: (value: typeof reservation) => void;
    const detailsRequest = new Promise<typeof reservation>((resolve) => {
      resolveDetails = resolve;
    });
    mockReservations = [reservation];
    mockReservationAPI.getReservation.mockReturnValueOnce(detailsRequest);
    renderWithRole("server");

    const trigger = await screen.findByRole("button", {
      name: /viewReservationAria|View reservation/i,
    });
    act(() => trigger.focus());
    expect(trigger).toHaveFocus();
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(mockReservationAPI.getReservation).toHaveBeenCalledWith(1, 42),
    );

    // The production table can move focus while the async detail request is in
    // flight. The drawer must still remember the initiating control rather
    // than capturing document.body only after the request resolves.
    act(() => trigger.blur());
    await act(async () => {
      resolveDetails(reservation);
      await detailsRequest;
    });

    await screen.findByRole("dialog");
    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(trigger).toHaveFocus();
  });

  it("keeps host detail drawer operational but still hides settings", async () => {
    mockReservations = [buildReservation()];
    renderWithRole("host");

    await waitFor(() => expect(screen.getByText("Jane")).toBeInTheDocument());
    fireEvent.click(
      screen.getByRole("button", { name: /viewReservationAria|View reservation/i }),
    );

    await waitFor(() =>
      expect(
        screen.getByText("businessDashboard.reservations.guestDetails"),
      ).toBeInTheDocument(),
    );
    expect(
      screen.getByText("businessDashboard.reservations.assignTable"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("businessDashboard.reservations.quickActions"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: /^businessDashboard\.reservations\.edit\b/i,
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: /^businessDashboard\.reservations\.cancel\b/i,
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("businessDashboard.reservations.tabs.settings"),
    ).not.toBeInTheDocument();
  });

  it("does not show disabled placeholder while reservation status is still loading", async () => {
    mockSettingsPromise = new Promise(() => undefined);
    renderWithRole("server");

    // While status loads, the shell renders only the skeleton — no header yet.
    await waitFor(() =>
      expect(screen.getByRole("status")).toBeInTheDocument(),
    );
    expect(
      screen.queryByTestId("reservations-disabled-placeholder"),
    ).not.toBeInTheDocument();
  });
});
