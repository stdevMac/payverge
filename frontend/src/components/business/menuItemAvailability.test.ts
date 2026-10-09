import {
  isMenuItemAvailableForBundle,
  filterAvailableMenuItems,
} from "./menuItemAvailability";

describe("L3-16 bundle item availability", () => {
  it("excludes is_available=false (86'd) items", () => {
    const items = [
      { id: "1", name: "A", is_available: true },
      { id: "2", name: "B", is_available: false },
      { id: "3", name: "C" },
    ];
    expect(filterAvailableMenuItems(items).map((i) => i.id)).toEqual([
      "1",
      "3",
    ]);
    expect(isMenuItemAvailableForBundle({ is_available: false })).toBe(false);
  });

  // Residual: inventory-driven 86 must also be dropped from the picker.
  it("excludes inventory_out even when is_available is still true", () => {
    const items = [
      { id: "1", name: "A", is_available: true, orderability_state: "available" },
      {
        id: "2",
        name: "B",
        is_available: true,
        orderability_state: "inventory_out",
      },
    ];
    expect(filterAvailableMenuItems(items).map((i) => i.id)).toEqual(["1"]);
    expect(
      isMenuItemAvailableForBundle({
        is_available: true,
        orderability_state: "inventory_out",
      }),
    ).toBe(false);
  });
});
