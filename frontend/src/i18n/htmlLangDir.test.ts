import {
  resolveHtmlLang,
  resolveHtmlDir,
  resolveOperatorLocale,
} from "./htmlLangDir";

// SSR <html lang>/<dir> derivation. The middleware writes x-payverge-locale;
// layout.tsx reads it and must emit a correct first-paint lang + dir BEFORE any
// client effect runs. Operator routes carry en|es|es-ar; guest storefront
// ?lang= flows carry any canonical storefront code (e.g. ar, es-AR, fr).

describe("resolveHtmlLang", () => {
  test("maps the operator header values to canonical BCP-47 lang", () => {
    expect(resolveHtmlLang("en")).toBe("en");
    expect(resolveHtmlLang("es")).toBe("es");
    // es-ar must become canonical es-AR, never collapse to en.
    expect(resolveHtmlLang("es-ar")).toBe("es-AR");
  });

  test("passes through a canonical storefront code from a guest ?lang= flow", () => {
    expect(resolveHtmlLang("ar")).toBe("ar");
    expect(resolveHtmlLang("es-AR")).toBe("es-AR");
    expect(resolveHtmlLang("fr")).toBe("fr");
  });

  test("resolves case-insensitively against the registry", () => {
    // BCP-47 tags are case-insensitive; a lowercased "ar" or odd-cased "ES-AR"
    // must still land on the canonical registry casing.
    expect(resolveHtmlLang("ES-AR")).toBe("es-AR");
  });

  test("defaults unknown / missing values to en", () => {
    expect(resolveHtmlLang(null)).toBe("en");
    expect(resolveHtmlLang(undefined)).toBe("en");
    expect(resolveHtmlLang("")).toBe("en");
    expect(resolveHtmlLang("zz")).toBe("en");
  });
});

describe("resolveHtmlDir", () => {
  test("returns rtl for Arabic, ltr for LTR locales — driven by the registry", () => {
    expect(resolveHtmlDir("ar")).toBe("rtl");
    expect(resolveHtmlDir("en")).toBe("ltr");
    expect(resolveHtmlDir("es")).toBe("ltr");
    expect(resolveHtmlDir("es-ar")).toBe("ltr");
    expect(resolveHtmlDir("es-AR")).toBe("ltr");
  });

  test("defaults unknown / missing values to ltr", () => {
    expect(resolveHtmlDir(null)).toBe("ltr");
    expect(resolveHtmlDir("zz")).toBe("ltr");
  });
});

describe("resolveOperatorLocale", () => {
  test("keeps operator-tier codes (en/es/es-AR) for SimpleTranslationProvider", () => {
    expect(resolveOperatorLocale("en")).toBe("en");
    expect(resolveOperatorLocale("es")).toBe("es");
    expect(resolveOperatorLocale("es-ar")).toBe("es-AR");
    expect(resolveOperatorLocale("ES-AR")).toBe("es-AR");
  });

  test("collapses guest-only storefront codes to en (operator UI never ships them)", () => {
    // A guest /b/slug?lang=ar carries ar in the header for <html lang>/<dir>,
    // but the operator dashboard provider must not be seeded with ar.
    expect(resolveOperatorLocale("ar")).toBe("en");
    expect(resolveOperatorLocale("fr")).toBe("en");
    expect(resolveOperatorLocale(null)).toBe("en");
  });
});
