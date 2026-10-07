import type { MarketingCrop } from "@/api/marketing";
import type { Scene, SceneNode } from "../scene/types";
import type { NormalizedRect } from "../templates/types";
import { sampleTrack, trackMatches, type AnimProperty, type AnimTrack } from "./tracks";

/**
 * Evaluate a built scene at a point in time.
 *
 * This is the whole of Wave 3's "motion walker", and it is deliberately not a
 * walker over pixels: it takes a `Scene` and returns a `Scene`, and the SAME
 * `renderScene` paints the result. There is no second text painter, no second
 * geometry resolver and no second scrim rule — which is the invariant
 * `../scene/renderScene.ts` exists to protect.
 *
 * Purity is not decoration here. jsdom has no `VideoEncoder`, no
 * `OffscreenCanvas` and no `captureStream`, so this function is the only part of
 * the motion pipeline that can be tested exhaustively. Everything effectful is
 * pushed downstream of it and injected.
 *
 * The scene is assumed ALREADY BUILT. `buildScene` runs once per export, never
 * per frame: it calls `solveStack`, which re-measures and re-wraps every band,
 * so running it per frame would cost 180x the solver AND re-break lines mid-clip
 * as the type shifts. Nothing this function writes feeds measurement.
 */

/**
 * Mirror of `DEFAULT_CROP` in `../templates/renderPost.ts`. Restated rather than
 * imported so this module stays free of the renderer's image cache and error
 * logger; `cropDefault.parity.test.ts` pins the two together.
 */
export const MOTION_DEFAULT_CROP: MarketingCrop = { x: 0.5, y: 0.5, zoom: 1 };

/**
 * A clip base for nodes that do not yet carry one. Logos have a diameter-only
 * `LogoRect` (no `h`); every other node has a full `NormalizedRect`. A wipe
 * against a logo is not a shipped channel, but the type union still reaches
 * this path, so we invent an `h` equal to the diameter rather than crash.
 */
function clipBase(node: SceneNode): NormalizedRect {
  if (node.clip) return node.clip;
  if (node.kind === "logo") {
    return { x: node.rect.x, y: node.rect.y, w: node.rect.w, h: node.rect.w };
  }
  return node.rect;
}

/**
 * Write one animated value onto a copy of a node.
 *
 * The `switch` is exhaustive over `AnimProperty` with no `default:`, on purpose.
 * `AnimProperty` is a closed union and this function is annotated as returning
 * `SceneNode`, so adding a member without adding its case here is a compile
 * error — where a `default:` would have silently dropped the new channel and
 * shipped a preset that animates nothing.
 */
function withProperty(
  node: SceneNode,
  property: AnimProperty,
  value: number,
): SceneNode {
  switch (property) {
    case "opacity":
      return { ...node, opacity: value };
    case "rect.y":
      // Narrow by kind so the union of `LogoRect | NormalizedRect` does not
      // collapse into a shape missing `h` when reassigned onto a non-logo node.
      if (node.kind === "logo") {
        return { ...node, rect: { ...node.rect, y: value } };
      }
      return { ...node, rect: { ...node.rect, y: value } };
    case "clip.w": {
      // An unset clip starts life as the node's own rect, so a wipe reveals the
      // node rather than revealing an arbitrary corner of the canvas.
      const base = clipBase(node);
      return { ...node, clip: { ...base, w: value } };
    }
    case "clip.h": {
      const base = clipBase(node);
      return { ...node, clip: { ...base, h: value } };
    }
    case "crop.zoom":
    case "crop.x":
    case "crop.y": {
      // Only a photo has a crop. A crop track aimed anywhere else is dropped
      // rather than throwing: a preset is data, and one bad entry must not take
      // the whole export down.
      if (node.kind !== "photo") return node;
      const crop = node.crop ?? MOTION_DEFAULT_CROP;
      if (property === "crop.zoom") return { ...node, crop: { ...crop, zoom: value } };
      if (property === "crop.x") return { ...node, crop: { ...crop, x: value } };
      return { ...node, crop: { ...crop, y: value } };
    }
  }
}

/**
 * The scene as it stands at `timeMs`.
 *
 * Returns the INPUT object unchanged when nothing matched, so the preview loop
 * can skip a repaint on identity, and clones only the nodes a track touched.
 * Node order is preserved and never re-sorted: the input is already sorted by z
 * (`buildScene` guarantees it via `sortNodes`), and no animatable property can
 * change a z value.
 */
export function evaluateSceneAt(
  scene: Scene,
  tracks: readonly AnimTrack[],
  timeMs: number,
): Scene {
  if (!tracks.length) return scene;
  let changed = false;
  const nodes = scene.nodes.map((node) => {
    let next = node;
    tracks.forEach((track) => {
      if (!trackMatches(track, node)) return;
      next = withProperty(next, track.property, sampleTrack(track, timeMs));
    });
    if (next !== node) changed = true;
    return next;
  });
  return changed ? { ...scene, nodes } : scene;
}
