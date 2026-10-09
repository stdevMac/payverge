import {
  VENUES_OVERVIEW_PATH,
  getBusinessDashboardPath,
  getBusinessFirstValuePath,
  getBusinessNotificationSettingsPath,
  getBusinessPageEditorPath,
  getOperatorDashboardPath,
  isPublicBusinessProtocolPath,
  resolveInitialSettingsTab,
} from "./businessUrl";

describe("operator vs public business routes (#377)", () => {
  it("treats /business/:id as the public custom-URL protocol shim", () => {
    expect(isPublicBusinessProtocolPath("/business/demo-admin-8-ai-pro")).toBe(
      true,
    );
    expect(isPublicBusinessProtocolPath("/business/42")).toBe(true);
    expect(isPublicBusinessProtocolPath("/business/42/")).toBe(true);
    expect(
      isPublicBusinessProtocolPath("/business/demo-admin-8-ai-pro?ref=nav"),
    ).toBe(true);
  });

  it("does not treat operator or registration paths as the public protocol", () => {
    expect(
      isPublicBusinessProtocolPath("/business/demo-admin-8-ai-pro/dashboard"),
    ).toBe(false);
    expect(
      isPublicBusinessProtocolPath(
        "/business/demo-admin-8-ai-pro/dashboard?tab=bills",
      ),
    ).toBe(false);
    expect(
      isPublicBusinessProtocolPath(
        "/business/demo-admin-8-ai-pro/bills/387/alternative-payments",
      ),
    ).toBe(false);
    expect(isPublicBusinessProtocolPath("/business/register")).toBe(false);
    expect(isPublicBusinessProtocolPath("/b/demo-admin-8-ai-pro")).toBe(false);
  });

  it("builds operator dashboard destinations from a route param (slug or numeric)", () => {
    expect(getOperatorDashboardPath("demo-admin-8-ai-pro")).toBe(
      "/business/demo-admin-8-ai-pro/dashboard",
    );
    expect(getOperatorDashboardPath("42", "bills")).toBe(
      "/business/42/dashboard?tab=bills",
    );
    expect(getOperatorDashboardPath("demo-admin-8-ai-pro")).not.toBe(
      "/business/demo-admin-8-ai-pro",
    );
    expect(
      isPublicBusinessProtocolPath(
        getOperatorDashboardPath("demo-admin-8-ai-pro"),
      ),
    ).toBe(false);
  });
});

describe("getBusinessFirstValuePath", () => {
  it("routes a newly created workspace into the menu first task", () => {
    expect(
      getBusinessFirstValuePath({ id: 42, business_id: "cafe-luna" }),
    ).toBe("/business/cafe-luna/dashboard?tab=menu&onboarding=first-value");
  });
});

describe("getBusinessNotificationSettingsPath", () => {
  it("deep-links to the Settings tab + notifications sub-section", () => {
    const business = { business_id: "mara-core-kitchen", id: 7 };
    expect(getBusinessNotificationSettingsPath(business)).toBe(
      "/business/mara-core-kitchen/dashboard?tab=settings&section=notifications",
    );
  });

  it("falls back to the numeric id when no string business_id exists", () => {
    expect(getBusinessNotificationSettingsPath({ id: 42 })).toBe(
      "/business/42/dashboard?tab=settings&section=notifications",
    );
  });

  it("builds on top of the shared dashboard path", () => {
    const business = { id: 9 };
    expect(getBusinessNotificationSettingsPath(business)).toContain(
      getBusinessDashboardPath(business),
    );
  });
});

describe("getBusinessPageEditorPath", () => {
  it("deep-links to the Business Page editor tab (mounts BusinessPageEditor)", () => {
    const business = { business_id: "mara-core-kitchen", id: 7 };
    expect(getBusinessPageEditorPath(business)).toBe(
      "/business/mara-core-kitchen/dashboard?tab=business-page",
    );
  });

  it("deep-links to Contact when Settings bounces there (#225)", () => {
    expect(getBusinessPageEditorPath({ id: 42 }, "contact")).toBe(
      "/business/42/dashboard?tab=business-page&section=contact",
    );
    // Essentials is the default — omit it so the URL stays clean.
    expect(getBusinessPageEditorPath({ id: 42 }, "essentials")).toBe(
      "/business/42/dashboard?tab=business-page",
    );
  });

  it("never emits the dead /design route", () => {
    expect(getBusinessPageEditorPath({ id: 42 })).toBe(
      "/business/42/dashboard?tab=business-page",
    );
    expect(getBusinessPageEditorPath({ id: 42 })).not.toContain("/design");
  });

  it("builds on top of the shared dashboard path", () => {
    const business = { id: 9 };
    expect(getBusinessPageEditorPath(business)).toContain(
      getBusinessDashboardPath(business),
    );
  });
});

describe("resolveInitialSettingsTab", () => {
  it("returns the requested section when it is a real settings tab", () => {
    expect(resolveInitialSettingsTab("notifications")).toBe("notifications");
    expect(resolveInitialSettingsTab("payments")).toBe("payments");
    expect(resolveInitialSettingsTab("localization")).toBe("localization");
    expect(resolveInitialSettingsTab("profile")).toBe("profile");
  });

  it("defaults to profile for null, empty, or unknown sections", () => {
    expect(resolveInitialSettingsTab(null)).toBe("profile");
    expect(resolveInitialSettingsTab(undefined)).toBe("profile");
    expect(resolveInitialSettingsTab("")).toBe("profile");
    expect(resolveInitialSettingsTab("bogus")).toBe("profile");
  });
});

describe("VENUES_OVERVIEW_PATH", () => {
  it("carries the ?venues=all opt-out the dashboard checks before its single-venue redirect", () => {
    const url = new URL(VENUES_OVERVIEW_PATH, "https://pos.example");
    expect(url.pathname).toBe("/dashboard");
    expect(url.searchParams.get("venues")).toBe("all");
  });
});
