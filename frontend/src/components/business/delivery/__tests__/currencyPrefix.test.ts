import { deriveCurrencyPrefix } from "../currencyPrefix";

describe("deriveCurrencyPrefix", () => {
  it("keeps the narrow symbol for guest surfaces by default", () => {
    expect(deriveCurrencyPrefix("USD")).toBe("$");
    expect(deriveCurrencyPrefix("ARS")).toBe("$");
  });

  it("does not show a bare $ for a peso venue on operator settings", () => {
    expect(deriveCurrencyPrefix("ARS", "symbol")).toBe("ARS");
    expect(deriveCurrencyPrefix("USD", "symbol")).toBe("$");
    expect(deriveCurrencyPrefix("EUR", "symbol")).toBe("€");
  });

  it("falls back to the code for an invalid currency", () => {
    expect(deriveCurrencyPrefix("NOTACODE", "symbol")).toBe("NOTACODE");
  });
});
