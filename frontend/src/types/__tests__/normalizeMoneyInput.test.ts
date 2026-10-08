import { asMoneyString, normalizeMoneyInput } from "@/types/alternativePayments";

// R3-BP-6: the money-input validation gate must agree with asMoneyString, so a
// value that normalizeMoneyInput accepts never throws at submit time and a value
// it rejects is caught before submit with a field-level error.
describe("normalizeMoneyInput", () => {
  it("accepts dot decimals", () => {
    expect(normalizeMoneyInput("10.50")).toBe("10.50");
    expect(normalizeMoneyInput("10.5")).toBe("10.50");
    expect(normalizeMoneyInput("1234")).toBe("1234.00");
  });

  it("accepts comma decimals (es-AR operators)", () => {
    expect(normalizeMoneyInput("10,50")).toBe("10.50");
    expect(normalizeMoneyInput("0,99")).toBe("0.99");
  });

  it("strips thousands separators", () => {
    expect(normalizeMoneyInput("1.234,56")).toBe("1234.56");
    expect(normalizeMoneyInput("1,234.56")).toBe("1234.56");
  });

  it("rejects more than two decimal places", () => {
    expect(normalizeMoneyInput("10.999")).toBeNull();
    expect(normalizeMoneyInput("10,999")).toBeNull();
  });

  it("rejects zero, empty, and garbage", () => {
    expect(normalizeMoneyInput("")).toBeNull();
    expect(normalizeMoneyInput("   ")).toBeNull();
    expect(normalizeMoneyInput("0")).toBeNull();
    expect(normalizeMoneyInput("0.00")).toBeNull();
    expect(normalizeMoneyInput("abc")).toBeNull();
  });

  it("handles a leading-decimal shorthand", () => {
    // ".50" previously passed a parseFloat check then threw at asMoneyString.
    expect(normalizeMoneyInput(".50")).toBe("0.50");
  });

  it("produces output asMoneyString always accepts", () => {
    for (const raw of ["10,50", "1.234,56", ".50", "1234", "0,99"]) {
      const normalized = normalizeMoneyInput(raw);
      expect(normalized).not.toBeNull();
      // Must not throw — this is the invariant the fix relies on.
      expect(() => asMoneyString(normalized as string)).not.toThrow();
    }
  });
});
