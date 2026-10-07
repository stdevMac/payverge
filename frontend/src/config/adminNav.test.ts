/** @jest-environment node */
import { findAdminNavItem, ADMIN_NAV_ITEMS } from "@/config/adminNav";

describe("adminNav", () => {
  it("finds dashboard for exact /admin path", () => {
    expect(findAdminNavItem("/admin")?.key).toBe("dashboard");
  });

  it("finds nested routes by prefix", () => {
    expect(findAdminNavItem("/admin/users")?.key).toBe("users");
    expect(findAdminNavItem("/admin/plugins/stripe")?.key).toBe("plugins");
  });

  it("exposes the instance admin sections and no SaaS billing or sales consoles", () => {
    const keys = ADMIN_NAV_ITEMS.map((item) => item.key);
    expect(keys).toEqual(
      expect.arrayContaining([
        "dashboard",
        "users",
        "businesses",
        "plugins",
        "emails",
        "errors",
        "fiscal",
        "translations",
        "escalations",
        "analytics",
        "demo",
      ]),
    );
    expect(keys).not.toContain("stripe");
    expect(keys).not.toContain("leads");
    expect(keys).not.toContain("referrals");
    expect(findAdminNavItem("/admin/stripe")).toBeUndefined();
  });
});
