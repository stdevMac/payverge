import fixture from "./__fixtures__/role-default-permissions.json";
import { resolveStaffTabs } from "./staffTabAccess";

describe("resolveStaffTabs vs role defaults", () => {
  it("manager includes reservations, crm, delivery, schedule, accounting", () => {
    const tabs = resolveStaffTabs(fixture.manager);
    for (const t of [
      "overview",
      "reservations",
      "crm",
      "delivery",
      "schedule",
      "accounting",
      "menu",
      "bills",
    ] as const) {
      expect(tabs).toContain(t);
    }
    expect(tabs).not.toContain("director-console");
    expect(tabs).not.toContain("subscriptions");
  });

  it("server includes delivery + reservations, not crm/schedule", () => {
    const tabs = resolveStaffTabs(fixture.server);
    expect(tabs).toContain("delivery");
    expect(tabs).toContain("reservations");
    expect(tabs).not.toContain("crm");
    expect(tabs).not.toContain("schedule");
    expect(tabs).not.toContain("accounting");
  });

  it("host has reservations, not delivery", () => {
    const tabs = resolveStaffTabs(fixture.host);
    expect(tabs).toContain("reservations");
    expect(tabs).not.toContain("delivery");
  });

  it("kitchen has kitchen, not delivery", () => {
    const tabs = resolveStaffTabs(fixture.kitchen);
    expect(tabs).toContain("kitchen");
    expect(tabs).not.toContain("delivery");
  });
});
