import {
  isManualEightySix,
  isMenuItemInventoryOut,
  isUnavailableForFilter,
  menuItemManualAvailable,
} from "./menuAvailabilityFilter";

describe("isUnavailableForFilter [L3-8]", () => {
  it("treats manual unavailable as unavailable", () => {
    expect(isUnavailableForFilter({ is_available: false })).toBe(true);
  });

  it("treats hard inventory_out as unavailable", () => {
    expect(
      isUnavailableForFilter({
        is_available: false,
        orderability_state: "inventory_out",
      }),
    ).toBe(true);
  });

  it("keeps inventory_out on a still-manual-available dish in the unavailable filter", () => {
    expect(
      isUnavailableForFilter({
        is_available: true,
        orderability_state: "inventory_out",
      }),
    ).toBe(true);
    expect(
      isManualEightySix({
        is_available: true,
        orderability_state: "inventory_out",
      }),
    ).toBe(false);
  });

  it("keeps low-stock warn available", () => {
    expect(
      isUnavailableForFilter({
        is_available: true,
        orderability_state: "inventory_warning",
      }),
    ).toBe(false);
  });

  it("keeps fully available items available", () => {
    expect(
      isUnavailableForFilter({
        is_available: true,
        orderability_state: "available",
      }),
    ).toBe(false);
  });

  it("does not 86 Harvest Bowl from leftover inventory_status when orderability is available", () => {
    const leftoverBowl = {
      id: "demo-bowl",
      is_available: true,
      orderability_state: "available" as const,
      inventory_status: "out_of_stock",
    };
    expect(isMenuItemInventoryOut(leftoverBowl)).toBe(false);
    expect(isUnavailableForFilter(leftoverBowl)).toBe(false);
  });
});

describe("menuItemManualAvailable [#727]", () => {
  // Live venue 142 payload for demo-bife after the operator menu started
  // reporting the effective flag: guests and the builder now agree the dish is
  // unsellable, and the operator's own switch is still ON.
  const inventoryBlockedBife = {
    id: "demo-bife",
    is_available: false,
    manual_available: true,
    orderability_state: "inventory_out" as const,
    inventory_status: "out_of_stock",
  };

  it("reads the stored switch, not the effective flag", () => {
    expect(menuItemManualAvailable(inventoryBlockedBife)).toBe(true);
  });

  it("does not call an inventory-blocked dish hand-pulled", () => {
    expect(isManualEightySix(inventoryBlockedBife)).toBe(false);
    expect(isMenuItemInventoryOut(inventoryBlockedBife)).toBe(true);
    expect(isUnavailableForFilter(inventoryBlockedBife)).toBe(true);
  });

  it("still reports a hand-pulled dish as manually 86'd", () => {
    const pulled = {
      is_available: false,
      manual_available: false,
      orderability_state: "manual_disabled" as const,
    };
    expect(menuItemManualAvailable(pulled)).toBe(false);
    expect(isManualEightySix(pulled)).toBe(true);
  });

  it("reports a dish that is both hand-pulled and out of stock as manually 86'd", () => {
    expect(
      isManualEightySix({
        is_available: false,
        manual_available: false,
        orderability_state: "inventory_out",
      }),
    ).toBe(true);
  });

  it("falls back to is_available when the field is absent", () => {
    // Legacy caches, the add-item form and optimistic local rows never carry
    // manual_available; they must behave exactly as they did before #727.
    expect(menuItemManualAvailable({ is_available: true })).toBe(true);
    expect(menuItemManualAvailable({ is_available: false })).toBe(false);
    expect(menuItemManualAvailable({})).toBe(true);
    expect(menuItemManualAvailable({ manual_available: null, is_available: false })).toBe(false);
  });

  it("keeps sellable dishes sellable", () => {
    const ensalada = {
      id: "demo-ensalada",
      is_available: true,
      manual_available: true,
      orderability_state: "available" as const,
    };
    expect(menuItemManualAvailable(ensalada)).toBe(true);
    expect(isUnavailableForFilter(ensalada)).toBe(false);
  });
});
