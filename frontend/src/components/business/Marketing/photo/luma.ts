/**
 * Perceptual luma, 0–255, in the Rec.601 weighting.
 *
 * This module is a LEAF on purpose, for the same reason
 * `../artDirection/typeface.ts` is one. Five things now measure brightness the
 * same way — `sampleRegionLuminance` in the walker, the analysis cell grid, the
 * Sobel and Laplacian planes, the exposure histogram, and the scrim boost
 * decision — and they agree by all calling this, not by all happening to spell
 * the same three multiplications.
 *
 * It is emphatically NOT `relativeLuminance` from `../artDirection/color.ts`,
 * which is WCAG 2.1's 0–1 sRGB-linear quantity used for contrast ratios. Both
 * are called "luminance" in this feature; they are not interchangeable and
 * neither is a rescaling of the other. Use this one wherever a value is
 * compared against `ScrimNode.luminanceThreshold`, and that one wherever a
 * value is compared against a contrast ratio.
 *
 * Nothing here may import from `../scene/`, `../composition/` or
 * `../templates/`: the walker depends on it.
 */

export const LUMA_R = 0.299;
export const LUMA_G = 0.587;
export const LUMA_B = 0.114;

/** Channels are 0–255; the result is 0–255. No clamping — callers pass bytes. */
export function perceptualLuma(r: number, g: number, b: number): number {
  return LUMA_R * r + LUMA_G * g + LUMA_B * b;
}
