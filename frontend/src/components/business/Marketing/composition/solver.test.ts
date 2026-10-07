import {
  solveStack,
  type Band,
  type SolveInput,
  type SolvedBand,
} from "./solver";
import {
  ASPECT_DIMS,
  type AspectRatio,
  type NormalizedRect,
} from "../templates/types";

const BOUNDS = { x: 0.06, y: 0.06, w: 0.88, h: 0.88 };
const CANVAS_W = 1080;
const CANVAS_H = 1350;
/** Mirrors MIN_SCALE in solver.ts; the tests below pin that value. */
const MIN_SCALE = 0.55;

/**
 * The px a CSS font string was built for. The solver hands `measure` a finished
 * font string rather than a bare size, so a stand-in metric has to read the size
 * back out of it — which is also what makes these tests notice if the size ever
 * stops reaching the measurer.
 */
const pxOf = (cssFont: string): number => {
  const match = cssFont.match(/ (\d+)px /);
  if (!match) throw new Error(`not a CSS font string: ${cssFont}`);
  return Number(match[1]);
};

/** Deterministic stand-in for ctx.measureText: 0.5em per character. */
const measure = (text: string, cssFont: string) =>
  text.length * pxOf(cssFont) * 0.5;

const band = (key: string, over: Partial<Band> = {}): Band => ({
  key,
  text: "Milanesa",
  sizePct: 0.06,
  maxLines: 1,
  lineHeight: 1.2,
  gapPct: 0.02,
  align: "left",
  font: "sans",
  weight: 400,
  ...over,
});

/** Fills in the canvas/bounds/measure boilerplate so each test shows only its
 *  distinguishing input. */
const solve = (over: Partial<SolveInput> = {}): Record<string, SolvedBand> =>
  solveStack({
    bands: [band("a")],
    bounds: BOUNDS,
    canvasW: CANVAS_W,
    canvasH: CANVAS_H,
    anchor: "bottom",
    measure,
    ...over,
  });

/**
 * Guarantees 1 and 2 in one place: every rect stays inside `bounds`, and no
 * two rects overlap vertically. Shared so the negative-input regression
 * tests below assert the exact same contract as the "happy path" tests
 * above, rather than a weaker approximation of it.
 *
 * The non-empty check is load-bearing: every band in this file has text, so an
 * empty result means the solver dropped them all — a post with no words on it.
 * Without it the per-rect loops would pass vacuously and report success.
 */
function expectValidStack(
  out: Record<string, SolvedBand>,
  bounds: NormalizedRect,
) {
  expect(Object.keys(out).length).toBeGreaterThan(0);
  const rects = Object.values(out).map((solved) => solved.rect);
  rects.forEach((rect) => {
    expect(rect.x).toBeGreaterThanOrEqual(bounds.x - 1e-9);
    expect(rect.y).toBeGreaterThanOrEqual(bounds.y - 1e-9);
    expect(rect.x + rect.w).toBeLessThanOrEqual(bounds.x + bounds.w + 1e-9);
    expect(rect.y + rect.h).toBeLessThanOrEqual(bounds.y + bounds.h + 1e-9);
  });
  const stacked = [...rects].sort((p, q) => p.y - q.y);
  for (let i = 1; i < stacked.length; i += 1) {
    expect(stacked[i].y).toBeGreaterThanOrEqual(
      stacked[i - 1].y + stacked[i - 1].h - 1e-9,
    );
  }
}

