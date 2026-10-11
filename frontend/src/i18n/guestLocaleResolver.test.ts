import {
  GUEST_LOCALE_COOKIE,
  buildGuestLangHref,
  guestPathLocaleFromPathname,
  parseAcceptLanguageHeader,
  resolveGuestEntryLocale,
  resolveGuestLocale,
  resolveGuestRequestLocale,
  writeGuestLocaleCookie,
  readGuestLocaleCookie,
} from "./guestLocaleResolver";

describe("resolveGuestLocale", () => {
  it("honours explicit ?lang=", () => {
    expect(resolveGuestLocale("es").locale).toBe("es");
    expect(resolveGuestLocale("es").isExplicit).toBe(true);
    expect(resolveGuestLocale("es-AR").locale).toBe("es-AR");
    expect(resolveGuestLocale("ES-ar").locale).toBe("es-AR");
  });

  it("defaults to English when lang is missing or unknown", () => {
    expect(resolveGuestLocale(null)).toEqual({
      locale: "en",
      isExplicit: false,
    });
    expect(resolveGuestLocale("zz").locale).toBe("en");
  });
});

describe("resolveGuestEntryLocale", () => {
  it("prefers explicit ?lang= over Accept-Language", () => {
    const resolved = resolveGuestEntryLocale({
      langParam: "es",
      acceptLanguage: "en-US,en;q=0.9",
    });
    expect(resolved).toEqual({ locale: "es", isExplicit: true });
  });

  it("uses Accept-Language when ?lang= is absent", () => {
    expect(
      resolveGuestEntryLocale({
        langParam: null,
        acceptLanguage: "es-ES,es;q=0.9,en;q=0.8",
      }).locale,
    ).toBe("es");
    expect(
      resolveGuestEntryLocale({
        langParam: undefined,
        acceptLanguage: "es-AR,es;q=0.9",
      }).locale,
    ).toBe("es-AR");
  });

  it("maps es-419 browser tags to es-AR", () => {
    expect(
      resolveGuestEntryLocale({
        acceptLanguage: "es-419,es;q=0.8",
      }).locale,
    ).toBe("es-AR");
  });

  it("accepts browserLanguages array (client path)", () => {
    expect(
      resolveGuestEntryLocale({
        browserLanguages: ["pt-BR", "pt", "en"],
      }).locale,
    ).toBe("pt");
  });

  it("falls back to English", () => {
    expect(resolveGuestEntryLocale({}).locale).toBe("en");
    expect(
      resolveGuestEntryLocale({ acceptLanguage: "xx-YY" }).locale,
    ).toBe("en");
  });
});

describe("parseAcceptLanguageHeader", () => {
  it("splits and strips quality values", () => {
    expect(parseAcceptLanguageHeader("es-AR,es;q=0.9,en;q=0.8")).toEqual([
      "es-AR",
      "es",
      "en",
    ]);
  });

  it("handles empty input", () => {
    expect(parseAcceptLanguageHeader(null)).toEqual([]);
    expect(parseAcceptLanguageHeader("")).toEqual([]);
  });
});

describe("resolveGuestRequestLocale (PG-21 / PG-12)", () => {
  it("prefers ?lang= over guest cookie and Accept-Language", () => {
    expect(
      resolveGuestRequestLocale({
        langParam: "ja",
        guestCookie: "es",
        acceptLanguage: "fr,en;q=0.8",
      }),
    ).toEqual({ locale: "ja", isExplicit: true });
  });

  it("uses guest cookie when ?lang= is absent (user pick survives SSR)", () => {
    expect(
      resolveGuestRequestLocale({
        langParam: null,
        guestCookie: "es-AR",
        acceptLanguage: "en-US,en;q=0.9",
      }),
    ).toEqual({ locale: "es-AR", isExplicit: true });
  });

  it("uses Accept-Language when neither ?lang= nor guest cookie is set", () => {
    expect(
      resolveGuestRequestLocale({
        acceptLanguage: "es-ES,es;q=0.9,en;q=0.8",
      }).locale,
    ).toBe("es");
  });
});

describe("guest locale cookie helpers (node-safe)", () => {
  it("exports the cookie name used by middleware", () => {
    expect(GUEST_LOCALE_COOKIE).toBe("payverge_guest_locale");
  });

  it("writeGuestLocaleCookie is a no-op without document", () => {
    expect(() => writeGuestLocaleCookie("ja")).not.toThrow();
  });

  it("readGuestLocaleCookie returns null without document", () => {
    expect(readGuestLocaleCookie()).toBeNull();
  });
});

describe("guestPathLocaleFromPathname (#860)", () => {
  it("reads /es and /es-ar storefront prefixes as explicit guest locales", () => {
    expect(guestPathLocaleFromPathname("/es/b/parrilla-quebracho-azul")).toBe("es");
    expect(guestPathLocaleFromPathname("/es-ar/b/bodegon-mesa-larga")).toBe(
      "es-AR",
    );
    expect(guestPathLocaleFromPathname("/es/t/FV214XU12D")).toBe("es");
    expect(guestPathLocaleFromPathname("/es-ar/t/FV214XU12D/menu")).toBe(
      "es-AR",
    );
  });

  it("does not invent a locale for bare /b or /t paths", () => {
    expect(guestPathLocaleFromPathname("/b/parrilla-quebracho-azul")).toBeNull();
    expect(guestPathLocaleFromPathname("/t/FV214XU12D")).toBeNull();
    expect(guestPathLocaleFromPathname("/")).toBeNull();
  });
});

describe("buildGuestLangHref (#401)", () => {
  it("preserves storefront tab hashes across locale changes", () => {
    expect(
      buildGuestLangHref("/b/demo-kitchen", "lang=es", "en", "#reservations"),
    ).toBe("/b/demo-kitchen?lang=en#reservations");
    expect(
      buildGuestLangHref("/b/demo-kitchen", "?lang=es", "en", "menu"),
    ).toBe("/b/demo-kitchen?lang=en#menu");
    expect(
      buildGuestLangHref("/b/demo-kitchen", "lang=es&utm=1", "es-AR", "#delivery"),
    ).toBe("/b/demo-kitchen?lang=es-AR&utm=1#delivery");
  });

  it("omits an empty hash and still writes ?lang=", () => {
    expect(buildGuestLangHref("/t/TABLE-1", "", "es-AR")).toBe(
      "/t/TABLE-1?lang=es-AR",
    );
  });
});
