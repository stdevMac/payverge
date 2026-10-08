import {
  compositeOver,
  conditionSurface,
  contrastRatio,
  parseHex,
  pickReadableColor,
  relativeLuminance,
  scaleLuminance,
  toHex,
  unreadableBand,
} from "./color";

describe("parseHex", () => {
  it("parses 6-digit and 3-digit hex", () => {
    expect(parseHex("#1a6b6a")).toEqual({ r: 26, g: 107, b: 106 });
    expect(parseHex("#fff")).toEqual({ r: 255, g: 255, b: 255 });
  });

  it("returns null for garbage", () => {
    expect(parseHex("teal")).toBeNull();
    expect(parseHex("")).toBeNull();
  });
});

describe("toHex", () => {
  it("round-trips through parseHex", () => {
    expect(toHex({ r: 26, g: 107, b: 106 })).toBe("#1a6b6a");
  });

  it("clamps out-of-range channels", () => {
    expect(toHex({ r: -5, g: 300, b: 0 })).toBe("#00ff00");
  });
});

describe("relativeLuminance", () => {
  it("is 0 for black and 1 for white", () => {
    expect(relativeLuminance({ r: 0, g: 0, b: 0 })).toBeCloseTo(0, 5);
    expect(relativeLuminance({ r: 255, g: 255, b: 255 })).toBeCloseTo(1, 5);
  });
});

describe("contrastRatio", () => {
  it("is 21 for black on white", () => {
    expect(
      contrastRatio({ r: 0, g: 0, b: 0 }, { r: 255, g: 255, b: 255 }),
    ).toBeCloseTo(21, 2);
  });

  it("is symmetric", () => {
    const a = { r: 26, g: 107, b: 106 };
    const b = { r: 250, g: 249, b: 246 };
    expect(contrastRatio(a, b)).toBeCloseTo(contrastRatio(b, a), 10);
  });
});

describe("pickReadableColor", () => {
  it("picks the first candidate that clears the threshold", () => {
    expect(pickReadableColor(["#ffffff", "#1c1917"], "#faf9f6", 4.5)).toBe(
      "#1c1917",
    );
  });

  it("falls back to the highest-contrast candidate when none clear it", () => {
    expect(pickReadableColor(["#fefefe", "#f0f0f0"], "#ffffff", 4.5)).toBe(
      "#f0f0f0",
    );
  });

  it("returns the backdrop-safe default when candidates are empty", () => {
    expect(pickReadableColor([], "#1c1917", 4.5)).toBe("#ffffff");
  });
});

describe("compositeOver", () => {
  it("returns the backdrop at alpha 0 and the source at alpha 1", () => {
    expect(compositeOver("#c2410c", "#6264ee", 0)).toBe("#6264ee");
    expect(compositeOver("#c2410c", "#6264ee", 1)).toBe("#c2410c");
  });

  it("blends the sRGB channels, which is what canvas globalAlpha does", () => {
    // 0.12 * 194 + 0.88 * 98 = 109.52 -> 110 = 0x6e
    // 0.12 * 65  + 0.88 * 100 = 95.8  -> 96  = 0x60
    // 0.12 * 12  + 0.88 * 238 = 210.88 -> 211 = 0xd3
    expect(compositeOver("#c2410c", "#6264ee", 0.12)).toBe("#6e60d3");
    // Linear-light compositing would give #7a68d5 here — visibly lighter, and
    // NOT what the canvas paints. Pinned so the space cannot be "corrected".
    expect(compositeOver("#ffffff", "#000000", 0.5)).toBe("#808080");
  });

  it("clamps a nonsense alpha instead of extrapolating past the endpoints", () => {
    expect(compositeOver("#ffffff", "#000000", -3)).toBe("#000000");
    expect(compositeOver("#ffffff", "#000000", 4)).toBe("#ffffff");
    expect(compositeOver("#ffffff", "#000000", Number.NaN)).toBe("#000000");
  });

  it("degrades to the backdrop when either color is unparseable", () => {
    expect(compositeOver("teal", "#6264ee", 0.5)).toBe("#6264ee");
    expect(compositeOver("#ffffff", "teal", 0.5)).toBe("teal");
  });
});

