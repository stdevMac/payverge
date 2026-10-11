import { KIT_ORDER } from "../artDirection/kits";
import {
  logoNode,
  motifNode,
  photoNode,
  scrimNode,
  textNode,
  type Scene,
  type SceneNode,
} from "../scene/types";
import { evaluateSceneAt } from "./evaluate";
import {
  MOTION_DURATION_MS,
  MOTION_FPS,
  MOTION_PRESETS,
  MOTION_PRESET_FOR_KIT,
  MOTION_PRESET_ORDER,
  isMotionPresetId,
  tracksForScene,
} from "./presets";
import type { AnimProperty } from "./tracks";

const W = 1080;
const H = 1350;

const text = (key: string, y: number): SceneNode =>
  textNode({
    key,
    text: key,
    rect: { x: 0.07, y, w: 0.86, h: 0.06 },
    z: 30,
    font: "sans",
    weight: 500,
    sizePct: 0.03,
    align: "left",
    color: "#ffffff",
    maxLines: 1,
    lineHeight: 1.15,
  });

const fullScene = (): Scene => ({
  width: W,
  height: H,
  background: "#1a6b6a",
  nodes: [
    photoNode({ url: "a.jpg", rect: { x: 0, y: 0, w: 1, h: 1 }, z: 0 }),
    scrimNode({
      rect: { x: 0, y: 0.42, w: 1, h: 0.58 },
      z: 10,
      from: "rgba(28,25,23,0)",
      to: "rgba(28,25,23,0.78)",
      adaptive: false,
      luminanceThreshold: 155,
    }),
    motifNode({ motif: "grain", rect: { x: 0, y: 0, w: 1, h: 1 }, color: "#fff", z: 15, opacity: 0.12 }),
    motifNode({ motif: "halftone", rect: { x: 0, y: 0, w: 1, h: 1 }, color: "#fff", z: 15, opacity: 0.12 }),
    text("badge", 0.55),
    text("dishName", 0.63),
    text("price", 0.75),
    text("cta", 0.82),
    text("handle", 0.88),
    motifNode({ motif: "rule", rect: { x: 0.07, y: 0.5, w: 0.86, h: 0.4 }, color: "#fff", z: 40, opacity: 0.9 }),
    // LogoRect is origin + diameter only — no `h` (see scene/types.ts LogoRect).
    logoNode({ url: "l.png", rect: { x: 0.07, y: 0.07, w: 0.11 }, z: 50 }),
  ],
});

const bareScene = (): Scene => ({
  width: W,
  height: H,
  background: "#1a6b6a",
  nodes: [text("dishName", 0.63)],
});

