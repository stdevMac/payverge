import { getTranslation } from "../getTranslation";
import { sentenceCaseLeaf } from "../sentenceCaseLeaf";

/**
 * #618 — aging.openCheckWarning / seatedUnknown must ship in every operator
 * locale. A missing key sentence-cases to "Open check warning" / "Seated
 * unknown" and paints the SENTADOS column. Compare to sentenceCaseLeaf, not
 * an English regex, so locale-cased leaves fail the same way.
 */
describe("tableManager aging copy (production message files)", () => {
  const PREFIX = "businessDashboard.dashboard.tableManager.aging";
  const LEAVES = ["openCheckWarning", "seatedUnknown"] as const;

  it.each(["en", "es", "es-AR"] as const)(
    "ships real aging copy in %s, not a sentence-cased leaf",
    (locale) => {
      for (const leaf of LEAVES) {
        const key = `${PREFIX}.${leaf}`;
        const value = getTranslation(key, locale);
        expect(typeof value).toBe("string");
        expect(value).toBeTruthy();
        expect(value).not.toBe(sentenceCaseLeaf(key));
        expect(value).not.toBe(sentenceCaseLeaf(leaf));
      }
    },
  );
});
