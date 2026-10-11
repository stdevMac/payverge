import { resolveRuntimeLocale } from "../SimpleTranslationProvider";
import { getRequestLocale } from "@/utils/requestLocale";

// The operator SSR locale contract: middleware computes getRequestLocale(path)
// → x-payverge-locale → layout.tsx seeds SimpleTranslationProvider's
// initialLocale; on the client resolveRuntimeLocale re-derives from the path +
// saved cookie. Both must route /es-ar/* to es-AR (not fall back to en/es).
describe("es-AR runtime/SSR locale resolution", () => {
  test("getRequestLocale maps the es-ar route prefix", () => {
    expect(getRequestLocale("/es-ar")).toBe("es-ar");
    expect(getRequestLocale("/es-ar/blog")).toBe("es-ar");
    expect(getRequestLocale("/es/blog")).toBe("es");
    expect(getRequestLocale("/dashboard")).toBe("en");
  });

  test("resolveRuntimeLocale prefers the es-ar route prefix", () => {
    expect(resolveRuntimeLocale("/es-ar/blog", null)).toBe("es-AR");
    expect(resolveRuntimeLocale("/es/blog", null)).toBe("es");
  });

  test("resolveRuntimeLocale falls back to a saved operator locale off-route", () => {
    expect(resolveRuntimeLocale("/dashboard", "es-AR")).toBe("es-AR");
    expect(resolveRuntimeLocale("/dashboard", "es")).toBe("es");
    expect(resolveRuntimeLocale("/dashboard", null)).toBe("en");
    // an invalid saved value degrades to the default, never crashes
    expect(resolveRuntimeLocale("/dashboard", "zz")).toBe("en");
  });

  test("resolveRuntimeLocale honors saved locale on unprefixed auth/register routes (#402)", () => {
    expect(resolveRuntimeLocale("/business/register", "es")).toBe("es");
    expect(resolveRuntimeLocale("/forgot-password", "es-AR")).toBe("es-AR");
    expect(resolveRuntimeLocale("/staff/login", "es")).toBe("es");
  });

  test("guest public paths (/b/, /t/) ignore the route prefix and use the saved locale", () => {
    // GuestTranslationProvider owns guest pages; the operator resolver must not
    // hijack them off a leading path segment.
    expect(resolveRuntimeLocale("/b/some-slug", "es-AR")).toBe("es-AR");
    expect(resolveRuntimeLocale("/t/table-code", "es")).toBe("es");
  });
});
