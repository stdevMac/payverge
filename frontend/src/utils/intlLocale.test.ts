import { intlLocaleFor } from "./intlLocale";

describe("intlLocaleFor", () => {
  describe("preserves the legacy dashboard mapping", () => {
    it("maps base 'es' to 'es'", () => {
      expect(intlLocaleFor("es")).toBe("es");
    });

    it("maps 'fr' to 'fr'", () => {
      expect(intlLocaleFor("fr")).toBe("fr");
    });

    it("maps 'pt' to 'pt'", () => {
      expect(intlLocaleFor("pt")).toBe("pt");
    });

    it("maps 'ar' to 'ar'", () => {
      expect(intlLocaleFor("ar")).toBe("ar");
    });

    it("maps 'hi' to 'hi'", () => {
      expect(intlLocaleFor("hi")).toBe("hi");
    });

    it("defaults 'en' to 'en-US'", () => {
      expect(intlLocaleFor("en")).toBe("en-US");
    });

    it("defaults unknown/empty input to 'en-US'", () => {
      expect(intlLocaleFor("")).toBe("en-US");
      expect(intlLocaleFor("zz")).toBe("en-US");
      expect(intlLocaleFor(undefined as unknown as string)).toBe("en-US");
    });
  });

  describe("fixes the es-AR drift (the bug the binary es-ES/en-US ternary caused)", () => {
    it("maps canonical 'es-AR' to 'es-AR' (Argentine), not 'es'", () => {
      expect(intlLocaleFor("es-AR")).toBe("es-AR");
    });

    it("resolves 'es-AR' case-insensitively (BCP-47 tags are case-insensitive)", () => {
      expect(intlLocaleFor("es-ar")).toBe("es-AR");
      expect(intlLocaleFor("ES-AR")).toBe("es-AR");
    });
  });

  describe("registry-aware: every canonical guest locale maps to itself (a valid BCP-47 tag)", () => {
    it.each([
      "de",
      "it",
      "zh",
      "ja",
      "ko",
      "ru",
      "th",
      "nl",
      "tr",
      "vi",
      "pl",
      "sv",
      "da",
      "no",
    ])("maps '%s' to itself", (code) => {
      expect(intlLocaleFor(code)).toBe(code);
    });
  });

  describe("produces tags Intl can consume", () => {
    it.each(["es", "es-AR", "fr", "pt", "ar", "hi", "de", "zh", "en", ""])(
      "intlLocaleFor(%j) yields a usable Intl locale",
      (code) => {
        const tag = intlLocaleFor(code);
        // Constructing with the resolved tag must not throw.
        expect(() => new Intl.NumberFormat(tag)).not.toThrow();
        expect(() => new Intl.DateTimeFormat(tag)).not.toThrow();
      },
    );
  });

  describe("Spanish decimal-comma formatting is preserved (dashboard behavior)", () => {
    it("formats money with the Spanish decimal comma for 'es'", () => {
      const out = new Intl.NumberFormat(intlLocaleFor("es"), {
        style: "currency",
        currency: "USD",
      }).format(55);
      expect(out).toMatch(/55,00/);
      expect(out).not.toBe("$55.00");
    });

    it("formats money in Argentine convention for 'es-AR', distinct from 'en-US'", () => {
      const arOut = new Intl.NumberFormat(intlLocaleFor("es-AR"), {
        style: "currency",
        currency: "USD",
      }).format(1234.5);
      const enOut = new Intl.NumberFormat(intlLocaleFor("xx"), {
        style: "currency",
        currency: "USD",
      }).format(1234.5);
      // es-AR uses a decimal comma; en-US uses a decimal dot.
      expect(arOut).toMatch(/,50/);
      expect(enOut).toMatch(/\.50/);
    });
  });
});
