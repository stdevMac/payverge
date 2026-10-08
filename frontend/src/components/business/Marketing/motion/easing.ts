/**
 * Easing curves for the motion timeline.
 *
 * A LEAF module, for the same reason `../artDirection/typeface.ts` is one:
 * nothing here may import from `./tracks`, `./evaluate` or `./presets`. The
 * timeline samples through this and the in-app preview loop samples through the
 * same functions, so a curve change reaches the exported video and the preview
 * in one commit or reaches neither.
 *
 * Every curve maps 0 -> 0 and 1 -> 1. Only `easeOutBack` leaves the [0, 1] range
 * in between, and that overshoot is the point: it is what makes a badge pop read
 * as a pop rather than as a fade. Nothing here clamps the OUTPUT.
 */

export type EasingId =
  | "linear"
  | "easeOutCubic"
  | "easeInOutCubic"
  | "easeOutQuint"
  | "easeOutBack";

export type EasingFn = (progress: number) => number;

/**
 * The classic Penner overshoot constant. ~10% past the target, which reads as
 * deliberate at 30fps; larger values read as a bounce and smaller ones vanish.
 */
const BACK_OVERSHOOT = 1.70158;

export const EASINGS: Record<EasingId, EasingFn> = {
  linear: (t) => t,
  easeOutCubic: (t) => 1 - (1 - t) ** 3,
  easeInOutCubic: (t) => (t < 0.5 ? 4 * t ** 3 : 1 - (-2 * t + 2) ** 3 / 2),
  easeOutQuint: (t) => 1 - (1 - t) ** 5,
  easeOutBack: (t) =>
    1 + (BACK_OVERSHOOT + 1) * (t - 1) ** 3 + BACK_OVERSHOOT * (t - 1) ** 2,
};

export const EASING_ORDER: readonly EasingId[] = [
  "linear",
  "easeOutCubic",
  "easeInOutCubic",
  "easeOutQuint",
  "easeOutBack",
];

/**
 * Narrow a value of unknown provenance — an easing id off a stored preset — to
 * one that is guaranteed to have a function in `EASINGS`. Use this instead of a
 * cast: the cast turns a typo into `EASINGS[id](t)` on `undefined` and a thrown
 * frame loop; the guard turns it into a fall back to `linear`.
 */
export function isEasingId(value: unknown): value is EasingId {
  return typeof value === "string" && (EASING_ORDER as string[]).includes(value);
}

/**
 * Sample a curve at `progress`, clamping the INPUT to [0, 1].
 *
 * The input clamp is what makes the frame loop total: a track sampled before its
 * delay or after its duration must hold its endpoint rather than extrapolate a
 * cubic off to infinity. A non-finite progress resolves to an endpoint too —
 * `NaN` becomes 0 — because a single bad number must not paint a NaN rect.
 */
export function ease(id: EasingId, progress: number): number {
  if (Number.isNaN(progress)) return EASINGS[id](0);
  const clamped = Math.min(1, Math.max(0, progress));
  return EASINGS[id](clamped);
}
