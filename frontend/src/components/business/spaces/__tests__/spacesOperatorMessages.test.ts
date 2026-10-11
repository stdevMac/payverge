/**
 * Operator-tier i18n key-existence guard for the spaces/tables editor.
 *
 * PropertiesPanel.tsx renders capacity validation copy through `t(...)`, which
 * resolves via getTranslation. getTranslation NEVER returns falsy — a missing
 * key renders the sentence-cased leaf ("Capacity min max"), so an identity
 * `t` stub in a component test cannot detect the gap. These assertions read
 * the REAL message files instead.
 */
import enSpacesTables from "@/i18n/messages/en/spacesTables.json";
import esSpacesTables from "@/i18n/messages/es/spacesTables.json";

type AnyRecord = Record<string, any>;

/** Keys PropertiesPanel.tsx resolves for the capacity invariant (L3-24). */
const CAPACITY_VALIDATION_KEYS = ["capacityMinMax", "capacityRange"] as const;

describe("spaces operator messages", () => {
  it("waiting-to-place KPI copy exists in en and es", () => {
    const en = (enSpacesTables as AnyRecord).aggregate;
    const es = (esSpacesTables as AnyRecord).aggregate;
    for (const key of [
      "waitingToPlace",
      "waitingToPlaceHint",
      "waitingToPlaceCount_one",
      "waitingToPlaceCount_other",
      "maxSeatsFromTablesHint",
    ]) {
      expect(typeof en[key]).toBe("string");
      expect(en[key].length).toBeGreaterThan(0);
      expect(typeof es[key]).toBe("string");
      expect(es[key].length).toBeGreaterThan(0);
    }
    expect(en.waitingToPlaceCount_other).toContain("{count}");
    expect(es.waitingToPlaceCount_other).toContain("{count}");
  });

  it("capacity validation keys exist in en and es (L3-24)", () => {
    const en = (enSpacesTables as AnyRecord).editor.properties;
    const es = (esSpacesTables as AnyRecord).editor.properties;
    for (const key of CAPACITY_VALIDATION_KEYS) {
      expect(typeof en[key]).toBe("string");
      expect(en[key].length).toBeGreaterThan(0);
      expect(typeof es[key]).toBe("string");
      expect(es[key].length).toBeGreaterThan(0);
    }
  });
});