describe("scaleLuminance", () => {
  it("hits the target luminance in both directions", () => {
    // Only to 3dp, and that is the whole point of CONTRAST_MARGIN: the scale
    // itself is exact, but `relativeLuminance` rounds its argument to integer
    // channels, so measuring the result re-introduces the rounding error the
    // margin exists to absorb.
    const down = scaleLuminance({ r: 99, g: 102, b: 241 }, 0.1)!;
    expect(down).not.toBeNull();
    expect(relativeLuminance(down)).toBeCloseTo(0.1, 3);

    const up = scaleLuminance({ r: 99, g: 102, b: 241 }, 0.2)!;
    expect(up).not.toBeNull();
    expect(relativeLuminance(up)).toBeCloseTo(0.2, 3);
  });

  it("returns null for black, which no scale can lift off zero", () => {
    // 0 * k is 0 for every k, so there is no factor that reaches the target and
    // no chromaticity to preserve. Without the guard the multiply is 0 * +/-Inf
    // = NaN and the caller silently gets black back as if it had succeeded.
    expect(scaleLuminance({ r: 0, g: 0, b: 0 }, 0.5)).toBeNull();
    expect(scaleLuminance({ r: 0, g: 0, b: 0 }, -0.04)).toBeNull();
  });

  it("returns null rather than clipping a channel past full", () => {
    // Blue is already 250; reaching this luminance needs it past 255, and
    // clamping there would desaturate instead of preserving chromaticity.
    expect(scaleLuminance({ r: 108, g: 108, b: 250 }, 0.6)).toBeNull();
  });
});

/** The pair every non-pill text slot picks from: white and ink-900. */
const FOREGROUNDS = ["#ffffff", "#1c1917"];

const luminanceOf = (hex: string): number => {
  const rgb = parseHex(hex);
  if (!rgb) throw new Error(`unparseable color: ${hex}`);
  return relativeLuminance(rgb);
};

/** The ratio the foreground `pickReadableColor` would then choose actually gets. */
const appliedRatio = (surface: string): number => {
  const chosen = parseHex(pickReadableColor(FOREGROUNDS, surface, 4.5));
  const backdrop = parseHex(surface);
  if (!chosen || !backdrop) throw new Error(`unparseable color: ${surface}`);
  return contrastRatio(chosen, backdrop);
};

describe("unreadableBand", () => {
  it("derives the band where neither white nor ink-900 reaches 4.5:1", () => {
    const band = unreadableBand(FOREGROUNDS, 4.5);
    expect(band.low).toBeCloseTo(0.18333, 5);
    expect(band.high).toBeCloseTo(0.22018, 5);
    // Narrow, which is exactly why the hole went unnoticed — and non-empty,
    // which is why it matters.
    expect(band.high - band.low).toBeCloseTo(0.03685, 5);
  });

  it("reports an empty band when there is nothing to move toward", () => {
    const band = unreadableBand([], 4.5);
    expect(band.high).toBeLessThanOrEqual(band.low);
    expect(unreadableBand(["not a color"], 4.5).high).toBeLessThanOrEqual(
      unreadableBand(["not a color"], 4.5).low,
    );
  });

  it("widens as the required ratio rises", () => {
    const aa = unreadableBand(FOREGROUNDS, 4.5);
    const aaa = unreadableBand(FOREGROUNDS, 7);
    expect(aaa.low).toBeLessThan(aa.low);
    expect(aaa.high).toBeGreaterThan(aa.high);
  });
});

