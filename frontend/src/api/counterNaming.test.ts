import {
  desiredCounterName,
  isApplyableDefaultCounterName,
  isGeneratedCounterName,
  isSeedDefaultCounterName,
  listCounterNamingMismatches,
} from "./counterNaming";

describe("counterNaming (#192)", () => {
  it("builds desired names from the prefix", () => {
    expect(desiredCounterName("D", 1)).toBe("D1");
    expect(desiredCounterName("  BAR ", 3)).toBe("BAR3");
    expect(desiredCounterName("   ", 1)).toBe("C1");
  });

  it("detects compact generated names", () => {
    expect(isGeneratedCounterName("C4", 4)).toBe(true);
    expect(isGeneratedCounterName("D1", 1)).toBe(true);
    expect(isGeneratedCounterName("Counter 1", 1)).toBe(false);
    expect(isGeneratedCounterName("C11", 1)).toBe(false);
  });

  it("detects seed default Counter/Mostrador/Pickup labels", () => {
    expect(isSeedDefaultCounterName("Counter 1", 1)).toBe(true);
    expect(isSeedDefaultCounterName("Mostrador 2", 2)).toBe(true);
    expect(isSeedDefaultCounterName("Pickup 3", 3)).toBe(true);
    expect(isSeedDefaultCounterName("Barra principal", 1)).toBe(false);
    expect(isSeedDefaultCounterName("Counter 2", 1)).toBe(false);
  });

  it("lists applyable mismatches for Counter N under prefix D", () => {
    const mismatches = listCounterNamingMismatches(
      [
        { id: 10, counter_number: 1, name: "Counter 1" },
        { id: 11, counter_number: 2, name: "Counter 2" },
        { id: 12, counter_number: 3, name: "Barra principal" },
        { id: 13, counter_number: 4, name: "D4" },
      ],
      "D",
    );
    expect(mismatches).toEqual([
      {
        id: 10,
        counterNumber: 1,
        current: "Counter 1",
        desired: "D1",
        applyable: true,
      },
      {
        id: 11,
        counterNumber: 2,
        current: "Counter 2",
        desired: "D2",
        applyable: true,
      },
      {
        id: 12,
        counterNumber: 3,
        current: "Barra principal",
        desired: "D3",
        applyable: false,
      },
    ]);
    expect(isApplyableDefaultCounterName("Counter 1", 1)).toBe(true);
    expect(isApplyableDefaultCounterName("Barra principal", 3)).toBe(false);
  });
});
