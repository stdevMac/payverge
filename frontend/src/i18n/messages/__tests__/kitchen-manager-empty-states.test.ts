import enMessages from "../en";
import esMessages from "../es";

const TAB_KEYS = ["approved", "in_kitchen", "ready", "all"] as const;

describe("kitchenManager.emptyStates (K-4 guard)", () => {
  const trees = {
    en: (enMessages as Record<string, any>).businessDashboard?.dashboard
      ?.kitchenManager?.emptyStates,
    es: (esMessages as Record<string, any>).businessDashboard?.dashboard
      ?.kitchenManager?.emptyStates,
  };

  for (const locale of ["en", "es"] as const) {
    it(`${locale}: every kitchen tab has a title AND a description (no stray keys)`, () => {
      const emptyStates = trees[locale];
      expect(emptyStates).toBeDefined();
      for (const tab of TAB_KEYS) {
        expect(typeof emptyStates[tab]?.title).toBe("string");
        expect(typeof emptyStates[tab]?.description).toBe("string");
        expect(emptyStates[tab]?.empty).toBeUndefined();
      }
    });
  }

  it("es is genuinely translated, not copied English", () => {
    for (const tab of TAB_KEYS) {
      expect(trees.es[tab]?.description).not.toBe(trees.en[tab]?.description);
    }
  });
});
