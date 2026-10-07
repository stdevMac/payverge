import { BLUR_FLOOR } from "./edges";
import type { PhotoAnalysis } from "./types";

type ReadinessVerdict = "ready" | "weak" | "reshoot";

export interface PhotoReadiness {
  verdict: ReadinessVerdict;
  /**
   * 0–100. For SORTING a library and for a coarse chip, never for display as a
   * precise figure: the penalties below are judgement calls, and a "73" implies
   * a precision this does not have.
   */
  score: number;
  /**
   * i18n key suffixes under `photoReadiness.reasons.*`, most severe first.
   * Present even on a passing photo — some of these are advisories, not
   * failures. See `noQuietArea`.
   */
  reasons: string[];
}

/**
 * Penalties, in score points. Each is a judgement about how much a defect costs
 * a finished post, and the three that matter most are the three the renderer
 * cannot fix:
 *
 *  - focus. No grade recovers it, so it is worth more than everything else
 *    combined at its worst.
 *  - clipping. Detail that is gone is gone; brightness only moves what is left,
 *    which is why `gradeFor` refuses to correct into a clipped end.
 *  - exposure. The grade DOES correct this, up to ±30%, so it is penalized
 *    less than the two above and the thresholds sit outside what the grade can
 *    rescue.
 *
 * `noQuietArea` is the odd one out: a busy frame with no negative space is
 * perfectly postable — `cornerCard` is the family for it — so it is a 15-point
 * advisory that leaves a good photo in "ready".
 *
 * `extremeAspect` is also advisory: fitCover never squashes, but a panorama or
 * ultra-tall phone shot will discard most of the frame into every common post
 * aspect. Flag it so operators re-shoot or generate a native hero.
 */
const VERY_SOFT_PENALTY = 60;
const SOFT_PENALTY = 40;
const EXPOSURE_PENALTY = 25;
const CLIPPING_PENALTY = 20;
const CROWDED_PENALTY = 15;
const EXTREME_ASPECT_PENALTY = 12;

/** Below `BLUR_FLOOR / 2` a photo is not soft, it is out of focus. */
const VERY_SOFT_DIVISOR = 2;

/** Outside this range the grade's ±30% cannot bring the photo back to target. */
const UNDEREXPOSED_LUMA = 60;
const OVEREXPOSED_LUMA = 200;

/** Clipping fractions an operator would notice in a finished post. */
const HIGHLIGHT_CLIP_LIMIT = 0.06;
const SHADOW_CLIP_LIMIT = 0.1;

/**
 * Source width/height ratio outside this band crops heavily into every common
 * post canvas (1:1, 4:5, 9:16). 2.1 ≈ wider than 21:10 panorama; 0.48 ≈ taller
 * than 9:18.7 (beyond Story). Phone 9:16 (~0.562) and landscape menu 3:2 (1.5)
 * stay inside the band.
 */
const EXTREME_ASPECT_WIDE = 2.1;
const EXTREME_ASPECT_TALL = 0.48;

const READY_SCORE = 75;
const WEAK_SCORE = 45;

/**
 * Whether intrinsic source dimensions are so wide or tall that cover-fit into
 * feed/story frames will discard most of the photo. Pure; safe with missing
 * dimensions (returns false — no false alarm when size is unknown).
 */
export function isExtremeSourceAspect(
  sourceWidth: number | undefined,
  sourceHeight: number | undefined,
): boolean {
  if (
    !sourceWidth ||
    !sourceHeight ||
    !Number.isFinite(sourceWidth) ||
    !Number.isFinite(sourceHeight) ||
    sourceWidth <= 0 ||
    sourceHeight <= 0
  ) {
    return false;
  }
  const ratio = sourceWidth / sourceHeight;
  return ratio >= EXTREME_ASPECT_WIDE || ratio <= EXTREME_ASPECT_TALL;
}

/**
 * Fraction of the source area that survives cover-fit into a destination
 * rectangle at zoom 1 (DEFAULT_CROP). 1 = no crop; lower = more discard.
 * Used by tests and composer diagnostics; readiness uses the coarser
 * `isExtremeSourceAspect` band so library scoring does not depend on a target.
 */
export function coverFitRetention(
  srcW: number,
  srcH: number,
  dstW: number,
  dstH: number,
): number {
  if (srcW <= 0 || srcH <= 0 || dstW <= 0 || dstH <= 0) return 0;
  const srcRatio = srcW / srcH;
  const dstRatio = dstW / dstH;
  // cover-fit keeps min(srcArea, area of the cover window). The cover window
  // is the largest rect of dstRatio that fits in the source:
  let coverW: number;
  let coverH: number;
  if (srcRatio > dstRatio) {
    coverH = srcH;
    coverW = srcH * dstRatio;
  } else {
    coverW = srcW;
    coverH = srcW / dstRatio;
  }
  return (coverW * coverH) / (srcW * srcH);
}

/**
 * Score one analysed photo.
 *
 * A null analysis is NOT scored as perfect and NOT scored as unknown: it is a
 * photo the browser could not read back, which in production overwhelmingly
 * means the CORS gate is closed on its bucket, and a panel that quietly
 * certified those would tell an operator their library is fine when the app
 * cannot see any of it.
 */
export function readinessFor(
  analysis: PhotoAnalysis | null,
): PhotoReadiness {
  if (!analysis) {
    return { verdict: "reshoot", score: 0, reasons: ["unreadable"] };
  }

  const { meanLuma, shadowClipping, highlightClipping } = analysis.exposure;
  const faults: Array<{ reason: string; penalty: number }> = [];

  if (analysis.blurScore < BLUR_FLOOR / VERY_SOFT_DIVISOR) {
    faults.push({ reason: "verySoft", penalty: VERY_SOFT_PENALTY });
  } else if (analysis.blurScore < BLUR_FLOOR) {
    faults.push({ reason: "soft", penalty: SOFT_PENALTY });
  }

  if (meanLuma < UNDEREXPOSED_LUMA) {
    faults.push({ reason: "underexposed", penalty: EXPOSURE_PENALTY });
  } else if (meanLuma > OVEREXPOSED_LUMA) {
    faults.push({ reason: "overexposed", penalty: EXPOSURE_PENALTY });
  }

  if (highlightClipping > HIGHLIGHT_CLIP_LIMIT) {
    faults.push({ reason: "blownHighlights", penalty: CLIPPING_PENALTY });
  }
  if (shadowClipping > SHADOW_CLIP_LIMIT) {
    faults.push({ reason: "crushedShadows", penalty: CLIPPING_PENALTY });
  }

  if (analysis.busy && analysis.negativeSpace === "none") {
    faults.push({ reason: "noQuietArea", penalty: CROWDED_PENALTY });
  }

  if (isExtremeSourceAspect(analysis.sourceWidth, analysis.sourceHeight)) {
    faults.push({ reason: "extremeAspect", penalty: EXTREME_ASPECT_PENALTY });
  }

  const penalty = faults.reduce((sum, fault) => sum + fault.penalty, 0);
  const score = Math.max(0, Math.min(100, 100 - penalty));

  return {
    score,
    verdict:
      score >= READY_SCORE ? "ready" : score >= WEAK_SCORE ? "weak" : "reshoot",
    // Stable sort (ES2019+), so equal-penalty faults keep the order they were
    // pushed in and the same photo always lists its reasons the same way.
    reasons: [...faults]
      .sort((left, right) => right.penalty - left.penalty)
      .map((fault) => fault.reason),
  };
}
