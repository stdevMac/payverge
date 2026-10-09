import {
  isOfferDiscountValid,
  offerDiscountErrorKey,
} from "./offerDiscountValidation";

describe("L3-14 offer discount bounds", () => {
  it("rejects percentage > 100 (500% defect)", () => {
    expect(isOfferDiscountValid("percentage", 500)).toBe(false);
    expect(offerDiscountErrorKey("percentage", 500)).toBe(
      "messages.discountPercentMax",
    );
  });

  it("accepts percentage 1..100 and fixed > 0", () => {
    expect(isOfferDiscountValid("percentage", 100)).toBe(true);
    expect(isOfferDiscountValid("percentage", 10)).toBe(true);
    expect(isOfferDiscountValid("fixed", 5)).toBe(true);
    expect(isOfferDiscountValid("fixed", 500)).toBe(true);
  });

  it("rejects zero and negative", () => {
    expect(isOfferDiscountValid("percentage", 0)).toBe(false);
    expect(isOfferDiscountValid("fixed", -1)).toBe(false);
  });
});
