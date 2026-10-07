/**
 * L4-22 — language default should follow operator locale, not hard "en".
 */
import { defaultCaptionLanguage } from "../defaultCaptionLanguage";

describe("defaultCaptionLanguage (L4-22)", () => {
  it("prefers profile then business language", () => {
    expect(
      defaultCaptionLanguage({
        profileLang: "en",
        businessLang: "es",
        operatorLocale: "es",
      }),
    ).toBe("en");
    expect(
      defaultCaptionLanguage({
        profileLang: "",
        businessLang: "es",
        operatorLocale: "en",
      }),
    ).toBe("es");
  });

  it("derives es from operator locale when no profile/business default", () => {
    expect(
      defaultCaptionLanguage({
        profileLang: null,
        businessLang: null,
        operatorLocale: "es-AR",
      }),
    ).toBe("es");
  });
});