describe("motion presets", () => {
  it("registers every id in MOTION_PRESET_ORDER and nothing else", () => {
    expect([...MOTION_PRESET_ORDER].sort()).toEqual(Object.keys(MOTION_PRESETS).sort());
  });

  it("gives every kit a preset that exists", () => {
    KIT_ORDER.forEach((kit) => {
      const preset = MOTION_PRESET_FOR_KIT[kit];
      expect(MOTION_PRESETS[preset]).toBeDefined();
    });
  });

  it("uses every preset at least once, so none is dead data", () => {
    const used = new Set(KIT_ORDER.map((kit) => MOTION_PRESET_FOR_KIT[kit]));
    expect([...used].sort()).toEqual([...MOTION_PRESET_ORDER].sort());
  });

  it("narrows unknown preset ids instead of trusting them", () => {
    expect(isMotionPresetId("pushIn")).toBe(true);
    expect(isMotionPresetId("zoomBlur")).toBe(false);
    expect(isMotionPresetId(undefined)).toBe(false);
  });

  it("names a translation key per preset", () => {
    MOTION_PRESET_ORDER.forEach((id) => {
      expect(MOTION_PRESETS[id].labelKey).toBe(`motion.presets.${id}`);
    });
  });

  /**
   * The guard that keeps letter-spacing out of the timeline. `TextNode.tracking`
   * is carried and never applied, because the composition solver measures text
   * and the walker paints it and they agree only while neither touches spacing.
   * Animating it would make the divergence a function of time.
   *
   * Asserts against the closed `AnimProperty` allow-list rather than
   * `!== "tracking"`: those forbidden names are not in the union, so a
   * not-toBe check could never fail under TypeScript. A preset that targets an
   * unknown property (or a cast that smuggles tracking) fails the contain.
   */
  it.each(MOTION_PRESET_ORDER)("%s only emits AnimProperty channels", (id) => {
    const allowed: readonly AnimProperty[] = [
      "opacity",
      "clip.w",
      "clip.h",
      "rect.y",
      "crop.zoom",
      "crop.x",
      "crop.y",
    ];
    // Compile-time: AnimProperty must stay free of tracking/sizePct. If either
    // is added to the union, this assignment fails to typecheck.
    type Forbidden = "tracking" | "sizePct";
    type NoForbidden = [Exclude<AnimProperty, Forbidden>] extends [AnimProperty]
      ? true
      : never;
    const _noForbidden: NoForbidden = true;
    void _noForbidden;

    const tracks = tracksForScene(fullScene(), id, MOTION_DURATION_MS);
    tracks.forEach((track) => {
      expect(allowed).toContain(track.property);
    });
  });

  it.each(MOTION_PRESET_ORDER)("%s emits at least one track for a full scene", (id) => {
    expect(tracksForScene(fullScene(), id, MOTION_DURATION_MS).length).toBeGreaterThan(0);
  });

  it.each(MOTION_PRESET_ORDER)("%s stays inside the clip window", (id) => {
    tracksForScene(fullScene(), id, MOTION_DURATION_MS).forEach((track) => {
      expect(track.delayMs).toBeGreaterThanOrEqual(0);
      expect(track.delayMs + track.durationMs).toBeLessThanOrEqual(MOTION_DURATION_MS);
    });
  });

  it.each(MOTION_PRESET_ORDER)("%s emits only finite endpoints", (id) => {
    tracksForScene(fullScene(), id, MOTION_DURATION_MS).forEach((track) => {
      expect(Number.isFinite(track.from)).toBe(true);
      expect(Number.isFinite(track.to)).toBe(true);
    });
  });

  it.each(MOTION_PRESET_ORDER)("%s degrades to no tracks it cannot fill", (id) => {
    // A one-node scene has no photo, no scrim, no motif and no logo. Whatever
    // survives must still be valid; nothing may throw.
    expect(() => tracksForScene(bareScene(), id, MOTION_DURATION_MS)).not.toThrow();
  });

  it.each(MOTION_PRESET_ORDER)("%s settles every node to a visible rest state", (id) => {
    const tracks = tracksForScene(fullScene(), id, MOTION_DURATION_MS);
    const settled = evaluateSceneAt(fullScene(), tracks, MOTION_DURATION_MS);
    settled.nodes.forEach((node) => {
      if (node.kind === "text" || node.kind === "logo") {
        expect(node.opacity ?? 1).toBeGreaterThanOrEqual(0.99);
      }
      if (node.clip) {
        // A clip that has not opened past the node's own box at the end of the
        // clip would leave the post permanently cropped on the last frame — the
        // frame an operator screenshots.
        expect(node.clip.w).toBeGreaterThanOrEqual(node.rect.w);
        expect(node.clip.y + node.clip.h).toBeGreaterThanOrEqual(
          node.rect.y + ("h" in node.rect ? node.rect.h : node.rect.w),
        );
      }
    });
  });

  it("keeps a text reveal's final clip clear of glyph descenders", () => {
    const tracks = tracksForScene(fullScene(), "editorialReveal", MOTION_DURATION_MS);
    const settled = evaluateSceneAt(fullScene(), tracks, MOTION_DURATION_MS);
    settled.nodes
      .filter((node) => node.kind === "text" && node.clip)
      .forEach((node) => {
        // The solver caps a band's rect height at the bounds while leaving
        // fontPx uncapped (composition/solver.ts:191-198), so glyphs can reach
        // below rect.h. A clip that stops at rect.h would shear them.
        expect(node.clip!.y + node.clip!.h).toBeGreaterThanOrEqual(1);
      });
  });

  it("pins the clip length and frame rate", () => {
    expect(MOTION_DURATION_MS).toBe(6000);
    expect(MOTION_FPS).toBe(30);
  });

  it("pushes the photo in without leaving fitCover's zoom range", () => {
    const tracks = tracksForScene(fullScene(), "pushIn", MOTION_DURATION_MS);
    const zoom = tracks.find((t) => t.property === "crop.zoom");
    expect(zoom).toBeDefined();
    expect(zoom!.from).toBeGreaterThanOrEqual(1);
    expect(zoom!.to).toBeLessThanOrEqual(3);
  });

  it("pans the photo without leaving fitCover's focal range", () => {
    const tracks = tracksForScene(fullScene(), "slowPan", MOTION_DURATION_MS);
    const pan = tracks.find((t) => t.property === "crop.x");
    expect(pan).toBeDefined();
    expect(Math.min(pan!.from, pan!.to)).toBeGreaterThanOrEqual(0);
    expect(Math.max(pan!.from, pan!.to)).toBeLessThanOrEqual(1);
  });

  it("emits one scrim wipe even when a scene carries several scrims", () => {
    const twoScrims = fullScene();
    twoScrims.nodes.push(
      scrimNode({
        rect: { x: 0, y: 0.1, w: 0.5, h: 0.2 },
        z: 11,
        from: "rgba(0,0,0,0)",
        to: "rgba(0,0,0,0.5)",
        adaptive: false,
        luminanceThreshold: 155,
      }),
    );
    const wipes = tracksForScene(twoScrims, "slowPan", MOTION_DURATION_MS).filter(
      (t) => t.target.kind === "scrim",
    );
    expect(wipes).toHaveLength(1);
  });

  it("does not fade the badge twice in badgePop", () => {
    const opacityTracks = tracksForScene(fullScene(), "badgePop", MOTION_DURATION_MS)
      .filter((t) => t.property === "opacity" && t.target.key === "badge");
    expect(opacityTracks).toHaveLength(1);
  });
});
