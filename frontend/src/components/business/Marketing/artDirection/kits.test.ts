import type { TemplateStyle } from "../templates/types";
import {
  KITS,
  KIT_ORDER,
  isKitId,
  kitForLegacyTemplate,
  type KitId,
} from "./kits";

describe("kits", () => {
  it("defines exactly the six Wave 1 kits", () => {
    expect(KIT_ORDER).toEqual([
      "editorial",
      "bold",
      "minimal",
      "chalkboard",
      "linen",
      "ticket",
    ]);
  });

  it("every ordered kit has a definition whose id matches its key", () => {
    KIT_ORDER.forEach((id) => {
      expect(KITS[id]).toBeDefined();
      expect(KITS[id].id).toBe(id);
    });
  });

  it("every kit carries a non-empty motif set and a label key", () => {
    KIT_ORDER.forEach((id) => {
      expect(KITS[id].motifSet.length).toBeGreaterThan(0);
      expect(KITS[id].labelKey).toBe(`kits.${id}`);
    });
  });

  it("maps every legacy template id to a kit of the same name", () => {
    const legacy: TemplateStyle[] = ["editorial", "bold", "minimal"];
    legacy.forEach((template) => {
      expect(kitForLegacyTemplate(template)).toBe(template as KitId);
    });
  });

  it("falls back to editorial for an unknown legacy template", () => {
    expect(kitForLegacyTemplate("nonsense" as TemplateStyle)).toBe("editorial");
  });

  // `creative_snapshot.kit` arrives off the wire as a bare string, so a value
  // this build has no definition for is a normal event, not a bug. Narrowing
  // through this guard is what keeps such a value out of `KITS[...]`, where it
  // would resolve to undefined and render a blank post.
  it("recognises exactly the registered kit ids and nothing else", () => {
    KIT_ORDER.forEach((id) => expect(isKitId(id)).toBe(true));
    [
      "",
      "  chalkboard  ",
      "Chalkboard",
      "chalk",
      "posterStack",
      "neon-vaporwave",
      "toString",
      null,
      undefined,
      7,
      {},
    ].forEach((value) => expect(isKitId(value)).toBe(false));
  });

  it("narrows an unknown wire value to a kit that has a definition", () => {
    const fromWire: string = "neon-vaporwave";
    const kit: KitId = isKitId(fromWire)
      ? fromWire
      : kitForLegacyTemplate("bold");
    expect(KITS[kit]).toBeDefined();
    expect(kit).toBe("bold");
  });
});
