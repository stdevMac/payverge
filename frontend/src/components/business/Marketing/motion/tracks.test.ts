import { motifNode, photoNode, textNode } from "../scene/types";
import { sampleTrack, trackMatches, type AnimTrack } from "./tracks";

const RECT = { x: 0.1, y: 0.6, w: 0.8, h: 0.1 };

const track = (over: Partial<AnimTrack> = {}): AnimTrack => ({
  target: { kind: "photo" },
  property: "crop.zoom",
  from: 1,
  to: 1.08,
  easing: "linear",
  delayMs: 0,
  durationMs: 1000,
  ...over,
});

const PHOTO = photoNode({ url: "a.jpg", rect: RECT, z: 0 });
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
const BADGE = { ...DISH, key: "badge", text: "CHEF'S PICK" };
const RULE = motifNode({ motif: "rule", rect: RECT, color: "#ffffff", z: 40 });
const GRAIN = motifNode({ motif: "grain", rect: RECT, color: "#ffffff", z: 15 });

describe("trackMatches", () => {
  it("matches on kind alone when no narrower is given", () => {
    expect(trackMatches(track(), PHOTO)).toBe(true);
    expect(trackMatches(track(), DISH)).toBe(false);
  });

  it("narrows a text track by slot key", () => {
    const t = track({ target: { kind: "text", key: "badge" }, property: "opacity" });
    expect(trackMatches(t, BADGE)).toBe(true);
    expect(trackMatches(t, DISH)).toBe(false);
  });

  it("matches every text node when the key is omitted", () => {
    const t = track({ target: { kind: "text" }, property: "opacity" });
    expect(trackMatches(t, BADGE)).toBe(true);
    expect(trackMatches(t, DISH)).toBe(true);
  });

  it("narrows a motif track by motif id", () => {
    const t = track({ target: { kind: "motif", motif: "grain" }, property: "opacity" });
    expect(trackMatches(t, GRAIN)).toBe(true);
    expect(trackMatches(t, RULE)).toBe(false);
  });

  it("never matches a node of a different kind, however narrow the target", () => {
    const t = track({ target: { kind: "text", key: "badge" }, property: "opacity" });
    expect(trackMatches(t, PHOTO)).toBe(false);
    expect(trackMatches(t, GRAIN)).toBe(false);
  });
});

describe("sampleTrack", () => {
  it("holds `from` before the delay elapses", () => {
    const t = track({ delayMs: 500 });
    expect(sampleTrack(t, 0)).toBe(1);
    expect(sampleTrack(t, 499)).toBe(1);
    expect(sampleTrack(t, 500)).toBe(1);
  });

  it("interpolates linearly across the duration", () => {
    expect(sampleTrack(track(), 500)).toBeCloseTo(1.04, 10);
  });

  it("holds `to` at and after the end", () => {
    expect(sampleTrack(track(), 1000)).toBe(1.08);
    expect(sampleTrack(track(), 99999)).toBe(1.08);
  });

  it("snaps to `to` for a zero-duration track", () => {
    const t = track({ durationMs: 0, delayMs: 200 });
    expect(sampleTrack(t, 199)).toBe(1);
    expect(sampleTrack(t, 201)).toBe(1.08);
  });

  it("snaps to `to` for a negative duration rather than dividing by it", () => {
    expect(sampleTrack(track({ durationMs: -50 }), 10)).toBe(1.08);
  });

  it("applies the named curve, not linear", () => {
    const eased = sampleTrack(track({ easing: "easeOutQuint" }), 500);
    const linear = sampleTrack(track({ easing: "linear" }), 500);
    expect(eased).toBeGreaterThan(linear);
  });

  /**
   * A single NaN reaching a rect is a whole frame painted at NaN coordinates,
   * and canvas silently draws nothing. Holding `from` makes the failure a frozen
   * animation rather than a blank video.
   */
  it("holds `from` for a non-finite time", () => {
    expect(sampleTrack(track(), Number.NaN)).toBe(1);
    expect(sampleTrack(track(), Number.POSITIVE_INFINITY)).toBe(1.08);
  });

  it("interpolates downward as happily as upward", () => {
    expect(sampleTrack(track({ from: 1, to: 0 }), 250)).toBeCloseTo(0.75, 10);
  });
});
