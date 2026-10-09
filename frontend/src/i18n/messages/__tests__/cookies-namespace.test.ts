import enMessages from "../en";
import esMessages from "../es";
import esArOverrides from "../es-ar";

describe("cookies i18n namespace", () => {
  const requiredKeys = [
    "banner.title",
    "banner.description",
    "banner.acceptAll",
    "banner.declineAll",
    "banner.essentialLabel",
    "banner.essentialDescription",
    "banner.analyticsLabel",
    "banner.analyticsDescription",
    "banner.marketingLabel",
    "banner.marketingDescription",
    "banner.savePreferences",
    "banner.privacyLink",
    "footer.preferences",
  ];

  const resolve = (root: any, dotted: string) =>
    dotted.split(".").reduce((acc, k) => acc?.[k], root);

  it("en defines the full cookies namespace", () => {
    expect(enMessages.cookies).toBeDefined();
    for (const key of requiredKeys) {
      expect(typeof resolve(enMessages.cookies, key)).toBe("string");
    }
  });

  it("es defines the full cookies namespace (no English fallback)", () => {
    expect(esMessages.cookies).toBeDefined();
    for (const key of requiredKeys) {
      const en = resolve(enMessages.cookies, key);
      const es = resolve(esMessages.cookies, key);
      expect(typeof es).toBe("string");
      expect(es).not.toBe(en); // genuinely translated, not copied English
    }
  });

  it("es-AR override layer is valid (voseo-only divergences, no missing keys by design)", () => {
    // es-AR inherits es via deepMerge; the override file may be partial.
    // If present, every key it defines must be a string.
    const cookies = (esArOverrides as Record<string, any>).cookies;
    if (cookies) {
      const flatCheck = (obj: any) => {
        for (const v of Object.values(obj)) {
          if (typeof v === "object" && v !== null) flatCheck(v);
          else expect(typeof v).toBe("string");
        }
      };
      flatCheck(cookies);
    }
  });
});
