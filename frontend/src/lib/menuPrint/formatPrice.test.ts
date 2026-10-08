import { formatMenuPrice } from "./formatPrice";
import type { MenuPriceFormatSpec } from "./types";

type PriceSpec = MenuPriceFormatSpec;

function priceSpec(overrides: Partial<PriceSpec> = {}): PriceSpec {
  return {
    mode: "nested",
    leaders: false,
    symbol: "off",
    decimals: "strip-zeros",
    ...overrides,
  };
}

describe("formatMenuPrice", () => {
  describe("symbol off", () => {
    it("USD strip-zeros whole amount has no decimals and no symbol", () => {
      expect(
        formatMenuPrice(
          24,
          "USD",
          priceSpec({ decimals: "strip-zeros", symbol: "off" }),
          "en",
        ),
      ).toBe("24");
    });

    it("USD strip-zeros fractional keeps two decimals", () => {
      expect(
        formatMenuPrice(
          24.5,
          "USD",
          priceSpec({ decimals: "strip-zeros", symbol: "off" }),
          "en",
        ),
      ).toBe("24.50");
    });

    it("USD always shows two decimals", () => {
      expect(
        formatMenuPrice(
          24,
          "USD",
          priceSpec({ decimals: "always", symbol: "off" }),
          "en",
        ),
      ).toBe("24.00");
      expect(
        formatMenuPrice(
          24.5,
          "USD",
          priceSpec({ decimals: "always", symbol: "off" }),
          "en",
        ),
      ).toBe("24.50");
    });

    it("EUR strip-zeros with es locale uses comma for fractional", () => {
      const whole = formatMenuPrice(
        24,
        "EUR",
        priceSpec({ decimals: "strip-zeros", symbol: "off" }),
        "es",
      );
      expect(whole).toBe("24");
      const frac = formatMenuPrice(
        24.5,
        "EUR",
        priceSpec({ decimals: "strip-zeros", symbol: "off" }),
        "es",
      );
      // es locales use comma as decimal separator
      expect(frac).toMatch(/24[,.]50/);
      expect(frac).not.toContain("€");
      expect(frac).not.toMatch(/EUR/i);
    });

    it("ARS strip-zeros bare numeral", () => {
      expect(
        formatMenuPrice(
          1500,
          "ARS",
          priceSpec({ decimals: "strip-zeros", symbol: "off" }),
          "es",
        ),
      ).toBe("1500");
      const frac = formatMenuPrice(
        1500.5,
        "ARS",
        priceSpec({ decimals: "strip-zeros", symbol: "off" }),
        "es",
      );
      expect(frac).toMatch(/1[.,\s]?500[,.]50|1500[,.]50/);
    });
  });

  describe("symbol locale (narrowSymbol)", () => {
    it("USD en includes a currency symbol", () => {
      const out = formatMenuPrice(
        24,
        "USD",
        priceSpec({ decimals: "strip-zeros", symbol: "locale" }),
        "en",
      );
      expect(out).toMatch(/\$/);
      expect(out).toMatch(/24/);
      // strip-zeros: no trailing .00
      expect(out).not.toMatch(/24\.00/);
    });

    it("USD en always keeps two decimals with symbol", () => {
      const out = formatMenuPrice(
        24,
        "USD",
        priceSpec({ decimals: "always", symbol: "locale" }),
        "en",
      );
      expect(out).toMatch(/\$/);
      expect(out).toMatch(/24\.00/);
    });

    it("EUR es includes euro symbol", () => {
      const out = formatMenuPrice(
        12.5,
        "EUR",
        priceSpec({ decimals: "strip-zeros", symbol: "locale" }),
        "es",
      );
      expect(out).toMatch(/€/);
      expect(out).toMatch(/12[,.]50/);
    });

    it("ARS es with symbol", () => {
      const out = formatMenuPrice(
        100,
        "ARS",
        priceSpec({ decimals: "strip-zeros", symbol: "locale" }),
        "es",
      );
      // narrowSymbol for ARS is typically $ or ARS-specific
      expect(out).toMatch(/100/);
      expect(out.length).toBeGreaterThan(3);
    });
  });

  describe("zero-decimal currencies (JPY, KRW)", () => {
    it("JPY never shows fake decimals with strip-zeros", () => {
      const off = formatMenuPrice(
        1200,
        "JPY",
        priceSpec({ decimals: "strip-zeros", symbol: "off" }),
        "ja",
      );
      expect(off).not.toMatch(/\./);
      expect(off).toMatch(/1[,.]?200|1200/);
    });

    it("JPY never shows fake decimals even with decimals always", () => {
      const off = formatMenuPrice(
        1200,
        "JPY",
        priceSpec({ decimals: "always", symbol: "off" }),
        "ja",
      );
      // No decimal point (thousands separators like "1,200" are fine)
      expect(off).not.toMatch(/\.\d/);
      const digitsOnly = off.replace(/[^\d]/g, "");
      expect(digitsOnly).toBe("1200");
    });

    it("JPY with locale symbol still has no fractional part", () => {
      const out = formatMenuPrice(
        2500,
        "JPY",
        priceSpec({ decimals: "always", symbol: "locale" }),
        "ja",
      );
      // Digits must be exactly 2500 — no invented fractional minor units
      expect(out.replace(/[^\d]/g, "")).toBe("2500");
      // No Latin decimal point before trailing digits (e.g. "2500.00")
      expect(out).not.toMatch(/\.\d{2}/);
    });

    it("KRW never invents decimals", () => {
      const out = formatMenuPrice(
        15000,
        "KRW",
        priceSpec({ decimals: "always", symbol: "off" }),
        "en",
      );
      expect(out).not.toMatch(/[.,]\d{2}$/);
      expect(out.replace(/[^\d]/g, "")).toBe("15000");
    });
  });

  describe("edge cases", () => {
    it("formats zero price", () => {
      expect(
        formatMenuPrice(
          0,
          "USD",
          priceSpec({ decimals: "strip-zeros", symbol: "off" }),
          "en",
        ),
      ).toBe("0");
      expect(
        formatMenuPrice(
          0,
          "USD",
          priceSpec({ decimals: "always", symbol: "off" }),
          "en",
        ),
      ).toBe("0.00");
    });

    it("formats small fractional prices", () => {
      expect(
        formatMenuPrice(
          0.5,
          "USD",
          priceSpec({ decimals: "strip-zeros", symbol: "off" }),
          "en",
        ),
      ).toBe("0.50");
      expect(
        formatMenuPrice(
          0.99,
          "USD",
          priceSpec({ decimals: "always", symbol: "off" }),
          "en",
        ),
      ).toBe("0.99");
    });

    it("does not re-divide dollars (24 stays 24, not 0.24)", () => {
      // Wire shape is already dollars; 2400 cents would already be 24.00 from API.
      expect(
        formatMenuPrice(
          24,
          "USD",
          priceSpec({ symbol: "off", decimals: "always" }),
          "en",
        ),
      ).toBe("24.00");
      expect(
        formatMenuPrice(
          24.99,
          "USD",
          priceSpec({ symbol: "off", decimals: "always" }),
          "en",
        ),
      ).toBe("24.99");
    });

    it("matrix: USD/EUR/ARS/JPY × strip-zeros/always × symbol off × en", () => {
      const cases: Array<{
        currency: string;
        price: number;
        decimals: PriceSpec["decimals"];
        expectNoSymbol: boolean;
        expectNoFakeDecimals?: boolean;
      }> = [
        {
          currency: "USD",
          price: 10,
          decimals: "strip-zeros",
          expectNoSymbol: true,
        },
        {
          currency: "USD",
          price: 10,
          decimals: "always",
          expectNoSymbol: true,
        },
        {
          currency: "EUR",
          price: 10,
          decimals: "strip-zeros",
          expectNoSymbol: true,
        },
        {
          currency: "EUR",
          price: 10.25,
          decimals: "always",
          expectNoSymbol: true,
        },
        {
          currency: "ARS",
          price: 500,
          decimals: "strip-zeros",
          expectNoSymbol: true,
        },
        {
          currency: "ARS",
          price: 500.75,
          decimals: "always",
          expectNoSymbol: true,
        },
        {
          currency: "JPY",
          price: 800,
          decimals: "strip-zeros",
          expectNoSymbol: true,
          expectNoFakeDecimals: true,
        },
        {
          currency: "JPY",
          price: 800,
          decimals: "always",
          expectNoSymbol: true,
          expectNoFakeDecimals: true,
        },
      ];

      for (const c of cases) {
        const out = formatMenuPrice(
          c.price,
          c.currency,
          priceSpec({ decimals: c.decimals, symbol: "off" }),
          "en",
        );
        expect(out).not.toMatch(/\$|€|¥|USD|EUR|ARS|JPY/);
        if (c.expectNoFakeDecimals) {
          // Zero-decimal: digits must equal the whole yen amount
          expect(out.replace(/[^\d]/g, "")).toBe(String(c.price));
          expect(out).not.toMatch(/\.\d/);
        }
        if (
          c.decimals === "strip-zeros" &&
          Number.isInteger(c.price) &&
          c.currency !== "JPY"
        ) {
          // Whole amounts strip zeros for 2-decimal currencies
          expect(out).not.toMatch(/[.,]00$/);
        }
      }
    });
  });
});
