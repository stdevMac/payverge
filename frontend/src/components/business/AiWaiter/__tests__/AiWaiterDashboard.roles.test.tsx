import fixture from "@/constants/__fixtures__/role-default-permissions.json";
import { getAiWaiterCapabilities } from "@/utils/staffAuth";

// Unit-level guard for the permission→sub-view mapping the dashboard consumes.
// (Full RTL render of AiWaiterDashboard pulls heavy NextUI/SSR deps; the
// component reads getAiWaiterCapabilities, so we lock the contract here and
// assert the visible sub-tab set derived from it.)
function visibleSubTabs(
  permissions: string[] | null,
  isOwner: boolean,
): string[] {
  const caps = getAiWaiterCapabilities(permissions, isOwner);
  const tabs: string[] = [];
  if (caps.canViewConfig) tabs.push("overview");
  if (caps.canViewConversations) tabs.push("monitor");
  if (caps.canViewInsights) tabs.push("insights");
  return tabs;
}

describe("AiWaiterDashboard sub-view gating", () => {
  it("owner sees config + monitor + insights", () => {
    expect(visibleSubTabs(null, true)).toEqual([
      "overview",
      "monitor",
      "insights",
    ]);
  });
  it("manager sees monitor + insights, no config", () => {
    expect(visibleSubTabs(fixture.manager, false)).toEqual([
      "monitor",
      "insights",
    ]);
  });
  it("server sees monitor only", () => {
    expect(visibleSubTabs(fixture.server, false)).toEqual(["monitor"]);
  });
  it("host sees monitor only", () => {
    expect(visibleSubTabs(fixture.host, false)).toEqual(["monitor"]);
  });
  it("empty permissions fail closed for staff", () => {
    expect(visibleSubTabs([], false)).toEqual([]);
    expect(visibleSubTabs(null, false)).toEqual([]);
  });
});
