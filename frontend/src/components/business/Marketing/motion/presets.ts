import type { KitId, MotifId } from "../artDirection/kits";
import type { Scene } from "../scene/types";
import type { AnimTrack } from "./tracks";

/**
 * The per-kit motion vocabulary.
 *
 * A preset is a RECIPE, not a fixed track list: it reads the built scene and
 * emits tracks for what it finds. That matters because `buildScene` filters a
 * kit's motifs through the composition's own list (buildScene.ts:434-436), so
 * the same kit produces different node sets on different layouts. A recipe that
 * finds nothing emits nothing, and the clip is a still — a degradation, not a
 * crash.
 *
 * Everything here is pure data plus arithmetic. No canvas, no codec, no time.
 */

export const MOTION_DURATION_MS = 6000;
export const MOTION_FPS = 30;

export type MotionPresetId =
  | "pushIn"
  | "slowPan"
  | "editorialReveal"
  | "badgePop"
  | "grainDrift"
  | "ticketSlide";

export interface MotionPreset {
  id: MotionPresetId;
  /** Operator-facing name; see motion.presets.* in marketingDashboard.json. */
  labelKey: string;
  build: (scene: Scene, durationMs: number) => AnimTrack[];
}

/**
 * Push-in amplitude.
 *
 * KNOWN LIMITATION, and the reason this is 1.08 rather than a subtler 1.03:
 * `fitCover` (../templates/renderPost.ts:157-162) rounds sx/sy/sw/sh to whole
 * source pixels. A push-in therefore advances the source rect in integer steps,
 * and on a small source a small amplitude produces a visible stair-step rather
 * than a glide. Raising the amplitude raises the per-frame delta above the
 * quantum for realistic sources. It does not eliminate the quantization — the
 * real fix is to let `fitCover` return floats, which `drawImage` accepts — but
 * that function is shared with the static export path and is not this wave's to
 * change.
 */
const PUSH_IN_TO = 1.08;

/** Pan travel, centred on the default focal point so neither end clips out. */
const PAN_FROM = 0.42;
const PAN_TO = 0.58;

const TEXT_REVEAL_MS = 520;
const TEXT_STAGGER_MS = 130;
const TEXT_REVEAL_START_MS = 250;

const LOGO_SETTLE_MS = 680;
const LOGO_SETTLE_START_MS = 900;
/** Rise distance for the logo settle, as a fraction of canvas HEIGHT. */
const LOGO_RISE = 0.018;

const BADGE_POP_MS = 560;
const BADGE_POP_START_MS = 200;
/** Rise distance for the badge pop, as a fraction of canvas HEIGHT. */
const BADGE_RISE = 0.03;

const SCRIM_WIPE_MS = 900;

/** Texture drift endpoints, straddling `TEXTURE_OPACITY` (0.12) in buildScene. */
const DRIFT_FROM = 0.06;
const DRIFT_TO = 0.18;

/**
 * A text reveal's final clip must clear the glyphs, not the rect.
 *
 * `solveStack` caps a band's rect height at the bounds while leaving `fontPx`
 * uncapped and says so (composition/solver.ts:191-198): "Capping at the bounds
 * makes the rect honest about the space the band is allowed to occupy and
 * dishonest about how far its glyphs actually reach." A clip that settles at
 * `rect.h` would therefore shear descenders on exactly the bands the solver
 * worked hardest on. Opening the clip to the bottom of the canvas costs nothing
 * — at rest, the node is unclipped in every direction that matters.
 */
function revealHeightFor(rect: { y: number; h: number }): number {
  return Math.max(rect.h, 1 - rect.y);
}

function photoPushIn(durationMs: number): AnimTrack[] {
  return [
    {
      target: { kind: "photo" },
      property: "crop.zoom",
      from: 1,
      to: PUSH_IN_TO,
      easing: "easeInOutCubic",
      delayMs: 0,
      durationMs,
    },
  ];
}

function photoPan(durationMs: number): AnimTrack[] {
  return [
    {
      target: { kind: "photo" },
      property: "crop.x",
      from: PAN_FROM,
      to: PAN_TO,
      easing: "easeInOutCubic",
      delayMs: 0,
      durationMs,
    },
  ];
}