describe("conditionSurface", () => {
  const condition = (hex: string) => conditionSurface(hex, FOREGROUNDS, 4.5);

  /*
   * Three real Tailwind 500 shades — the indigo, violet and purple ones — sit
   * inside the dead band, and all three are ordinary brand-primary choices.
   * Unconditioned they leave every foreground under the AA floor: 4.47 / 4.23 /
   * 3.96 for white and 3.92 / 4.13 / 4.42 for ink-900.
   */
  const IN_BAND = ["#6366f1", "#8b5cf6", "#a855f7"];

  it.each(IN_BAND)("moves %s out of the dead band", (hex) => {
    const band = unreadableBand(FOREGROUNDS, 4.5);
    // Guard against a vacuous case: the fixture must actually be in the band.
    expect(luminanceOf(hex)).toBeGreaterThan(band.low);
    expect(luminanceOf(hex)).toBeLessThan(band.high);
    expect(appliedRatio(hex)).toBeLessThan(4.5);

    const conditioned = condition(hex);

    expect(conditioned).not.toBe(hex);
    const luminance = luminanceOf(conditioned);
    expect(luminance <= band.low || luminance >= band.high).toBe(true);
  });

  /*
   * The assertion the whole change exists for, and the one the margin defends:
   * the ratio is measured on the FINAL, rounded, integer-RGB color, not on the
   * float luminance the scaling produced. Targeting the band edge exactly
   * instead of overshooting it lands these at 4.478 / 4.490 / 4.504 — two of
   * three under the floor, and invisible to any check made before rounding.
   */
  it.each(IN_BAND)("leaves %s readable at 4.5:1 after rounding", (hex) => {
    expect(appliedRatio(condition(hex))).toBeGreaterThanOrEqual(4.5);
  });

  it("preserves hue and saturation, because it scales the linear channels", () => {
    // Scaling every linear channel by one factor is a pure luminance move:
    // chromaticity, and so the hue angle and the saturation, come out
    // unchanged. Only the integer-RGB rounding at the end perturbs them, and
    // only in the third decimal.
    const hsv = (hex: string) => {
      const rgb = parseHex(hex);
      if (!rgb) throw new Error(`unparseable color: ${hex}`);
      const [r, g, b] = [rgb.r / 255, rgb.g / 255, rgb.b / 255];
      const max = Math.max(r, g, b);
      const min = Math.min(r, g, b);
      const delta = max - min;
      let hue = 0;
      if (delta > 0) {
        if (max === r) hue = 60 * (((g - b) / delta) % 6);
        else if (max === g) hue = 60 * ((b - r) / delta + 2);
        else hue = 60 * ((r - g) / delta + 4);
      }
      return { hue: (hue + 360) % 360, saturation: max === 0 ? 0 : delta / max };
    };

    IN_BAND.forEach((hex) => {
      const before = hsv(hex);
      const after = hsv(condition(hex));
      // A degree of hue and 0.01 of saturation: rounding-scale, not a recolour.
      expect(Math.abs(after.hue - before.hue)).toBeLessThan(1);
      expect(after.saturation).toBeCloseTo(before.saturation, 2);
    });
  });

  it("returns a color outside the band byte-identical", () => {
    // The brand teal, the cream surface, ink-900, and a shorthand/uppercase
    // form: an unnecessary re-serialization would show up here as a changed
    // string even though the color is the same.
    ["#1a6b6a", "#faf9f6", "#1c1917", "#FFF", "  #000000  "].forEach((hex) => {
      expect(condition(hex)).toBe(hex);
    });
  });

  it("leaves the colors just outside each edge alone and conditions the ones just inside", () => {
    const band = unreadableBand(FOREGROUNDS, 4.5);
    // Neighbouring greys, one integer channel step apart on each side of each
    // edge: 118/119 straddle the low edge, 129/130 the high one.
    expect(luminanceOf("#767676")).toBeLessThan(band.low);
    expect(luminanceOf("#828282")).toBeGreaterThan(band.high);
    expect(condition("#767676")).toBe("#767676");
    expect(condition("#828282")).toBe("#828282");

    expect(luminanceOf("#777777")).toBeGreaterThan(band.low);
    expect(luminanceOf("#818181")).toBeLessThan(band.high);
    expect(condition("#777777")).not.toBe("#777777");
    expect(condition("#818181")).not.toBe("#818181");
    expect(appliedRatio(condition("#777777"))).toBeGreaterThanOrEqual(4.5);
    expect(appliedRatio(condition("#818181"))).toBeGreaterThanOrEqual(4.5);
  });

  it("moves to the nearer edge, darkening below and lightening above", () => {
    // #777777 sits just above the low edge, #818181 just below the high one.
    expect(luminanceOf(condition("#777777"))).toBeLessThan(
      luminanceOf("#777777"),
    );
    expect(luminanceOf(condition("#818181"))).toBeGreaterThan(
      luminanceOf("#818181"),
    );
  });

  it("darkens rather than desaturating when lightening would clip a channel", () => {
    // Blue is already at 250 here, so scaling up to the high edge overflows it
    // even though the high edge is the nearer one; the only
    // chromaticity-preserving move left is down.
    const nearWhiteBlue = "#0000fa";
    const inBandButClipping = "#6c6cfa";
    expect(parseHex(inBandButClipping)?.b).toBe(250);
    const band = unreadableBand(FOREGROUNDS, 4.5);
    const luminance = luminanceOf(inBandButClipping);
    expect(luminance).toBeGreaterThan(band.low);
    expect(luminance).toBeLessThan(band.high);

    const conditioned = condition(inBandButClipping);

    expect(luminanceOf(conditioned)).toBeLessThan(luminance);
    expect(appliedRatio(conditioned)).toBeGreaterThanOrEqual(4.5);
    // Untouched control: outside the band, so it never reaches the clip branch.
    expect(condition(nearWhiteBlue)).toBe(nearWhiteBlue);
  });

  it("returns an unparseable surface unchanged", () => {
    expect(conditionSurface("teal", FOREGROUNDS, 4.5)).toBe("teal");
    expect(conditionSurface("#6366f1", [], 4.5)).toBe("#6366f1");
  });

  /*
   * `candidates` and `minRatio` are free parameters, so the white / ink-900
   * pair's convenient +0.1833 lower edge is not a property of the function. A
   * candidate set with no light foreground bands BELOW zero luminance, and
   * there darkening cannot escape at all — it can only clamp to black, which is
   * still inside the band. "Darkening is always available" is true of the
   * operation and false of the escape, and the difference is the whole finding.
   */
  describe("when one edge is unreachable", () => {
    const INK_ONLY = ["#1c1917"];

    const inkBand = () => unreadableBand(INK_ONLY, 4.5);

    it("bands below zero luminance, so darkening cannot leave it", () => {
      // Not a contrived set: this is the assembler's own dark foreground, and
      // it is what the band looks like without a light one to pair with.
      expect(inkBand().low).toBeLessThan(0);
      expect(inkBand().high).toBeCloseTo(0.22018, 5);
    });

    it.each(["#404040", "#333333"])(
      "takes the far edge rather than returning %s still inside the band",
      (hex) => {
        const band = inkBand();
        // Not vacuous: in the band, and nearer the unreachable edge.
        expect(luminanceOf(hex)).toBeGreaterThan(band.low);
        expect(luminanceOf(hex)).toBeLessThan(band.high);
        expect(luminanceOf(hex) - band.low).toBeLessThan(
          band.high - luminanceOf(hex),
        );

        const conditioned = conditionSurface(hex, INK_ONLY, 4.5);

        // Clamping to black is what darkening does here, and black is still
        // inside the band.
        expect(conditioned).not.toBe("#000000");
        const luminance = luminanceOf(conditioned);
        expect(luminance <= band.low || luminance >= band.high).toBe(true);
      },
    );

    it("returns the surface unchanged when neither edge is reachable", () => {
      // Blue is already 250, so lightening out of the top clips it; darkening
      // out of the bottom is the unreachable edge above. Returning the brand
      // colour untouched is the honest answer — clipping the lighten would
      // reach the edge only by desaturating, and no colour inside the band is
      // readable anyway, so a recolour would buy nothing.
      const clipping = "#0000fa";
      const band = inkBand();
      expect(luminanceOf(clipping)).toBeGreaterThan(band.low);
      expect(luminanceOf(clipping)).toBeLessThan(band.high);

      expect(conditionSurface(clipping, INK_ONLY, 4.5)).toBe(clipping);
    });
  });

  /*
   * A layer painted between the surface and the type — the kit texture, at
   * `Z.texture` — is what the type reads against, so it is the COMPOSITE that
   * has to clear the band, not the surface underneath it.
   */
  describe("under an overlay", () => {
    const ACCENT = { color: "#c2410c", alpha: 0.12 };

    it("moves a surface whose composite is in the band even though it is not", () => {
      // #ab57fb is what #a855f7 conditions to on its own: out of the band by
      // itself, and dragged back in by the accent laid over it.
      const alone = "#ab57fb";
      const band = unreadableBand(FOREGROUNDS, 4.5);
      expect(luminanceOf(alone)).toBeGreaterThanOrEqual(band.high);
      expect(
        luminanceOf(compositeOver(ACCENT.color, alone, ACCENT.alpha)),
      ).toBeLessThan(band.high);

      const conditioned = conditionSurface(alone, FOREGROUNDS, 4.5, ACCENT);

      expect(conditioned).not.toBe(alone);
      const composed = luminanceOf(
        compositeOver(ACCENT.color, conditioned, ACCENT.alpha),
      );
      expect(composed <= band.low || composed >= band.high).toBe(true);
    });

    it("leaves a surface whose composite already clears the band byte-identical", () => {
      // The brand teal: outside the band with the accent over it too, so the
      // overlay must not become an excuse to restyle it.
      expect(conditionSurface("#1a6b6a", FOREGROUNDS, 4.5, ACCENT)).toBe(
        "#1a6b6a",
      );
    });

    it("keeps the surface's own hue while moving the composite", () => {
      // The move is still a scale of the SURFACE's linear channels, so the
      // brand colour keeps its chromaticity however the overlay is tinted.
      const conditioned = conditionSurface("#a855f7", FOREGROUNDS, 4.5, ACCENT);
      const before = parseHex("#a855f7")!;
      const after = parseHex(conditioned)!;
      expect(after.r / after.b).toBeCloseTo(before.r / before.b, 1);
      expect(after.g / after.b).toBeCloseTo(before.g / before.b, 1);
    });

    it("ignores an overlay it cannot paint", () => {
      const painted = conditionSurface("#a855f7", FOREGROUNDS, 4.5, ACCENT);
      const unpaintable = conditionSurface("#a855f7", FOREGROUNDS, 4.5, {
        color: "teal",
        alpha: 0.12,
      });
      expect(unpaintable).toBe(conditionSurface("#a855f7", FOREGROUNDS, 4.5));
      expect(unpaintable).not.toBe(painted);
    });
  });
});
