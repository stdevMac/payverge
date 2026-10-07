/** @jest-environment jsdom */
import React from "react";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import StaffRoleManagement from "./StaffRoleManagement";
import * as RBACAPI from "@/api/rbac";

jest.mock("@/api/rbac", () => ({
  getStaffPermissions: jest.fn(),
  grantCustomPermission: jest.fn(),
  revokeCustomPermission: jest.fn(),
  denyPermission: jest.fn(),
  removePermissionDeny: jest.fn(),
  getStaffAuditLog: jest.fn(),
  changeStaffRole: jest.fn(),
  deactivateStaff: jest.fn(),
  reactivateStaff: jest.fn(),
  SENSITIVE_PERMISSIONS: new Set([
    "bills:refund",
    "payroll:write",
    "fiscal:credit",
    "fiscal:credentials",
    "director:read",
    "director:write",
    "financial:withdraw",
    "financial:write",
    "staff:permissions",
    "staff:deactivate",
  ]),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    const stripped =
      key.replace(
        /^businessDashboard\.dashboard\.staffManagement\.rbac\./,
        "",
      ) || key;
    const categoryMatch = stripped.match(/^rbacCategories\.(.+)$/);
    if (categoryMatch) {
      const category = categoryMatch[1];
      if (category === "crm") return "Crm";
      return category.charAt(0).toUpperCase() + category.slice(1);
    }
    return stripped;
  },
}));

jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("./StaffCompensationSection", () => ({
  __esModule: true,
  default: () => null,
}));

const mockGetStaffPermissions = RBACAPI.getStaffPermissions as jest.Mock;
const mockGrant = RBACAPI.grantCustomPermission as jest.Mock;
const mockRevoke = RBACAPI.revokeCustomPermission as jest.Mock;
const mockDeny = RBACAPI.denyPermission as jest.Mock;
const mockRemoveDeny = RBACAPI.removePermissionDeny as jest.Mock;
const mockAudit = RBACAPI.getStaffAuditLog as jest.Mock;
const mockDeactivate = RBACAPI.deactivateStaff as jest.Mock;
const mockReactivate = RBACAPI.reactivateStaff as jest.Mock;

const staffMember = {
  id: 7,
  name: "Alex Server",
  email: "alex@example.com",
  role: "server" as const,
  is_active: true,
};

const permFixture: RBACAPI.StaffPermissions = {
  // Effective = role defaults + custom grant − denies
  permissions: ["bills:read", "analytics:sales"],
  role_permissions: ["bills:read", "menu:read"],
  custom_grants: ["analytics:sales"],
  custom_denies: ["menu:read"],
};

function renderModal(
  props: Partial<React.ComponentProps<typeof StaffRoleManagement>> = {},
) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={qc}>
      <StaffRoleManagement
        isOpen
        onClose={jest.fn()}
        staffMember={staffMember}
        businessId="42"
        onStaffUpdated={jest.fn()}
        isOwner={false}
        canEditPermissions
        {...props}
      />
    </QueryClientProvider>,
  );
}