/**
 * A staggered top-down wipe plus a fade, per text band, in scene order.
 *
 * `skipKeys` exists so a preset that animates one slot specially — `badgePop`
 * and its badge — does not also emit a second opacity track for it. Two tracks
 * on one property of one node is last-writer-wins in `evaluateSceneAt`, which
 * would silently discard the special treatment.
 */
function textReveal(
  scene: Scene,
  startMs: number,
  skipKeys: readonly string[] = [],
): AnimTrack[] {
  const tracks: AnimTrack[] = [];
  scene.nodes.forEach((node) => {
    if (node.kind !== "text") return;
    if (skipKeys.includes(node.key)) return;
    const delayMs = startMs + tracks.length * (TEXT_STAGGER_MS / 2);
    tracks.push(
      {
        target: { kind: "text", key: node.key },
        property: "clip.h",
        from: 0,
        to: revealHeightFor(node.rect),
        easing: "easeOutQuint",
        delayMs,
        durationMs: TEXT_REVEAL_MS,
      },
      {
        target: { kind: "text", key: node.key },
        property: "opacity",
        from: 0,
        to: 1,
        easing: "easeOutCubic",
        delayMs,
        durationMs: TEXT_REVEAL_MS,
      },
    );
  });
  return tracks;
}

/**
 * One wipe, from the FIRST scrim, however many the scene carries.
 *
 * A track's target is `{ kind: "scrim" }` with no narrower — scrim nodes have no
 * discriminator to select on — so a second track would match the same nodes and
 * simply overwrite the first. Emitting one is honest about that rather than
 * emitting several that fight.
 */
function scrimWipe(scene: Scene): AnimTrack[] {
  const scrim = scene.nodes.find((node) => node.kind === "scrim");
  if (!scrim) return [];
  return [
    {
      target: { kind: "scrim" },
      property: "clip.w",
      from: 0,
      to: scrim.rect.w,
      easing: "easeInOutCubic",
      delayMs: 0,
      durationMs: SCRIM_WIPE_MS,
    },
  ];
}

function logoSettle(scene: Scene): AnimTrack[] {
  const logo = scene.nodes.find((node) => node.kind === "logo");
  if (!logo) return [];
  return [
    {
      target: { kind: "logo" },
      property: "rect.y",
      from: logo.rect.y - LOGO_RISE,
      to: logo.rect.y,
      easing: "easeOutBack",
      delayMs: LOGO_SETTLE_START_MS,
      durationMs: LOGO_SETTLE_MS,
    },
    {
      target: { kind: "logo" },
      property: "opacity",
      from: 0,
      to: 1,
      easing: "easeOutCubic",
      delayMs: LOGO_SETTLE_START_MS,
      durationMs: LOGO_SETTLE_MS,
    },
  ];
}

/**
 * The badge "pop", built from a rise with an overshoot curve rather than a scale.
 *
 * There is no scale channel and there deliberately is not going to be one.
 * Animating `sizePct` would resize type the composition solver never measured —
 * the same measure/paint divergence class that `../artDirection/typeface.ts`
 * exists to close — and a canvas transform would mean a new capability on the
 * shared walker for one preset. `easeOutBack` on `rect.y` supplies the overshoot
 * that makes the element read as popping into place.
 */
function badgePop(scene: Scene): AnimTrack[] {
  const badge = scene.nodes.find((node) => node.kind === "text" && node.key === "badge");
  if (!badge) return [];
  return [
    {
      target: { kind: "text", key: "badge" },
      property: "rect.y",
      from: badge.rect.y + BADGE_RISE,
      to: badge.rect.y,
      easing: "easeOutBack",
      delayMs: BADGE_POP_START_MS,
      durationMs: BADGE_POP_MS,
    },
    {
      target: { kind: "text", key: "badge" },
      property: "opacity",
      from: 0,
      to: 1,
      easing: "easeOutCubic",
      delayMs: BADGE_POP_START_MS,
      durationMs: BADGE_POP_MS,
    },
  ];
}

