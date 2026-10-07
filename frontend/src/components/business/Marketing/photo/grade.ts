import { toHex } from "../artDirection/color";
import type { PhotoAnalysis } from "./types";

/**
 * What a kit's grade preset becomes once the photo has been measured.
 *
 * Two outputs, because Canvas 2D can express one of these and not the other:
 *
 *  - `filter` is CSS filter-function syntax, assignable to `ctx.filter`, to an
 *    SVG/CSS `filter` for print, and to a CSS filter for video. Deliberately
 *    renderer-neutral, matching `PhotoNode.filter`.
 *  - `whiteBalance` is a colour to `multiply`-blend over the photo, because the
 *    CSS filter set has no per-channel gain. `blur`, `brightness`, `contrast`,
 *    `grayscale`, `hue-rotate`, `invert`, `opacity`, `saturate` and `sepia`
 *    move all three channels together or rotate hue wholesale; none of them is
 *    "scale red by 0.94". A source-over multiply IS a per-channel scale, so it
 *    is the correct primitive rather than a trick. A walker that cannot
 *    multiply-blend must drop it, exactly as it would drop `filter`.
 */
export interface Grade {
  filter: string;
  /** `#rrggbb` to multiply over the photo, or null when the photo is neutral. */
  whiteBalance: string | null;
}

/** Mid-grey: the exposure the auto-brightness term aims at. */
const TARGET_LUMA = 128;

/**
 * Brightness bounds. The job is to rescue a photo, not rebuild it: past roughly
 * ±30% the correction stops reading as a fix and starts reading as a filter,
 * and a photo that needs more than this is a reshoot — which is what the
 * readiness panel is for.
 */
const MIN_BRIGHTNESS = 0.85;
const MAX_BRIGHTNESS = 1.3;

/** Under this, a brightness term is invisible and not worth emitting. */
const BRIGHTNESS_EPSILON = 0.005;

/**
 * Clipping fraction past which a correction is refused in that direction.
 * Brightening a photo with blown highlights spreads the blowout; darkening one
 * whose shadows are already crushed loses the last of the shadow detail.
 */
const CLIPPING_GUARD = 0.02;

/**
 * Floor on each white-balance multiplier.
 *
 * A full grey-world correction assumes the scene averages to neutral, which
 * restaurant photography deliberately violates — warm tungsten light and a
 * wooden table are the look, not a cast. 0.82 corrects roughly the first fifth
 * of a strong cast and leaves the rest, so a warm photo stays warm and a photo
 * shot under a green fluorescent still loses its worst.
 */
const WB_FLOOR = 0.82;

/** Above this, the cast is under 2% on every channel — not worth a draw call. */
const WB_SKIP = 0.98;

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

/** Three decimals: finer than any display can resolve, stable across runs. */
function round3(value: number): number {
  return Math.round(value * 1000) / 1000;
}

interface WhiteBalance {
  color: string;
  /** Mean of the three multipliers: how much light the multiply removes. */
  meanMultiplier: number;
}

/**
 * Grey-world white balance, expressed as multipliers no greater than 1.
 *
 * Grey-world takes the average of each channel to be the same neutral, so the
 * gain that neutralizes the cast is `meanOfAllChannels / channelMean`. Those
 * gains straddle 1 — one channel is always boosted — and a multiply can only
 * darken, so all three are divided by the largest, which turns "boost blue"
 * into "hold blue, cut red and green". The light lost that way comes back as a
 * `brightness()` term in `gradeFor`.
 */
function whiteBalanceFor(means: {
  r: number;
  g: number;
  b: number;
}): WhiteBalance | null {
  const mean = (means.r + means.g + means.b) / 3;
  if (mean <= 0) return null;

  const gains = {
    r: mean / Math.max(1, means.r),
    g: mean / Math.max(1, means.g),
    b: mean / Math.max(1, means.b),
  };
  const largest = Math.max(gains.r, gains.g, gains.b);
  if (largest <= 0) return null;

  const multipliers = {
    r: Math.max(WB_FLOOR, gains.r / largest),
    g: Math.max(WB_FLOOR, gains.g / largest),
    b: Math.max(WB_FLOOR, gains.b / largest),
  };
  if (Math.min(multipliers.r, multipliers.g, multipliers.b) >= WB_SKIP) {
    return null;
  }

  return {
    color: toHex({
      r: multipliers.r * 255,
      g: multipliers.g * 255,
      b: multipliers.b * 255,
    }),
    meanMultiplier: (multipliers.r + multipliers.g + multipliers.b) / 3,
  };
}

/**
 * The kit's declared grade, corrected for what the photo actually is.
 *
 * With no analysis the kit preset passes through unchanged, so a CORS-blocked
 * or undecodable photo renders exactly as it did before this wave — the same
 * degradation every other consumer of the analysis takes.
 */
export function gradeFor(
  kitFilter: string,
  analysis: PhotoAnalysis | null,
): Grade {
  if (!analysis) return { filter: kitFilter, whiteBalance: null };

  const { meanLuma, shadowClipping, highlightClipping, channelMeans } =
    analysis.exposure;

  let brightness = TARGET_LUMA / Math.max(1, meanLuma);
  if (highlightClipping > CLIPPING_GUARD) brightness = Math.min(brightness, 1);
  if (shadowClipping > CLIPPING_GUARD) brightness = Math.max(brightness, 1);

  const balance = whiteBalanceFor(channelMeans);
  if (balance) brightness /= balance.meanMultiplier;

  const settled = round3(clamp(brightness, MIN_BRIGHTNESS, MAX_BRIGHTNESS));
  const terms = [kitFilter.trim()];
  if (Math.abs(settled - 1) >= BRIGHTNESS_EPSILON) {
    terms.push(`brightness(${settled})`);
  }

  return {
    filter: terms.filter(Boolean).join(" "),
    whiteBalance: balance?.color ?? null,
  };
}
