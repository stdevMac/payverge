/**
 * The adaptive-scrim boost formula, in one place.
 *
 * A LEAF, for the same reason `./geometry.ts` and `../artDirection/typeface.ts`
 * are leaves. Two callers need this answer and they reach it by different
 * routes: `applyScrim` in `./canvasPrimitives.ts` measures the canvas it
 * has already painted, and `buildScene` measures the source photo through the
 * analysis grid. If those two computed the boost separately, a post would
 * darken by one amount in the browser and another in the print or motion
 * walker, from the same scene.
 *
 * Nothing here may import from `./buildScene`, `./renderScene` or
 * `../templates/`: both of those callers depend on it.
 */

/** ink-900, the tint every kit's dark scrim already lays down. */
const BOOST_TINT = "28,25,23";

/** Opacity at the threshold itself — enough to matter, low enough to be a nudge. */
const BOOST_BASE = 0.55;

/** Luma units of over-brightness per unit of extra opacity. */
const BOOST_SLOPE = 400;

/** Ceiling, so the brightest photo still shows through rather than going flat. */
const BOOST_MAX = 0.82;

/**
 * The extra gradient to lay over an authored scrim, or null when none is
 * needed.
 *
 * `luminance` is PERCEPTUAL LUMA, 0–255, the quantity `perceptualLuma` in
 * `../photo/luma.ts` produces — not WCAG relative luminance. `threshold` is
 * `ScrimNode.luminanceThreshold`, in the same units. Passing the wrong one
 * silently boosts every scrim to the cap, because 0–1 luminance is always below
 * a 155 threshold.
 */
export function adaptiveScrimBoost(
  luminance: number,
  threshold: number,
): { from: string; to: string } | null {
  if (!Number.isFinite(luminance) || luminance <= threshold) return null;
  const opacity = Math.min(
    BOOST_MAX,
    BOOST_BASE + (luminance - threshold) / BOOST_SLOPE,
  );
  // Three decimals keep the string stable across IEEE-754 noise while matching
  // the precision of every other opacity this feature emits.
  const rounded = Math.round(opacity * 1000) / 1000;
  return {
    from: `rgba(${BOOST_TINT},0)`,
    to: `rgba(${BOOST_TINT},${rounded})`,
  };
}
