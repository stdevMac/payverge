import {
  sortNodes,
  textNode,
  photoNode,
  scrimNode,
  logoNode,
  shapeNode,
  motifNode,
} from "./types";

describe("scene nodes", () => {
  it("sorts nodes by ascending z so painting order is deterministic", () => {
    const nodes = [
      textNode({
        key: "dishName",
        text: "Milanesa",
        rect: { x: 0, y: 0, w: 1, h: 0.1 },
        z: 30,
        font: "serif",
        weight: 600,
        sizePct: 0.07,
        align: "left",
        color: "#faf9f6",
        maxLines: 2,
        lineHeight: 1.1,
      }),
      photoNode({ url: "a.jpg", rect: { x: 0, y: 0, w: 1, h: 1 }, z: 0 }),
      scrimNode({
        rect: { x: 0, y: 0.5, w: 1, h: 0.5 },
        z: 10,
        from: "rgba(0,0,0,0)",
        to: "rgba(0,0,0,0.8)",
        adaptive: true,
        luminanceThreshold: 155,
      }),
    ];
    expect(sortNodes(nodes).map((n) => n.kind)).toEqual([
      "photo",
      "scrim",
      "text",
    ]);
  });

  // Regression test on the LANGUAGE guarantee, not on a hand-written tiebreaker:
  // `Array.prototype.sort` has been required to be stable since ES2019, which is
  // why `sortNodes` carries no `|| index - index` term. If a future runtime or
  // transpile target ever broke that, this is what would catch it.
  it("is a stable sort for equal z values", () => {
    const a = textNode({
      key: "a",
      text: "A",
      rect: { x: 0, y: 0, w: 1, h: 0.1 },
      z: 5,
      font: "sans",
      weight: 400,
      sizePct: 0.05,
      align: "left",
      color: "#1c1917",
      maxLines: 1,
      lineHeight: 1.1,
    });
    const b = textNode({
      key: "b",
      text: "B",
      rect: { x: 0, y: 0, w: 1, h: 0.1 },
      z: 5,
      font: "sans",
      weight: 400,
      sizePct: 0.05,
      align: "left",
      color: "#1c1917",
      maxLines: 1,
      lineHeight: 1.1,
    });
    expect(sortNodes([a, b]).map((n) => n.key)).toEqual(["a", "b"]);
  });

  it("does not mutate the array it is given", () => {
    const photo = photoNode({
      url: "a.jpg",
      rect: { x: 0, y: 0, w: 1, h: 1 },
      z: 0,
    });
    const scrim = scrimNode({
      rect: { x: 0, y: 0.5, w: 1, h: 0.5 },
      z: 10,
      from: "rgba(0,0,0,0)",
      to: "rgba(0,0,0,0.8)",
      adaptive: false,
      luminanceThreshold: 155,
    });
    const nodes = [scrim, photo];

    const sorted = sortNodes(nodes);

    expect(sorted).not.toBe(nodes);
    expect(nodes).toEqual([scrim, photo]);
    expect(nodes[0]).toBe(scrim);
    expect(nodes[1]).toBe(photo);
  });

  it("stamps kind onto a text node without adding or defaulting any field", () => {
    const node = textNode({
      key: "cta",
      text: "Book now",
      rect: { x: 0.06, y: 0.82, w: 0.4, h: 0.08 },
      z: 40,
      font: "sans",
      weight: 600,
      sizePct: 0.03,
      align: "center",
      color: "#1c1917",
      maxLines: 1,
      lineHeight: 1.2,
      uppercase: true,
      tracking: 0.06,
      pill: { fill: "#faf9f6", padX: 0.02, padY: 0.012 },
    });

    expect(node).toEqual({
      kind: "text",
      key: "cta",
      text: "Book now",
      rect: { x: 0.06, y: 0.82, w: 0.4, h: 0.08 },
      z: 40,
      font: "sans",
      weight: 600,
      sizePct: 0.03,
      align: "center",
      color: "#1c1917",
      maxLines: 1,
      lineHeight: 1.2,
      uppercase: true,
      tracking: 0.06,
      pill: { fill: "#faf9f6", padX: 0.02, padY: 0.012 },
    });
  });

  /**
   * The logo is painted as a DISC — `drawLogo` in ../templates/renderPost.ts
   * takes `{ x, y, size }`, rounds `size * canvasWidth`, and paints a circle of
   * it — so a height was never anything the walker could honour. It carried one
   * anyway, computed by `buildScene` from the aspect ratio and thrown away by
   * every consumer.
   *
   * That is worse than merely wasted: the Scene IR is the contract the planned
   * motion and print walkers are being written against, and a field the canvas
   * walker ignores reads to those authors as a field they must implement. This
   * pins the rect to exactly what is read.
   */
  it("gives a logo node an origin and a diameter, and no height", () => {
    const node = logoNode({
      url: "logo.png",
      rect: { x: 0.82, y: 0.065, w: 0.1 },
      z: 50,
    });

    expect(node).toEqual({
      kind: "logo",
      url: "logo.png",
      rect: { x: 0.82, y: 0.065, w: 0.1 },
      z: 50,
    });
    expect(Object.keys(node.rect).sort()).toEqual(["w", "x", "y"]);
  });
});

describe("per-node opacity", () => {
  const RECT = { x: 0.1, y: 0.2, w: 0.5, h: 0.1 };

  it("is carried by every node factory, not just motifs", () => {
    expect(
      photoNode({ url: "a.jpg", rect: RECT, z: 0, opacity: 0.5 }).opacity,
    ).toBe(0.5);
    expect(shapeNode({ rect: RECT, fill: "#000", z: 5, opacity: 0.5 }).opacity).toBe(
      0.5,
    );
    expect(
      textNode({
        key: "dishName",
        text: "Milanesa",
        rect: RECT,
        z: 30,
        font: "sans",
        weight: 700,
        sizePct: 0.07,
        align: "left",
        color: "#fff",
        maxLines: 2,
        lineHeight: 1.2,
        opacity: 0.5,
      }).opacity,
    ).toBe(0.5);
    expect(logoNode({ url: "l.png", rect: RECT, z: 50, opacity: 0.5 }).opacity).toBe(
      0.5,
    );
    expect(
      scrimNode({
        rect: RECT,
        z: 10,
        from: "rgba(0,0,0,0)",
        to: "rgba(0,0,0,0.8)",
        adaptive: false,
        luminanceThreshold: 155,
        opacity: 0.5,
      }).opacity,
    ).toBe(0.5);
    expect(
      motifNode({ motif: "rule", rect: RECT, color: "#fff", z: 40, opacity: 0.5 })
        .opacity,
    ).toBe(0.5);
  });

  it("stays absent when the caller omits it", () => {
    expect(photoNode({ url: "a.jpg", rect: RECT, z: 0 }).opacity).toBeUndefined();
    expect(
      motifNode({ motif: "rule", rect: RECT, color: "#fff", z: 40 }).opacity,
    ).toBeUndefined();
  });
});