describe("StaffRoleManagement custom permissions", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGetStaffPermissions.mockResolvedValue(permFixture);
    mockAudit.mockResolvedValue({ audit_logs: [] });
    mockGrant.mockResolvedValue({ message: "ok" });
    mockRevoke.mockResolvedValue({ message: "ok" });
    mockDeny.mockResolvedValue({ message: "ok" });
    mockRemoveDeny.mockResolvedValue({ message: "ok" });
  });

  it("loads permissions when the modal is open and canEditPermissions is set", async () => {
    renderModal();
    await waitFor(() =>
      expect(mockGetStaffPermissions).toHaveBeenCalledWith("42", "7"),
    );
    expect(
      await screen.findByText("customPermissions.title"),
    ).toBeInTheDocument();
    expect(screen.getByText("analytics:sales")).toBeInTheDocument();
  });

  it("does not load or show custom permissions UI without canEditPermissions/isOwner", async () => {
    renderModal({ canEditPermissions: false, isOwner: false });
    await waitFor(() => expect(mockAudit).toHaveBeenCalled());
    expect(mockGetStaffPermissions).not.toHaveBeenCalled();
    expect(screen.queryByText("analytics:sales")).not.toBeInTheDocument();
  });

  it("grants a permission not already in the effective set", async () => {
    renderModal();
    await waitFor(() =>
      expect(mockGetStaffPermissions).toHaveBeenCalledWith("42", "7"),
    );

    // Expand CRM category and grant crm:read (not held)
    fireEvent.click(screen.getByText("Crm"));
    const grantBtn = await screen.findByTestId("grant-perm-crm:read");
    fireEvent.click(grantBtn);

    await waitFor(() =>
      expect(mockGrant).toHaveBeenCalledWith("42", "7", {
        permission: "crm:read",
        reason: undefined,
      }),
    );
  });

  it("sends optional reason when granting", async () => {
    renderModal();
    await waitFor(() => expect(mockGetStaffPermissions).toHaveBeenCalled());

    const reasonField = screen.getByPlaceholderText(
      "customPermissions.reasonPlaceholder",
    );
    fireEvent.change(reasonField, { target: { value: "needs CRM access" } });

    fireEvent.click(screen.getByText("Crm"));
    fireEvent.click(await screen.findByTestId("grant-perm-crm:read"));

    await waitFor(() =>
      expect(mockGrant).toHaveBeenCalledWith("42", "7", {
        permission: "crm:read",
        reason: "needs CRM access",
      }),
    );
  });

  it("shows revoke only for custom grants; role defaults get deny not revoke", async () => {
    renderModal();
    await waitFor(() => expect(mockGetStaffPermissions).toHaveBeenCalled());

    // Custom grant appears in the grants list with a revoke control
    const grantsSection = screen
      .getByText("customPermissions.grantsTitle")
      .closest("div");
    expect(grantsSection).toBeTruthy();
    expect(
      within(grantsSection as HTMLElement).getByTestId(
        "revoke-perm-analytics:sales",
      ),
    ).toBeInTheDocument();

    // Expand bills category — bills:read is role-default + effective → Deny
    fireEvent.click(screen.getByText("Bills"));
    expect(
      await screen.findByTestId("deny-perm-bills:read"),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId("revoke-perm-bills:read"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("revoke-perm-cat-bills:read"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("grant-perm-bills:read"),
    ).not.toBeInTheDocument();
  });

  it("shows denied state and clears deny via API", async () => {
    renderModal();
    await waitFor(() => expect(mockGetStaffPermissions).toHaveBeenCalled());

    expect(
      screen.getByText("customPermissions.deniesTitle"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("clear-deny-menu:read")).toBeInTheDocument();

    fireEvent.click(screen.getByText("Menu"));
    expect(
      await screen.findByTestId("denied-chip-menu:read"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("clear-deny-cat-menu:read")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("clear-deny-menu:read"));
    await waitFor(() =>
      expect(mockRemoveDeny).toHaveBeenCalledWith("42", "7", {
        permission: "menu:read",
        reason: undefined,
      }),
    );
  });

  it("denies an effective role permission via API", async () => {
    renderModal();
    await waitFor(() => expect(mockGetStaffPermissions).toHaveBeenCalled());

    fireEvent.click(screen.getByText("Bills"));
    fireEvent.click(await screen.findByTestId("deny-perm-bills:read"));

    await waitFor(() =>
      expect(mockDeny).toHaveBeenCalledWith("42", "7", {
        permission: "bills:read",
        reason: undefined,
      }),
    );
  });

  it("gates sensitive grant changes behind the confirmation modal and cancels safely", async () => {
    renderModal({ isOwner: true });
    await waitFor(() => expect(mockGetStaffPermissions).toHaveBeenCalled());

    fireEvent.click(screen.getByText("Director"));
    fireEvent.click(await screen.findByTestId("grant-perm-director:read"));

    // A styled confirmation modal appears; the grant has not fired yet.
    const dialog = await screen.findByText(
      "customPermissions.sensitiveConfirmTitle",
    );
    expect(dialog).toBeInTheDocument();
    expect(mockGrant).not.toHaveBeenCalled();

    // Cancelling drops the pending action without calling the API.
    fireEvent.click(
      screen.getByText("customPermissions.sensitiveConfirmCancel"),
    );
    await waitFor(() =>
      expect(
        screen.queryByText("customPermissions.sensitiveConfirmTitle"),
      ).not.toBeInTheDocument(),
    );
    expect(mockGrant).not.toHaveBeenCalled();
  });

  it("performs the sensitive grant only after confirming in the modal", async () => {
    renderModal({ isOwner: true });
    await waitFor(() => expect(mockGetStaffPermissions).toHaveBeenCalled());

    fireEvent.click(screen.getByText("Director"));
    fireEvent.click(await screen.findByTestId("grant-perm-director:read"));

    fireEvent.click(
      await screen.findByText("customPermissions.sensitiveConfirmButton"),
    );

    await waitFor(() =>
      expect(mockGrant).toHaveBeenCalledWith("42", "7", {
        permission: "director:read",
        reason: undefined,
      }),
    );
  });

  it("revokes a custom grant via the API", async () => {
    renderModal();
    await waitFor(() => expect(mockGetStaffPermissions).toHaveBeenCalled());

    fireEvent.click(screen.getByTestId("revoke-perm-analytics:sales"));

    await waitFor(() =>
      expect(mockRevoke).toHaveBeenCalledWith("42", "7", {
        permission: "analytics:sales",
        reason: undefined,
      }),
    );
  });

  it("shows custom permissions when isOwner even without canEditPermissions", async () => {
    renderModal({ isOwner: true, canEditPermissions: false });
    await waitFor(() =>
      expect(mockGetStaffPermissions).toHaveBeenCalledWith("42", "7"),
    );
    expect(screen.getByText("analytics:sales")).toBeInTheDocument();
  });

  // L5-24 Path B: deactivate must not fire until ConfirmationModal confirm.
  it("does not call deactivateStaff until the confirm modal is confirmed", async () => {
    mockDeactivate.mockResolvedValue({ message: "ok" });
    renderModal();
    await waitFor(() => expect(mockAudit).toHaveBeenCalled());

    const reasonField = screen.getByPlaceholderText(
      "staffStatus.deactivate.reasonPlaceholder",
    );
    fireEvent.change(reasonField, { target: { value: "left the team" } });

    fireEvent.click(screen.getByText("staffStatus.deactivate.button"));

    // Confirm modal with soft/reversible copy appears; API not yet called.
    expect(
      await screen.findByText("staffStatus.deactivate.confirmTitle"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("staffStatus.deactivate.confirmDescription"),
    ).toBeInTheDocument();
    expect(mockDeactivate).not.toHaveBeenCalled();

    fireEvent.click(
      screen.getByText("staffStatus.deactivate.confirmAction"),
    );
    await waitFor(() =>
      expect(mockDeactivate).toHaveBeenCalledWith("42", "7", {
        reason: "left the team",
      }),
    );
  });

  it("filters grant catalog to actor grantable permissions", async () => {
    renderModal({
      grantablePermissions: ["crm:read", "bills:read", "menu:read"],
    });
    await waitFor(() => expect(mockGetStaffPermissions).toHaveBeenCalled());

    // CRM is grantable
    fireEvent.click(screen.getByText("Crm"));
    expect(
      await screen.findByTestId("grant-perm-crm:read"),
    ).toBeInTheDocument();

    // Director is outside actor role set — category omitted
    expect(screen.queryByText("Director")).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("grant-perm-director:read"),
    ).not.toBeInTheDocument();
  });

  it("shows full catalog for owners even without grantablePermissions", async () => {
    renderModal({ isOwner: true, grantablePermissions: null });
    await waitFor(() => expect(mockGetStaffPermissions).toHaveBeenCalled());
    fireEvent.click(screen.getByText("Director"));
    expect(
      await screen.findByTestId("grant-perm-director:read"),
    ).toBeInTheDocument();
  });

  it("reactivates an inactive staff membership and refreshes the staff list", async () => {
    mockReactivate.mockResolvedValue({ message: "ok" });
    const onStaffUpdated = jest.fn();
    const onClose = jest.fn();
    renderModal({
      staffMember: { ...staffMember, is_active: false },
      onStaffUpdated,
      onClose,
    });

    fireEvent.click(
      await screen.findByRole("button", {
        name: "staffStatus.reactivate.button",
      }),
    );

    await waitFor(() =>
      expect(mockReactivate).toHaveBeenCalledWith("42", "7", { reason: "" }),
    );
    expect(onStaffUpdated).toHaveBeenCalledTimes(1);
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
