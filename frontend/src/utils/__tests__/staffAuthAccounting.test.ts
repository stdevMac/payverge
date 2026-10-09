import fixture from "@/constants/__fixtures__/role-default-permissions.json";
import { getAllowedStaffTabs, hasStaffPermission } from "@/utils/staffAuth";

describe("staff accounting permissions", () => {
  it("includes the accounting tab for managers", () => {
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

  it("treats accounting as a financial permission for managers only", () => {
    expect(hasStaffPermission(fixture.manager, "financial")).toBe(true);
    expect(hasStaffPermission(fixture.server, "financial")).toBe(false);
  });
});
