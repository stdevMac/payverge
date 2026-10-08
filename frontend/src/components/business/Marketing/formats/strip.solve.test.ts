import { COMPOSITIONS } from "../composition/compositions";
import { solveStack, type Band, type MeasureFn } from "../composition/solver";
import { FORMATS } from "./formats";

/**
 * The 4:1 strip is the registry's hardest canvas: 300px of height for a stack
 * whose five bands want roughly 280px at their declared sizes, before the four
 * width-relative gaps. The solver's answer is a designed one — settle at
 * MIN_SCALE, then clip from the end so the badge and the dish name survive —
 * and this suite pins it so a change to MIN_SCALE, SCALE_STEP or a band size
 * shows up as a failing assertion rather than as a silently emptier banner.
 *
 * The arithmetic, at canvasW 1200 / canvasH 300, for `photoBottomStack`'s
 * bounds (safeBounds clamps {y 0.5, h 0.44} against the strip's 0.10..0.90 safe
 * area to h 0.40, i.e. 120px) with standardBands(1):
 *
 *   badge     0.026 * 1200 * 1.15 = 35.88px
 *   dishName  0.075 * 1200 * 1.06 = 95.40px
 *   price     0.042 * 1200 * 1.15 = 57.96px
 *   cta       0.028 * 1200 * 1.15 = 38.64px
 *   handle    0.022 * 1200 * 1.15 = 30.36px
 *   gap       0.018 * 1200        = 21.60px (width-relative; see solver gapBelow)
 *
 * All five at scale 1: 258.24 + 86.40 = 344.64 > 120, so the descent runs to
 * MIN_SCALE 0.55, where 142.03 + 86.40 = 228.43 is still over. Clipping then
 * drops `handle`, `cta` and `price` until badge + dishName fit
 * (72.20 + 21.60 = 93.80 <= 120). Two bands survive.
 *
 * (The Wave 4 plan draft assumed height-relative gaps of 5.4px and three
 * survivors; gapPct is width-relative post-Wave-1, so the pin records the
 * real clip set rather than the draft arithmetic.)
 */

/** Deterministic and short enough that every band wraps to exactly one line. */
const measure: MeasureFn = (text, cssFont) => {
  const px = Number.parseFloat(cssFont.match(/([\d.]+)px/)?.[1] ?? "20");
  return text.length * px * 0.52;
};

const SHORT_COPY: Record<string, string> = {
  badge: "PICK",
  dishName: "Milanesa",
  price: "$12",
  cta: "Order",
  handle: "@casa",
};

function solveStrip() {
  const format = FORMATS.strip;
  const composition = COMPOSITIONS.photoBottomStack;
  const bounds = composition.boundsFor(format);
  const bands: Band[] = composition.bandsFor(format).map((template) => ({
    ...template,
    text: SHORT_COPY[template.key] ?? "x",
    font: "sans" as const,
    weight: 500 as const,
  }));
  return {
    bounds,
    solved: solveStack({
      bands,
      bounds,
      canvasW: format.px.w,
      canvasH: format.px.h,
      anchor: composition.anchor,
      measure,
    }),
  };
}

describe("the strip format solves to a banner, not a poster", () => {
  it("clamps the envelope to the strip's safe area", () => {
    const { bounds } = solveStrip();
    expect(bounds.x).toBeCloseTo(0.07, 10);
    expect(bounds.y).toBeCloseTo(0.5, 10);
    expect(bounds.w).toBeCloseTo(0.86, 10);
    expect(bounds.h).toBeCloseTo(0.4, 10);
  });

  it("keeps badge and dishName and clips price, cta and handle", () => {
    const { solved } = solveStrip();
    expect(Object.keys(solved).sort()).toEqual(["badge", "dishName"]);
  });

  it("settles at the legibility floor rather than overflowing", () => {
    const { solved } = solveStrip();
    // 0.075 * 1200 * 0.55. Compared with a tolerance because settleScale
    // descends by repeated subtraction and accumulates float drift.
    expect(solved.dishName.fontPx).toBeCloseTo(49.5, 3);
  });

  it("never overflows the envelope it was given", () => {
    const { bounds, solved } = solveStrip();
    Object.values(solved).forEach((band) => {
      expect(band.rect.y).toBeGreaterThanOrEqual(bounds.y - 1e-9);
      expect(band.rect.y + band.rect.h).toBeLessThanOrEqual(
        bounds.y + bounds.h + 1e-6,
      );
    });
  });

  it("shrinks the mark so it does not swallow the banner", () => {
    const anchor = COMPOSITIONS.photoBottomStack.logoAnchorFor(FORMATS.strip);
    // MAX_LOGO_HEIGHT_FRACTION 0.22 over a vScale of 1200/300 = 4.
    expect(anchor.size).toBeCloseTo(0.055, 10);
    // And it stays inside the safe area, ring included.
    expect(anchor.y).toBeGreaterThanOrEqual(FORMATS.strip.safe.y);
    expect(anchor.y + anchor.size * 4).toBeLessThanOrEqual(
      FORMATS.strip.safe.y + FORMATS.strip.safe.h + 1e-9,
    );
  });
});
