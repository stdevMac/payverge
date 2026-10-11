import { Z_DROPDOWN, Z_FLOATING_PILL, Z_FAB, Z_CART_PANEL, Z_MODAL, Z_TOAST } from "../designLayers";

describe("designLayers — z-index scale", () => {
  test("scale is strictly increasing", () => {
    const scale = [Z_DROPDOWN, Z_FLOATING_PILL, Z_FAB, Z_CART_PANEL, Z_MODAL, Z_TOAST];
    for (let i = 1; i < scale.length; i++) {
      expect(scale[i]).toBeGreaterThan(scale[i - 1]);
    }
  });

  test("scale values stay below the 9999 ceiling reserved for legacy code", () => {
    expect(Z_TOAST).toBeLessThan(1000);
  });
});
