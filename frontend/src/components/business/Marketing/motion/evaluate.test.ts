import {
  logoNode,
  motifNode,
  photoNode,
  scrimNode,
  textNode,
  type Scene,
} from "../scene/types";
import { evaluateSceneAt, MOTION_DEFAULT_CROP } from "./evaluate";
import type { AnimTrack } from "./tracks";

const W = 1080;
const H = 1350;
const RECT = { x: 0.1, y: 0.6, w: 0.8, h: 0.1 };

const DISH = textNode({
  key: "dishName",
  text: "Milanesa",
  rect: RECT,
  z: 30,
  font: "sans",
  weight: 700,
  sizePct: 0.07,
  align: "left",
  color: "#ffffff",
  maxLines: 2,
  lineHeight: 1.2,
});
const PHOTO = photoNode({ url: "a.jpg", rect: { x: 0, y: 0, w: 1, h: 1 }, z: 0 });
const SCRIM = scrimNode({
  rect: { x: 0, y: 0.5, w: 1, h: 0.5 },
  z: 10,
  from: "rgba(0,0,0,0)",
  to: "rgba(0,0,0,0.8)",
  adaptive: false,
  luminanceThreshold: 155,
});
// LogoRect is origin + diameter only — no `h` (see scene/types.ts LogoRect).
const LOGO = logoNode({ url: "l.png", rect: { x: 0.07, y: 0.07, w: 0.11 }, z: 50 });
const GRAIN = motifNode({ motif: "grain", rect: RECT, color: "#ffffff", z: 15, opacity: 0.12 });

const scene = (over: Partial<Scene> = {}): Scene => ({
  width: W,
  height: H,
  background: "#1a6b6a",
  nodes: [PHOTO, SCRIM, DISH, GRAIN, LOGO],
  ...over,
});

const fade: AnimTrack = {
  target: { kind: "text", key: "dishName" },
  property: "opacity",
  from: 0,
  to: 1,
  easing: "linear",
  delayMs: 0,
  durationMs: 1000,
};

