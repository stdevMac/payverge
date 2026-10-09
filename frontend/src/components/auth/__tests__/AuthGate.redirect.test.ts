import {
  buildAuthRedirectUrl,
  buildForgotPasswordHref,
  buildLoginReturnHref,
} from "../authRedirect";

describe("buildAuthRedirectUrl", () => {
  it("targets /dashboard (full AuthModal), not passwordless /staff/login", () => {
    const url = buildAuthRedirectUrl("/business/mara/dashboard?tab=bills");
    expect(url.startsWith("/dashboard")).toBe(true);
    expect(url).not.toContain("/staff/login");
  });

  it("carries the original path through as an encoded redirect param", () => {
    const url = buildAuthRedirectUrl("/business/mara/dashboard?tab=bills");
    expect(url).toContain(
      `redirect=${encodeURIComponent("/business/mara/dashboard?tab=bills")}`,
    );
  });

  it("returns unauthenticated installed-app launches to the protected resolver", () => {
    expect(buildAuthRedirectUrl("/app")).toBe("/dashboard?redirect=%2Fapp");
  });
});

describe("forgot-password redirect preservation (#403)", () => {
  const deepLink = "/business/1/dashboard?tab=accounting&sub=invoices";

  it("forwards a multi-query same-origin destination into recovery", () => {
    expect(buildForgotPasswordHref(deepLink)).toBe(
      `/forgot-password?redirect=${encodeURIComponent(deepLink)}`,
    );
  });

  it("returns that destination on back-to-login and post-reset login", () => {
    expect(buildLoginReturnHref(deepLink)).toBe(
      `/dashboard?redirect=${encodeURIComponent(deepLink)}`,
    );
  });

  it("drops external/open-redirect bait", () => {
    expect(buildForgotPasswordHref("https://evil.example/phish")).toBe(
      "/forgot-password",
    );
    expect(buildForgotPasswordHref("//evil.example")).toBe("/forgot-password");
    expect(buildLoginReturnHref("https://evil.example/phish")).toBe(
      "/dashboard",
    );
  });

  it("defaults when no redirect is present", () => {
    expect(buildForgotPasswordHref(null)).toBe("/forgot-password");
    expect(buildLoginReturnHref(undefined)).toBe("/dashboard");
  });

  it("sends es and es-AR recovery back to the localized sign-in dialog (#882)", () => {
    expect(buildLoginReturnHref(undefined, "es")).toBe(
      "/dashboard?auth=signin&lang=es",
    );
    expect(buildLoginReturnHref(undefined, "es-AR")).toBe(
      "/dashboard?auth=signin&lang=es-AR",
    );
    expect(buildLoginReturnHref(deepLink, "es")).toBe(
      `/dashboard?auth=signin&lang=es&redirect=${encodeURIComponent(deepLink)}`,
    );
    expect(buildLoginReturnHref(undefined, "en")).toBe("/dashboard");
  });
});
