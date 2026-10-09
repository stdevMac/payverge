import { ACCOUNT_TABS, resolveAccountTab } from "./accountTabs";

describe("resolveAccountTab", () => {
  it("exposes the three account tabs in display order", () => {
    expect(ACCOUNT_TABS).toEqual(["account", "notifications", "privacy"]);
  });

  it("returns the requested tab when valid", () => {
    expect(resolveAccountTab("notifications")).toBe("notifications");
    expect(resolveAccountTab("privacy")).toBe("privacy");
    expect(resolveAccountTab("account")).toBe("account");
  });

  it("defaults to account for missing or unknown tabs", () => {
    expect(resolveAccountTab(null)).toBe("account");
    expect(resolveAccountTab(undefined)).toBe("account");
    expect(resolveAccountTab("")).toBe("account");
    expect(resolveAccountTab("settings")).toBe("account");
  });
});
