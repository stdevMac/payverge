import { FORMATS } from "./formats";
import {
  cropMarkRects,
  pageBoxMm,
  pxToMm,
  pxToPt,
  rectToMm,
  trimOriginMm,
  HAIRLINE_MM,
} from "./printGeometry";

const TENT = FORMATS["5:7"];
const SPEC = TENT.print!;

describe("printGeometry", () => {
  it("converts pixels to millimetres at the format's dpi", () => {
    expect(pxToMm(300, 300)).toBeCloseTo(25.4, 9);
    expect(pxToMm(1575, 300)).toBeCloseTo(133.35, 9);
    expect(pxToMm(0, 300)).toBe(0);
  });

  it("converts pixels to points at the format's dpi", () => {
    // 72pt to the inch, so at 300 dpi one point is 300/72 px.
    expect(pxToPt(300, 300)).toBeCloseTo(72, 9);
    expect(pxToPt(118.125, 300)).toBeCloseTo(28.35, 6);
  });

  // Chromium silently falls back to A4 when @page size carries `auto` for a
  // fixed-format piece (recorded in the bill-alert print stabilization design).
  // The page box therefore has to be two explicit lengths, and it has to be big
  // enough to hold the marks outside the bleed.
  it("sizes the page to trim plus bleed plus marks on every side", () => {
    const page = pageBoxMm(SPEC);
    expect(page.wMm).toBeCloseTo(127 + 2 * (3.175 + 5), 9);
    expect(page.hMm).toBeCloseTo(177.8 + 2 * (3.175 + 5), 9);
    expect(page.wMm).toBeCloseTo(143.35, 9);
    expect(page.hMm).toBeCloseTo(194.15, 9);
  });

  it("places the trim box inset by mark length plus bleed", () => {
    const origin = trimOriginMm(SPEC);
    expect(origin.leftMm).toBeCloseTo(8.175, 9);
    expect(origin.topMm).toBeCloseTo(8.175, 9);
  });

  it("maps a normalized rect onto the bleed box, offset by the mark gutter", () => {
    // Full bleed covers exactly the bleed box, starting one mark length in.
    const full = rectToMm({ x: 0, y: 0, w: 1, h: 1 }, TENT);
    expect(full.leftMm).toBeCloseTo(5, 9);
    expect(full.topMm).toBeCloseTo(5, 9);
    expect(full.wMm).toBeCloseTo(133.35, 9);
    expect(full.hMm).toBeCloseTo(184.15, 9);

    const half = rectToMm({ x: 0.5, y: 0.25, w: 0.5, h: 0.5 }, TENT);
    expect(half.leftMm).toBeCloseTo(5 + 133.35 / 2, 9);
    expect(half.topMm).toBeCloseTo(5 + 184.15 / 4, 9);
  });

  it("emits eight crop marks, two per corner, offset outward by the bleed", () => {
    const marks = cropMarkRects(SPEC);
    expect(marks).toHaveLength(8);
    marks.forEach((mark) => {
      const isHorizontal = mark.wMm > mark.hMm;
      expect(isHorizontal ? mark.wMm : mark.hMm).toBeCloseTo(5, 9);
      expect(isHorizontal ? mark.hMm : mark.wMm).toBeCloseTo(HAIRLINE_MM, 9);
    });

    // The top-left horizontal mark runs from the page edge to the mark length,
    // stopping 3.175mm short of the trim corner so it never touches live art.
    const topLeftHorizontal = marks.find(
      (m) => m.leftMm === 0 && m.wMm > m.hMm && m.topMm < 100,
    );
    expect(topLeftHorizontal).toBeDefined();
    // Hairline is centred on the trim line, so top is inset by half the stroke.
    expect(topLeftHorizontal!.topMm).toBeCloseTo(8.175 - HAIRLINE_MM / 2, 9);
    expect(topLeftHorizontal!.wMm).toBeCloseTo(5, 9);
  });

  it("keeps every mark inside the page", () => {
    const page = pageBoxMm(SPEC);
    cropMarkRects(SPEC).forEach((mark) => {
      expect(mark.leftMm).toBeGreaterThanOrEqual(-1e-9);
      expect(mark.topMm).toBeGreaterThanOrEqual(-1e-9);
      expect(mark.leftMm + mark.wMm).toBeLessThanOrEqual(page.wMm + 1e-9);
      expect(mark.topMm + mark.hMm).toBeLessThanOrEqual(page.hMm + 1e-9);
    });
  });
});
