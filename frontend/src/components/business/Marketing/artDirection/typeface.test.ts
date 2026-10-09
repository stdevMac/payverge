import { cssFontString } from "./typeface";

/**
 * `cssFontString` is the one builder the composition solver measures through
 * and the canvas walker paints through. Because both sides call it, a bug
 * inside it moves both sides together and the parity suite in
 * `../scene/fontParity.test.ts` stays green — so the string it emits has to be
 * pinned here, literally, or nothing pins it at all.
 */
describe("cssFontString", () => {
  it("emits weight, rounded px and family, in CSS font order", () => {
    expect(cssFontString({ font: "sans", weight: 500, px: 42 })).toBe(
      "500 42px 'DM Sans', sans-serif",
    );
  });

  it("names DM Serif Display, with Georgia and the generic behind it", () => {
    expect(cssFontString({ font: "serif", weight: 400, px: 42 })).toBe(
      "400 42px 'DM Serif Display', Georgia, serif",
    );
  });

  it.each([400, 500, 600, 700] as const)(
    "carries a sans weight through unchanged (%s)",
    (weight) => {
      expect(cssFontString({ font: "sans", weight, px: 20 })).toBe(
        `${weight} 20px 'DM Sans', sans-serif`,
      );
    },
  );

  /**
   * DM Serif Display ships a single weight, so anything above 400 is synthetic
   * bold — a smeared outline, not a real face. Forcing it here rather than at
   * the two call sites is what keeps the solver measuring the same 400 the
   * walker paints.
   */
  it.each([500, 600, 700] as const)(
    "forces serif to 400 rather than asking for synthetic bold (%s)",
    (weight) => {
      expect(cssFontString({ font: "serif", weight, px: 20 })).toBe(
        "400 20px 'DM Serif Display', Georgia, serif",
      );
    },
  );

  /**
   * Rounding lives in the builder so the solver (which holds an exact `fontPx`)
   * and the walker (which holds `sizePct * canvasW` — the same number after a
   * divide and a multiply) cannot land on two different integers and paint text
   * measured at another size.
   */
  it("rounds the size, so a float and its round-trip agree", () => {
    const px = 0.075 * 1.15 * 1080;
    const spec = { font: "sans", weight: 700 } as const;

    expect(cssFontString({ ...spec, px })).toBe("700 93px 'DM Sans', sans-serif");
    expect(cssFontString({ ...spec, px: (px / 1080) * 1080 })).toBe(
      cssFontString({ ...spec, px }),
    );
    expect(cssFontString({ ...spec, px: 41.5 })).toBe(
      "700 42px 'DM Sans', sans-serif",
    );
  });
});
