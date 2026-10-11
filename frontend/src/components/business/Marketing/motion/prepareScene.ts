import type { Scene, SceneNode } from "../scene/types";
import type { NormalizedRect } from "../templates/types";

/**
 * Resolve every adaptive scrim to a fixed one, once, before the frame loop runs.
 *
 * Two reasons, and the second is the one that would have been found late:
 *
 * 1. COST. `renderScene`'s scrim painter calls `sampleRegionLuminance`, which is
 *    `ctx.getImageData` — a raster readback, and by the walker's own comment the
 *    most expensive thing in the walk. Left alone, a 180-frame clip performs 180
 *    full-band readbacks of a 1080x1350 canvas.
 * 2. CORRECTNESS. The scrim boost is a function of the pixels underneath, and
 *    those pixels MOVE: a push-in changes the sampled luma every frame, so the
 *    scrim strength would visibly breathe across the clip. Freezing it is not an
 *    optimisation, it is the intended look.
 *
 * The boost formula below is `applyScrim`'s (../templates/renderPost.ts:596-608)
 * restated as a colour rather than as a second gradient pass. It has to match:
 * the poster frame is rendered by the ordinary static path with the adaptive
 * scrim live, and a mismatch would make frame 0 of the video differ from the
 * poster the Library shows.
 */

/** The luma at or below which `applyScrim` leaves the scrim as authored. */
type Luma = number;

/** Samples the 0-255 perceptual luma of a region of the t=0 frame. */
export type LuminanceSampler = (rect: NormalizedRect) => Luma;

export interface PrepareOptions {
  /**
   * False when the scene carries no photo. `renderScene` skips the readback in
   * that case (the background is a flat brand fill and the boost would be
   * noise), so this must skip it too or the two disagree about frame 0.
   */
  hasPhoto?: boolean;
}

/** Mirrors `applyScrim`'s boost ceiling and slope exactly. */
const BOOST_CEILING = 0.82;
const BOOST_BASE = 0.55;
const BOOST_DIVISOR = 400;

/** The ink the adaptive boost paints, matching `applyScrim`'s literal. */
const BOOST_RGB = "28,25,23";

function rectKey(rect: NormalizedRect): string {
  return `${rect.x},${rect.y},${rect.w},${rect.h}`;
}

export function prepareSceneForMotion(
  scene: Scene,
  sample: LuminanceSampler,
  options: PrepareOptions = {},
): Scene {
  const hasPhoto = options.hasPhoto ?? true;
  if (!scene.nodes.some((node) => node.kind === "scrim" && node.adaptive)) {
    return scene;
  }

  // Keyed by rect, like the walker's own per-render memo, so two scrims over
  // different bands each read their own band rather than inheriting the first
  // one's answer.
  const lumaByRect = new Map<string, Luma>();
  const lumaFor = (rect: NormalizedRect): Luma => {
    const key = rectKey(rect);
    const cached = lumaByRect.get(key);
    if (cached !== undefined) return cached;
    const measured = sample(rect);
    lumaByRect.set(key, measured);
    return measured;
  };

  const nodes: SceneNode[] = scene.nodes.map((node) => {
    if (node.kind !== "scrim" || !node.adaptive) return node;
    if (!hasPhoto) return { ...node, adaptive: false };
    const luma = lumaFor(node.rect);
    if (!(luma > node.luminanceThreshold)) return { ...node, adaptive: false };
    const opacity = Math.min(
      BOOST_CEILING,
      BOOST_BASE + (luma - node.luminanceThreshold) / BOOST_DIVISOR,
    );
    return { ...node, adaptive: false, to: `rgba(${BOOST_RGB},${opacity})` };
  });

  return { ...scene, nodes };
}
