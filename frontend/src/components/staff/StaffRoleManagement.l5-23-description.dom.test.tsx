/** @jest-environment jsdom */
/**
 * D1 / L5-23: permission descriptions must RENDER in the custom-permissions
 * list — not merely exist as a helper. Expanding CRM must show the
 * human description for crm:read under the raw key, not only the slug.
 */
import React from "react";
import {
  fireEvent,
  render,
  screen,
  waitFor,
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
  SENSITIVE_PERMISSIONS: new Set(["financial:withdraw"]),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const enRbac = require("@/i18n/messages/en/rbacPermissions.json") as Record<
    string,
    { label?: string; description?: string }
  >;
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: (key: string) => {
      // StaffRoleManagement permT uses keys under rbacPermissions.*
      if (key.startsWith("rbacPermissions.")) {
        const rest = key.slice("rbacPermissions.".length);
        // keys are like "crm:read.description"
        const lastDot = rest.lastIndexOf(".");
        if (lastDot > 0) {
          const slug = rest.slice(0, lastDot);
          const field = rest.slice(lastDot + 1) as "label" | "description";
          const entry = enRbac[slug];
          if (entry?.[field]) return entry[field] as string;
        }
      }
      if (key.startsWith("businessDashboard.dashboard.staffManagement.rbac.")) {
        const stripped = key.replace(
          /^businessDashboard\.dashboard\.staffManagement\.rbac\./,
          "",
        );
        if (stripped.startsWith("rbacCategories.")) {
          const cat = stripped.replace("rbacCategories.", "");
          if (cat === "crm") return "Crm";
          return cat.charAt(0).toUpperCase() + cat.slice(1);
        }
        return stripped;
      }
      return key;
    },
  };
});

jest.mock("react-hot-toast", () => ({
  toast: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("./StaffCompensationSection", () => ({
  __esModule: true,
  default: () => null,
}));

const mockGetStaffPermissions = RBACAPI.getStaffPermissions as jest.Mock;
const mockAudit = RBACAPI.getStaffAuditLog as jest.Mock;

describe("StaffRoleManagement L5-23 permission description DOM (D1)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockGetStaffPermissions.mockResolvedValue({
      permissions: ["bills:read"],
      role_permissions: ["bills:read"],
      custom_grants: [],
      custom_denies: [],
    });
    mockAudit.mockResolvedValue({ audit_logs: [] });
  });

  it("renders the human description for crm:read under the permission row", async () => {
    const enRbac = require("@/i18n/messages/en/rbacPermissions.json") as Record<
      string,
      { description?: string }
    >;
    const expectedDesc = enRbac["crm:read"]?.description;
    expect(expectedDesc).toBeTruthy();
    expect(expectedDesc).not.toMatch(/^crm:read$/);

    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    render(
      <QueryClientProvider client={qc}>
        <StaffRoleManagement
          isOpen
          onClose={jest.fn()}
          staffMember={{
            id: 7,
            name: "Alex Server",
            email: "alex@example.com",
            role: "server",
            is_active: true,
          }}
          businessId="42"
          onStaffUpdated={jest.fn()}
          isOwner={false}
          canEditPermissions
        />
      </QueryClientProvider>,
    );

    await waitFor(() =>
      expect(mockGetStaffPermissions).toHaveBeenCalledWith("42", "7"),
    );

    fireEvent.click(await screen.findByText("Crm"));

    // Label may be humanized; description must appear as real product copy.
    expect(await screen.findByText(expectedDesc!)).toBeInTheDocument();
    // Must not only show the raw developer key as the only text for that row.
    const row = screen.getByText(expectedDesc!).closest("li");
    expect(row).toBeTruthy();
    expect(row!.textContent).toMatch(/crm|CRM|customer/i);
  });
});
