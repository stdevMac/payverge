import type { PhotoAnalysis } from "./types";
import {
  coverFitRetention,
  isExtremeSourceAspect,
  readinessFor,
} from "./readiness";

function analysis(over: {
  blurScore?: number;
  meanLuma?: number;
  shadowClipping?: number;
  highlightClipping?: number;
  busy?: boolean;
  negativeSpace?: PhotoAnalysis["negativeSpace"];
  sourceWidth?: number;
  sourceHeight?: number;
}): PhotoAnalysis {
  return {
    luma: { size: 1, values: [128] },
    edges: { size: 1, values: [0] },
    blurScore: over.blurScore ?? 500,
    exposure: {
      histogram: new Array(16).fill(0),
      meanLuma: over.meanLuma ?? 128,
      shadowClipping: over.shadowClipping ?? 0,
      highlightClipping: over.highlightClipping ?? 0,
      channelMeans: { r: 128, g: 128, b: 128 },
    },
    subject: { x: 0.2, y: 0.2, w: 0.5, h: 0.5 },
    focal: { x: 0.45, y: 0.45 },
    negativeSpace: over.negativeSpace ?? "bottom",
    busy: over.busy ?? false,
    dominantColors: ["#c2410c"],
    sourceWidth: over.sourceWidth,
    sourceHeight: over.sourceHeight,
  };
}

describe("readinessFor", () => {
  it("passes a sharp, well-exposed photo with a quiet band", () => {
    expect(readinessFor(analysis({}))).toEqual({
      verdict: "ready",
      score: 100,
      reasons: [],
    });
  });

  it("marks a soft photo weak", () => {
    const readiness = readinessFor(analysis({ blurScore: 100 }));
    expect(readiness.verdict).toBe("weak");
    expect(readiness.score).toBe(60);
    expect(readiness.reasons).toEqual(["soft"]);
  });

  it("sends a badly out-of-focus photo back for a reshoot", () => {
    const readiness = readinessFor(analysis({ blurScore: 20 }));
    expect(readiness.verdict).toBe("reshoot");
    expect(readiness.reasons).toEqual(["verySoft"]);
  });

  it("orders reasons by severity", () => {
    const readiness = readinessFor(
      analysis({
        meanLuma: 40,
        shadowClipping: 0.3,
        busy: true,
        negativeSpace: "none",
      }),
    );
    expect(readiness.reasons).toEqual([
      "underexposed",
      "crushedShadows",
      "noQuietArea",
    ]);
    expect(readiness.score).toBe(40);
    expect(readiness.verdict).toBe("reshoot");
  });

  it("flags blown highlights", () => {
    expect(readinessFor(analysis({ highlightClipping: 0.1 })).reasons).toEqual([
      "blownHighlights",
    ]);
  });

  it("flags an overexposed photo", () => {
    expect(readinessFor(analysis({ meanLuma: 220 })).reasons).toContain(
      "overexposed",
    );
  });

  // A busy photo with nowhere quiet is still usable — cornerCard exists for
  // exactly this — so it is an advisory on a passing photo, not a failure.
  it("advises about a crowded frame without failing it", () => {
    const readiness = readinessFor(
      analysis({ busy: true, negativeSpace: "none" }),
    );
    expect(readiness.verdict).toBe("ready");
    expect(readiness.reasons).toEqual(["noQuietArea"]);
  });

  it("never certifies a photo it could not read", () => {
    expect(readinessFor(null)).toEqual({
      verdict: "reshoot",
      score: 0,
      reasons: ["unreadable"],
    });
  });

  it("never reports a negative score", () => {
    const readiness = readinessFor(
      analysis({
        blurScore: 10,
        meanLuma: 250,
        highlightClipping: 0.5,
        shadowClipping: 0.5,
        busy: true,
        negativeSpace: "none",
      }),
    );
    expect(readiness.score).toBe(0);
  });

  it("flags extreme source aspects as an advisory without failing a sharp photo", () => {
    // 3:1 panorama — wider than EXTREME_ASPECT_WIDE (2.1).
    const readiness = readinessFor(
      analysis({ sourceWidth: 3000, sourceHeight: 1000 }),
    );
    expect(readiness.reasons).toEqual(["extremeAspect"]);
    expect(readiness.verdict).toBe("ready");
    expect(readiness.score).toBe(88);
  });

  it("leaves normal phone and menu aspects unflagged", () => {
    // 9:16 phone story and 3:2 menu landscape stay inside the band.
    expect(
      readinessFor(analysis({ sourceWidth: 1080, sourceHeight: 1920 })).reasons,
    ).toEqual([]);
    expect(
      readinessFor(analysis({ sourceWidth: 3000, sourceHeight: 2000 })).reasons,
    ).toEqual([]);
  });
});

describe("isExtremeSourceAspect", () => {
  it.each([
    [4000, 1000, true], // 4:1 strip-like
    [1000, 3000, true], // ultra-tall
    [1080, 1350, false], // 4:5 feed
    [1080, 1920, false], // 9:16 story
    [1920, 1080, false], // 16:9 still under 2.1
    [0, 100, false],
    [undefined, 100, false],
  ] as const)(
    "(%s×%s) → %s",
    (w, h, expected) => {
      expect(isExtremeSourceAspect(w, h)).toBe(expected);
    },
  );
});

describe("coverFitRetention", () => {
  it("keeps the full square source in a square destination", () => {
    expect(coverFitRetention(1000, 1000, 1000, 1000)).toBeCloseTo(1, 6);
  });

  it("discards sides of a wide source into a tall destination (no squash)", () => {
    // 2:1 landscape into 1:1 — cover keeps half the area (height-limited).
    expect(coverFitRetention(2000, 1000, 1000, 1000)).toBeCloseTo(0.5, 6);
  });

  it("discards top/bottom of a tall phone photo into feed 4:5", () => {
    // 9:16 (0.5625) into 4:5 (0.8) — width-limited cover keeps 0.5625/0.8 of height.
    const retention = coverFitRetention(1080, 1920, 1080, 1350);
    expect(retention).toBeCloseTo(1080 / 1920 / (1080 / 1350), 5);
    expect(retention).toBeLessThan(1);
    expect(retention).toBeGreaterThan(0.5);
  });
});
