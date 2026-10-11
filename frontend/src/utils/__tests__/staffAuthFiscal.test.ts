import fixture from "@/constants/__fixtures__/role-default-permissions.json";
import {
  getAllowedStaffTabs,
  getPermissionCheckedActiveTab,
  isDashboardPrincipalResolved,
} from "@/utils/staffAuth";

describe("staff fiscal dashboard access", () => {
  // fiscal is a legacy URL alias of accounting; resolveStaffTabs emits "accounting"
  // when the principal has fiscal:read or financial:read.
  it("includes the accounting tab for managers (fiscal access surface)", () => {
    expect(
      getAllowedStaffTabs("manager", { permissions: fixture.manager }),
    ).toContain("accounting");
  });

  it("does not include the accounting tab for service staff", () => {
    expect(
      getAllowedStaffTabs("server", { permissions: fixture.server }),
    ).not.toContain("accounting");
    expect(
      getAllowedStaffTabs("host", { permissions: fixture.host }),
    ).not.toContain("accounting");
    expect(
      getAllowedStaffTabs("kitchen", { permissions: fixture.kitchen }),
    ).not.toContain("accounting");
  });

  it("falls back before rendering unauthorized fiscal deep links after auth resolves", () => {
    expect(
      getPermissionCheckedActiveTab({
        activeTab: "fiscal",
        allowedTabs: getAllowedStaffTabs("server", {
          permissions: fixture.server,
        }),
        authResolved: true,
      }),
    ).toBe("overview");
  });

  it("keeps fiscal deep links while auth is still unresolved", () => {
    expect(
      getPermissionCheckedActiveTab({
        activeTab: "fiscal",
        allowedTabs: ["overview"],
        authResolved: false,
      }),
    ).toBe("fiscal");
  });

  it("keeps staff dashboard deep links unresolved until profile and permissions hydrate", () => {
    expect(
      isDashboardPrincipalResolved({
        isStaffUser: true,
        hasStaffData: false,
        staffPermissionsLoading: false,
        isWeb3User: false,
        isOAuthUser: false,
      }),
    ).toBe(false);

    expect(
      isDashboardPrincipalResolved({
        isStaffUser: true,
        hasStaffData: true,
        staffPermissionsLoading: true,
        isWeb3User: false,
        isOAuthUser: false,
      }),
    ).toBe(false);

    expect(
      isDashboardPrincipalResolved({
        isStaffUser: true,
        hasStaffData: true,
        staffPermissionsLoading: false,
        isWeb3User: false,
        isOAuthUser: false,
      }),
    ).toBe(true);
  });
});
