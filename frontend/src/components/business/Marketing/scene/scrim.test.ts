import { adaptiveScrimBoost } from "./scrim";

describe("adaptiveScrimBoost", () => {
  it("adds nothing over a band already dark enough", () => {
    expect(adaptiveScrimBoost(100, 155)).toBeNull();
    expect(adaptiveScrimBoost(155, 155)).toBeNull();
  });

  it("ramps opacity with how much brighter the band is", () => {
    // 0.55 + (175 − 155) / 400 = 0.60
    expect(adaptiveScrimBoost(175, 155)).toEqual({
      from: "rgba(28,25,23,0)",
      to: "rgba(28,25,23,0.6)",
    });
  });

  it("caps the boost so a bright band never becomes opaque", () => {
    // Against threshold 155, even pure white only reaches 0.80. Drop the
    // threshold so the uncapped formula exceeds BOOST_MAX and the ceiling bites.
    expect(adaptiveScrimBoost(255, 0)?.to).toBe("rgba(28,25,23,0.82)");
  });
});
