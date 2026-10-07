import { shouldOfferVarianceAdjustment } from "./cashRegisterCloseActions";

it("offers the variance adjustment only for a non-zero variance", () => {
  expect(shouldOfferVarianceAdjustment(-12.5)).toBe(true);
  expect(shouldOfferVarianceAdjustment(3)).toBe(true);
  expect(shouldOfferVarianceAdjustment(0)).toBe(false);
  expect(shouldOfferVarianceAdjustment(undefined)).toBe(false);
  expect(shouldOfferVarianceAdjustment(null as unknown as undefined)).toBe(
    false,
  );
});
