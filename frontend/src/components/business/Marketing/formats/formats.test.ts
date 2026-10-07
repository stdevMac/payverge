import { readFileSync } from "node:fs";
import { join } from "node:path";
import {
  ASPECT_DIMS,
  PLATFORM_CONTENT_BOUNDS,
  type AspectRatio,
} from "../templates/types";
import {
  FORMATS,
  FORMAT_ORDER,
  PRINT_FORMATS,
  SCREEN_FORMATS,
  formatForLegacyAspect,
  isFormatId,
  type FormatId,
} from "./formats";

describe("format registry", () => {
  it("keeps FORMAT_ORDER in sync with FORMATS", () => {
    expect(new Set(FORMAT_ORDER)).toEqual(new Set(Object.keys(FORMATS)));
    expect(FORMAT_ORDER).toHaveLength(new Set(FORMAT_ORDER).size);
  });

  it("gives every definition an id matching its key", () => {
    FORMAT_ORDER.forEach((id) => {
      expect(FORMATS[id].id).toBe(id);
    });
  });

  it("partitions cleanly into screen and print", () => {
    expect([...SCREEN_FORMATS, ...PRINT_FORMATS].sort()).toEqual(
      [...FORMAT_ORDER].sort(),
    );
    SCREEN_FORMATS.forEach((id) => expect(FORMATS[id].medium).toBe("screen"));
    PRINT_FORMATS.forEach((id) => expect(FORMATS[id].medium).toBe("print"));
  });

  it("declares a print spec on print formats and none on screen formats", () => {
    FORMAT_ORDER.forEach((id) => {
      const format = FORMATS[id];
      if (format.medium === "print") expect(format.print).not.toBeNull();
      else expect(format.print).toBeNull();
    });
  });

  it("gives every format positive pixel dimensions", () => {
    FORMAT_ORDER.forEach((id) => {
      expect(FORMATS[id].px.w).toBeGreaterThan(0);
      expect(FORMATS[id].px.h).toBeGreaterThan(0);
    });
  });

  it("keeps every safe area inside the canvas", () => {
    FORMAT_ORDER.forEach((id) => {
      const { safe } = FORMATS[id];
      expect(safe.x).toBeGreaterThanOrEqual(0);
      expect(safe.y).toBeGreaterThanOrEqual(0);
      expect(safe.w).toBeGreaterThan(0);
      expect(safe.h).toBeGreaterThan(0);
      expect(safe.x + safe.w).toBeLessThanOrEqual(1);
      expect(safe.y + safe.h).toBeLessThanOrEqual(1);
    });
  });

  // The registry supersedes ASPECT_DIMS and PLATFORM_CONTENT_BOUNDS but does not
  // delete them (templates/templates.ts still keys layouts by the legacy aspect).
  // Two sources of truth can drift; this is what stops them.
  it("mirrors ASPECT_DIMS exactly for the three legacy formats", () => {
    (Object.keys(ASPECT_DIMS) as AspectRatio[]).forEach((aspect) => {
      expect(FORMATS[aspect as FormatId].px).toEqual(ASPECT_DIMS[aspect]);
    });
  });

  it("mirrors PLATFORM_CONTENT_BOUNDS exactly for the three legacy formats", () => {
    (Object.keys(PLATFORM_CONTENT_BOUNDS) as AspectRatio[]).forEach((aspect) => {
      expect(FORMATS[aspect as FormatId].safe).toEqual(
        PLATFORM_CONTENT_BOUNDS[aspect],
      );
    });
  });

  it("round trips every legacy aspect through formatForLegacyAspect", () => {
    (Object.keys(ASPECT_DIMS) as AspectRatio[]).forEach((aspect) => {
      const format = formatForLegacyAspect(aspect);
      expect(format.id).toBe(aspect);
      expect(format.legacyAspect).toBe(aspect);
    });
  });

  it("narrows only exact registry ids", () => {
    expect(isFormatId("4:5")).toBe(true);
    expect(isFormatId(" 4:5")).toBe(false);
    expect(isFormatId("4:5 ")).toBe(false);
    expect(isFormatId("4:6")).toBe(false);
    expect(isFormatId("")).toBe(false);
    expect(isFormatId(undefined)).toBe(false);
    expect(isFormatId(45)).toBe(false);
  });
});

describe("Wave 4 screen formats", () => {
  it("registers wide and strip", () => {
    expect(FORMAT_ORDER).toContain("wide");
    expect(FORMAT_ORDER).toContain("strip");
    expect(FORMATS.wide.medium).toBe("screen");
    expect(FORMATS.strip.medium).toBe("screen");
    expect(FORMATS.wide.legacyAspect).toBeNull();
    expect(FORMATS.strip.legacyAspect).toBeNull();
  });

  it("gives wide a landscape canvas and strip a banner canvas", () => {
    expect(FORMATS.wide.px).toEqual({ w: 1920, h: 1080 });
    expect(FORMATS.strip.px).toEqual({ w: 1200, h: 300 });
  });

  // The claim this whole wave rests on. compositions.ts must contain no literal
  // keyed by "wide" or "strip" — the geometry comes from format.safe and the
  // format's own ratio.
  it("adds no per-format literal to compositions.ts", () => {
    const source = readFileSync(
      join(__dirname, "..", "composition", "compositions.ts"),
      "utf8",
    );
    expect(source).not.toContain('"wide"');
    expect(source).not.toContain('"strip"');
  });
});

describe("the 5:7 table tent", () => {
  it("registers as the only print format", () => {
    expect(PRINT_FORMATS).toEqual(["5:7"]);
    expect(FORMATS["5:7"].medium).toBe("print");
    expect(FORMATS["5:7"].legacyAspect).toBeNull();
  });

  it("declares a 5in x 7in trim at 300 dpi with a 1/8in bleed", () => {
    const print = FORMATS["5:7"].print!;
    expect(print.dpi).toBe(300);
    expect(print.trimWMm).toBeCloseTo(127, 6); // 5in
    expect(print.trimHMm).toBeCloseTo(177.8, 6); // 7in
    expect(print.bleedMm).toBeCloseTo(3.175, 6); // 1/8in
    expect(print.markLengthMm).toBe(5);
  });

  // px IS the bleed box rasterized at dpi, not the trim box. Full-bleed art has
  // to cover the bleed, and the scene's normalized 0..1 space is what covers it.
  it("sizes px as the bleed box at 300 dpi", () => {
    const format = FORMATS["5:7"];
    const print = format.print!;
    const mmPerPx = 25.4 / print.dpi;
    expect(format.px.w * mmPerPx).toBeCloseTo(print.trimWMm + 2 * print.bleedMm, 6);
    expect(format.px.h * mmPerPx).toBeCloseTo(print.trimHMm + 2 * print.bleedMm, 6);
    expect(format.px).toEqual({ w: 1575, h: 2175 });
  });

  it("keeps its safe area clear of the trim edge", () => {
    const format = FORMATS["5:7"];
    const print = format.print!;
    const mmPerPx = 25.4 / print.dpi;
    // Distance from the safe edge to the TRIM edge, in mm.
    const insetFromTrimMm = format.safe.x * format.px.w * mmPerPx - print.bleedMm;
    expect(insetFromTrimMm).toBeGreaterThan(5);
  });
});
