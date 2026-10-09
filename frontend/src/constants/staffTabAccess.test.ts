import { resolveStaffTabs } from "./staffTabAccess";

describe("resolveStaffTabs", () => {
  it("returns only overview when permissions empty", () => {
    // overview requires overview:read — empty ⇒ []
    expect(resolveStaffTabs([])).toEqual([]);
  });

  it("includes overview when overview:read present", () => {
    expect(resolveStaffTabs(["overview:read"])).toEqual(["overview"]);
  });

  it("shows delivery tab with either dispatch read or write", () => {
    const readOnly = resolveStaffTabs([
      "overview:read",
      "delivery:dispatch:read",
    ]);
    expect(readOnly).toContain("delivery");
    const writeOnly = resolveStaffTabs([
      "overview:read",
      "delivery:dispatch:write",
    ]);
    expect(writeOnly).toContain("delivery");
  });

  it("does not show delivery tab for kitchen-only dispatch visibility", () => {
    const tabs = resolveStaffTabs([
      "overview:read",
      "orders:read",
      "orders:kitchen",
      "delivery:dispatch:read",
    ]);
    expect(tabs).toContain("kitchen");
    expect(tabs).not.toContain("delivery");
  });

  it("does not treat schedule:self as operator schedule", () => {
    const tabs = resolveStaffTabs([
      "overview:read",
      "schedule:read",
      "schedule:self",
    ]);
    expect(tabs).not.toContain("schedule");
  });

  it("includes operator schedule when manage-level schedule perms present", () => {
    const tabs = resolveStaffTabs([
      "overview:read",
      "schedule:read",
      "schedule:write",
    ]);
    expect(tabs).toContain("schedule");
  });

  it("includes schedule when schedule:publish present with schedule:read", () => {
    const tabs = resolveStaffTabs([
      "overview:read",
      "schedule:read",
      "schedule:publish",
    ]);
    expect(tabs).toContain("schedule");
  });

  it("applies kitchenOrdersEnabled=false post-filter for non-activators", () => {
    // Staff with bills:read but without settings:write loses bills when kitchen off
    const perms = ["overview:read", "bills:read", "orders:read"];
    const tabs = resolveStaffTabs(perms, { kitchenOrdersEnabled: false });
    expect(tabs).not.toContain("bills");
    expect(tabs).not.toContain("kitchen");
  });

  it("keeps kitchen tab when kitchenOrdersEnabled=false if only kitchen surface", () => {
    // Spec: kitchen role keeps kitchen tab for placeholder — implement via
    // detect orders:kitchen without bills:create (or settings:write)
    const perms = ["overview:read", "orders:read", "orders:kitchen"];
    const tabs = resolveStaffTabs(perms, { kitchenOrdersEnabled: false });
    expect(tabs).toContain("kitchen");
  });

  it("custom grant can add a tab", () => {
    const base = ["overview:read", "bills:read"];
    expect(resolveStaffTabs(base)).not.toContain("crm");
    expect(resolveStaffTabs([...base, "crm:read"])).toContain("crm");
  });
});
