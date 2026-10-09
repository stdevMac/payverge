import { liveMapNodeAffordance } from "./liveMapNode";

describe("L3-26 live map node without status feed", () => {
  it("marks missing live data with affordance (not silent inert)", () => {
    const node = liveMapNodeAffordance(false);
    expect(node.clickable).toBe(false);
    expect(node.showNoLiveDataLabel).toBe(true);
    expect(node.opacity).toBeLessThan(1);
  });

  it("keeps full opacity + clickable when live status exists", () => {
    const node = liveMapNodeAffordance(true);
    expect(node.clickable).toBe(true);
    expect(node.showNoLiveDataLabel).toBe(false);
    expect(node.opacity).toBe(1);
  });
});
