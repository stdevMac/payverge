import { perceptualLuma } from "./luma";
import type { ExposureStats, ImageDataLike } from "./types";

/**
 * Histogram resolution. 16 buckets of 16 luma values each: coarse enough that a
 * 4096-pixel downsample fills them meaningfully, fine enough to see a photo
 * bunched into the bottom third. Nothing downstream indexes a specific bucket
 * except the two clipping fractions, which are computed from raw luma rather
 * than from bucket membership so this number can change without moving them.
 */
export const EXPOSURE_BUCKETS = 16;

/** At or below this, shadow detail is gone. */
const SHADOW_CLIP = 8;
/** At or above this, highlight detail is gone. */
const HIGHLIGHT_CLIP = 247;

/** See `lumaPlane`: transparent pixels carry meaningless channel values. */
const ALPHA_FLOOR = 128;

/** What an empty or fully transparent image reports: perfectly neutral. */
const NEUTRAL = 128;

/**
 * Luma distribution, clipping fractions and per-channel means.
 *
 * The neutral fallback for an empty image is deliberate and is not a silent
 * failure: every consumer of this either divides by a channel mean
 * (`grade.ts`, which would produce Infinity) or compares `meanLuma` against a
 * target (`readiness.ts`, which would call a missing photo underexposed). 128
 * makes both no-ops, so an unanalysable photo is graded and scored as though it
 * needed nothing — which is the correct behaviour when nothing is known.
 */
export function exposureStats(img: ImageDataLike): ExposureStats {
  const histogram = new Array<number>(EXPOSURE_BUCKETS).fill(0);
  const { data, width, height } = img;
  const total = Math.max(0, width) * Math.max(0, height);

  let lumaSum = 0;
  let rSum = 0;
  let gSum = 0;
  let bSum = 0;
  let shadow = 0;
  let highlight = 0;
  let count = 0;

  for (let index = 0; index < total; index += 1) {
    const o = index * 4;
    if (data[o + 3] < ALPHA_FLOOR) continue;
    const r = data[o];
    const g = data[o + 1];
    const b = data[o + 2];
    const luma = perceptualLuma(r, g, b);
    // Bucket against a rounded byte: Rec.601 coefficients sum to just under 1
    // in IEEE float, so a pure grey (128,128,128) yields ~127.999… and floor
    // would land it in the wrong 16-wide bucket without the round.
    const lumaByte = Math.min(255, Math.max(0, Math.round(luma)));
    const bucket = Math.min(
      EXPOSURE_BUCKETS - 1,
      Math.max(0, Math.floor((lumaByte * EXPOSURE_BUCKETS) / 256)),
    );
    histogram[bucket] += 1;
    lumaSum += luma;
    rSum += r;
    gSum += g;
    bSum += b;
    if (luma <= SHADOW_CLIP) shadow += 1;
    if (luma >= HIGHLIGHT_CLIP) highlight += 1;
    count += 1;
  }

  if (count === 0) {
    return {
      histogram,
      meanLuma: NEUTRAL,
      shadowClipping: 0,
      highlightClipping: 0,
      channelMeans: { r: NEUTRAL, g: NEUTRAL, b: NEUTRAL },
    };
  }

  return {
    histogram: histogram.map((bucket) => bucket / count),
    meanLuma: lumaSum / count,
    shadowClipping: shadow / count,
    highlightClipping: highlight / count,
    channelMeans: { r: rSum / count, g: gSum / count, b: bSum / count },
  };
}
