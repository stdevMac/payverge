import { projectMenuOrderability } from "./useSharedMenu";
import type { MenuCategory } from "@/api/business";
import { asDollars } from "@/types/money";

describe("projectMenuOrderability (L3-16)", () => {
  it("stamps inventory_out onto items so the picker can exclude them", () => {
    const cats: MenuCategory[] = [
      {
        name: "Mains",
        description: "",
        items: [
          { id: "steak", name: "Steak", description: "", price: asDollars(20), is_available: true },
          { id: "soup", name: "Soup", description: "", price: asDollars(8), is_available: true },
        ],
      },
    ];
    const projected = projectMenuOrderability(cats, {
      steak: { state: "inventory_out", orderable: false },
      soup: { state: "available", orderable: true },
    });
    expect(projected[0].items[0].is_available).toBe(false);
    expect(projected[0].items[0].orderability_state).toBe("inventory_out");
    expect(projected[0].items[1].is_available).toBe(true);
  });
});
