import { SECONDARY_TABS, PRIMARY_TABS, PRIMARY_TAB_GROUPS } from "../sidebar/sidebarConfig";

describe("sidebarConfig — fiscal restored + finance group (Task 32)", () => {
  it("lists fiscal under the Setup group so it is reachable from the sidebar", () => {
    // Finding #64: fiscal was declared in dashboard-tabs.json but missing from
    // both PRIMARY and SECONDARY, reachable only by typing ?tab=fiscal.
    expect(SECONDARY_TABS).toContain("fiscal");
    expect(PRIMARY_TABS).not.toContain("fiscal");
  });

  it("lists exactly one Accounting entry under Finance (not Setup)", () => {
    expect(PRIMARY_TAB_GROUPS.find((g) => g.id === "finance")?.tabs).toContain(
      "accounting",
    );
    expect(SECONDARY_TABS).not.toContain("accounting");
    expect(
      [...PRIMARY_TABS, ...SECONDARY_TABS].filter((k) => k === "accounting"),
    ).toHaveLength(1);
  });
});
