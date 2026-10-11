import { photoNode, scrimNode, textNode, type Scene } from "../scene/types";
import { prepareSceneForMotion } from "./prepareScene";

const W = 1080;
const H = 1350;

const SCRIM_RECT = { x: 0, y: 0.42, w: 1, h: 0.58 };

const adaptive = scrimNode({
  rect: SCRIM_RECT,
  z: 10,
  from: "rgba(28,25,23,0)",
  to: "rgba(28,25,23,0.78)",
  adaptive: true,
  luminanceThreshold: 155,
});

const fixed = scrimNode({
  rect: SCRIM_RECT,
  z: 11,
  from: "rgba(250,249,246,0)",
  to: "rgba(250,249,246,0.94)",
  adaptive: false,
  luminanceThreshold: 155,
});

const DISH = textNode({
  key: "dishName",
  text: "Milanesa",
  rect: { x: 0.07, y: 0.6, w: 0.86, h: 0.08 },
  z: 30,
  font: "sans",
  weight: 700,
  sizePct: 0.07,
  align: "left",
  color: "#ffffff",
  maxLines: 2,
  lineHeight: 1.2,
});

const scene = (nodes: Scene["nodes"]): Scene => ({
  width: W,
  height: H,
  background: "#1a6b6a",
  nodes,
});

describe("prepareSceneForMotion", () => {
  it("returns the same object when there is no adaptive scrim", () => {
    const base = scene([fixed, DISH]);
    expect(prepareSceneForMotion(base, () => 220)).toBe(base);
  });

  it("clears the adaptive flag so no frame ever reads the canvas back", () => {
    const out = prepareSceneForMotion(scene([adaptive]), () => 220);
    const scrim = out.nodes[0];
    expect(scrim.kind === "scrim" && scrim.adaptive).toBe(false);
  });

  it("samples each distinct rect exactly once", () => {
    const seen: string[] = [];
    prepareSceneForMotion(scene([adaptive, { ...adaptive, z: 12 }]), (rect) => {
      seen.push(`${rect.x},${rect.y},${rect.w},${rect.h}`);
      return 220;
    });
    expect(seen).toHaveLength(1);
  });

  it("bakes the boost into `to` when the underlying photo is bright", () => {
    const out = prepareSceneForMotion(scene([adaptive]), () => 255);
    const scrim = out.nodes[0];
    // applyScrim's boost: min(0.82, 0.55 + (luma - threshold) / 400).
    // (255 - 155) / 400 = 0.25, so 0.80.
    expect(scrim.kind === "scrim" && scrim.to).toBe("rgba(28,25,23,0.8)");
  });

  it("caps the baked boost at applyScrim's own ceiling", () => {
    const out = prepareSceneForMotion(
      scene([{ ...adaptive, luminanceThreshold: 0 }]),
      () => 255,
    );
    const scrim = out.nodes[0];
    expect(scrim.kind === "scrim" && scrim.to).toBe("rgba(28,25,23,0.82)");
  });

  it("leaves `to` alone when the photo is already dark enough", () => {
    const out = prepareSceneForMotion(scene([adaptive]), () => 100);
    const scrim = out.nodes[0];
    expect(scrim.kind === "scrim" && scrim.to).toBe("rgba(28,25,23,0.78)");
    expect(scrim.kind === "scrim" && scrim.adaptive).toBe(false);
  });

  it("never samples when the scene has no photo", () => {
    const sample = jest.fn(() => 255);
    const out = prepareSceneForMotion(scene([adaptive, DISH]), sample, {
      hasPhoto: false,
    });
    expect(sample).not.toHaveBeenCalled();
    const scrim = out.nodes[0];
    expect(scrim.kind === "scrim" && scrim.adaptive).toBe(false);
    expect(scrim.kind === "scrim" && scrim.to).toBe("rgba(28,25,23,0.78)");
  });

  it("leaves every non-scrim node untouched", () => {
    const base = scene([photoNode({ url: "a.jpg", rect: { x: 0, y: 0, w: 1, h: 1 }, z: 0 }), adaptive, DISH]);
    const out = prepareSceneForMotion(base, () => 255);
    expect(out.nodes[0]).toBe(base.nodes[0]);
    expect(out.nodes[2]).toBe(base.nodes[2]);
  });
});
