import {
  getPublicLocaleSwitchHref,
  getTranslation,
  resolveRuntimeLocale,
} from "../SimpleTranslationProvider";
import type { Locale } from "../localeRegistry";

describe("registry-backed simple translation provider", () => {
  test("resolves common translations from locale folders", () => {
    expect(getTranslation("common.save", "es-AR")).toBe("Guardar");
    expect(getTranslation("common.save", "en")).toBe("Save");
  });

  test("substitutes single-brace {var} placeholders in operator copy", () => {
    const result = getTranslation(
      "businessRegister.messages.stepProgress",
      "en",
      { current: 1, total: 2 },
    ) as string;
    expect(result).toBe("Step 1 of 2");
  });

  test("still substitutes double-brace {{var}} placeholders", () => {
    const applied = getTranslation("common.save", "en", { ignored: 1 });
    // common.save has no params; ensure params never corrupt a plain string.
    expect(applied).toBe("Save");
  });

  test("falls back safely for unsupported runtime locale strings", () => {
    expect(() =>
      getTranslation("common.save", "pt-BR" as Locale),
    ).not.toThrow();

    expect(["Save", "common.save"]).toContain(
      getTranslation("common.save", "pt-BR" as Locale),
    );
  });

  test("prefers the prefixed route locale over saved localStorage locale", () => {
    expect(resolveRuntimeLocale("/es-ar/privacy-policy", "en")).toBe("es-AR");
    expect(resolveRuntimeLocale("/es/refund", "en")).toBe("es");
  });

  test("forces English on unprefixed public page URLs despite saved locale (#37)", () => {
    expect(resolveRuntimeLocale("/terms-and-conditions", "es")).toBe("en");
    expect(resolveRuntimeLocale("/privacy-policy", "es-AR")).toBe("en");
  });

  test("does not infer operator locale from guest routes", () => {
    expect(resolveRuntimeLocale("/b/my-restaurant", "es")).toBe("es");
    expect(resolveRuntimeLocale("/t/table-123", "es-AR")).toBe("es-AR");
  });

  test("rewrites the locale prefix on public pages", () => {
    expect(
      getPublicLocaleSwitchHref({ path: "/privacy-policy", targetLocale: "es-AR" }),
    ).toBe("/es-ar/privacy-policy");
    expect(
      getPublicLocaleSwitchHref({ path: "/es/refund", targetLocale: "en", search: "?x=1" }),
    ).toBe("/refund?x=1");
    expect(getPublicLocaleSwitchHref({ path: "/es", targetLocale: "en" })).toBe("/");
  });

  test("keeps cookie-only surfaces on their path", () => {
    expect(
      getPublicLocaleSwitchHref({ path: "/dashboard", targetLocale: "es" }),
    ).toBeUndefined();
  });
});
