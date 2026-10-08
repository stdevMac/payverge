import enMessages from "../en";
import esMessages from "../es";

// E2 (inventory-empty-state): the activation card never told operators that
// inventory is optional — add one clarifying line so businesses that don't
// need stock tracking aren't confused into thinking it's required setup.
describe("inventoryManager.activationCard.description copy (R-E2 guard)", () => {
  const trees = {
    en: (enMessages as Record<string, any>).businessDashboard?.inventoryManager
      ?.activationCard,
    es: (esMessages as Record<string, any>).businessDashboard?.inventoryManager
      ?.activationCard,
  };

  it("en: mentions inventory is optional and lists the value it unlocks", () => {
    expect(trees.en.description).toMatch(/optional/i);
    expect(trees.en.description).toMatch(/stock counts/i);
    expect(trees.en.description).toMatch(/waste tracking/i);
    expect(trees.en.description).toMatch(/food-cost analytics/i);
  });

  it("es: mentions inventory is optional and lists the value it unlocks", () => {
    expect(trees.es.description).toMatch(/opcional/i);
    expect(trees.es.description).toMatch(/stock/i);
    expect(trees.es.description).toMatch(/mermas/i);
    expect(trees.es.description).toMatch(/costos/i);
  });

  it("es is genuinely translated, not copied English", () => {
    expect(trees.es.description).not.toBe(trees.en.description);
  });
});
