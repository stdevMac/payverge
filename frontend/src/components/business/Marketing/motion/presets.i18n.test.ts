import en from "@/i18n/messages/en/marketingDashboard.json";
import es from "@/i18n/messages/es/marketingDashboard.json";
import { MOTION_PRESETS, MOTION_PRESET_ORDER } from "./presets";

/**
 * Same shape as ../artDirection/kits.parity.test.ts. A preset whose label key is
 * missing renders as the raw key in the picker — visible, but only to whoever
 * opens that drawer in that locale, which is nobody until an operator does.
 */
const at = (tree: Record<string, unknown>, path: string): unknown =>
  path.split(".").reduce<unknown>((node, key) => {
    if (node && typeof node === "object")
      return (node as Record<string, unknown>)[key];
    return undefined;
  }, tree);

describe("motion preset copy", () => {
  it.each(MOTION_PRESET_ORDER)("%s has an English label", (id) => {
    const label = at(en, MOTION_PRESETS[id].labelKey);
    expect(typeof label).toBe("string");
    expect(label).not.toBe("");
  });

  it.each(MOTION_PRESET_ORDER)("%s has a Spanish label", (id) => {
    const label = at(es, MOTION_PRESETS[id].labelKey);
    expect(typeof label).toBe("string");
    expect(label).not.toBe("");
  });

  it("ships every operator-facing motion string in both locales", () => {
    const keys = [
      "motion.label",
      "motion.still",
      "motion.hint",
      "motion.download",
      "motion.rendering",
      "motion.cancel",
      "motion.cancelled",
      "motion.unsupported",
      "motion.failed",
      "motion.reducedMotion",
      "motion.libraryBadge",
    ];
    keys.forEach((key) => {
      expect(typeof at(en, key)).toBe("string");
      expect(typeof at(es, key)).toBe("string");
    });
  });
});