/** A slow alpha drift on a full-bleed texture, if the scene actually carries it. */
function textureDrift(scene: Scene, motif: MotifId, durationMs: number): AnimTrack[] {
  const present = scene.nodes.some(
    (node) => node.kind === "motif" && node.motif === motif,
  );
  if (!present) return [];
  return [
    {
      target: { kind: "motif", motif },
      property: "opacity",
      from: DRIFT_FROM,
      to: DRIFT_TO,
      easing: "linear",
      delayMs: 0,
      durationMs,
    },
  ];
}

export const MOTION_PRESETS: Record<MotionPresetId, MotionPreset> = {
  pushIn: {
    id: "pushIn",
    labelKey: "motion.presets.pushIn",
    build: (scene, durationMs) => [
      ...photoPushIn(durationMs),
      ...textReveal(scene, TEXT_REVEAL_START_MS),
      ...logoSettle(scene),
    ],
  },
  slowPan: {
    id: "slowPan",
    labelKey: "motion.presets.slowPan",
    build: (scene, durationMs) => [
      ...photoPan(durationMs),
      ...scrimWipe(scene),
      ...textReveal(scene, TEXT_REVEAL_START_MS),
      ...logoSettle(scene),
    ],
  },
  editorialReveal: {
    id: "editorialReveal",
    labelKey: "motion.presets.editorialReveal",
    build: (scene, durationMs) => [
      ...photoPushIn(durationMs),
      ...textReveal(scene, TEXT_REVEAL_START_MS),
      ...logoSettle(scene),
    ],
  },
  badgePop: {
    id: "badgePop",
    labelKey: "motion.presets.badgePop",
    build: (scene, durationMs) => [
      ...photoPushIn(durationMs),
      ...badgePop(scene),
      ...textReveal(scene, BADGE_POP_START_MS + BADGE_POP_MS, ["badge"]),
      ...logoSettle(scene),
    ],
  },
  grainDrift: {
    id: "grainDrift",
    labelKey: "motion.presets.grainDrift",
    build: (scene, durationMs) => [
      ...photoPushIn(durationMs),
      ...textureDrift(scene, "grain", durationMs),
      ...textReveal(scene, TEXT_REVEAL_START_MS),
      ...logoSettle(scene),
    ],
  },
  ticketSlide: {
    id: "ticketSlide",
    labelKey: "motion.presets.ticketSlide",
    build: (scene, durationMs) => [
      ...photoPushIn(durationMs),
      ...textureDrift(scene, "halftone", durationMs),
      ...scrimWipe(scene),
      ...textReveal(scene, TEXT_REVEAL_START_MS),
      ...logoSettle(scene),
    ],
  },
};

export const MOTION_PRESET_ORDER: readonly MotionPresetId[] = [
  "pushIn",
  "slowPan",
  "editorialReveal",
  "badgePop",
  "grainDrift",
  "ticketSlide",
];

/**
 * The default preset for each art-direction kit. Every preset is used at least
 * once (pinned by presets.test.ts), so none of them is dead data an operator can
 * never reach through the default path.
 */
export const MOTION_PRESET_FOR_KIT: Record<KitId, MotionPresetId> = {
  editorial: "editorialReveal",
  bold: "badgePop",
  minimal: "pushIn",
  chalkboard: "grainDrift",
  linen: "slowPan",
  ticket: "ticketSlide",
};

/**
 * Narrow a value of unknown provenance — a `creative_snapshot.motion_preset` off
 * the wire — to an id that is guaranteed to have a definition. The backend only
 * length- and charset-bounds this field (see `normalizeMarketingSnapshotIdent`),
 * exactly as it does composition, so a build older or newer than the server can
 * legitimately be handed an id it has never seen. Use this, never a cast.
 */
export function isMotionPresetId(value: unknown): value is MotionPresetId {
  return (
    typeof value === "string" && (MOTION_PRESET_ORDER as string[]).includes(value)
  );
}

/** The tracks a preset produces for a specific built scene. */
export function tracksForScene(
  scene: Scene,
  preset: MotionPresetId,
  durationMs: number,
): AnimTrack[] {
  return MOTION_PRESETS[preset].build(scene, durationMs);
}
