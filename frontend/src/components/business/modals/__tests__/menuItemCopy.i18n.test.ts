/**
 * Related to issue 130 / 150: plate-cost copy is restaurant language, not COGS.
 */
import en from "@/i18n/messages/en/businessDashboard.json";
import es from "@/i18n/messages/es/businessDashboard.json";

const enItems = en.dashboard.menuBuilder.items;
const esItems = es.dashboard.menuBuilder.items;

describe("menu item food-cost copy", () => {
  it("uses Food cost (per plate) and a full unclipped helper in en + es", () => {
    expect(enItems.itemCogs).toBe("Food cost (per plate)");
    expect(enItems.itemCogs.toLowerCase()).not.toContain("cogs");
    expect(enItems.itemCogsHint).toMatch(/recipe/i);
    expect(enItems.itemCogsHint.endsWith(".")).toBe(true);

    expect(esItems.itemCogs).toBe("Costo de comida (por plato)");
    expect(esItems.itemCogs.toLowerCase()).not.toContain("cogs");
    expect(esItems.itemCogsHint).toMatch(/receta/i);
  });
});
