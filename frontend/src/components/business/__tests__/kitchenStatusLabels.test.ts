import { orderStatusTranslationKey } from "../kitchenStatusLabels";

describe("orderStatusTranslationKey", () => {
  it("uses the unknown translation key when an order status is blank", () => {
    expect(orderStatusTranslationKey("")).toBe("orderStatuses.unknown");
    expect(orderStatusTranslationKey("   ")).toBe("orderStatuses.unknown");
    expect(orderStatusTranslationKey(null)).toBe("orderStatuses.unknown");
    expect(orderStatusTranslationKey(undefined)).toBe("orderStatuses.unknown");
  });

  it("uses the concrete order status translation key when present", () => {
    expect(orderStatusTranslationKey("ready")).toBe("orderStatuses.ready");
    expect(orderStatusTranslationKey("in_kitchen")).toBe(
      "orderStatuses.in_kitchen",
    );
  });
});
