import { EASINGS, EASING_ORDER, ease, isEasingId } from "./easing";

describe("easing", () => {
  it("registers every id in EASING_ORDER and nothing else", () => {
    expect([...EASING_ORDER].sort()).toEqual(Object.keys(EASINGS).sort());
  });

  it.each(EASING_ORDER)("%s starts at 0 and ends at 1", (id) => {
    expect(EASINGS[id](0)).toBeCloseTo(0, 10);
    expect(EASINGS[id](1)).toBeCloseTo(1, 10);
  });

  it.each(["linear", "easeOutCubic", "easeInOutCubic", "easeOutQuint"] as const)(
    "%s is monotonically non-decreasing",
    (id) => {
      let previous = -Infinity;
      for (let step = 0; step <= 100; step += 1) {
        const value = EASINGS[id](step / 100);
        expect(value).toBeGreaterThanOrEqual(previous);
        previous = value;
      }
    },
  );

  /**
   * easeOutBack is the one curve that must NOT be monotonic — the overshoot past
   * 1 is what makes a badge read as a pop rather than a fade. Clamping its output
   * would silently turn every pop into an ease-out, which is exactly the sort of
   * "tidy up" a future reader might attempt.
   */
  it("easeOutBack overshoots past 1 before settling", () => {
    const peak = Math.max(
      ...Array.from({ length: 101 }, (_, step) => EASINGS.easeOutBack(step / 100)),
    );
    expect(peak).toBeGreaterThan(1);
    expect(EASINGS.easeOutBack(1)).toBeCloseTo(1, 10);
  });

  it("clamps the input, not the output", () => {
    expect(ease("easeOutCubic", -3)).toBeCloseTo(0, 10);
    expect(ease("easeOutCubic", 7)).toBeCloseTo(1, 10);
    expect(ease("easeOutBack", 0.75)).toBeGreaterThan(1);
  });

  it("treats a non-finite progress as the start of the curve", () => {
    expect(ease("easeOutCubic", Number.NaN)).toBe(0);
    expect(ease("easeOutCubic", Number.POSITIVE_INFINITY)).toBe(1);
  });

  it("narrows unknown ids instead of trusting them", () => {
    expect(isEasingId("easeOutCubic")).toBe(true);
    expect(isEasingId("bounce")).toBe(false);
    expect(isEasingId(42)).toBe(false);
    expect(isEasingId(undefined)).toBe(false);
  });
});
