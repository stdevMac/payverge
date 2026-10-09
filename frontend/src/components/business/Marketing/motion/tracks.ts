import type { MotifId } from "../artDirection/kits";
import type { SceneNode, SceneNodeKind } from "../scene/types";
import { ease, type EasingId } from "./easing";

/**
 * One animatable channel. Deliberately a CLOSED union: `evaluateSceneAt`
 * switches on it exhaustively, so adding a member without adding its writer is a
 * compile error rather than a silently ignored track.
 *
 * `tracking` is deliberately absent and must stay absent. `TextNode.tracking` is
 * carried but not applied (see ../scene/renderScene.ts and
 * ../artDirection/typeface.ts): the composition solver measures text and the
 * walker paints it, and they agree to the character only because neither of them
 * touches letter-spacing. Animating it would make the painted width diverge from
 * the measured width AS A FUNCTION OF TIME — strictly worse than the static
 * divergence commit 947f18519 exists to close. `presets.test.ts` asserts no
 * shipped preset targets it.
 *
 * `sizePct` is absent for the same reason: resizing type the solver never
 * measured is the same bug wearing a different hat. Scale-like motion is
 * expressed as `rect.y` with an overshoot curve — see `badgePop` in ./presets.ts.
 */
export type AnimProperty =
  | "opacity"
  | "clip.w"
  | "clip.h"
  | "rect.y"
  | "crop.zoom"
  | "crop.x"
  | "crop.y";

/**
 * Which nodes a track drives. `kind` is required; `key` and `motif` are optional
 * narrowers. Scene nodes carry no ids — `TextNode.key` and `MotifNode.motif` are
 * the only discriminators that exist — so this is the whole selector vocabulary.
 */
interface AnimTarget {
  kind: SceneNodeKind;
  /** `TextNode.key`. Omitted matches every text node. */
  key?: string;
  /** `MotifNode.motif`. Omitted matches every motif node. */
  motif?: MotifId;
}

export interface AnimTrack {
  target: AnimTarget;
  property: AnimProperty;
  from: number;
  to: number;
  easing: EasingId;
  /** Milliseconds from the start of the clip. */
  delayMs: number;
  /** Milliseconds. Zero or negative snaps to `to` the instant `delayMs` passes. */
  durationMs: number;
}

export function trackMatches(track: AnimTrack, node: SceneNode): boolean {
  if (track.target.kind !== node.kind) return false;
  if (track.target.key !== undefined) {
    if (node.kind !== "text" || node.key !== track.target.key) return false;
  }
  if (track.target.motif !== undefined) {
    if (node.kind !== "motif" || node.motif !== track.target.motif) return false;
  }
  return true;
}

/**
 * The value this track holds at `timeMs`.
 *
 * Total by construction: every path returns either an endpoint or an
 * interpolation between two finite endpoints. A track sampled outside its window
 * holds the nearer endpoint rather than extrapolating, and a zero or negative
 * duration snaps rather than dividing — which is what keeps a malformed preset
 * from painting Infinity into a rect.
 */
export function sampleTrack(track: AnimTrack, timeMs: number): number {
  if (Number.isNaN(timeMs)) return track.from;
  const elapsed = timeMs - track.delayMs;
  if (!(elapsed > 0)) return track.from;
  if (track.durationMs <= 0 || elapsed >= track.durationMs) return track.to;
  return (
    track.from + (track.to - track.from) * ease(track.easing, elapsed / track.durationMs)
  );
}
