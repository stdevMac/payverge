/** @jest-environment node */
import { resolveBackendLocaleSeed } from "../operatorLocaleBridge";

describe("resolveBackendLocaleSeed — OP-2 backend-preference → render bridge", () => {
  test("applies a saved Spanish backend preference when no explicit locale is stored", () => {
    expect(
      resolveBackendLocaleSeed({ saved: "es", storedLocale: null }),
    ).toBe("es");
  });

  test("applies a saved Argentine-Spanish backend preference", () => {
    expect(
      resolveBackendLocaleSeed({ saved: "es-AR", storedLocale: null }),
    ).toBe("es-AR");
  });

  test("treats an empty stored locale the same as no stored locale", () => {
    expect(resolveBackendLocaleSeed({ saved: "es", storedLocale: "" })).toBe(
      "es",
    );
  });

  test("does NOT clobber an explicit in-session locale pick", () => {
    // Operator already picked English this session (locale key set); a saved
    // Spanish backend preference must not override the explicit choice.
    expect(
      resolveBackendLocaleSeed({ saved: "es", storedLocale: "en" }),
    ).toBeNull();
    // Even when the stored pick equals the saved preference, there is nothing
    // to apply (already in sync) — returning null avoids a redundant setLocale.
    expect(
      resolveBackendLocaleSeed({ saved: "es", storedLocale: "es" }),
    ).toBeNull();
  });

  test("rejects non-operator / guest-only locale codes", () => {
    // "fr"/"ja" are guest-tier menu-translation locales, not operator dashboard
    // locales — they must never seed the operator render path.
    expect(
      resolveBackendLocaleSeed({ saved: "fr", storedLocale: null }),
    ).toBeNull();
    expect(
      resolveBackendLocaleSeed({ saved: "ja", storedLocale: null }),
    ).toBeNull();
  });

  test("returns null for empty / missing saved preference", () => {
    expect(
      resolveBackendLocaleSeed({ saved: "", storedLocale: null }),
    ).toBeNull();
    expect(
      resolveBackendLocaleSeed({ saved: null, storedLocale: null }),
    ).toBeNull();
    expect(
      resolveBackendLocaleSeed({ saved: undefined, storedLocale: null }),
    ).toBeNull();
  });

  test("rejects unsupported / garbage codes", () => {
    expect(
      resolveBackendLocaleSeed({ saved: "xx-YY", storedLocale: null }),
    ).toBeNull();
  });
});
