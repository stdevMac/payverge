import type { PhotoAnalysis } from "./types";
import { gradeFor } from "./grade";

const KIT = "saturate(1.04) contrast(1.03)";

function analysis(over: {
  meanLuma?: number;
  shadowClipping?: number;
  highlightClipping?: number;
  channelMeans?: { r: number; g: number; b: number };
}): PhotoAnalysis {
  return {
    luma: { size: 1, values: [128] },
    edges: { size: 1, values: [0] },
    blurScore: 500,
    exposure: {
      histogram: new Array(16).fill(0),
      meanLuma: over.meanLuma ?? 128,
      shadowClipping: over.shadowClipping ?? 0,
      highlightClipping: over.highlightClipping ?? 0,
      channelMeans: over.channelMeans ?? { r: 128, g: 128, b: 128 },
    },
    subject: { x: 0, y: 0, w: 1, h: 1 },
    focal: { x: 0.5, y: 0.5 },
    negativeSpace: "none",
    busy: false,
    dominantColors: [],
  };
}

describe("gradeFor", () => {
  it("passes the kit preset through untouched when nothing was measured", () => {
    expect(gradeFor(KIT, null)).toEqual({ filter: KIT, whiteBalance: null });
  });

  it("adds nothing to a neutral, correctly exposed photo", () => {
    expect(gradeFor(KIT, analysis({}))).toEqual({
      filter: KIT,
      whiteBalance: null,
    });
  });

  it("lifts an underexposed photo, up to the cap", () => {
    // 128 / 96 = 1.333, capped at 1.3 so a dark photo is rescued, not rebuilt.
    expect(gradeFor(KIT, analysis({ meanLuma: 96 })).filter).toBe(
      `${KIT} brightness(1.3)`,
    );
  });

  it("pulls back an overexposed photo, down to the floor", () => {
    expect(
      gradeFor(KIT, analysis({ meanLuma: 210, highlightClipping: 0.05 }))
        .filter,
    ).toBe(`${KIT} brightness(0.85)`);
  });

  // A high-contrast photo whose shadows are already crushed must not be pushed
  // further down just because its mean sits above target.
  it("refuses to darken a photo whose shadows are already gone", () => {
    expect(
      gradeFor(KIT, analysis({ meanLuma: 180, shadowClipping: 0.2 })).filter,
    ).toBe(KIT);
  });

  it("neutralizes a mild colour cast with a multiply and compensates for it", () => {
    const grade = gradeFor(
      KIT,
      analysis({ channelMeans: { r: 132, g: 128, b: 124 } }),
    );
    expect(grade.whiteBalance).toBe("#f0f7ff");
    expect(grade.filter).toBe(`${KIT} brightness(1.032)`);
  });

  // The floor is what keeps a deliberately warm restaurant photo warm instead
  // of being scrubbed to studio neutral.
  it("clamps a strong cast rather than fully neutralizing it", () => {
    const grade = gradeFor(
      KIT,
      analysis({ channelMeans: { r: 150, g: 120, b: 90 } }),
    );
    expect(grade.whiteBalance).toBe("#d1d1ff");
    expect(grade.filter).toBe(`${KIT} brightness(1.136)`);
  });

  it("ignores a cast too small to see", () => {
    expect(
      gradeFor(KIT, analysis({ channelMeans: { r: 129, g: 128, b: 128 } }))
        .whiteBalance,
    ).toBeNull();
  });

  it("emits a bare term when the kit declares no filter", () => {
    expect(gradeFor("", analysis({ meanLuma: 96 })).filter).toBe(
      "brightness(1.3)",
    );
  });

  it("survives a black frame without dividing by zero", () => {
    const grade = gradeFor(
      KIT,
      analysis({ meanLuma: 0, channelMeans: { r: 0, g: 0, b: 0 } }),
    );
    expect(grade.whiteBalance).toBeNull();
    expect(grade.filter).toBe(`${KIT} brightness(1.3)`);
  });
});