describe("evaluateSceneAt", () => {
  it("returns the same scene object when there are no tracks", () => {
    const base = scene();
    expect(evaluateSceneAt(base, [], 250)).toBe(base);
  });

  it("returns the same scene object when no track matches any node", () => {
    const base = scene();
    const orphan: AnimTrack = { ...fade, target: { kind: "text", key: "price" } };
    expect(evaluateSceneAt(base, [orphan], 250)).toBe(base);
  });

  it("never mutates the input scene", () => {
    const base = scene();
    const before = JSON.stringify(base);
    evaluateSceneAt(base, [fade], 500);
    expect(JSON.stringify(base)).toBe(before);
  });

  it("writes opacity onto the matching node only", () => {
    const out = evaluateSceneAt(scene(), [fade], 500);
    const dish = out.nodes.find((n) => n.kind === "text");
    expect(dish?.opacity).toBeCloseTo(0.5, 10);
    expect(out.nodes.find((n) => n.kind === "photo")?.opacity).toBeUndefined();
  });

  it("preserves node order, so the z ladder never needs re-sorting", () => {
    const out = evaluateSceneAt(scene(), [fade], 500);
    expect(out.nodes.map((n) => n.kind)).toEqual([
      "photo",
      "scrim",
      "text",
      "motif",
      "logo",
    ]);
    expect(out.nodes.map((n) => n.z)).toEqual([0, 10, 30, 15, 50]);
  });

  it("preserves width, height and background", () => {
    const out = evaluateSceneAt(scene(), [fade], 500);
    expect(out.width).toBe(W);
    expect(out.height).toBe(H);
    expect(out.background).toBe("#1a6b6a");
  });

  it("derives a clip rect from the node rect when the node has none", () => {
    const wipe: AnimTrack = {
      target: { kind: "text", key: "dishName" },
      property: "clip.h",
      from: 0,
      to: RECT.h,
      easing: "linear",
      delayMs: 0,
      durationMs: 1000,
    };
    const out = evaluateSceneAt(scene(), [wipe], 500);
    expect(out.nodes.find((n) => n.kind === "text")?.clip).toEqual({
      x: RECT.x,
      y: RECT.y,
      w: RECT.w,
      h: RECT.h / 2,
    });
  });

  it("keeps an authored clip's other three edges when animating one", () => {
    const authored = { ...DISH, clip: { x: 0.2, y: 0.7, w: 0.5, h: 0.05 } };
    const wipe: AnimTrack = {
      target: { kind: "text", key: "dishName" },
      property: "clip.w",
      from: 0,
      to: 0.5,
      easing: "linear",
      delayMs: 0,
      durationMs: 1000,
    };
    const out = evaluateSceneAt(scene({ nodes: [authored] }), [wipe], 1000);
    expect(out.nodes[0].clip).toEqual({ x: 0.2, y: 0.7, w: 0.5, h: 0.05 });
  });

  it("seeds a photo crop from the shared default when the node has none", () => {
    const push: AnimTrack = {
      target: { kind: "photo" },
      property: "crop.zoom",
      from: 1,
      to: 1.08,
      easing: "linear",
      delayMs: 0,
      durationMs: 1000,
    };
    const out = evaluateSceneAt(scene(), [push], 1000);
    const photo = out.nodes.find((n) => n.kind === "photo");
    expect(photo?.kind === "photo" && photo.crop).toEqual({
      x: MOTION_DEFAULT_CROP.x,
      y: MOTION_DEFAULT_CROP.y,
      zoom: 1.08,
    });
  });

  it("preserves an operator crop's focal point while animating zoom", () => {
    const cropped = photoNode({
      url: "a.jpg",
      rect: { x: 0, y: 0, w: 1, h: 1 },
      z: 0,
      crop: { x: 0.32, y: 0.71, zoom: 1.4 },
    });
    const push: AnimTrack = {
      target: { kind: "photo" },
      property: "crop.zoom",
      from: 1.4,
      to: 1.5,
      easing: "linear",
      delayMs: 0,
      durationMs: 1000,
    };
    const out = evaluateSceneAt(scene({ nodes: [cropped] }), [push], 1000);
    const photo = out.nodes[0];
    expect(photo.kind === "photo" && photo.crop).toEqual({
      x: 0.32,
      y: 0.71,
      zoom: 1.5,
    });
  });

  it("ignores a crop track aimed at a node that has no crop", () => {
    const push: AnimTrack = {
      target: { kind: "logo" },
      property: "crop.zoom",
      from: 1,
      to: 1.08,
      easing: "linear",
      delayMs: 0,
      durationMs: 1000,
    };
    const out = evaluateSceneAt(scene({ nodes: [LOGO] }), [push], 1000);
    expect(out.nodes[0]).toEqual(LOGO);
  });

  it("applies several tracks to one node without either losing the other", () => {
    const rise: AnimTrack = {
      target: { kind: "text", key: "dishName" },
      property: "rect.y",
      from: 0.7,
      to: 0.6,
      easing: "linear",
      delayMs: 0,
      durationMs: 1000,
    };
    const out = evaluateSceneAt(scene(), [fade, rise], 500);
    const dish = out.nodes.find((n) => n.kind === "text");
    expect(dish?.opacity).toBeCloseTo(0.5, 10);
    expect(dish?.rect.y).toBeCloseTo(0.65, 10);
  });

  it("overrides an authored opacity rather than compounding with it", () => {
    const drift: AnimTrack = {
      target: { kind: "motif", motif: "grain" },
      property: "opacity",
      from: 0.06,
      to: 0.18,
      easing: "linear",
      delayMs: 0,
      durationMs: 1000,
    };
    const out = evaluateSceneAt(scene(), [drift], 1000);
    expect(out.nodes.find((n) => n.kind === "motif")?.opacity).toBeCloseTo(0.18, 10);
  });
});
