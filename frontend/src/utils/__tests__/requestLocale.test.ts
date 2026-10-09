import {
  hasDedicatedLocalePage,
  isUnprefixedPublicPagePath,
  preferStoredOperatorLocale,
  resolveOperatorRequestLocale,
  stripOperatorLocalePrefix,
} from "../requestLocale";

describe("preferStoredOperatorLocale — operator dashboard pin (#617)", () => {
  it("prefers stored English over an es-AR SSR locale on /business", () => {
    expect(
      preferStoredOperatorLocale({
        pathname: "/business/demo/dashboard",
        storedLocale: "en",
        initialLocale: "es-AR",
      }),
    ).toBe("en");
  });

  it("does not override marketing or prefixed paths", () => {
    expect(
      preferStoredOperatorLocale({
        pathname: "/terms-and-conditions",
        storedLocale: "en",
        initialLocale: "es-AR",
      }),
    ).toBeNull();
    expect(
      preferStoredOperatorLocale({
        pathname: "/es/privacy-policy",
        storedLocale: "en",
        initialLocale: "es",
      }),
    ).toBeNull();
  });
});

describe("resolveOperatorRequestLocale — path source of truth (#37)", () => {
  it("returns English for unprefixed public page despite cookie/AL/?lang=", () => {
    expect(
      resolveOperatorRequestLocale({
        pathname: "/terms-and-conditions",
        explicitLocale: "es",
        persistedLocale: "es-AR",
        acceptLanguage: "es-AR,es;q=0.9",
      }),
    ).toBe("en");
  });

  it("honors path prefixes", () => {
    expect(
      resolveOperatorRequestLocale({ pathname: "/es/terms-and-conditions" }),
    ).toBe("es");
    expect(
      resolveOperatorRequestLocale({ pathname: "/es-ar/privacy-policy" }),
    ).toBe("es-AR");
  });

  it("still uses cookie on dashboard routes", () => {
    expect(
      resolveOperatorRequestLocale({
        pathname: "/dashboard",
        persistedLocale: "es",
      }),
    ).toBe("es");
  });

  it("does not let Accept-Language flip operator chrome when no cookie is set (#617)", () => {
    expect(
      resolveOperatorRequestLocale({
        pathname: "/business/demo/dashboard?tab=tables",
        acceptLanguage: "es-AR,es;q=0.9,en;q=0.8",
      }),
    ).toBe("en");
    expect(
      resolveOperatorRequestLocale({
        pathname: "/dashboard",
        acceptLanguage: "es,es-AR;q=0.9",
      }),
    ).toBe("en");
  });

  it("still honors an explicit English cookie over Accept-Language on tab URLs (#617)", () => {
    expect(
      resolveOperatorRequestLocale({
        pathname: "/business/demo/dashboard",
        persistedLocale: "en",
        acceptLanguage: "es-AR,es;q=0.9",
      }),
    ).toBe("en");
  });

  it.each([
    "/business/register",
    "/forgot-password",
    "/staff/login",
  ])("honors a persisted Spanish locale on auth/register route %s (#402)", (pathname) => {
    expect(
      resolveOperatorRequestLocale({
        pathname,
        persistedLocale: "es",
      }),
    ).toBe("es");
    expect(
      resolveOperatorRequestLocale({
        pathname,
        persistedLocale: "es-AR",
      }),
    ).toBe("es-AR");
  });

  it("keeps unprefixed public pages English even when auth cookie is Spanish", () => {
    expect(
      resolveOperatorRequestLocale({
        pathname: "/privacy-policy",
        persistedLocale: "es",
      }),
    ).toBe("en");
  });
});

describe("locale prefix helpers", () => {
  it("strips /es and /es-ar prefixes", () => {
    expect(stripOperatorLocalePrefix("/es")).toBe("/");
    expect(stripOperatorLocalePrefix("/es/terms-and-conditions")).toBe(
      "/terms-and-conditions",
    );
    expect(stripOperatorLocalePrefix("/es-ar/privacy-policy")).toBe("/privacy-policy");
  });

  it("detects unprefixed public page paths", () => {
    expect(isUnprefixedPublicPagePath("/privacy-policy")).toBe(true);
    expect(isUnprefixedPublicPagePath("/es/privacy-policy")).toBe(false);
    expect(isUnprefixedPublicPagePath("/dashboard")).toBe(false);
    expect(isUnprefixedPublicPagePath("/business/register")).toBe(false);
    expect(isUnprefixedPublicPagePath("/forgot-password")).toBe(false);
    expect(isUnprefixedPublicPagePath("/staff/login")).toBe(false);
  });

  it("knows which locale pages are dedicated vs rewrite-backed", () => {
    // Locale roots rewrite to the instance home ("/") with the path locale.
    expect(hasDedicatedLocalePage("es", "/")).toBe(false);
    expect(hasDedicatedLocalePage("es-AR", "/")).toBe(false);
    expect(hasDedicatedLocalePage("es", "/privacy-policy")).toBe(false);
    expect(hasDedicatedLocalePage("es", "/refund/foo")).toBe(false);
    expect(hasDedicatedLocalePage("es", "/staff/home")).toBe(true);
    expect(hasDedicatedLocalePage("es-AR", "/staff/home")).toBe(true);
  });
});