describe("solveStack", () => {
  it("returns a rect and a font size for every band with text", () => {
    const out = solve({
      bands: [band("dishName"), band("price", { text: "$12" })],
    });
    expect(Object.keys(out).sort()).toEqual(["dishName", "price"]);
    // Content that fits needs no shrinking, so fontPx is the band's full size.
    expect(out.dishName.fontPx).toBeCloseTo(0.06 * CANVAS_W, 6);
    expect(out.price.fontPx).toBeCloseTo(0.06 * CANVAS_W, 6);
  });

  it("omits bands whose text is empty", () => {
    const out = solve({
      bands: [band("dishName"), band("price", { text: "  " })],
    });
    expect(Object.keys(out)).toEqual(["dishName"]);
  });

  it("keeps every rect inside the bounds", () => {
    const out = solve({ bands: [band("a"), band("b"), band("c"), band("d")] });
    expectValidStack(out, BOUNDS);
  });

  it("never overlaps two rects vertically", () => {
    const out = solve({ bands: [band("a"), band("b"), band("c")] });
    expectValidStack(out, BOUNDS);
  });

  it("scales type down rather than overflowing when content is too tall", () => {
    const many = Array.from({ length: 12 }, (_, i) =>
      band(`b${i}`, { sizePct: 0.1 }),
    );
    const out = solve({ bands: many });
    // Every band survives: shrinking the type is what makes them fit. Clipping
    // would also satisfy the height assertion below, so without this the test
    // would pass with type scaling disabled entirely.
    expect(Object.keys(out)).toHaveLength(12);
    const total = Object.values(out).reduce((sum, s) => sum + s.rect.h, 0);
    expect(total).toBeLessThanOrEqual(BOUNDS.h + 1e-9);
    expectValidStack(out, BOUNDS);
  });

  it("reports the settled type scale through fontPx", () => {
    // Four bands at this size overflow at full scale but fit one step down, so
    // the stack settles at an intermediate scale rather than at MIN_SCALE.
    const out = solve({
      bands: Array.from({ length: 4 }, (_, i) =>
        band(`d${i}`, { sizePct: 0.22, text: "Hi" }),
      ),
    });
    expect(Object.keys(out)).toHaveLength(4);
    expect(out.d0.fontPx).toBeCloseTo(0.22 * 0.95 * CANVAS_W, 6);
    // The rect the caller lays out and the size it paints at must come from the
    // same settled scale — the whole reason fontPx is returned rather than
    // recomputed by the caller.
    expect(out.d0.rect.h).toBeCloseTo((out.d0.fontPx * 1.2) / CANVAS_H, 6);
  });

  it("clips from the end when the stack still overflows at MIN_SCALE", () => {
    const ids = Array.from({ length: 12 }, (_, i) => `b${i}`);
    const out = solve({ bands: ids.map((id) => band(id, { sizePct: 0.2 })) });
    const kept = Object.keys(out);
    expect(kept.length).toBeLessThan(ids.length);
    // Callers list the most important band first, so survivors are a prefix.
    expect(kept).toEqual(ids.slice(0, kept.length));
    expect(out.b0).toBeDefined();
    expect(out.b11).toBeUndefined();
    // Type stops shrinking at the legibility floor; the overflow is absorbed by
    // dropping bands, not by shrinking further.
    expect(out.b0.fontPx).toBeCloseTo(0.2 * MIN_SCALE * CANVAS_W, 6);
    expectValidStack(out, BOUNDS);
  });

  it.each(["top", "bottom"] as const)(
    "keeps a single over-tall band inside the bounds (anchor %s)",
    (anchor) => {
      // One band whose text is taller than the whole bounds even at MIN_SCALE.
      const out = solve({
        bands: [band("a", { sizePct: 1.2, lineHeight: 2 })],
        anchor,
      });
      expect(Object.keys(out)).toEqual(["a"]);
      expect(out.a.rect.h).toBeLessThanOrEqual(BOUNDS.h + 1e-9);
      expect(out.a.rect.y).toBeGreaterThanOrEqual(BOUNDS.y - 1e-9);
      expectValidStack(out, BOUNDS);
      // The rect is capped to the bounds but fontPx is not, so a caller can see
      // that the glyphs will overrun the box it was given.
      expect((out.a.fontPx * 2) / CANVAS_H).toBeGreaterThan(out.a.rect.h);
    },
  );

  it("anchors to the top when asked", () => {
    const out = solve({ anchor: "top" });
    expect(out.a.rect.y).toBeCloseTo(BOUNDS.y, 6);
  });

  /**
   * A width is only a width in a given font. The solver used to take a bare
   * `fontPx` and leave the face and the weight to whatever the measurer had
   * pinned its context to — one font for every band, while the walker painted
   * six. Both of the divergences that produced UNDER-measure the text, so
   * `wrapText` fits too many characters per line and the band overruns the
   * bounds this solver exists to enforce.
   */
  it("measures each band in that band's own face and weight", () => {
    const seen: string[] = [];
    solve({
      bands: [
        band("dishName", { font: "serif", weight: 700 }),
        band("price", { text: "$12", font: "sans", weight: 500 }),
      ],
      measure: (text, cssFont) => {
        seen.push(cssFont);
        return measure(text, cssFont);
      },
    });

    const px = Math.round(0.06 * CANVAS_W);
    expect(new Set(seen)).toEqual(
      new Set([
        // Serif is pinned to 400 by the shared builder, weight 700 and all.
        `400 ${px}px 'DM Serif Display', Georgia, serif`,
        `500 ${px}px 'DM Sans', sans-serif`,
      ]),
    );
  });

  /**
   * `sizePct` is a fraction of canvas WIDTH and `gapPct` used to be a fraction
   * of canvas HEIGHT, so one declaration meant three different amounts of ink:
   * a 0.018 gap was 19.4px on a square, 24.3px on a portrait and 34.6px on a
   * story. Nothing in a composition file said so — the two fields sit adjacent
   * in the same object literal and read as the same kind of number — so every
   * tuning pass was really tuning the square and letting the other two drift.
   *
   * All three aspects share a 1080px width (`ASPECT_DIMS`), so resolving the
   * gap against width is what makes a declared number mean one thing. It also
   * makes the gap commensurate with the type it separates, which is the
   * property spacing actually needs: both sides of the ratio now scale with the
   * same dimension, so a 0.018 gap under a 0.075 headline stays the same
   * fraction of that headline on every canvas.
   */
  it("resolves the same declared gap to the same device pixels at every aspect", () => {
    const gapsPx = (Object.keys(ASPECT_DIMS) as AspectRatio[]).map((aspect) => {
      const { w: canvasW, h: canvasH } = ASPECT_DIMS[aspect];
      const out = solve({
        bands: [band("dishName"), band("price", { text: "$12" })],
        canvasW,
        canvasH,
        anchor: "top",
      });
      const gap =
        out.price.rect.y - (out.dishName.rect.y + out.dishName.rect.h);
      return Math.round(gap * canvasH * 100) / 100;
    });
    // Pinned to the value, not merely to agreement: `new Set(gapsPx).size === 1`
    // is also satisfied by three zeroes, i.e. by dropping gaps entirely.
    expect(gapsPx).toEqual([21.6, 21.6, 21.6]);
    expect(gapsPx[0]).toBeCloseTo(0.02 * CANVAS_W, 6);
  });

  it("wraps long text into extra lines up to maxLines", () => {
    const out = solve({
      bands: [
        band("dishName", {
          text: "A very long dish name that will not fit on one line at all",
          maxLines: 3,
        }),
      ],
    });
    const oneLineHeight = (0.06 * CANVAS_W * 1.2) / CANVAS_H;
    expect(out.dishName.rect.h).toBeGreaterThan(oneLineHeight * 1.5);
  });
});

