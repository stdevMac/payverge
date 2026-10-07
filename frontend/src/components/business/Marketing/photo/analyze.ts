import { extractDominantColors } from "../artDirection/palette";
import type { PhotoSignals } from "../composition/chooser";
import { edgeGrid, laplacianVariance } from "./edges";
import { exposureStats } from "./exposure";
import { lumaGrid } from "./grid";
import {
  busyFrom,
  focalPointFrom,
  negativeSpaceFrom,
  subjectBoxFrom,
} from "./regions";
import type { ImageDataLike, PhotoAnalysis } from "./types";

/**
 * Swatches pulled from the photo. Only the most distinct from the brand primary
 * survives `harmonizePalette`, so this is a shortlist rather than a palette: 5
 * gives median-cut enough buckets to separate a plate, a garnish and a
 * background without paying for splits nothing reads.
 */
export const DOMINANT_COLOR_COUNT = 5;

/**
 * Everything derived from one photo, in one pass over one downsample.
 *
 * Pure: takes `{ data, width, height }`, never a canvas, so it is tested with
 * synthetic arrays under a jest config that maps `^canvas$` to a mock. The
 * canvas-touching half — turning a decoded `HTMLImageElement` into that
 * argument — is `toAnalysisImageData` in `./downsample.ts`, which is the only
 * file in this folder that knows a canvas exists.
 *
 * Deliberately NOT memoized here: caching is `./cache.ts`'s job, keyed by photo
 * URL, because that is the identity a cache entry is actually about. A memo on
 * the `ImageDataLike` would key on a freshly-allocated object and never hit.
 */
export function analyzePhoto(img: ImageDataLike): PhotoAnalysis {
  const luma = lumaGrid(img);
  const edges = edgeGrid(img);
  const subject = subjectBoxFrom(edges);
  return {
    luma,
    edges,
    blurScore: laplacianVariance(img),
    exposure: exposureStats(img),
    subject,
    focal: focalPointFrom(subject),
    negativeSpace: negativeSpaceFrom(edges),
    busy: busyFrom(edges),
    dominantColors: extractDominantColors(img, DOMINANT_COLOR_COUNT),
  };
}

/**
 * The two fields `chooseComposition` reads, or null.
 *
 * Null propagates rather than defaulting: the chooser's photo branches are
 * written to be skipped when nothing is known (`photo?.busy`), and inventing
 * `{ negativeSpace: "none", busy: false }` for an unanalysable photo would be a
 * fabricated measurement that reads identically to a real calm one.
 */
export function photoSignalsFrom(
  analysis: PhotoAnalysis | null,
): PhotoSignals | null {
  if (!analysis) return null;
  return { negativeSpace: analysis.negativeSpace, busy: analysis.busy };
}
