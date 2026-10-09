import { DEFAULT_CROP } from "../templates/renderPost";
import { MOTION_DEFAULT_CROP } from "./evaluate";

/**
 * `evaluate.ts` cannot import `DEFAULT_CROP` at runtime without pulling
 * `renderPost.ts` — and with it the image cache, the font loader and the Sentry
 * error logger — into a module whose whole value is being pure and canvas-free.
 * So it restates the constant, and this test is the thing that stops the two
 * drifting. Same pattern as `../artDirection/kits.parity.test.ts`.
 */
it("keeps the motion default crop identical to the renderer's", () => {
  expect(MOTION_DEFAULT_CROP).toEqual(DEFAULT_CROP);
});
