/** @jest-environment jsdom */
/**
 * Related to issue 818: Staff → People sat on the TeamSkeleton forever while
 * Schedule happily listed the same five people. Two dishonest gates caused it:
 * a stalled access query pinned the skeleton even when the shared staff cache
 * was already warm, and staff/access failures degraded into a fake cold-start
 * checklist or a fake suspended lock instead of an honest error.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import StaffManagement from "./StaffManagement";
import * as StaffAPI from "../../api/staff";
import { positionsApi } from "@/api/positions";
import { queryKeys } from "@/api/queryKeys";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));
jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("../../api/staff", () => ({
  getBusinessStaff: jest.fn(),
  inviteStaff: jest.fn(),
  removeStaff: jest.fn(),
  resendInvitation: jest.fn(),
}));
jest.mock("@/api/positions", () => ({ positionsApi: { list: jest.fn() } }));

let mockTier: {
  hasAccess: boolean;
  loading: boolean;
  isError: boolean;
} = { hasAccess: true, loading: false, isError: false };
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => mockTier,
}));

jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ isStaffUser: false }),
}));
jest.mock("./chat/TeamChatPanel", () => ({ __esModule: true, default: () => null }));
jest.mock("./chat/AnnouncementComposer", () => ({ __esModule: true, default: () => null }));
jest.mock("./engagement/EngagementPanel", () => ({ __esModule: true, default: () => null }));
jest.mock("./schedule/LiveFloorBoard", () => ({
  __esModule: true,
  default: () => <div data-testid="live-floor-board" />,
}));
jest.mock("./DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => <div data-testid="locked-tab-view" />,
}));
jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: [],
    rolePermissions: [],
  }),
}));

const mockedStaff = StaffAPI as unknown as { getBusinessStaff: jest.Mock };
const mockedPositions = positionsApi as unknown as { list: jest.Mock };

const ROSTER = {
  staff: [
    { id: 1, name: "Ana Cook", email: "ana@x.test", role: "manager", is_active: true },
    { id: 2, name: "Beto Waiter", email: "beto@x.test", role: "waiter", is_active: true },
  ],
  pending_invitations: [],
};

function renderTeam(opts: { warmCache?: boolean } = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  if (opts.warmCache) {
    // Schedule (ScheduleBuilder) shares this exact key — visiting it first
    // leaves the roster in the cache.
    qc.setQueryData(queryKeys.staff.list("42"), ROSTER);
  }
  return render(
    <QueryClientProvider client={qc}>
      <StaffManagement businessId="42" />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mockTier = { hasAccess: true, loading: false, isError: false };
  mockedPositions.list.mockResolvedValue([]);
  mockedStaff.getBusinessStaff.mockResolvedValue(ROSTER);
});

describe("StaffManagement load honesty (issue 818)", () => {
  it("renders the warm roster instead of pinning the skeleton on a stalled tier query", async () => {
    mockTier = { hasAccess: false, loading: true, isError: false };
    renderTeam({ warmCache: true });
    expect(await screen.findByText("Ana Cook")).toBeInTheDocument();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("shows an honest error with retry when the staff list fails, not the cold-start checklist", async () => {
    mockedStaff.getBusinessStaff.mockRejectedValue(new Error("boom"));
    renderTeam();
    expect(
      await screen.findByText(
        "businessDashboard.dashboard.staffManagement.error.loadStaffData",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "businessDashboard.dashboard.staffManagement.error.retry",
      }),
    ).toBeInTheDocument();
    expect(screen.queryByText("dashboardSetup.title")).not.toBeInTheDocument();
  });

  it("does not swap in the suspended lock when the access probe itself failed", async () => {
    mockTier = { hasAccess: false, loading: false, isError: true };
    renderTeam();
    await waitFor(() =>
      expect(screen.queryByTestId("locked-tab-view")).not.toBeInTheDocument(),
    );
    // The tier failure must not block the roster either.
    expect(await screen.findByText("Ana Cook")).toBeInTheDocument();
  });
});
