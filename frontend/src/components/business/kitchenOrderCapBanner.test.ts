import {
  parentOwnsKitchenOrders,
  shouldShowKitchenOrderCapBanner,
} from "./kitchenOrderCapBanner";

describe("kitchen order-cap banner ownership (#684)", () => {
  it("treats an empty parent map as owned (dashboard always passes {})", () => {
    expect(parentOwnsKitchenOrders(undefined)).toBe(false);
    expect(parentOwnsKitchenOrders({})).toBe(true);
    expect(parentOwnsKitchenOrders({ 1: [] })).toBe(true);
  });

  it("keeps Kitchen quiet when the parent owns the map, even if capped", () => {
    expect(
      shouldShowKitchenOrderCapBanner({
        parentOwnsOrders: true,
        fallbackCapped: true,
      }),
    ).toBe(false);
    expect(
      shouldShowKitchenOrderCapBanner({
        parentOwnsOrders: true,
        fallbackCapped: false,
      }),
    ).toBe(false);
  });

  it("lets standalone Kitchen paint from its own fallback cap", () => {
    expect(
      shouldShowKitchenOrderCapBanner({
        parentOwnsOrders: false,
        fallbackCapped: true,
      }),
    ).toBe(true);
    expect(
      shouldShowKitchenOrderCapBanner({
        parentOwnsOrders: false,
        fallbackCapped: false,
      }),
    ).toBe(false);
  });
});
