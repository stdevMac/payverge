import { resolveRect } from "./geometry";

describe("resolveRect", () => {
  /**
   * The single normalized→pixel conversion every walker site now shares. Tested
   * against a non-square canvas with four distinct rect fields, so no axis
   * transposition survives by coincidence.
   */
  it("resolveRect resolves x/w against width and y/h against height", () => {
    expect(resolveRect({ x: 0.25, y: 0.5, w: 0.5, h: 0.2 }, 1000, 400)).toEqual(
      {
        x: 250,
        y: 200,
        w: 500,
        h: 80,
      },
    );
  });
});
