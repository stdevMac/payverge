/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import StaffManagement from "./StaffManagement";
import * as StaffAPI from "../../api/staff";
import { positionsApi } from "@/api/positions";

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
jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: () => ({ hasAccess: true, loading: false }),
}));
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ isStaffUser: false }),
}));
// Heavy communication children have their own coverage — stub them out.
jest.mock("./chat/TeamChatPanel", () => ({ __esModule: true, default: () => null }));
jest.mock("./chat/AnnouncementComposer", () => ({ __esModule: true, default: () => null }));
jest.mock("./engagement/EngagementPanel", () => ({ __esModule: true, default: () => null }));
// Live floor (who's on tonight) self-fetches; stub to keep this suite focused.
jest.mock("./schedule/LiveFloorBoard", () => ({
  __esModule: true,
  default: () => <div data-testid="live-floor-board" />,
}));
jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({
    permissions: [],
    rolePermissions: [],
  }),
}));

const mockedStaff = StaffAPI as unknown as { getBusinessStaff: jest.Mock };
const mockedPositions = positionsApi as unknown as { list: jest.Mock };

function renderTeam(props: Partial<React.ComponentProps<typeof StaffManagement>> = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <StaffManagement businessId="42" {...props} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  jest.clearAllMocks();
  mockedPositions.list.mockResolvedValue([]);
});

describe("StaffManagement loading", () => {
  it("shows a structured skeleton (not dimmed content) while staff data loads", async () => {
    // Keep the fetch pending so we observe the loading branch.
    let resolveStaff: (v: unknown) => void = () => {};
    mockedStaff.getBusinessStaff.mockReturnValue(
      new Promise((res) => {
        resolveStaff = res;
      }),
    );
    renderTeam();
    // SkeletonList renders a polite live region labelled with the loading string.
    expect(await screen.findByRole("status")).toHaveAttribute(
      "aria-label",
      "common.loading",
    );
    // The loaded surfaces must NOT be present while loading (no faded/stale content).
    expect(screen.queryByText("dashboardSetup.title")).not.toBeInTheDocument();

    resolveStaff({ staff: [], pending_invitations: [] });
    await waitFor(() =>
      expect(screen.getByText("dashboardSetup.title")).toBeInTheDocument(),
    );
  });

  // L5-21: Comunicación must not blank behind TeamSkeleton while staffQuery loads.
  it("does not gate the communication sub-tab on staffQuery loading (L5-21)", async () => {
    mockedStaff.getBusinessStaff.mockReturnValue(new Promise(() => {}));
    renderTeam({ subTab: "communication", canManageCommunication: true });
    // People-shaped skeleton must not replace chat while staff is still loading.
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    // Header stays mounted (shell S-9 + chat path). getTranslation stub echoes keys.
    expect(
      screen.getByRole("heading", {
        name: "businessDashboard.dashboard.staffManagement.title",
      }),
    ).toBeInTheDocument();
  });
});

describe("StaffManagement business timezone (R17)", () => {
  it("renders invitation dates in the business timezone, not the device/UTC clock", async () => {
    // expires_at is 2026-04-15T02:00:00Z. In Buenos Aires (UTC-3) that instant
    // is 2026-04-14 23:00 → calendar day Apr 14. The raw UTC/device day is
    // Apr 15. With businessTimezone set, the InvitationTable must show the
    // business-day (Apr 14) and never leak the UTC day (Apr 15).
    mockedStaff.getBusinessStaff.mockResolvedValue({
      staff: [],
      pending_invitations: [
        {
          id: 1,
          email: "cook@example.com",
          name: "Cook",
          role: "kitchen",
          business_id: 42,
          token: "tok",
          expires_at: "2026-04-15T02:00:00Z",
          created_at: "2026-04-10T00:00:00Z",
          status: "pending",
        },
      ],
    });
    renderTeam({ businessTimezone: "America/Argentina/Buenos_Aires" });

    const hasSubstring = (needle: string) => (content: string) =>
      content.includes(needle);
    // Business-day present…
    expect(
      await screen.findByText(hasSubstring("Apr 14")),
    ).toBeInTheDocument();
    // …and the UTC/device day absent.
    expect(screen.queryByText(hasSubstring("Apr 15"))).not.toBeInTheDocument();
  });
});

describe("StaffManagement L5-17 ConfirmationModal for resend/revoke", () => {
  it("opens ConfirmationModal for resend (not a bespoke Modal stack)", async () => {
    mockedStaff.getBusinessStaff.mockResolvedValue({
      staff: [],
      pending_invitations: [
        {
          id: 9,
          email: "new@example.com",
          name: "New Hire",
          role: "server",
          business_id: 42,
          token: "tok",
          expires_at: "2099-01-01T00:00:00Z",
          created_at: "2026-04-10T00:00:00Z",
          status: "pending",
        },
      ],
    });
    renderTeam();
    // Resend control on InvitationTable.
    const resend = await screen.findByRole("button", {
      name: "businessDashboard.dashboard.staffManagement.invitations.resendTooltip",
    });
    fireEvent.click(resend);
    // ConfirmationModal title (wired from resendModal.title).
    expect(
      await screen.findByText(
        "businessDashboard.dashboard.staffManagement.resendModal.title",
      ),
    ).toBeInTheDocument();
    // Description is a single body paragraph (ConfirmationModal), not the
    // old dual-header subtitle + branded callout stack.
    expect(
      screen.queryByText(
        "businessDashboard.dashboard.staffManagement.resendModal.subtitle",
      ),
    ).not.toBeInTheDocument();
  });
});

describe("StaffManagement cold start", () => {
  it("keeps the Invite Staff button reachable with zero staff and invitations", async () => {
    mockedStaff.getBusinessStaff.mockResolvedValue({
      staff: [],
      pending_invitations: [],
    });
    renderTeam();
    // Cold start renders the guided setup checklist…
    await waitFor(() =>
      expect(
        screen.getByText("dashboardSetup.title"),
      ).toBeInTheDocument(),
    );
    // …but the header CTA must never disappear — it's the primary action.
    expect(
      screen.getByRole("button", {
        name: /businessDashboard\.dashboard\.staffManagement\.buttons\.inviteStaff/,
      }),
    ).toBeInTheDocument();
  });
});
