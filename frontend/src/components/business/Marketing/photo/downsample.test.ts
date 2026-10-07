/** @jest-environment jsdom */
import { ANALYSIS_SIZE, toAnalysisImageData } from "./downsample";

interface StubCtx {
  drawImage: jest.Mock;
  getImageData: jest.Mock;
}

function stubCanvas(options: { taint?: boolean; noContext?: boolean } = {}) {
  const ctx: StubCtx = {
    drawImage: jest.fn(),
    getImageData: jest.fn((_x: number, _y: number, w: number, h: number) => {
      if (options.taint) {
        const error = new Error("tainted");
        error.name = "SecurityError";
        throw error;
      }
      return {
        data: new Uint8ClampedArray(w * h * 4).fill(200),
        width: w,
        height: h,
      };
    }),
  };
  jest
    .spyOn(HTMLCanvasElement.prototype, "getContext")
    .mockReturnValue(
      options.noContext ? null : (ctx as unknown as CanvasRenderingContext2D),
    );
  return ctx;
}

function image(naturalWidth: number, naturalHeight: number) {
  return { naturalWidth, naturalHeight } as unknown as HTMLImageElement;
}

afterEach(() => {
  jest.restoreAllMocks();
});

describe("toAnalysisImageData", () => {
  it("draws the whole photo into a square analysis buffer", () => {
    const ctx = stubCanvas();
    const result = toAnalysisImageData(image(2000, 1000));
    expect(result).not.toBeNull();
    expect(result?.width).toBe(ANALYSIS_SIZE);
    expect(result?.height).toBe(ANALYSIS_SIZE);
    expect(ctx.drawImage).toHaveBeenCalledWith(
      expect.anything(),
      0,
      0,
      2000,
      1000,
      0,
      0,
      ANALYSIS_SIZE,
      ANALYSIS_SIZE,
    );
  });

  it("honours an explicit size", () => {
    stubCanvas();
    expect(toAnalysisImageData(image(100, 100), 16)?.width).toBe(16);
  });

  // A bucket without Access-Control-Allow-Origin normally fails the LOAD, but a
  // same-origin-then-redirected asset can still taint. Returning null degrades
  // to the unanalysed path instead of taking the whole render down.
  it("returns null on a tainted canvas rather than throwing", () => {
    stubCanvas({ taint: true });
    expect(toAnalysisImageData(image(100, 100))).toBeNull();
  });

  it("returns null when a 2D context is unavailable", () => {
    stubCanvas({ noContext: true });
    expect(toAnalysisImageData(image(100, 100))).toBeNull();
  });

  it("returns null for an image that never decoded", () => {
    stubCanvas();
    expect(toAnalysisImageData(image(0, 0))).toBeNull();
  });
});
