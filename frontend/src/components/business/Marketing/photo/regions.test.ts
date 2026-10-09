import type { CellGrid } from "./types";
import {
  BUSY_EDGE,
  FULL_FRAME,
  busyFrom,
  focalPointFrom,
  mapCanvasRectToSource,
  meanOfCells,
  negativeSpaceFrom,
  subjectBoxFrom,
} from "./regions";

/** An 8×8 grid whose value is chosen per (row, col). */
function grid(at: (row: number, col: number) => number, size = 8): CellGrid {
  const values: number[] = [];
  for (let row = 0; row < size; row += 1) {
    for (let col = 0; col < size; col += 1) values.push(at(row, col));
  }
  return { size, values };
}

describe("meanOfCells", () => {
  const quarters: CellGrid = { size: 2, values: [0, 100, 200, 300] };

  it("averages the whole grid over the full frame", () => {
    expect(meanOfCells(quarters, FULL_FRAME)).toBeCloseTo(150, 6);
  });

  it("weights cells by their overlap with the rect", () => {
    expect(
      meanOfCells(quarters, { x: 0, y: 0, w: 0.5, h: 1 }),
    ).toBeCloseTo(100, 6);
    expect(
      meanOfCells(quarters, { x: 0, y: 0, w: 1, h: 0.5 }),
    ).toBeCloseTo(50, 6);
    // Half of the top-left cell and half of the top-right: still their mean.
    expect(
      meanOfCells(quarters, { x: 0.25, y: 0, w: 0.5, h: 0.5 }),
    ).toBeCloseTo(50, 6);
  });

  it("falls back to the grid mean for a degenerate rect", () => {
    expect(meanOfCells(quarters, { x: 0.5, y: 0.5, w: 0, h: 0 })).toBeCloseTo(
      150,
      6,
    );
  });
});

describe("negativeSpaceFrom", () => {
  it("finds a quiet top band", () => {
    expect(negativeSpaceFrom(grid((row) => (row <= 2 ? 0 : 0.5)))).toBe("top");
  });

  it("finds a quiet bottom band", () => {
    expect(negativeSpaceFrom(grid((row) => (row >= 5 ? 0 : 0.5)))).toBe(
      "bottom",
    );
  });

  it("finds a quiet centre", () => {
    expect(
      negativeSpaceFrom(
        grid((row, col) =>
          row >= 3 && row <= 4 && col >= 2 && col <= 5 ? 0 : 0.5,
        ),
      ),
    ).toBe("center");
  });

  it("reports none when no band is distinctively quieter", () => {
    expect(negativeSpaceFrom(grid(() => 0.5))).toBe("none");
  });

  it("reports none on a flat photo, where no band is quieter than another", () => {
    expect(negativeSpaceFrom(grid(() => 0))).toBe("none");
  });

  // No composition can exploit a side band yet; returning one would be a value
  // no consumer reads. Wave 4's wide and strip formats are where they land.
  it("never returns a side band today", () => {
    const answers = [
      negativeSpaceFrom(grid((_row, col) => (col <= 2 ? 0 : 0.5))),
      negativeSpaceFrom(grid((_row, col) => (col >= 5 ? 0 : 0.5))),
    ];
    answers.forEach((answer) => {
      expect(answer).not.toBe("left");
      expect(answer).not.toBe("right");
    });
  });
});

describe("busyFrom", () => {
  it("calls a high-energy frame busy", () => {
    expect(busyFrom(grid(() => BUSY_EDGE + 0.01))).toBe(true);
  });

  it("does not call a calm frame busy", () => {
    expect(busyFrom(grid(() => BUSY_EDGE - 0.01))).toBe(false);
  });
});

describe("subjectBoxFrom", () => {
  it("boxes the high-energy cells", () => {
    const box = subjectBoxFrom(
      grid((row, col) =>
        row >= 1 && row <= 2 && col >= 5 && col <= 6 ? 1 : 0,
      ),
    );
    expect(box).toEqual({ x: 0.625, y: 0.125, w: 0.25, h: 0.25 });
  });

  it("falls back to the full frame when nothing stands out", () => {
    expect(subjectBoxFrom(grid(() => 0))).toEqual(FULL_FRAME);
  });
});

describe("focalPointFrom", () => {
  it("is the centre of the box", () => {
    expect(focalPointFrom({ x: 0.625, y: 0.125, w: 0.25, h: 0.25 })).toEqual({
      x: 0.75,
      y: 0.25,
    });
    expect(focalPointFrom(FULL_FRAME)).toEqual({ x: 0.5, y: 0.5 });
  });

  it("clamps a box that reaches outside the frame", () => {
    expect(focalPointFrom({ x: 0.9, y: -0.4, w: 0.4, h: 0.2 })).toEqual({
      x: 1,
      y: 0,
    });
  });
});

describe("mapCanvasRectToSource", () => {
  const natural = { w: 100, h: 200 };

  it("maps a canvas band onto an uncropped full-bleed photo", () => {
    expect(
      mapCanvasRectToSource(
        { x: 0, y: 0.5, w: 1, h: 0.5 },
        FULL_FRAME,
        { sx: 0, sy: 0, sw: 100, sh: 200 },
        natural,
      ),
    ).toEqual({ x: 0, y: 0.5, w: 1, h: 0.5 });
  });

  it("accounts for the cover-fit source window", () => {
    expect(
      mapCanvasRectToSource(
        { x: 0, y: 0.5, w: 1, h: 0.5 },
        FULL_FRAME,
        { sx: 20, sy: 40, sw: 60, sh: 120 },
        natural,
      ),
    ).toEqual({ x: 0.2, y: 0.5, w: 0.6, h: 0.3 });
  });

  it("clips the band to the image area before mapping", () => {
    // A bottom band over a photo panel that stops at 0.58: only the sliver from
    // 0.5 to 0.58 sits on the photograph.
    const mapped = mapCanvasRectToSource(
      { x: 0, y: 0.5, w: 1, h: 0.5 },
      { x: 0, y: 0, w: 1, h: 0.58 },
      { sx: 0, sy: 0, sw: 100, sh: 200 },
      natural,
    );
    expect(mapped).not.toBeNull();
    expect(mapped?.y).toBeCloseTo(0.5 / 0.58, 6);
    expect(mapped?.h).toBeCloseTo(0.08 / 0.58, 6);
  });

  it("returns null when the band never touches the photo", () => {
    expect(
      mapCanvasRectToSource(
        { x: 0, y: 0.6, w: 1, h: 0.4 },
        { x: 0, y: 0, w: 1, h: 0.58 },
        { sx: 0, sy: 0, sw: 100, sh: 200 },
        natural,
      ),
    ).toBeNull();
  });

  it("returns null for a degenerate photo", () => {
    expect(
      mapCanvasRectToSource(
        FULL_FRAME,
        FULL_FRAME,
        { sx: 0, sy: 0, sw: 100, sh: 200 },
        { w: 0, h: 0 },
      ),
    ).toBeNull();
  });
});
