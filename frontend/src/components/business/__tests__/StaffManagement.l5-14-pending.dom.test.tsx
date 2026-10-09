/**
 * D1 / L5-14: shell header "pending invitations" must exclude invites past
 * expires_at even when status is still "pending". The list may still show
 * them for resend; the header count must not.
 *
 * Pure countPendingInvitations unit tests pass if StaffManagement never uses
 * them. This mounts StaffManagement and asserts the shell stat value.
 *
 * Revert-proof: count only status==="pending" and ignore expires_at → header
 * shows 2 when one is expired.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import StaffManagement from "../StaffManagement";
import * as StaffAPI from "../../../api/staff";
import { positionsApi } from "@/api/positions";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));
jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("../../../api/staff", () => ({
  getBusinessStaff: jest.fn(),
  inviteStaff: jest.fn(),
  removeStaff: jest.fn(),
  resendInvitation: jest.fn(),
}));
jest.mock("@/api/positions", () => ({ positionsApi: { list: jest.fn() } }));
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({ hasAccess: true, loading: false }),
}));
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ isStaffUser: false }),
}));
jest.mock("../chat/TeamChatPanel", () => ({ __esModule: true, default: () => null }));
jest.mock("../chat/AnnouncementComposer", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../engagement/EngagementPanel", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("../schedule/LiveFloorBoard", () => ({
  __esModule: true,
  default: () => null,
}));
jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: [],
    rolePermissions: [],
  }),
}));
jest.mock("next/navigation", () => ({
  useSearchParams: () => new URLSearchParams(),
  useRouter: () => ({ push: jest.fn(), replace: jest.fn() }),
  usePathname: () => "/business/42/dashboard",
}));

const mockedStaff = StaffAPI as unknown as { getBusinessStaff: jest.Mock };
const mockedPositions = positionsApi as unknown as { list: jest.Mock };

function renderTeam() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <StaffManagement businessId="42" />
    </QueryClientProvider>,
  );
}

describe("StaffManagement L5-14 pending invite header (DOM)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockedPositions.list.mockResolvedValue([]);
    // One open + one wall-clock expired (status still pending).
    mockedStaff.getBusinessStaff.mockResolvedValue({
      staff: [
        {
          id: 1,
          name: "Active Server",
          email: "a@test.com",
          is_active: true,
          role: "server",
          last_login_at: new Date().toISOString(),
        },
      ],
      pending_invitations: [
        {
          id: 10,
          email: "open@test.com",
          status: "pending",
          expires_at: new Date(Date.now() + 7 * 864e5).toISOString(),
        },
        {
          id: 11,
          email: "stale@test.com",
          status: "pending",
          expires_at: new Date(Date.now() - 7 * 864e5).toISOString(),
        },
      ],
    });
  });

  it("header pending count is 1 (excludes expired pending), not 2", async () => {
    renderTeam();

    // Label for singular pending invite.
    const label = await screen.findByText(
      "businessDashboard.dashboard.staffManagement.shell.pendingInvitation",
    );
    const stat = label.parentElement;
    const text = stat?.textContent ?? "";
    // PageHeader: value digit + label key (no space in stubbed i18n).
    expect(text.startsWith("1")).toBe(true);
    expect(text.startsWith("2")).toBe(false);
    // Singular key only when count === 1.
    expect(
      screen.queryByText(
        "businessDashboard.dashboard.staffManagement.shell.pendingInvitations",
      ),
    ).toBeNull();

    await waitFor(() => {
      expect(mockedStaff.getBusinessStaff).toHaveBeenCalled();
    });
  });
});
