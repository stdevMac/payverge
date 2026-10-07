import {
  isMenuItemOrderable,
  isVenueClosedState,
  shouldShowItemInventoryWarningBadge,
  shouldShowItemPromotionOffers,
  shouldShowItemUnavailableBadge,
} from "./menuItemAvailability";

describe("menuItemAvailability closed presentation", () => {
  it("detects business_closed as venue closed state", () => {
    expect(isVenueClosedState("business_closed")).toBe(true);
    expect(isVenueClosedState("inventory_out")).toBe(false);
    expect(isVenueClosedState(undefined)).toBe(false);
  });

  it("suppresses unavailable badge when venue is closed without an item 86", () => {
    expect(
      shouldShowItemUnavailableBadge({
        orderabilityState: "business_closed",
        isAvailable: false,
      }),
    ).toBe(false);
  });

  it("keeps inventory 86 badge after hours so the card is not a normal dish", () => {
    expect(
      shouldShowItemUnavailableBadge({
        venueClosed: true,
        isAvailable: false,
        orderabilityState: "inventory_out",
      }),
    ).toBe(true);
    expect(
      shouldShowItemUnavailableBadge({
        orderabilityState: "business_closed",
        isAvailable: true,
        inventoryStatus: "out_of_stock",
      }),
    ).toBe(true);
  });

  it("shows unavailable badge for inventory_out when open", () => {
    expect(
      shouldShowItemUnavailableBadge({
        orderabilityState: "inventory_out",
        isAvailable: false,
      }),
    ).toBe(true);
  });

  it("shows unavailable badge when is_available false and not closed", () => {
    expect(
      shouldShowItemUnavailableBadge({
        isAvailable: false,
      }),
    ).toBe(true);
  });

  it("does not show unavailable badge when item is available", () => {
    expect(
      shouldShowItemUnavailableBadge({
        isAvailable: true,
      }),
    ).toBe(false);
  });

  it("shows unavailable badge from catalog inventory_status even when is_available stays true", () => {
    expect(
      shouldShowItemUnavailableBadge({
        isAvailable: true,
        inventoryStatus: "out_of_stock",
      }),
    ).toBe(true);
  });

  it("treats inventory_status out_of_stock as not orderable", () => {
    expect(
      isMenuItemOrderable({
        is_available: true,
        inventory_status: "out_of_stock",
      }),
    ).toBe(false);
  });

  it("gates promotion chips on venueClosed", () => {
    expect(shouldShowItemPromotionOffers(true)).toBe(false);
    expect(shouldShowItemPromotionOffers(false)).toBe(true);
  });

  it("suppresses inventory warning badges when closed", () => {
    expect(
      shouldShowItemInventoryWarningBadge({
        orderabilityState: "inventory_warning",
        venueClosed: true,
      }),
    ).toBe(false);
    expect(
      shouldShowItemInventoryWarningBadge({
        orderabilityState: "inventory_warning",
      }),
    ).toBe(true);
  });

  it("keeps isMenuItemOrderable strict (closed items stay non-orderable)", () => {
    expect(
      isMenuItemOrderable({
        is_available: false,
        orderability_state: "business_closed",
      }),
    ).toBe(false);
  });
});
