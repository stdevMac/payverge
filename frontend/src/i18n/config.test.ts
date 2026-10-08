import { getTranslation } from "./SimpleTranslationProvider";
import { languageFlags, languageNames, locales } from "./config";

describe("operator locale config", () => {
  it("exposes Argentine Spanish in the operator language switcher config", () => {
    expect(locales).toContain("es-AR");
    expect(languageNames["es-AR"]).toBe("Español (Argentina)");
    expect(languageFlags["es-AR"]).toBe("🇦🇷");
  });

  it("uses the Spanish message bundle for Argentine Spanish UI copy", () => {
    expect(getTranslation("navigation.home", "es-AR")).toBe("Inicio");
  });
});