// Regression coverage for negative numeric band inputs. Each case reproduces a
// concrete violation seen against the pre-fix solver: negative `gapPct` pulled
// later bands up into earlier ones and outside `bounds`; negative `sizePct` /
// `lineHeight` produced a negative-height band whose neighbor then overlapped
// it. Nothing in `Band` rules those values out at the type level, so the solver
// has to floor them itself rather than trusting callers.
describe("solveStack — negative numeric inputs", () => {
  it("clamps a negative gap to zero instead of overlapping bands (anchor bottom)", () => {
    const out = solve({
      bands: [band("a"), band("b"), band("c")].map((b) => ({
        ...b,
        gapPct: -0.1,
      })),
    });
    expectValidStack(out, BOUNDS);
  });

  it("clamps a negative gap to zero instead of overlapping bands (anchor top)", () => {
    const out = solve({
      bands: [band("a"), band("b"), band("c")].map((b) => ({
        ...b,
        gapPct: -0.1,
      })),
      anchor: "top",
    });
    expectValidStack(out, BOUNDS);
  });

  it("clamps a large negative gap to zero rather than placing rects outside bounds", () => {
    const out = solve({
      bands: [band("a"), band("b"), band("c")].map((b) => ({
        ...b,
        gapPct: -0.5,
      })),
      anchor: "top",
    });
    expectValidStack(out, BOUNDS);
  });

  it("treats a negative sizePct as zero instead of producing a negative-height band", () => {
    const out = solve({ bands: [band("a", { sizePct: -0.06 }), band("b")] });
    expectValidStack(out, BOUNDS);
    expect(out.a.rect.h).toBe(0);
    expect(out.a.fontPx).toBe(0);
  });

  it("treats a negative lineHeight as zero instead of producing a negative-height band", () => {
    const out = solve({ bands: [band("a", { lineHeight: -1.2 }), band("b")] });
    expectValidStack(out, BOUNDS);
    expect(out.a.rect.h).toBe(0);
  });
});
