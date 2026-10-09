import { asDollars, roundDollars, dollarsToCents, centsToDollars } from "./money";

describe("roundDollars (FIND-043 residual)", () => {
  it("eliminates qty×price binary float residue", () => {
    // Classic IEEE residue: 36 * 4.2 === 151.20000000000002
    expect(36 * 4.2).not.toBe(151.2);
    expect(roundDollars(36 * 4.2)).toBe(151.2);
  });

  it("rounds to nearest cent (IEEE half cases are float-noise; use clear deltas)", () => {
    // 1.005 is not exactly representable; use values that need real rounding.
    expect(roundDollars(1.004)).toBe(1);
    expect(roundDollars(1.006)).toBe(1.01);
    expect(roundDollars(10.996)).toBe(11);
  });

  it("preserves already-clean cents", () => {
    expect(roundDollars(12.34)).toBe(12.34);
    expect(roundDollars(0)).toBe(0);
  });

  it("works with asDollars / dollarsToCents brand chain", () => {
    const d = roundDollars(3 * 0.1); // 0.30000000000000004 raw
    expect(d).toBe(0.3);
    expect(dollarsToCents(d)).toBe(30);
    expect(centsToDollars(dollarsToCents(asDollars(1.99)))).toBe(1.99);
  });
});
